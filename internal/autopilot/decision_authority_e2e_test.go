package autopilot_test

import (
	"math"
	"testing"

	ap "github.com/loadequilibrium/loadequilibrium/internal/autopilot"
)

func Test_Autopilot_AdvisoryOnly_E2E(t *testing.T) {
	orch := buildAdvisoryRuntime()
	state := ap.RuntimeState{
		Plant: ap.CongestionState{
			Backlog:               80,
			ArrivalMean:           40,
			ArrivalVar:            0.4,
			ServiceRate:           10,
			ServiceEfficiency:     0.9,
			ConcurrencyLimit:      10,
			CapacityActive:        3,
			CapacityTarget:        3,
			RetryFactor:           0.2,
			TopologyAmplification: 1,
		},
		Rollout: ap.RolloutState{CapacityActive: 3},
		ID: ap.IdentificationState{
			ArrivalEstimate: 40,
			ArrivalVar:      0.4,
			ModelConfidence: 0.8,
			ArrivalUpper:    52,
		},
	}

	next, tel := orch.Tick(state, 42, 0.4)

	assertFinitePositive(t, "telemetry backlog", tel.Backlog)
	assertFinitePositive(t, "telemetry capacity", tel.Capacity)
	if math.IsNaN(tel.OverrideRate) || math.IsInf(tel.OverrideRate, 0) || tel.OverrideRate < 0 || tel.OverrideRate > 1 {
		t.Fatalf("override advisory out of range: %.4f", tel.OverrideRate)
	}
	if tel.Confidence < 0 || tel.Confidence > 1 || math.IsNaN(tel.Confidence) {
		t.Fatalf("confidence advisory out of range: %.4f", tel.Confidence)
	}
	if len(next.LastPlan) == 0 {
		t.Fatalf("autopilot must keep producing MPC trajectory advisory")
	}
}

func assertFinitePositive(t *testing.T, name string, v float64) {
	t.Helper()
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		t.Fatalf("%s is not a finite positive advisory: %.4f", name, v)
	}
}

func buildAdvisoryRuntime() *ap.RuntimeOrchestrator {
	tick := 1.0
	return &ap.RuntimeOrchestrator{
		Dt: tick,
		Predictor: &ap.Predictor{
			Dt:                       tick,
			MaxQueue:                 5000,
			BurstEntryRate:           0.10,
			BurstCollapseThreshold:   20,
			BurstIntensity:           0.35,
			ArrivalRiseGain:          0.25,
			ArrivalDropGain:          0.12,
			VarianceDecayRate:        0.10,
			RetryGain:                0.30,
			RetryDelayTau:            1.5,
			DisturbanceSigma:         0.05,
			DisturbanceInjectionGain: 0.02,
			DisturbanceBound:         0.40,
			TopologyCouplingK:        0.35,
			TopologyAdaptTau:         2.0,
			CacheAdaptTau:            2.0,
			LatencyGain:              0.50,
			BarrierExpK:              0.005,
			BarrierCap:               10000,
		},
		MPC: &ap.MPCOptimiser{
			Horizon:       4,
			Dt:            tick,
			ScenarioCount: 3,
			Deterministic: true,
			BurstProb:     0.20,
			LatencyCost:   0.5,
			VarianceBase:  0.2,
			SmoothCost:    0.1,
			TerminalCost:  0.3,
			SafetyBarrier: 0.15,
			RiskQuantile:  0.75,
			RiskWeight:    0.4,
			MaxCapacity:   6,
			MinCapacity:   0.5,
			MaxStepCap:    0.5,
			MaxStepRetry:  0.4,
			MaxStepCache:  0.3,
			InitTemp:      1,
			Cooling:       0.95,
			Iters:         8,
		},
		Safety: &ap.SafetyEngine{
			BaseMaxBacklog:     2000,
			BaseMaxLatency:     2500,
			Alpha:              0.4,
			Beta:               0.2,
			ArrivalGain:        0.01,
			DisturbanceGain:    0.2,
			TopologyGain:       0.2,
			RetryGain:          0.1,
			TailRiskBase:       0.15,
			AccelBaseWindow:    3,
			MaxCapacityRamp:    1,
			CapacityEffectTau:  1,
			TopologyDelayTau:   1,
			TerminalEnergyBase: 1e6,
			ContractionSlack:   0.2,
		},
		Rollout: &ap.RolloutController{
			Dt:                    tick,
			CapRampUpNormal:       2,
			CapRampUpEmergency:    0.9,
			CapRampDown:           0.4,
			RetryEnableRamp:       0.5,
			RetryDisableRamp:      0.3,
			CacheEnableRamp:       0.4,
			CacheDisableRamp:      0.3,
			WarmupTau:             1,
			ConfigLagTau:          2,
			QueueMax:              16,
			QueuePressureRampGain: 0.5,
			EmergencyBacklog:      300,
			DegradedBacklog:       150,
			RolloutTimeout:        2,
			MaxRetries:            3,
			SuccessProbBase:       0.95,
			InfraFailureGain:      0.4,
		},
		ID: &ap.IdentificationEngine{
			Dt:                  tick,
			FastGain:            0.35,
			SlowGain:            0.10,
			BlendGain:           0.10,
			VarGain:             0.10,
			BurstGain:           0.10,
			BurstDecay:          0.05,
			NoiseGain:           0.20,
			DriftGain:           0.05,
			BaseConfidenceFloor: 0.20,
			ConfidenceGain:      0.15,
			ReliabilityGain:     0.10,
			InfraSensitivity:    0.5,
			SLAWeightQueue:      0.5,
			SLAWeightLatency:    0.5,
			EVTFactor:           2.0,
			DampingGain:         0.10,
		},
		SLA_Backlog:       100,
		OverrideWindow:    16,
		DampingMin:        1,
		DampingMax:        3,
		TelemetryTau:      2,
		FailureScaleProb:  0,
		FailureConfigProb: 0,
	}
}
