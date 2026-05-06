package autopilot

import "testing"

func Test_DecisionPolicy_HoldsNearEquilibrium(t *testing.T) {
	decision := Decide(DecisionInput{
		Instability:    0.1,
		Confidence:     0.9,
		Anomaly:        Stable,
		Backlog:        1.0,
		Workers:        10.0,
		TargetCapacity: 10.0,
		Effectiveness:  0.8,
		Oscillation:    0.05,
		Trend:          0.0,
	})

	if decision.Action != "hold" {
		t.Fatalf("expected hold near equilibrium, got %q", decision.Action)
	}
}
