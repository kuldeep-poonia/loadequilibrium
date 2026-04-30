package control

import (
	"math"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

func TestAuthorityConsumesAdvisoryAndEmitsSingleDirective(t *testing.T) {
	a := NewAuthority()
	decision := a.Decide(AuthorityInput{
		ServiceID: "svc-a",
		Tick:      1,
		Now:       time.Unix(1, 0),
		State:     baseAuthorityState(),
		Config:    AuthorityConfig{TargetUtilisation: 0.70, TickSeconds: 1, MaxScaleDelta: 0.30},
		Advisory: AdvisoryBundle{
			Autopilot:    AutopilotAdvice{MinReplicas: 2, MaxReplicas: 8, PredictedBacklog: 30, Confidence: 0.8},
			Intelligence: IntelligenceAdvice{Regime: int(RegimeStressed), RiskEWMA: 0.35, SmoothCost: 0.05, CostBias: 0.1},
			Policy:       PolicyAdvice{DesiredReplicas: 5, MinReplicas: 1, MaxReplicas: 9, Risk: 0.25, Confidence: 0.7},
			Sandbox:      SandboxAdvice{CapacityDelta: 0.2, RiskScore: 0.3, Urgency: 0.4, Confidence: 0.8},
		},
		OptimizerCandidates: []optimisation.ControlCandidate{
			{ServiceID: "svc-a", ScaleFactor: 1.25, Score: 0.1, Feasible: true, Source: "test"},
			{ServiceID: "svc-a", ScaleFactor: 2.50, Score: 9.0, Feasible: false, RiskScore: 0.95, Source: "test"},
		},
	})

	if !decision.Directive.Active {
		t.Fatalf("control authority did not emit an active directive")
	}
	if decision.Directive.ServiceID != "svc-a" {
		t.Fatalf("directive service mismatch: %q", decision.Directive.ServiceID)
	}
	if decision.Directive.ScaleFactor < 0.45 || decision.Directive.ScaleFactor > 10 {
		t.Fatalf("scale outside actuator envelope: %.3f", decision.Directive.ScaleFactor)
	}
	if decision.Bounds.MinReplicas < 2 || decision.Bounds.MaxReplicas > 8 {
		t.Fatalf("autopilot/policy bounds were not enforced: %+v", decision.Bounds)
	}
	if decision.Quality.CandidateCount == 0 {
		t.Fatalf("control authority did not generate/evaluate candidates")
	}
	assertUsedSignal(t, decision.Quality.UsedSignals, "optimizer.ranked_candidates")
	assertUsedSignal(t, decision.Quality.UsedSignals, "autopilot.trajectory")
	assertUsedSignal(t, decision.Quality.UsedSignals, "intelligence.cost_shape")
}

func TestAuthorityDetectsContradictoryAdvisoryBounds(t *testing.T) {
	a := NewAuthority()
	decision := a.Decide(AuthorityInput{
		ServiceID: "svc-b",
		Tick:      1,
		State:     baseAuthorityState(),
		Config:    AuthorityConfig{TargetUtilisation: 0.70, TickSeconds: 1, MaxScaleDelta: 0.30},
		Advisory: AdvisoryBundle{
			Autopilot: AutopilotAdvice{MinReplicas: 9, MaxReplicas: 9, InstabilityRisk: 0.9, Confidence: 0.5},
			Policy:    PolicyAdvice{DesiredReplicas: 3, MinReplicas: 1, MaxReplicas: 5, Risk: 0.2},
		},
	})

	if len(decision.Quality.Contradictions) == 0 {
		t.Fatalf("expected contradictory bounds to be flagged")
	}
	if decision.Bundle.Replicas < decision.Bounds.MinReplicas || decision.Bundle.Replicas > decision.Bounds.MaxReplicas {
		t.Fatalf("selected bundle violates repaired bounds: bundle=%+v bounds=%+v", decision.Bundle, decision.Bounds)
	}
}

func TestAuthoritySteadyLoadDoesNotOscillate(t *testing.T) {
	a := NewAuthority()
	state := baseAuthorityState()
	var prev float64
	totalVariation := 0.0
	for tick := uint64(1); tick <= 30; tick++ {
		d := a.Decide(AuthorityInput{
			ServiceID: "steady",
			Tick:      tick,
			State:     state,
			Config:    AuthorityConfig{TargetUtilisation: 0.70, TickSeconds: 1, MaxScaleDelta: 0.30},
			Advisory: AdvisoryBundle{
				Autopilot:    AutopilotAdvice{MinReplicas: 1, MaxReplicas: 8, Confidence: 0.9},
				Intelligence: IntelligenceAdvice{Regime: int(RegimeCalm), RiskEWMA: 0.05},
				Policy:       PolicyAdvice{DesiredReplicas: state.Replicas, MinReplicas: 1, MaxReplicas: 8, Risk: 0.05, Confidence: 0.9},
			},
		}).Directive
		if math.IsNaN(d.ScaleFactor) || math.IsInf(d.ScaleFactor, 0) {
			t.Fatalf("invalid steady scale: %.3f", d.ScaleFactor)
		}
		if prev != 0 {
			step := math.Abs(d.ScaleFactor - prev)
			if step > 0.300001 {
				t.Fatalf("rate limit violated: prev=%.3f next=%.3f", prev, d.ScaleFactor)
			}
			totalVariation += step
		}
		prev = d.ScaleFactor
	}
	if totalVariation > 1.5 {
		t.Fatalf("steady load oscillated too much: total scale variation %.3f", totalVariation)
	}
}

func TestAuthoritySpikeResponseIsBoundedAndDefensive(t *testing.T) {
	a := NewAuthority()
	state := baseAuthorityState()
	state.PredictedArrival = 90
	state.ArrivalRate = 90
	state.QueueDepth = 120
	state.Utilisation = 2.25
	state.Risk = 0.85

	first := a.Decide(AuthorityInput{
		ServiceID: "spike",
		Tick:      1,
		State:     state,
		Config:    AuthorityConfig{TargetUtilisation: 0.70, TickSeconds: 1, MaxScaleDelta: 0.50},
		Advisory: AdvisoryBundle{
			Autopilot:    AutopilotAdvice{MinReplicas: 4, MaxReplicas: 12, PredictedBacklog: 200, InstabilityRisk: 0.8, OverrideRate: 0.6, Confidence: 0.6, Warning: true},
			Intelligence: IntelligenceAdvice{Regime: int(RegimeUnstable), RiskEWMA: 0.8, AnomalyScore: 0.7},
			Policy:       PolicyAdvice{DesiredReplicas: 8, MinReplicas: 1, MaxReplicas: 12, Risk: 0.7},
			Sandbox:      SandboxAdvice{CapacityDelta: 0.5, RiskScore: 0.75, RiskUp: true},
		},
		OptimizerCandidates: []optimisation.ControlCandidate{
			{ServiceID: "spike", ScaleFactor: 2.0, Score: 0.1, Feasible: true},
		},
	}).Directive

	if first.ScaleFactor < 1.0 {
		t.Fatalf("high-risk spike should not downscale: %.3f", first.ScaleFactor)
	}
	if first.ScaleFactor > 3.0 {
		t.Fatalf("spike response ran away on first decision: %.3f", first.ScaleFactor)
	}

	second := a.Decide(AuthorityInput{
		ServiceID: "spike",
		Tick:      2,
		State:     state,
		Config:    AuthorityConfig{TargetUtilisation: 0.70, TickSeconds: 1, MaxScaleDelta: 0.50},
		Advisory:  AdvisoryBundle{Autopilot: AutopilotAdvice{MinReplicas: 4, MaxReplicas: 12, InstabilityRisk: 0.8, Warning: true}},
	}).Directive
	if math.Abs(second.ScaleFactor-first.ScaleFactor) > 0.500001 {
		t.Fatalf("spike response violated configured rate limit: %.3f -> %.3f", first.ScaleFactor, second.ScaleFactor)
	}
}

func TestAuthorityRegressionBeatsLegacyLateFusionStepChanges(t *testing.T) {
	a := NewAuthority()
	state := baseAuthorityState()
	var prevNew, prevLegacy float64
	var newMaxStep, legacyMaxStep float64

	for tick := uint64(1); tick <= 10; tick++ {
		policyReplicas := 2
		sandboxDelta := -0.3
		autoRisk := 0.2
		if tick%2 == 0 {
			policyReplicas = 8
			sandboxDelta = 0.8
			autoRisk = 0.8
		}

		newScale := a.Decide(AuthorityInput{
			ServiceID: "regression",
			Tick:      tick,
			State:     state,
			Config:    AuthorityConfig{TargetUtilisation: 0.70, TickSeconds: 1, MaxScaleDelta: 0.30},
			Advisory: AdvisoryBundle{
				Autopilot: AutopilotAdvice{MinReplicas: 1, MaxReplicas: 10, InstabilityRisk: autoRisk},
				Policy:    PolicyAdvice{DesiredReplicas: policyReplicas, MinReplicas: 1, MaxReplicas: 10, Risk: autoRisk},
				Sandbox:   SandboxAdvice{CapacityDelta: sandboxDelta, RiskScore: autoRisk},
			},
		}).Directive.ScaleFactor

		legacyScale := legacyLateFusionScale(float64(policyReplicas)/float64(state.Replicas), 1+sandboxDelta, 1+autoRisk)
		if prevNew != 0 {
			newMaxStep = math.Max(newMaxStep, math.Abs(newScale-prevNew))
			legacyMaxStep = math.Max(legacyMaxStep, math.Abs(legacyScale-prevLegacy))
		}
		prevNew = newScale
		prevLegacy = legacyScale
	}

	if newMaxStep > 0.300001 {
		t.Fatalf("new authority exceeded rate limit: %.3f", newMaxStep)
	}
	if legacyMaxStep <= newMaxStep {
		t.Fatalf("expected authority to reduce late-fusion step changes: legacy=%.3f new=%.3f", legacyMaxStep, newMaxStep)
	}
}

func baseAuthorityState() SystemState {
	return SystemState{
		Replicas:         4,
		QueueLimit:       80,
		RetryLimit:       3,
		CacheAggression:  0.2,
		QueueDepth:       20,
		PredictedArrival: 28,
		ArrivalRate:      28,
		ServiceRate:      10,
		Latency:          120,
		Risk:             0.05,
		Utilisation:      0.70,
		SLATarget:        500,
		MinReplicas:      1,
		MaxReplicas:      12,
		MinRetry:         1,
		MaxRetry:         8,
	}
}

func assertUsedSignal(t *testing.T, got []string, want string) {
	t.Helper()
	for _, v := range got {
		if v == want {
			return
		}
	}
	t.Fatalf("expected signal %q to be marked used; got %v", want, got)
}

func legacyLateFusionScale(policy, sandbox, autopilot float64) float64 {
	return (policy + sandbox + autopilot) / 3.0
}
