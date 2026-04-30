package intelligence

import "testing"

func TestAutonomyControlAdapterProducesEvolvingAdvisorySignals(t *testing.T) {
	actDim := 4
	policy := NewPGRuntimePolicy(NewPolicyGradientOptimizer(4))
	roll := NewPredictiveStabilityRollout(4, actDim)
	hazard := NewHazardValueCritic(8)
	rt := NewIntelligenceRuntime(RuntimeModules{
		Meta:    NewMetaAutonomyController(),
		Safety:  NewSafetyConstraintProjector(actDim),
		Rollout: roll,
		Hazard:  hazard,
		Fusion:  NewAutonomyDecisionFusion(actDim),
		Learner: NewAdaptiveSignalAdapter(NewAdaptiveSignalLearner(6)),
		Trainer: policy,
	}, actDim)
	orc := NewAutonomyOrchestrator(
		rt,
		NewAutonomyTelemetryModel(),
		NewSafetyConstraintProjector(actDim),
		roll,
		hazard,
		actDim,
	)
	adapter := NewAutonomyControlAdapter(orc, roll, actDim)
	adapter.BindPolicy(policy.Policy)

	calm := adapter.Step(InfraState{
		QueueDepth:       5,
		LatencyP95:       100,
		CPUUsage:         0.3,
		RetryRate:        0.1,
		CapacityPressure: 0.1,
		SLASeverity:      0.1,
		PerfScore:        0.9,
	})
	stressed := adapter.Step(InfraState{
		QueueDepth:       250,
		LatencyP95:       1200,
		CPUUsage:         1.2,
		RetryRate:        2.5,
		CapacityPressure: 1.0,
		SLASeverity:      1.2,
		PerfScore:        0.2,
	})

	if stressed.RiskEWMA <= calm.RiskEWMA {
		t.Fatalf("risk EWMA did not rise under stress: calm=%.4f stressed=%.4f", calm.RiskEWMA, stressed.RiskEWMA)
	}
	if stressed.Regime < calm.Regime {
		t.Fatalf("regime moved backwards under stress: calm=%d stressed=%d", calm.Regime, stressed.Regime)
	}
	if stressed.AnomalyScore < 0 || stressed.AnomalyScore > 1 {
		t.Fatalf("anomaly advisory out of range: %.4f", stressed.AnomalyScore)
	}
	if stressed.RiskWeight == 0 && stressed.SmoothCost == 0 && stressed.CostBias == 0 {
		t.Fatalf("intelligence cost-shaping advisory is dead")
	}
}
