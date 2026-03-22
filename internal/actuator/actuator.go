package actuator

import (
	"context"
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

// Actuator defines a safe, non-blocking execution interface
type Actuator interface {
	Dispatch(tickIndex uint64, directives map[string]optimisation.ControlDirective)
	Feedback() <-chan ActuationResult
	Close(ctx context.Context) error
}
