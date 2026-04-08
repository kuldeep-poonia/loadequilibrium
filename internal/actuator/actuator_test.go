package actuator_test

import (
	"context"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
)

func TestLogOnlyBackend(t *testing.T) {
	backend := &actuator.LogOnlyBackend{}

	snap := actuator.DirectiveSnapshot{
		TickIndex:   1,
		ServiceID:   "test-svc",
		DirectiveID: "d-001",
		ScaleFactor: 0.9,
	}

	err := backend.Execute(context.Background(), snap)
	if err != nil {
		t.Errorf("LogOnly backend: unexpected error %v", err)
	}
}

func TestDirectiveSnapshot(t *testing.T) {
	now := time.Now()

	snap := actuator.DirectiveSnapshot{
		TickIndex:         uint64(1),
		ServiceID:         "api-svc",
		DirectiveID:       "d-001",
		TargetUtilisation: 0.70,
		ScaleFactor:       1.0,
		UrgencyScore:      0.5,
		CreatedAt:         now,
	}

	if snap.ServiceID != "api-svc" {
		t.Errorf("DirectiveSnapshot: ServiceID mismatch")
	}
	if snap.ScaleFactor != 1.0 {
		t.Errorf("DirectiveSnapshot: ScaleFactor mismatch")
	}
}

func TestActuationResult(t *testing.T) {
	result := actuator.ActuationResult{
		TickIndex:   1,
		ServiceID:   "api-svc",
		DirectiveID: "d-001",
		Success:     true,
		Latency:     25 * time.Millisecond,
		Error:       nil,
	}

	if !result.Success {
		t.Errorf("ActuationResult: Success flag incorrect")
	}
	if result.Latency.Milliseconds() != 25 {
		t.Errorf("ActuationResult: Latency mismatch, got %d ms", result.Latency.Milliseconds())
	}
}

func BenchmarkDirectiveSnapshot(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = actuator.DirectiveSnapshot{
			TickIndex:   uint64(i),
			ServiceID:   "bench-svc",
			DirectiveID: "d-bench",
			ScaleFactor: 0.9,
			CreatedAt:   time.Now(),
		}
	}
}
