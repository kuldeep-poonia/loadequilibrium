package backends

import (
	"context"
	"errors"
	"log"
	"math"
	"sync"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/debug"
)

const minWorkers = 1
const maxWorkers = 10_000

// QueueBackend simulates worker scaling in-memory per service.
type QueueBackend struct {
	mu      sync.Mutex
	workers map[string]int
}

func NewQueueBackend() *QueueBackend {
	return &QueueBackend{
		workers: make(map[string]int),
	}
}

func (b *QueueBackend) Execute(ctx context.Context, snap actuator.DirectiveSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap.ServiceID == "" {
		return errors.New("queue backend: missing service id")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	current := b.workers[snap.ServiceID]
	if current < minWorkers {
		current = minWorkers
	}

	next := int(math.Round(float64(current) * snap.ScaleFactor))
	if next < minWorkers {
		next = minWorkers
	}
	if next > maxWorkers {
		next = maxWorkers
	}

	b.workers[snap.ServiceID] = next

	// ✅ ONLY LOG WHEN WORKER COUNT ACTUALLY CHANGES AND HOT PATH LOGS ARE ENABLED
	// Eliminates 100% of log spam during steady state operation by default
	if current != next && debug.HotPathLogsEnabled() {
		log.Printf("[actuator:queue] svc=%s workers=%d->%d scale=%.3f tick=%d",
			snap.ServiceID, current, next, snap.ScaleFactor, snap.TickIndex)
	}

	return nil
}

func (b *QueueBackend) WorkerCount(serviceID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := b.workers[serviceID]
	if count < minWorkers {
		return minWorkers
	}
	return count
}
