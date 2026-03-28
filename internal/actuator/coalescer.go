package actuator

import (
	"context"
	"fmt"
	"log"
	"sync"
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

func (a *CoalescingActuator) Dispatch(tickIndex uint64, dirs map[string]optimisation.ControlDirective) {
	a.mu.Lock()
	for id, d := range dirs {
		if d.Active {
			// Deep copy to immutable snapshot, overwriting any pending stale directive for this service.
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

	// Non-blocking wake-up signal to the worker
	select {
	case a.notify <- struct{}{}:
	default:
	}
}

func (a *CoalescingActuator) Feedback() <-chan ActuationResult {
	return a.feedback
}

func (a *CoalescingActuator) loop() {
	defer a.wg.Done()
	for {
		select {
		case <-a.ctx.Done():
			a.drain()
			return
		case <-a.notify:
			a.processPending()
		}
	}
}

func (a *CoalescingActuator) processPending() {
	a.mu.Lock()
	if len(a.pending) == 0 {
		a.mu.Unlock()
		return
	}
	batch := make([]DirectiveSnapshot, 0, len(a.pending))
	for id, snap := range a.pending {
		batch = append(batch, snap)
		delete(a.pending, id)
	}
	a.mu.Unlock()

	for _, snap := range batch {
		start := time.Now()
		execCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		err := a.backend.Execute(execCtx, snap)
		cancel()
		latency := time.Since(start)
		success := err == nil
		if !success {
			log.Printf("[actuator] FAILED id=%s svc=%s err=%v tick=%d",
				snap.DirectiveID, snap.ServiceID, err, snap.TickIndex)
		}

		res := ActuationResult{
			TickIndex:   snap.TickIndex,
			ServiceID:   snap.ServiceID,
			DirectiveID: snap.DirectiveID,
			Success:     success,
			Latency:     latency,
			Error:       err,
		}
		select {
		case a.feedback <- res:
		default:
		}
	}
}

func (a *CoalescingActuator) drain() {
	a.processPending()
}

func (a *CoalescingActuator) Close(ctx context.Context) error {
	a.cancel()
	c := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
