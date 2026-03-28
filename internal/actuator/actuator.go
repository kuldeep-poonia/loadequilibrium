package actuator

import (
	"context"
	"log"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

// DirectiveSnapshot represents an immutable, decoupled control directive
type DirectiveSnapshot struct {
	TickIndex         uint64
	ServiceID         string
	DirectiveID       string
	TargetUtilisation float64
	ScaleFactor       float64
	UrgencyScore      float64
	CreatedAt         time.Time
}

// ActuationResult is the asynchronous feedback from the actuator
type ActuationResult struct {
	TickIndex   uint64
	ServiceID   string
	DirectiveID string
	Success     bool
	Latency     time.Duration
	Error       error
}

// Backend executes a single directive against an external system.
// Implementations: HTTPBackend, LogOnlyBackend, KubernetesAnnotationBackend, etc.
// The call must be synchronous and respect context cancellation.
type Backend interface {
	Execute(ctx context.Context, snap DirectiveSnapshot) error
}

// LogOnlyBackend is the default no-op used when no real backend is configured.
// It logs the directive so operators can observe what would be actuated.
type LogOnlyBackend struct{}

func (b *LogOnlyBackend) Execute(_ context.Context, snap DirectiveSnapshot) error {
	log.Printf("[actuator:noop] svc=%s scale=%.3f tick=%d (no backend configured)",
		snap.ServiceID, snap.ScaleFactor, snap.TickIndex)
	return nil
}

// Actuator defines a safe, non-blocking execution interface
type Actuator interface {
	Dispatch(tickIndex uint64, directives map[string]optimisation.ControlDirective)
	Feedback() <-chan ActuationResult
	Close(ctx context.Context) error
}
