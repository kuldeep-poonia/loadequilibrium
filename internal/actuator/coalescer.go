package actuator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

// CoalescingActuator implements a map-coalesced dispatcher with feedback
type CoalescingActuator struct {
	mu       sync.Mutex
	pending  map[string]DirectiveSnapshot
	notify   chan struct{}
	feedback chan ActuationResult
	done     chan struct{}
	closed   uint32
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	backend  Backend
}

func NewCoalescingActuator(feedbackBuf int, backend Backend) *CoalescingActuator {
	if backend == nil {
		backend = &LogOnlyBackend{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &CoalescingActuator{
		pending:  make(map[string]DirectiveSnapshot),
		notify:   make(chan struct{}, 1),
		feedback: make(chan ActuationResult, feedbackBuf),
		done:     make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
		backend:  backend,
	}
	a.wg.Add(1)
	go a.loop()
	return a
}

// Dispatch updates the map and pokes the worker
func (a *CoalescingActuator) Dispatch(tickIndex uint64, dirs map[string]optimisation.ControlDirective) {
	a.mu.Lock()
	if atomic.LoadUint32(&a.closed) == 1 {
		a.mu.Unlock()
		return
	}

	for id, d := range dirs {
		if d.Active {
			a.pending[id] = DirectiveSnapshot{
				TickIndex:         tickIndex,
				ServiceID:         id,
				DirectiveID:       fmt.Sprintf("tick-%d-svc-%s", tickIndex, id),
				TargetUtilisation: d.TargetUtilisation,
				ScaleFactor:       d.ScaleFactor,
				UrgencyScore:      d.CostGradient,
				CreatedAt:         time.Now(),
			}
		}
	}
	a.mu.Unlock()

	// Non-blocking poke to the worker
	select {
	case a.notify <- struct{}{}:
	default:
	}
}

func (a *CoalescingActuator) Feedback() <-chan ActuationResult {
	return a.feedback
}

// loop handles the signals and the final drain
func (a *CoalescingActuator) loop() {
	defer a.wg.Done()
	// Using 'range' on a channel is the cleanest way to drain
	for range a.notify {
		a.processPending()
	}
	// FINAL DRAIN: One last check after the channel is closed
	a.processPending()
}

func (a *CoalescingActuator) processPending() {
	a.mu.Lock()
	if len(a.pending) == 0 {
		a.mu.Unlock()
		return
	}
	// Move the pending work to a local variable and clear the map
	work := a.pending
	a.pending = make(map[string]DirectiveSnapshot)
	a.mu.Unlock()

	// CRITICAL: Execute synchronously. 
	// Do NOT use 'go a.backend.Execute'. 
	// If this blocks, the loop blocks, and Close() will wait 
	// until this is completely finished.
	for _, snapshot := range work {
		start := time.Now()
		err := a.backend.Execute(a.ctx, snapshot)
		latency := time.Since(start)
		success := err == nil
		if !success {
			log.Printf("[actuator] FAILED id=%s svc=%s err=%v tick=%d",
				snapshot.DirectiveID, snapshot.ServiceID, err, snapshot.TickIndex)
		}

		res := ActuationResult{
			TickIndex:   snapshot.TickIndex,
			ServiceID:   snapshot.ServiceID,
			DirectiveID: snapshot.DirectiveID,
			Success:     success,
			Latency:     latency,
			Error:       err,
		}
		// Always block to send feedback - this channel is created with sufficient buffer
		// Dropping results violates the actuator contract that all executions are reported
		a.feedback <- res
	}
}

func (a *CoalescingActuator) drain() {
	a.processPending()
}

// Close stops the actuator and waits for the worker to drain pending tasks.
func (a *CoalescingActuator) Close(ctx context.Context) error {
	// Ensure we only close once
	if !atomic.CompareAndSwapUint32(&a.closed, 0, 1) {
		return nil
	}

	// 1. Signal the worker loop to terminate via the channel
	close(a.notify)

	// 2. Create a channel to track the WaitGroup completion
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	// 3. Wait for the worker to finish OR the context to timeout
	select {
	case <-done:
		// Clean exit: all pending tasks were processed
		a.cancel()
		return nil
	case <-ctx.Done():
		// Timeout: the test gave up waiting for the drain
		a.cancel() 
		return ctx.Err()
	}
}
