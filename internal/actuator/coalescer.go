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

const coalescingWindow = 2 * time.Millisecond

// CoalescingActuator implements a map-coalesced dispatcher with feedback
type CoalescingActuator struct {
	mu           sync.Mutex
	pending      map[string]DirectiveSnapshot
	notify       chan struct{}
	feedback     chan ActuationResult
	done         chan struct{}
	closed       uint32
	debugLogging uint32
	failureCount uint64
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	backend      Backend
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
	// ✅ FAST PATH: atomic check only - no mutex in hot path
	if atomic.LoadUint32(&a.closed) == 1 {
		return
	}

	a.mu.Lock()

	// ✅ DOUBLE CHECK AFTER ACQUIRING LOCK
	// Handles race where Close() executed between atomic check and mutex acquire
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

	// ✅ PERFORM SEND WHILE STILL HOLDING MUTEX
	// This guarantees atomicity between closed check and send
	select {
	case a.notify <- struct{}{}:
	default:
	}

	a.mu.Unlock()
}

func (a *CoalescingActuator) Feedback() <-chan ActuationResult {
	return a.feedback
}

// loop handles the signals and the final drain
func (a *CoalescingActuator) loop() {
	defer a.wg.Done()
	for range a.notify {
		time.Sleep(coalescingWindow)
		for {
			select {
			case _, ok := <-a.notify:
				if !ok {
					a.processPending()
					return
				}
			default:
				goto process
			}
		}
	process:
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
		// Use separate context for execution - never cancel pending work already in flight
		execCtx := context.Background()
		err := a.backend.Execute(execCtx, snapshot)
		latency := time.Since(start)
		success := err == nil
		if !success {
			atomic.AddUint64(&a.failureCount, 1)

			// Verbose debug logging only when explicitly enabled
			if atomic.LoadUint32(&a.debugLogging) == 1 {
				log.Printf("[actuator] FAILED id=%s svc=%s err=%v tick=%d",
					snapshot.DirectiveID, snapshot.ServiceID, err, snapshot.TickIndex)
			}
		}

		res := ActuationResult{
			TickIndex:   snapshot.TickIndex,
			ServiceID:   snapshot.ServiceID,
			DirectiveID: snapshot.DirectiveID,
			Success:     success,
			Latency:     latency,
			Error:       err,
		}
		select {
		case a.feedback <- res:
		default:
			if atomic.LoadUint32(&a.debugLogging) == 1 {
				log.Printf("[actuator] feedback dropped id=%s svc=%s tick=%d",
					snapshot.DirectiveID, snapshot.ServiceID, snapshot.TickIndex)
			}
		}
	}
}

func (a *CoalescingActuator) drain() {
	a.processPending()
}

// EnableDebugLogging turns on verbose failure logging. Default OFF.
func (a *CoalescingActuator) EnableDebugLogging(enable bool) {
	if enable {
		atomic.StoreUint32(&a.debugLogging, 1)
	} else {
		atomic.StoreUint32(&a.debugLogging, 0)
	}
}

// FailureCount returns total actuator failure count.
func (a *CoalescingActuator) FailureCount() uint64 {
	return atomic.LoadUint64(&a.failureCount)
}

// Close stops the actuator and waits for the worker to drain pending tasks.
func (a *CoalescingActuator) Close(ctx context.Context) error {
	// ✅ STANDARD FAST RACE-FREE SHUTDOWN PATTERN
	// 1. Atomic first - exit fast if already closed
	if !atomic.CompareAndSwapUint32(&a.closed, 0, 1) {
		return nil
	}

	// ✅ SAFETY POINT: NO NEW DISPATCH() CALLS WILL NOW ENTER CRITICAL SECTION
	// All Dispatch() calls either already passed the atomic check or will see closed == 1

	// ✅ 2. Take mutex to ensure all in-flight Dispatch() have exited critical section
	a.mu.Lock()
	a.mu.Unlock()

	// ✅ ALL SENDERS ARE NOW GONE. 100% SAFE TO CLOSE CHANNEL.
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
