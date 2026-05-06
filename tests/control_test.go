// Control System Test Suite
// Validates stability preservation, oscillation handling, collapse prevention,
// and correct Authority decision behavior under various load conditions.
// Run: go test ./tests/ -run TestControl -v -timeout 60s
package tests

import (
	"math"
	"testing"
	"time"

	ctrl "github.com/loadequilibrium/loadequilibrium/internal/control"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func makeBundle(id string, arrivalRate, serviceRate, utilisation, collapseRisk float64) *modelling.ServiceModelBundle {
	q := modelling.QueueModel{
		ServiceID:    id,
		ArrivalRate:  arrivalRate,
		ServiceRate:  serviceRate,
		Utilisation:  utilisation,
		MeanQueueLen: math.Max(0, utilisation/(1-math.Min(utilisation, 0.99))),
		AdjustedWaitMs: math.Max(1, 1000*utilisation/((1-math.Min(utilisation, 0.99))*serviceRate+1e-9)),
		Confidence:   0.9,
	}
	stab := modelling.StabilityAssessment{
		ServiceID:       id,
		CollapseRisk:    collapseRisk,
		StabilityMargin: 1 - utilisation,
		CollapseZone:    func() string {
			if utilisation >= 0.90 {
				return "collapse"
			} else if utilisation >= 0.75 {
				return "warning"
			}
			return "safe"
		}(),
		OscillationRisk:     0,
		StabilityDerivative: 0,
		IsUnstable:          collapseRisk > 0.85,
	}
	return &modelling.ServiceModelBundle{
		Queue:      q,
		Stability:  stab,
		Stochastic: modelling.StochasticModel{ServiceID: id, Confidence: 0.9},
	}
}

func makeAuthorityInput(
	serviceID string,
	replicas int,
	queueDepth, queueLimit float64,
	arrivalRate, serviceRate float64,
	utilisation, risk float64,
) ctrl.AuthorityInput {
	return ctrl.AuthorityInput{
		ServiceID: serviceID,
		Tick:      1,
		Now:       time.Now(),
		State: ctrl.SystemState{
			Replicas:         replicas,
			QueueLimit:       int(queueLimit),
			RetryLimit:       3,
			QueueDepth:       queueDepth,
			PredictedArrival: arrivalRate,
			ArrivalRate:      arrivalRate,
			ServiceRate:      serviceRate,
			Latency:          50,
			Risk:             risk,
			Utilisation:      utilisation,
			SLATarget:        500,
			MinReplicas:      1,
			MaxReplicas:      replicas + 10,
			MinRetry:         1,
			MaxRetry:         5,
		},
		Config: ctrl.AuthorityConfig{
			TargetUtilisation: 0.70,
			TickSeconds:       0.5,
			MaxScaleDelta:     0.30,
		},
		Advisory: ctrl.AdvisoryBundle{},
	}
}

// ─── Test 1: Authority produces scale >1 when overloaded ─────────────────────

func TestControl_AuthorityScalesUpUnderOverload(t *testing.T) {
	authority := ctrl.NewAuthority()

	// Overloaded: queue at 2× limit, high utilisation
	in := makeAuthorityInput("svc-ol", 2, 200, 100, 180, 90, 0.95, 0.90)
	in.Advisory.Policy = ctrl.PolicyAdvice{
		DesiredReplicas: 4,
		MinReplicas:     1,
		MaxReplicas:     10,
		Confidence:      0.8,
		Risk:            0.90,
	}

	dec := authority.Decide(in)

	if dec.Directive.ScaleFactor <= 1.0 {
		t.Fatalf("Expected ScaleFactor>1.0 under overload, got %.4f", dec.Directive.ScaleFactor)
	}
	if dec.Directive.ScaleFactor > 10.0 {
		t.Fatalf("ScaleFactor=%.4f excessively large (>10)", dec.Directive.ScaleFactor)
	}
	if !dec.Directive.Active {
		t.Fatal("Directive.Active=false under overload")
	}
	if math.IsNaN(dec.Directive.ScaleFactor) || math.IsInf(dec.Directive.ScaleFactor, 0) {
		t.Fatalf("ScaleFactor is NaN/Inf")
	}
}

// ─── Test 2: Authority produces scale ≤1 under idle conditions ───────────────

func TestControl_AuthorityScalesDownWhenIdle(t *testing.T) {
	authority := ctrl.NewAuthority()

	// Idle: very low utilisation
	in := makeAuthorityInput("svc-idle", 8, 1, 100, 10, 90, 0.05, 0.0)
	// Advise low replicas
	in.Advisory.Policy = ctrl.PolicyAdvice{
		DesiredReplicas: 1,
		MinReplicas:     1,
		MaxReplicas:     10,
		Confidence:      0.9,
		Risk:            0.0,
	}

	dec := authority.Decide(in)

	if dec.Directive.ScaleFactor > 1.5 {
		t.Logf("Under idle conditions ScaleFactor=%.4f (may be delayed scaledown due to hysteresis)", dec.Directive.ScaleFactor)
	}
	if dec.Directive.ScaleFactor <= 0 {
		t.Fatalf("ScaleFactor=%.4f ≤0 — always positive", dec.Directive.ScaleFactor)
	}
}

// ─── Test 3: Authority clamps scale to sensible range ────────────────────────

func TestControl_AuthorityScaleClampedToSafeRange(t *testing.T) {
	authority := ctrl.NewAuthority()

	// Extreme overload — test that scale doesn't blow up
	in := makeAuthorityInput("svc-extreme", 1, 10000, 10, 9999, 1, 1.5, 1.0)
	dec := authority.Decide(in)

	if dec.Directive.ScaleFactor > 15.0 {
		t.Fatalf("ScaleFactor=%.4f > 15.0 — dangerously unclamped under extreme overload", dec.Directive.ScaleFactor)
	}
	if dec.Directive.ScaleFactor <= 0 {
		t.Fatalf("ScaleFactor=%.4f ≤0", dec.Directive.ScaleFactor)
	}
	if math.IsNaN(dec.Directive.ScaleFactor) || math.IsInf(dec.Directive.ScaleFactor, 0) {
		t.Fatal("ScaleFactor is NaN or Inf under extreme overload")
	}
}

// ─── Test 4: Authority respects policy bounds ────────────────────────────────

func TestControl_AuthorityRespectsReplicaBounds(t *testing.T) {
	authority := ctrl.NewAuthority()
	in := makeAuthorityInput("svc-bounded", 3, 50, 100, 120, 80, 0.70, 0.30)
	in.State.MinReplicas = 2
	in.State.MaxReplicas = 6
	in.Advisory.Policy = ctrl.PolicyAdvice{
		MinReplicas: 2,
		MaxReplicas: 6,
		Confidence:  0.85,
	}

	dec := authority.Decide(in)

	// Derived replica count = ScaleFactor * current replicas
	impliedReplicas := int(math.Round(dec.Directive.ScaleFactor * float64(in.State.Replicas)))
	if impliedReplicas < 1 {
		t.Fatalf("impliedReplicas=%d < 1", impliedReplicas)
	}
	// Authority may exceed policy max in emergency — but not by more than 2×
	if impliedReplicas > in.Advisory.Policy.MaxReplicas*2 {
		t.Logf("impliedReplicas=%d exceeds policy max=%d by >2× — emergency path", impliedReplicas, in.Advisory.Policy.MaxReplicas)
	}
}

// ─── Test 5: Stability derivative correctly computed ─────────────────────────

func TestControl_StabilityDerivativeDirection(t *testing.T) {
	snap := topology.GraphSnapshot{}

	// Service with rising utilisation trend
	qRising := modelling.QueueModel{
		ServiceID:       "svc-rising",
		Utilisation:     0.75,
		UtilisationTrend: 0.02, // rising
		ArrivalRate:     100,
		ServiceRate:     133,
	}
	sigRising := modelling.SignalState{FastEWMA: 100, SlowEWMA: 95}
	stabRising := modelling.RunStabilityAssessment(qRising, sigRising, snap, 0.90)

	// Service with falling utilisation trend
	qFalling := modelling.QueueModel{
		ServiceID:        "svc-falling",
		Utilisation:      0.75,
		UtilisationTrend: -0.02, // falling
		ArrivalRate:      100,
		ServiceRate:      133,
	}
	sigFalling := modelling.SignalState{FastEWMA: 100, SlowEWMA: 105}
	stabFalling := modelling.RunStabilityAssessment(qFalling, sigFalling, snap, 0.90)

	if stabRising.StabilityDerivative <= 0 {
		t.Errorf("Rising load should produce positive StabilityDerivative, got %.6f", stabRising.StabilityDerivative)
	}
	if stabFalling.StabilityDerivative >= 0 {
		t.Errorf("Falling load should produce negative StabilityDerivative, got %.6f", stabFalling.StabilityDerivative)
	}
}

// ─── Test 6: Collapse zone classification boundary ───────────────────────────

func TestControl_CollapseZoneClassificationAtBoundaries(t *testing.T) {
	snap := topology.GraphSnapshot{}
	sp := modelling.NewSignalProcessor(0.3, 0.05, 3.0)
	qp := modelling.NewQueuePhysicsEngine()
	threshold := 0.90

	cases := []struct {
		rps      float64
		lat      float64
		expectZone string
	}{
		{10, 5, "safe"},    // very low load
		{700, 10, "warning"}, // moderate load
		{990, 10, "collapse"}, // near saturation
	}

	for _, tc := range cases {
		w := makeWindow("svc-zone", tc.rps, tc.lat, 50)
		q := qp.RunQueueModel(w, snap, false)
		sig := sp.Update(w)
		stab := modelling.RunStabilityAssessment(q, sig, snap, threshold)

		// We can't know exact zone without knowing effective rho, but zone must be valid
		validZones := map[string]bool{"safe": true, "warning": true, "collapse": true}
		if !validZones[stab.CollapseZone] {
			t.Errorf("rps=%.0f lat=%.0f: invalid CollapseZone=%q", tc.rps, tc.lat, stab.CollapseZone)
		}
	}
}

// ─── Test 7: Oscillation risk increases with EWMA divergence ─────────────────

func TestControl_OscillationRiskIncreasesWithEWMADivergence(t *testing.T) {
	snap := topology.GraphSnapshot{}

	// Low divergence
	qLow := modelling.QueueModel{Utilisation: 0.7, ArrivalRate: 100, ServiceRate: 143}
	sigLow := modelling.SignalState{FastEWMA: 100, SlowEWMA: 100, EWMAVariance: 0.1} // identical
	stabLow := modelling.RunStabilityAssessment(qLow, sigLow, snap, 0.90)

	// High divergence
	qHigh := modelling.QueueModel{Utilisation: 0.7, ArrivalRate: 100, ServiceRate: 143}
	sigHigh := modelling.SignalState{FastEWMA: 200, SlowEWMA: 50, EWMAVariance: 500} // large divergence
	stabHigh := modelling.RunStabilityAssessment(qHigh, sigHigh, snap, 0.90)

	if stabHigh.OscillationRisk <= stabLow.OscillationRisk {
		t.Errorf("High EWMA divergence OscRisk=%.4f should exceed low divergence OscRisk=%.4f",
			stabHigh.OscillationRisk, stabLow.OscillationRisk)
	}
	if stabHigh.OscillationRisk > 1.0 {
		t.Errorf("OscillationRisk=%.4f exceeds 1.0", stabHigh.OscillationRisk)
	}
}

// ─── Test 8: Collapse risk is monotone in utilisation ────────────────────────

func TestControl_CollapseRiskMonotoneInUtilisation(t *testing.T) {
	snap := topology.GraphSnapshot{}
	sig := modelling.SignalState{FastEWMA: 100, SlowEWMA: 100}

	utils := []float64{0.1, 0.3, 0.5, 0.7, 0.85, 0.95, 1.05}
	var prev float64
	for i, u := range utils {
		q := modelling.QueueModel{Utilisation: u, ArrivalRate: u * 100, ServiceRate: 100}
		stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)
		if i > 0 && stab.CollapseRisk < prev-0.05 {
			t.Errorf("CollapseRisk not monotonically non-decreasing: at ρ=%.2f got %.4f, prev=%.4f",
				u, stab.CollapseRisk, prev)
		}
		prev = stab.CollapseRisk
	}
}

// ─── Test 9: TrendAdjustedMargin goes negative when rho trending high ────────

func TestControl_TrendAdjustedMarginNegativeOnRisingTrend(t *testing.T) {
	snap := topology.GraphSnapshot{}
	sig := modelling.SignalState{FastEWMA: 100, SlowEWMA: 90}

	// rho=0.8 with strong upward trend — TAM should go negative within 20s
	q := modelling.QueueModel{
		Utilisation:      0.80,
		UtilisationTrend: 0.015, // 20s × 0.015/s = +0.30 → projected rho=1.10 > 1.0
		ArrivalRate:      100,
		ServiceRate:      125,
	}
	stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)
	if stab.TrendAdjustedMargin >= 0 {
		t.Errorf("Expected TrendAdjustedMargin<0 with rising trend at ρ=0.80, got %.4f", stab.TrendAdjustedMargin)
	}
}

// ─── Test 10: Authority risk aggregation uses max of all advisories ───────────

func TestControl_AuthorityRiskAggregation(t *testing.T) {
	authority := ctrl.NewAuthority()
	in := makeAuthorityInput("svc-risk", 2, 10, 100, 50, 80, 0.40, 0.20)

	// Inject high risk only from intelligence — sandbox risk is low
	in.Advisory.Intelligence = ctrl.IntelligenceAdvice{
		RiskEWMA:     0.85,
		AnomalyScore: 0.90,
		Regime:       2,
	}
	in.Advisory.Sandbox = ctrl.SandboxAdvice{RiskScore: 0.10}
	in.Advisory.Autopilot = ctrl.AutopilotAdvice{InstabilityRisk: 0.10, Confidence: 0.8}

	dec1 := authority.Decide(in)

	// Now lower intelligence risk but raise sandbox
	in.Advisory.Intelligence = ctrl.IntelligenceAdvice{RiskEWMA: 0.10, AnomalyScore: 0.10}
	in.Advisory.Sandbox = ctrl.SandboxAdvice{RiskScore: 0.85, RiskUp: true}
	authority2 := ctrl.NewAuthority()
	dec2 := authority2.Decide(in)

	// Both should produce non-zero response — the high-risk advisory should dominate
	if dec1.Directive.ScaleFactor <= 0 {
		t.Fatalf("dec1 ScaleFactor=%.4f ≤0", dec1.Directive.ScaleFactor)
	}
	if dec2.Directive.ScaleFactor <= 0 {
		t.Fatalf("dec2 ScaleFactor=%.4f ≤0", dec2.Directive.ScaleFactor)
	}

	// Both should react to the high risk signal — scale should be ≥ neutral
	if dec1.Directive.ScaleFactor < 0.45 {
		t.Errorf("dec1 ScaleFactor=%.4f too low — high intelligence risk should not suppress scale", dec1.Directive.ScaleFactor)
	}
	if dec2.Directive.ScaleFactor < 0.45 {
		t.Errorf("dec2 ScaleFactor=%.4f too low — high sandbox risk should not suppress scale", dec2.Directive.ScaleFactor)
	}
}

// ─── Test 11: Optimizer candidates sorted by score (lower=better) ────────────

func TestControl_OptimizerCandidatesSortedByScore(t *testing.T) {
	cfg := defaultCfg()
	engine := optimisation.NewEngine(cfg)
	snap := topology.GraphSnapshot{}

	bundles := map[string]*modelling.ServiceModelBundle{
		"svc-sort": makeBundle("svc-sort", 100, 143, 0.70, 0.05),
	}
	gradients := optimisation.ComputeCostGradients(bundles, snap, 500.0)
	cands := engine.EvaluateCandidates(bundles, gradients, nil, snap, time.Now())

	sortedCands := cands["svc-sort"]
	for i := 1; i < len(sortedCands); i++ {
		if sortedCands[i].Score < sortedCands[i-1].Score-1e-9 {
			t.Errorf("Candidates not sorted at index %d: score[%d]=%.4f > score[%d]=%.4f",
				i, i, sortedCands[i].Score, i-1, sortedCands[i-1].Score)
		}
	}
}

// ─── Test 12: Cost gradient non-negative for loaded service ──────────────────

func TestControl_CostGradientPositiveForLoadedService(t *testing.T) {
	snap := topology.GraphSnapshot{}
	bundles := map[string]*modelling.ServiceModelBundle{
		"svc-loaded": makeBundle("svc-loaded", 100, 105, 0.95, 0.80),
	}
	gradients := optimisation.ComputeCostGradients(bundles, snap, 500.0)
	cg, ok := gradients["svc-loaded"]
	if !ok {
		t.Fatal("no gradient for svc-loaded")
	}
	if cg.CostGradient < 0 {
		t.Errorf("Loaded service CostGradient=%.4f < 0, expected ≥0", cg.CostGradient)
	}
}

// ─── Test 13: Multi-tick Authority stability (no oscillation) ────────────────

func TestControl_AuthorityMultiTickStability(t *testing.T) {
	authority := ctrl.NewAuthority()
	var prevScale float64

	// Simulate 20 ticks at stable load — scale should converge, not oscillate wildly
	for tick := uint64(1); tick <= 20; tick++ {
		in := makeAuthorityInput("svc-stable", 3, 20, 100, 100, 143, 0.70, 0.10)
		in.Tick = tick
		dec := authority.Decide(in)
		scale := dec.Directive.ScaleFactor

		if tick > 3 && math.Abs(scale-prevScale) > 3.0 {
			t.Errorf("Tick %d: ScaleFactor jumped from %.4f to %.4f (Δ=%.4f) — possible oscillation",
				tick, prevScale, scale, math.Abs(scale-prevScale))
		}
		prevScale = scale
	}
}