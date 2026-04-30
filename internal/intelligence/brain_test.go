// Package intelligence — Brain Intelligence Test (INSTRUMENTED)
//
// Run:  go test ./internal/intelligence/ -v -run "Test(Fusion|Meta|Learner|Safety|PGO|BrainReport)"
//
// Every test prints actual values so you can judge quality, not just pass/fail.
// A JSON report is written to brain_test_report.json when TestBrainReport runs.

package intelligence

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"
)

// =============================================================================
// RESULT COLLECTOR  (shared across all tests)
// =============================================================================

type TestResult struct {
	Name    string            `json:"name"`
	Pass    bool              `json:"pass"`
	Values  map[string]string `json:"values"`
	Comment string            `json:"comment"`
}

var allResults []TestResult

func record(name string, pass bool, comment string, kv ...interface{}) {
	vals := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		vals[fmt.Sprintf("%v", kv[i])] = fmt.Sprintf("%v", kv[i+1])
	}
	allResults = append(allResults, TestResult{
		Name:    name,
		Pass:    pass,
		Values:  vals,
		Comment: comment,
	})
}

// =============================================================================
// SHARED HELPERS
// =============================================================================

func makeVec(dim int, val float64) []float64 {
	v := make([]float64, dim)
	for i := range v {
		v[i] = val
	}
	return v
}

func assertFinite(t *testing.T, label string, vals []float64) bool {
	t.Helper()
	for i, v := range vals {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s[%d] = %v — not finite", label, i, v)
			return false
		}
	}
	return true
}

func lowRiskIn(actDim int) FusionInput {
	return FusionInput{
		State:        makeVec(actDim, 0.05),
		StateDeriv:   makeVec(actDim, 0.0),
		Stability:    makeVec(actDim, 0.05),
		PolicyAction: makeVec(actDim, 1.0),
		MPCAction:    makeVec(actDim, 1.0),
		PolicyUnc:    0.1,
		MPCUnc:       0.1,
		HazardProb:   0.01,
		RiskForecast: []float64{0.02, 0.02, 0.02},
		Epistemic:    0.05,
		RegimeID:     0,
		PerfSignal:   0.85,
		SLASeverity:  0.0,
	}
}

func highRiskIn(actDim int) FusionInput {
	return FusionInput{
		State:        makeVec(actDim, 3.0),
		StateDeriv:   makeVec(actDim, 0.0),
		Stability:    makeVec(actDim, 4.0),
		PolicyAction: makeVec(actDim, 0.5),
		MPCAction:    makeVec(actDim, 0.5),
		PolicyUnc:    0.5,
		MPCUnc:       0.5,
		HazardProb:   0.95,
		RiskForecast: []float64{0.90, 0.92, 0.94, 0.96, 0.95},
		Epistemic:    0.90,
		RegimeID:     0,
		PerfSignal:   0.05,
		SLASeverity:  0.90,
	}
}

func warmupLearner(l *AdaptiveSignalLearner, steps int, base time.Time) {
	for i := 0; i < steps; i++ {
		l.Update(SignalVector{
			Timestamp:    base.Add(time.Duration(i) * time.Second),
			BacklogError: 1.0,
			LatencyError: 0.5,
			CPUError:     0.8,
		})
	}
}

// =============================================================================
// 1. AUTONOMY DECISION FUSION
// =============================================================================

func TestFusion_SafetyOverride_FiresAtHighRisk(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	out := f.Fuse(highRiskIn(3))

	pass := out.SafetyOverride
	t.Logf("  RiskScore      = %.6f  (want > dynamic threshold ~0.55)", out.RiskScore)
	t.Logf("  SafetyOverride = %v     (want true)", out.SafetyOverride)
	t.Logf("  Actions        = [%.4f, %.4f, %.4f]", out.Action[0], out.Action[1], out.Action[2])

	record(t.Name(), pass,
		"Safety override must trigger at high hazard",
		"risk_score", fmt.Sprintf("%.6f", out.RiskScore),
		"override", fmt.Sprintf("%v", out.SafetyOverride),
	)
	if !pass {
		t.Errorf("SafetyOverride must be true at high hazard — got false (risk=%.4f)", out.RiskScore)
	}
}

func TestFusion_SafetyOverride_SilentAtLowRisk(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	out := f.Fuse(lowRiskIn(3))

	pass := !out.SafetyOverride
	t.Logf("  RiskScore      = %.6f  (want < dynamic threshold ~0.55)", out.RiskScore)
	t.Logf("  SafetyOverride = %v     (want false)", out.SafetyOverride)

	record(t.Name(), pass,
		"No false-alarm override at low risk",
		"risk_score", fmt.Sprintf("%.6f", out.RiskScore),
		"override", fmt.Sprintf("%v", out.SafetyOverride),
	)
	if !pass {
		t.Errorf("SafetyOverride must be false at low risk — got true (risk=%.4f)", out.RiskScore)
	}
}

func TestFusion_RiskScore_AlwaysBounded(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	cases := map[string]FusionInput{"low": lowRiskIn(3), "high": highRiskIn(3)}
	allPass := true
	for name, in := range cases {
		out := f.Fuse(in)
		inBounds := out.RiskScore >= 0 && out.RiskScore <= 1
		t.Logf("  [%s] RiskScore = %.6f  (bounds [0.0, 1.0])", name, out.RiskScore)
		record("TestFusion_RiskScore_AlwaysBounded/"+name, inBounds,
			"Risk must be a valid probability",
			"risk_score", fmt.Sprintf("%.6f", out.RiskScore),
		)
		if !inBounds {
			t.Errorf("[%s] RiskScore=%.4f not in [0, 1]", name, out.RiskScore)
			allPass = false
		}
	}
	_ = allPass
}

func TestFusion_OutputActions_AlwaysFinite(t *testing.T) {
	f := NewAutonomyDecisionFusion(4)
	out := f.Fuse(highRiskIn(4))

	allOk := true
	for i, v := range out.Action {
		t.Logf("  action[%d] = %.6f  (NaN=%v Inf=%v)", i, v, math.IsNaN(v), math.IsInf(v, 0))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			allOk = false
		}
	}
	record(t.Name(), allOk, "No NaN/Inf may propagate through fusion pipeline",
		"action_0", fmt.Sprintf("%.6f", out.Action[0]),
		"action_1", fmt.Sprintf("%.6f", out.Action[1]),
	)
	if !allOk {
		t.Error("non-finite value found in output actions")
	}
}

func TestFusion_StochasticMargin_ZeroAtMaxRisk(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	a := makeVec(3, 1.5)
	out := f.stochasticMargin(a, 1.0)

	maxDelta := 0.0
	for i, v := range out {
		d := math.Abs(v - a[i])
		if d > maxDelta {
			maxDelta = d
		}
		t.Logf("  a[%d]=%.4f  out[%d]=%.4f  delta=%.2e", i, a[i], i, v, d)
	}
	t.Logf("  max delta = %.2e  (want < 1e-12 — sigma = 0.15*(1-1.0) = 0)", maxDelta)

	pass := maxDelta < 1e-12
	record(t.Name(), pass, "Exploration is zero when risk=1 (max vulnerability)",
		"max_delta", fmt.Sprintf("%.2e", maxDelta),
		"sigma_formula", "0.15*(1-risk)=0.0",
	)
	if !pass {
		t.Errorf("dithering at risk=1 must be 0, max delta=%.2e", maxDelta)
	}
}

func TestFusion_StochasticMargin_NonZeroAtZeroRisk(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	a := makeVec(3, 1.5)
	maxObserved := 0.0
	anyNonZero := false
	for i := 0; i < 50; i++ {
		out := f.stochasticMargin(a, 0.0)
		for j, v := range out {
			d := math.Abs(v - a[j])
			if d > maxObserved {
				maxObserved = d
			}
			if d > 1e-9 {
				anyNonZero = true
			}
		}
	}
	t.Logf("  max delta over 50 samples = %.6f  (sigma = 0.15*(1-0.0) = 0.075)", maxObserved)
	t.Logf("  any nonzero dithering found = %v  (want true)", anyNonZero)

	record(t.Name(), anyNonZero, "Exploration is active when risk=0 (safe regime)",
		"max_observed_delta", fmt.Sprintf("%.6f", maxObserved),
		"sigma_formula", "0.15*(1-0.0)=0.075",
	)
	if !anyNonZero {
		t.Error("expected non-zero dithering at risk=0")
	}
}

func TestFusion_RegimeBias_PositiveAfterGoodPerf(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	in := FusionInput{RegimeID: 5, PerfSignal: 0.9, SLASeverity: 0.0}
	for i := 0; i < 60; i++ {
		f.learnRegime(in)
	}
	b := f.bias(5)
	t.Logf("  regime 5 bias after 60 good-perf steps = %.6f  (want > 0)", b)
	t.Logf("  delta_per_step = 0.02*(0.9-0.5)+0.03*0 = 0.008 -> 60 steps -> clamped to 0.4")

	pass := b > 0
	record(t.Name(), pass, "System learns which regimes are productive",
		"regime_5_bias", fmt.Sprintf("%.6f", b),
		"expected_range", "[0, 0.4]",
	)
	if !pass {
		t.Errorf("regime 5 bias should be positive after good perf, got %.4f", b)
	}
}

func TestFusion_RegimeBias_NegativeAfterBadPerf(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	in := FusionInput{RegimeID: 7, PerfSignal: 0.0, SLASeverity: 0.0}
	for i := 0; i < 60; i++ {
		f.learnRegime(in)
	}
	b := f.bias(7)
	t.Logf("  regime 7 bias after 60 bad-perf steps = %.6f  (want < 0)", b)
	t.Logf("  delta_per_step = 0.02*(0.0-0.5)+0.03*0 = -0.01 -> 60 steps -> clamped to -0.4")

	pass := b < 0
	record(t.Name(), pass, "System learns to distrust failing regimes",
		"regime_7_bias", fmt.Sprintf("%.6f", b),
		"expected_range", "[-0.4, 0]",
	)
	if !pass {
		t.Errorf("regime 7 bias should be negative after bad perf, got %.4f", b)
	}
}

func TestFusion_OscillationDetector_BuildsPhaseEWOnAlternatingDerivative(t *testing.T) {
	f := NewAutonomyDecisionFusion(3)
	a := makeVec(3, 1.0)
	lo := FusionInput{StateDeriv: makeVec(3, 0.1)}
	hi := FusionInput{StateDeriv: makeVec(3, 12.0)}

	for i := 0; i < 40; i++ {
		if i%2 == 0 {
			f.freqDamp(a, hi)
		} else {
			f.freqDamp(a, lo)
		}
	}
	t.Logf("  phaseEW after 40 alternating steps = %.6f  (want > 0)", f.phaseEW)
	t.Logf("  freqEW                              = %.6f", f.freqEW)
	t.Logf("  formula: phaseEW += 0.08*|d - freqEW| per step")

	pass := f.phaseEW > 0
	record(t.Name(), pass, "Oscillation detector tracks derivative variance",
		"phaseEW", fmt.Sprintf("%.6f", f.phaseEW),
		"freqEW", fmt.Sprintf("%.6f", f.freqEW),
	)
	if !pass {
		t.Error("phaseEW must be > 0 after alternating high/low derivatives")
	}
}

// =============================================================================
// 2. META AUTONOMY CONTROLLER
// =============================================================================

func TestMeta_GovernanceMode_ShiftsToSafetyUnderCrisis(t *testing.T) {
	m := NewMetaAutonomyController()
	crisis := MetaInput{
		GlobalRisk: 0.95, RiskForecast: []float64{0.90, 0.93, 0.97},
		HazardUnc: 0.80, ModelUnc: 0.70, EpistemicTrend: 0.80,
		PerfSignal: 0.0, PerfTrend: -0.5, StabilityMargin: -0.5,
		SLASeverity: 0.95, OscPenalty: 0.80, CapacityPressure: 0.90,
	}
	var out MetaOutput
	for i := 0; i < 28; i++ {
		out = m.Step(crisis)
	}
	t.Logf("  GovernanceMode  = %d  (0=auto, 1=supervised, 2=safety -- want 2)", out.GovernanceMode)
	t.Logf("  AutonomyLevel   = %.6f", out.AutonomyLevel)
	t.Logf("  SafetyGain      = %.6f", out.SafetyGain)
	t.Logf("  ExplorationGate = %.6f", out.ExplorationGate)
	t.Logf("  modeBelief      = [auto=%.4f, sup=%.4f, safe=%.4f]",
		m.modeBelief[0], m.modeBelief[1], m.modeBelief[2])

	pass := out.GovernanceMode == 2
	record(t.Name(), pass, "Meta-controller escalates to safety mode under crisis",
		"governance_mode", fmt.Sprintf("%d", out.GovernanceMode),
		"autonomy_level", fmt.Sprintf("%.6f", out.AutonomyLevel),
		"mode_belief_auto", fmt.Sprintf("%.4f", m.modeBelief[0]),
		"mode_belief_safe", fmt.Sprintf("%.4f", m.modeBelief[2]),
	)
	if !pass {
		t.Errorf("expected GovernanceMode=2 under crisis, got %d", out.GovernanceMode)
	}
}

func TestMeta_GovernanceMode_StaysAutonomousUnderNominal(t *testing.T) {
	m := NewMetaAutonomyController()
	nominal := MetaInput{
		GlobalRisk: 0.05, RiskForecast: []float64{0.04, 0.05, 0.04},
		HazardUnc: 0.10, ModelUnc: 0.10, EpistemicTrend: 0.05,
		PerfSignal: 0.90, PerfTrend: 0.30, StabilityMargin: 0.80,
		SLASeverity: 0.0, OscPenalty: 0.0, Regime: 1,
	}
	var out MetaOutput
	for i := 0; i < 28; i++ {
		out = m.Step(nominal)
	}
	t.Logf("  GovernanceMode  = %d  (want 0 = autonomous)", out.GovernanceMode)
	t.Logf("  AutonomyLevel   = %.6f  (want close to 1.0)", out.AutonomyLevel)
	t.Logf("  modeBelief      = [auto=%.4f, sup=%.4f, safe=%.4f]",
		m.modeBelief[0], m.modeBelief[1], m.modeBelief[2])

	pass := out.GovernanceMode == 0
	record(t.Name(), pass, "Meta-controller stays autonomous under nominal conditions",
		"governance_mode", fmt.Sprintf("%d", out.GovernanceMode),
		"autonomy_level", fmt.Sprintf("%.6f", out.AutonomyLevel),
		"mode_belief_auto", fmt.Sprintf("%.4f", m.modeBelief[0]),
	)
	if !pass {
		t.Errorf("expected GovernanceMode=0, got %d", out.GovernanceMode)
	}
}

func TestMeta_ExplorationGate_NearZeroAtExtremeRisk(t *testing.T) {
	hiRisk := MetaInput{
		GlobalRisk: 2.0, EpistemicTrend: 2.0,
		EntropyProxy: 0.5, GradMagProxy: 0.5, ReplayNovelty: 0.5,
		PerfSignal: 0.5,
	}
	loRisk := MetaInput{
		GlobalRisk: 0.05, EpistemicTrend: 0.05,
		EntropyProxy: 0.5, GradMagProxy: 0.5, ReplayNovelty: 0.5,
		PerfSignal: 0.5,
	}
	outHi := NewMetaAutonomyController().Step(hiRisk)
	outLo := NewMetaAutonomyController().Step(loRisk)

	t.Logf("  ExplorationGate @ high risk = %.6f  (want < 0.05)", outHi.ExplorationGate)
	t.Logf("  ExplorationGate @ low  risk = %.6f", outLo.ExplorationGate)
	t.Logf("  Ratio (lo/hi)               = %.2fx  (want > 1)", outLo.ExplorationGate/(outHi.ExplorationGate+1e-9))
	t.Logf("  Formula: riskSupp = 1 - sigmoid(globalRisk + epistemicTrend)")

	pass := outHi.ExplorationGate < outLo.ExplorationGate && outHi.ExplorationGate <= 0.05
	record(t.Name(), pass, "No exploration when the system is blind and at risk",
		"gate_high_risk", fmt.Sprintf("%.6f", outHi.ExplorationGate),
		"gate_low_risk", fmt.Sprintf("%.6f", outLo.ExplorationGate),
		"ratio", fmt.Sprintf("%.2fx", outLo.ExplorationGate/(outHi.ExplorationGate+1e-9)),
	)
	if !pass {
		t.Errorf("exploration not suppressed at high risk: high=%.5f, low=%.5f",
			outHi.ExplorationGate, outLo.ExplorationGate)
	}
}

func TestMeta_AutonomyLevel_AlwaysBounded(t *testing.T) {
	m := NewMetaAutonomyController()
	cases := []struct {
		name string
		in   MetaInput
	}{
		{
			name: "crisis",
			in: MetaInput{
				GlobalRisk:  0.95,
				SLASeverity: 0.9,
				PerfSignal:  0.0,
			},
		},
		{
			name: "nominal",
			in: MetaInput{
				GlobalRisk:  0.01,
				SLASeverity: 0.0,
				PerfSignal:  1.0,
			},
		},
	}
	for _, c := range cases {
		out := m.Step(c.in)
		inBounds := out.AutonomyLevel >= 0 && out.AutonomyLevel <= 1
		t.Logf("  [%s] AutonomyLevel = %.6f  bounds=[0,1] ok=%v", c.name, out.AutonomyLevel, inBounds)
		record("TestMeta_AutonomyLevel/"+c.name, inBounds, "Autonomy level must be a valid fraction",
			"autonomy_level", fmt.Sprintf("%.6f", out.AutonomyLevel),
		)
		if !inBounds {
			t.Errorf("[%s] AutonomyLevel=%.4f out of [0,1]", c.name, out.AutonomyLevel)
		}
	}
}

func TestMeta_RegimeScore_ConvergesPositiveOnGoodRegime(t *testing.T) {
	m := NewMetaAutonomyController()
	in := MetaInput{Regime: 3, PerfSignal: 0.9, StabilityMargin: 0.8, SLASeverity: 0.0}
	for i := 0; i < 50; i++ {
		m.Step(in)
	}
	score := m.regimeScore[3]
	t.Logf("  regime 3 score after 50 good steps = %.6f  (want > 0)", score)
	t.Logf("  delta/step ~= 0.05*(0.9-0.5) + 0.04*0.8 = 0.052  -> capped at 2.0")

	pass := score > 0
	record(t.Name(), pass, "Regime scores build contextual memory of productive states",
		"regime_3_score", fmt.Sprintf("%.6f", score),
		"delta_per_step", "~0.052",
		"cap", "2.0",
	)
	if !pass {
		t.Errorf("regime 3 score should be positive, got %.4f", score)
	}
}

func TestMeta_SafetyGain_HigherWhenInstable(t *testing.T) {
	stable := MetaInput{StabilityMargin: 0.90, GlobalRisk: 0.10}
	unstable := MetaInput{StabilityMargin: -0.50, GlobalRisk: 0.90}

	outS := NewMetaAutonomyController().Step(stable)
	outU := NewMetaAutonomyController().Step(unstable)

	t.Logf("  SafetyGain [stable   sm=0.90 risk=0.10] = %.6f", outS.SafetyGain)
	t.Logf("  SafetyGain [unstable sm=-0.50 risk=0.90] = %.6f", outU.SafetyGain)
	t.Logf("  Difference = %.6f  (want > 0)", outU.SafetyGain-outS.SafetyGain)

	pass := outU.SafetyGain > outS.SafetyGain
	record(t.Name(), pass, "Safety gain amplifies near the instability boundary",
		"gain_stable", fmt.Sprintf("%.6f", outS.SafetyGain),
		"gain_unstable", fmt.Sprintf("%.6f", outU.SafetyGain),
		"delta", fmt.Sprintf("%.6f", outU.SafetyGain-outS.SafetyGain),
	)
	if !pass {
		t.Errorf("safety gain must be higher when unstable: stable=%.4f, unstable=%.4f",
			outS.SafetyGain, outU.SafetyGain)
	}
}

// =============================================================================
// 3. ADAPTIVE SIGNAL LEARNER
// =============================================================================

func TestLearner_RegimeProbs_SumToOneAlways(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()
	maxDrift := 0.0
	for i := 0; i < 30; i++ {
		r := l.Update(SignalVector{
			Timestamp:    now.Add(time.Duration(i) * time.Second),
			BacklogError: float64(i) * 0.1,
			LatencyError: float64(i) * 0.05,
		})
		s := 0.0
		for _, p := range r.RegimeProb {
			s += p
		}
		d := math.Abs(s - 1.0)
		if d > maxDrift {
			maxDrift = d
		}
		if d > 1e-6 {
			t.Errorf("step %d: regime probs sum to %.10f", i, s)
		}
	}
	t.Logf("  max |sum-1| over 30 steps = %.2e  (tolerance 1e-6)", maxDrift)
	record(t.Name(), maxDrift <= 1e-6, "Regime probability mass must be conserved",
		"max_drift", fmt.Sprintf("%.2e", maxDrift),
	)
}

func TestLearner_HorizonRisk_InUnitInterval(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()
	minR, maxR := 1.0, 0.0
	for i := 0; i < 30; i++ {
		r := l.Update(SignalVector{
			Timestamp:    now.Add(time.Duration(i) * time.Second),
			BacklogError: rand.Float64() * 3,
			CPUError:     rand.Float64() * 3,
		})
		if r.HorizonRisk < minR {
			minR = r.HorizonRisk
		}
		if r.HorizonRisk > maxR {
			maxR = r.HorizonRisk
		}
		if r.HorizonRisk < 0 || r.HorizonRisk > 1 {
			t.Errorf("step %d: HorizonRisk=%.4f out of [0,1]", i, r.HorizonRisk)
		}
	}
	t.Logf("  HorizonRisk range over 30 steps = [%.6f, %.6f]", minR, maxR)
	record(t.Name(), minR >= 0 && maxR <= 1, "Forward-roll risk must be a valid probability",
		"observed_min", fmt.Sprintf("%.6f", minR),
		"observed_max", fmt.Sprintf("%.6f", maxR),
	)
}

func TestLearner_Confidence_InUnitInterval(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()
	minC, maxC := 1.0, 0.0
	for i := 0; i < 30; i++ {
		r := l.Update(SignalVector{
			Timestamp:          now.Add(time.Duration(i) * time.Second),
			RetryAmplification: rand.Float64() * 4,
		})
		if r.Confidence < minC {
			minC = r.Confidence
		}
		if r.Confidence > maxC {
			maxC = r.Confidence
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			t.Errorf("step %d: Confidence=%.4f out of [0,1]", i, r.Confidence)
		}
	}
	t.Logf("  Confidence range over 30 steps = [%.6f, %.6f]", minC, maxC)
	record(t.Name(), minC >= 0 && maxC <= 1, "Confidence must be a calibrated probability",
		"observed_min", fmt.Sprintf("%.6f", minC),
		"observed_max", fmt.Sprintf("%.6f", maxC),
	)
}

func TestLearner_MahalScore_HighForOutlier(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()
	warmupLearner(l, 60, now)

	baseline := l.Update(SignalVector{
		Timestamp:    now.Add(60 * time.Second),
		BacklogError: 1.0, LatencyError: 0.5, CPUError: 0.8,
	})
	outlier := l.Update(SignalVector{
		Timestamp:    now.Add(61 * time.Second),
		BacklogError: 40.0, LatencyError: 40.0, ErrorRateError: 40.0,
		CPUError: 40.0, QueueDrift: 40.0, RetryAmplification: 40.0,
	})

	ratio := outlier.Score / (baseline.Score + 1e-9)
	t.Logf("  Baseline score  = %.6f", baseline.Score)
	t.Logf("  Outlier score   = %.6f  (40x magnitude signal)", outlier.Score)
	t.Logf("  Ratio           = %.2fx  (want > 1.0)", ratio)
	t.Logf("  Note: score = sqrt(abs((x-mu)^T Cov^-1 (x-mu)))")

	pass := outlier.Score > baseline.Score
	record(t.Name(), pass, "Mahalanobis correctly measures statistical surprise",
		"baseline_score", fmt.Sprintf("%.6f", baseline.Score),
		"outlier_score", fmt.Sprintf("%.6f", outlier.Score),
		"ratio", fmt.Sprintf("%.2fx", ratio),
	)
	if !pass {
		t.Errorf("outlier score (%.2f) must exceed baseline (%.2f)", outlier.Score, baseline.Score)
	}
}

func TestLearner_Confidence_DropsOnAnomaly(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()
	warmupLearner(l, 60, now)

	normal := l.Update(SignalVector{
		Timestamp:    now.Add(60 * time.Second),
		BacklogError: 1.0, LatencyError: 0.5, CPUError: 0.8,
	})
	anomaly := l.Update(SignalVector{
		Timestamp:    now.Add(61 * time.Second),
		BacklogError: 40.0, LatencyError: 40.0, ErrorRateError: 40.0,
		CPUError: 40.0, QueueDrift: 40.0, RetryAmplification: 40.0,
	})

	drop := normal.Confidence - anomaly.Confidence
	t.Logf("  Confidence [normal ] = %.6f", normal.Confidence)
	t.Logf("  Confidence [anomaly] = %.6f  (40x signal magnitude)", anomaly.Confidence)
	t.Logf("  Confidence drop      = %.6f  (want > 0)", drop)
	t.Logf("  Formula: conf = exp(-0.18 * Mahal) * observability")

	pass := anomaly.Confidence < normal.Confidence
	record(t.Name(), pass, "System knows it doesn't know — confidence drops on anomaly",
		"normal_confidence", fmt.Sprintf("%.6f", normal.Confidence),
		"anomaly_confidence", fmt.Sprintf("%.6f", anomaly.Confidence),
		"drop", fmt.Sprintf("%.6f", drop),
	)
	if !pass {
		t.Errorf("confidence must drop on anomaly: normal=%.5f anomaly=%.5f",
			normal.Confidence, anomaly.Confidence)
	}
}

func TestLearner_RegimeProbability_ConvergesFromUniform(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()
	initial := 1.0 / float64(l.K)

	for i := 0; i < 40; i++ {
		l.Update(SignalVector{
			Timestamp:    now.Add(time.Duration(i) * time.Second),
			BacklogError: 0.7, LatencyError: 0.4, CPUError: 0.3,
		})
	}

	maxProb := 0.0
	for _, p := range l.regProb {
		if p > maxProb {
			maxProb = p
		}
	}
	t.Logf("  Initial uniform  = %.4f  (1/%d)", initial, l.K)
	t.Logf("  Max regime prob after 40 consistent steps = %.6f  (want > %.4f)", maxProb, initial)
	t.Logf("  Regime probs = %v", l.regProb)

	pass := maxProb > initial
	record(t.Name(), pass, "Regime filter converges toward most likely state",
		"initial_uniform", fmt.Sprintf("%.4f", initial),
		"max_prob_after_40", fmt.Sprintf("%.6f", maxProb),
		"regime_probs", fmt.Sprintf("%v", l.regProb),
	)
	if !pass {
		t.Errorf("regime should converge (initial max=%.3f, got max=%.3f)", initial, maxProb)
	}
}

func TestLearner_HorizonRisk_HigherForLargeSignal(t *testing.T) {
	l := NewAdaptiveSignalLearner(6)
	now := time.Now()

	normal := l.Update(SignalVector{Timestamp: now})
	extreme := l.Update(SignalVector{
		Timestamp:    now.Add(time.Second),
		BacklogError: 20.0, LatencyError: 20.0, ErrorRateError: 20.0,
		CPUError: 20.0, QueueDrift: 20.0, RetryAmplification: 20.0,
	})

	t.Logf("  HorizonRisk [zero signal  ] = %.6f", normal.HorizonRisk)
	t.Logf("  HorizonRisk [20x magnitude] = %.6f  (want higher)", extreme.HorizonRisk)
	t.Logf("  Mechanism: large state -> large forward Mahal -> high tanh sum")

	pass := extreme.HorizonRisk > normal.HorizonRisk
	record(t.Name(), pass, "Forward-roll risk tracks signal magnitude correctly",
		"zero_risk", fmt.Sprintf("%.6f", normal.HorizonRisk),
		"extreme_risk", fmt.Sprintf("%.6f", extreme.HorizonRisk),
	)
	if !pass {
		t.Errorf("horizon risk must be higher for large signal: zero=%.4f extreme=%.4f",
			normal.HorizonRisk, extreme.HorizonRisk)
	}
}

// =============================================================================
// 4. SAFETY CONSTRAINT PROJECTOR
// =============================================================================

// BUG DOCUMENTED:
// ViolationNorm = vecNorm(diff(projected_x, in.Action))
// When in.Action contains NaN, the diff inherits NaN even though projected_x is clean.
// The integratedEnvelope guards the projected action but not the violation metric.
// Fix: sanitise in.Action before computing ViolationNorm in Project(), or return 0
// for the norm when the input itself is pathological.
func TestSafety_NaN_DoesNotPropagate(t *testing.T) {
	s := NewSafetyConstraintProjector(3)
	in := SafetyInput{
		Action:       []float64{math.NaN(), 1.0, -1.0},
		PrevAction:   makeVec(3, 0),
		State:        makeVec(3, 0.5),
		StabilityVec: makeVec(3, 1.0),
		Risk:         0.3,
	}
	out := s.Project(in)

	actionOk := assertFinite(t, "action", out.Action)
	violNaN := math.IsNaN(out.ViolationNorm)
	costNaN := math.IsNaN(out.ConstraintCost)

	t.Logf("  Projected actions = [%.4f, %.4f, %.4f]", out.Action[0], out.Action[1], out.Action[2])
	t.Logf("  ViolationNorm     = %v  (NaN=%v)  <-- KNOWN BUG", out.ViolationNorm, violNaN)
	t.Logf("  ConstraintCost    = %v  (NaN=%v)", out.ConstraintCost, costNaN)
	t.Logf("  ROOT CAUSE: diff(projected_x, in.Action) inherits NaN from in.Action[0]")
	t.Logf("  FIX NEEDED: sanitise in.Action before vecNorm(diff(...)) in Project()")

	record(t.Name(), !violNaN && !costNaN && actionOk,
		"KNOWN BUG: ViolationNorm=NaN when input action has NaN. diff() inherits it. Fix: sanitise in.Action before diff().",
		"action_finite", fmt.Sprintf("%v", actionOk),
		"violation_norm_nan", fmt.Sprintf("%v", violNaN),
		"constraint_cost_nan", fmt.Sprintf("%v", costNaN),
		"bug_location", "SafetyConstraintProjector.Project() line: cost = vecNorm(diff(x, in.Action))",
	)
	if violNaN || costNaN {
		t.Errorf("ViolationNorm/ConstraintCost NaN -- input sanitisation missing in Project()")
	}
}

func TestSafety_Inf_DoesNotPropagate(t *testing.T) {
	s := NewSafetyConstraintProjector(3)
	in := SafetyInput{
		Action:       []float64{math.Inf(1), math.Inf(-1), 1.0},
		PrevAction:   makeVec(3, 0),
		State:        makeVec(3, 0.5),
		StabilityVec: makeVec(3, 1.0),
		Risk:         0.3,
	}
	out := s.Project(in)
	ok := assertFinite(t, "action", out.Action)
	t.Logf("  Projected actions = [%.4f, %.4f, %.4f]", out.Action[0], out.Action[1], out.Action[2])
	t.Logf("  ViolationNorm     = %.6f", out.ViolationNorm)
	record(t.Name(), ok, "+-Inf actions are clamped by integratedEnvelope",
		"action_0", fmt.Sprintf("%.4f", out.Action[0]),
		"action_1", fmt.Sprintf("%.4f", out.Action[1]),
	)
}

func TestSafety_LargeAction_ReducedByProjection(t *testing.T) {
	s := NewSafetyConstraintProjector(4)
	in := SafetyInput{
		Action:       makeVec(4, 8.0),
		PrevAction:   makeVec(4, 0),
		State:        makeVec(4, 0.1),
		StabilityVec: makeVec(4, 1.0),
		Risk:         0.5,
	}
	out := s.Project(in)
	allReduced := true
	for i, v := range out.Action {
		reduced := v < in.Action[i]
		t.Logf("  action[%d]: requested=%.2f  projected=%.4f  reduced=%v", i, in.Action[i], v, reduced)
		if !reduced {
			allReduced = false
		}
	}
	t.Logf("  ViolationNorm  = %.6f  (distance pulled back from unsafe zone)", out.ViolationNorm)
	t.Logf("  ConstraintCost = %.6f", out.ConstraintCost)
	record(t.Name(), allReduced, "Projector enforces action feasibility",
		"requested", "8.0",
		"action_0_projected", fmt.Sprintf("%.4f", out.Action[0]),
		"violation_norm", fmt.Sprintf("%.6f", out.ViolationNorm),
	)
	if !allReduced {
		t.Error("one or more actions not reduced -- projector is a pass-through")
	}
}

func TestSafety_ActionBound_ShrinksWithRisk(t *testing.T) {
	s := NewSafetyConstraintProjector(2)
	base := SafetyInput{
		Action: makeVec(2, 5.0), PrevAction: makeVec(2, 0),
		State: makeVec(2, 0.1), StabilityVec: makeVec(2, 1.0),
	}
	loR := base
	loR.Risk = 0.1
	hiR := base
	hiR.Risk = 0.9

	outLo := s.Project(loR)
	outHi := s.Project(hiR)

	t.Logf("  action[0] @ risk=0.1 -> %.6f  (envelope up ~= 3.2 - 1.1*0.1 = 3.09)", outLo.Action[0])
	t.Logf("  action[0] @ risk=0.9 -> %.6f  (envelope up ~= 3.2 - 1.1*0.9 = 2.21)", outHi.Action[0])
	t.Logf("  Bound shrink         = %.6f", outLo.Action[0]-outHi.Action[0])

	pass := outHi.Action[0] < outLo.Action[0]
	record(t.Name(), pass, "Higher risk -> tighter action envelope (more conservative)",
		"action_risk_0.1", fmt.Sprintf("%.6f", outLo.Action[0]),
		"action_risk_0.9", fmt.Sprintf("%.6f", outHi.Action[0]),
		"bound_shrink", fmt.Sprintf("%.6f", outLo.Action[0]-outHi.Action[0]),
	)
	if !pass {
		t.Errorf("high-risk action (%.4f) must be <= low-risk action (%.4f)",
			outHi.Action[0], outLo.Action[0])
	}
}

func TestSafety_ViolationNorm_FiniteAndNonNegative(t *testing.T) {
	s := NewSafetyConstraintProjector(4)
	in := SafetyInput{
		Action: makeVec(4, 2.0), PrevAction: makeVec(4, 1.0),
		State: makeVec(4, 0.5), StabilityVec: makeVec(4, 1.0),
		Risk: 0.4, HazardProxy: 0.3, SLAWeight: 1.0, CapacityPress: 0.5,
	}
	out := s.Project(in)
	t.Logf("  ViolationNorm  = %.6f  (want finite and >= 0)", out.ViolationNorm)
	t.Logf("  ConstraintCost = %.6f", out.ConstraintCost)
	t.Logf("  NaN=%v  Inf=%v  negative=%v",
		math.IsNaN(out.ViolationNorm), math.IsInf(out.ViolationNorm, 0), out.ViolationNorm < 0)

	pass := !math.IsNaN(out.ViolationNorm) && !math.IsInf(out.ViolationNorm, 0) && out.ViolationNorm >= 0
	record(t.Name(), pass, "Violation norm is a valid non-negative distance",
		"violation_norm", fmt.Sprintf("%.6f", out.ViolationNorm),
		"constraint_cost", fmt.Sprintf("%.6f", out.ConstraintCost),
	)
	if !pass {
		t.Errorf("ViolationNorm not valid: %v", out.ViolationNorm)
	}
}

// =============================================================================
// 5. POLICY GRADIENT OPTIMIZER
// =============================================================================

func TestPGO_Actions_AlwaysWithinSafeBounds(t *testing.T) {
	p := NewPolicyGradientOptimizer(6)
	rng := rand.New(rand.NewSource(42))
	violations := 0
	minSO, maxSO := 6.0, 0.0
	for i := 0; i < 200; i++ {
		state := make([]float64, 6)
		for j := range state {
			state[j] = rng.Float64()*4 - 2
		}
		act := p.Act(state)
		if act.ScaleOut < minSO {
			minSO = act.ScaleOut
		}
		if act.ScaleOut > maxSO {
			maxSO = act.ScaleOut
		}
		if act.ScaleOut < 0 || act.ScaleOut > 6 ||
			act.RetryBackoff < 0 || act.RetryBackoff > 4 ||
			act.QueueShard < 0 || act.QueueShard > 5 ||
			act.CacheBoost < 0 || act.CacheBoost > 3 {
			violations++
			t.Errorf("trial %d: bounds violated SO=%.3f RB=%.3f QS=%.3f CB=%.3f",
				i, act.ScaleOut, act.RetryBackoff, act.QueueShard, act.CacheBoost)
		}
	}
	t.Logf("  200 random states tested -- violations = %d  (want 0)", violations)
	t.Logf("  ScaleOut observed range = [%.4f, %.4f]  (allowed [0, 6])", minSO, maxSO)
	t.Logf("  Safety shield: projectSafe clamps each dimension independently")
	record(t.Name(), violations == 0, "Safety shield holds across 200 random states",
		"violations", fmt.Sprintf("%d", violations),
		"scaleout_range", fmt.Sprintf("[%.4f, %.4f]", minSO, maxSO),
	)
}

func TestPGO_WeightNorm_BoundedAfterContinuousLearning(t *testing.T) {
	p := NewPolicyGradientOptimizer(6)
	rng := rand.New(rand.NewSource(77))
	normBefore := p.TotalWeightNorm()

	for i := 0; i < 250; i++ {
		state := make([]float64, 6)
		for j := range state {
			state[j] = rng.Float64()*2 - 1
		}
		p.Act(state)
		next := make([]float64, 6)
		for j := range next {
			next[j] = rng.Float64()*2 - 1
		}
		p.Observe(next, rng.Float64(), rng.Float64()*0.5, false)
	}

	normAfter := p.TotalWeightNorm()
	const ceiling = 8000.0

	t.Logf("  Weight norm before = %.4f", normBefore)
	t.Logf("  Weight norm after  = %.4f  (ceiling %.0f)", normAfter, ceiling)
	t.Logf("  Growth factor      = %.2fx", normAfter/(normBefore+1e-9))
	t.Logf("  Controls: clipRange=0.05, L2_lambda=0.01")

	pass := !math.IsNaN(normAfter) && !math.IsInf(normAfter, 0) && normAfter <= ceiling
	record(t.Name(), pass, "Gradient clipping + L2 regularisation prevent weight explosion",
		"norm_before", fmt.Sprintf("%.4f", normBefore),
		"norm_after", fmt.Sprintf("%.4f", normAfter),
		"ceiling", fmt.Sprintf("%.0f", ceiling),
		"growth_factor", fmt.Sprintf("%.2fx", normAfter/(normBefore+1e-9)),
	)
	if math.IsNaN(normAfter) || math.IsInf(normAfter, 0) {
		t.Fatalf("weight norm is NaN/Inf: %v", normAfter)
	}
	if normAfter > ceiling {
		t.Errorf("weight norm %.2f > ceiling %.0f -- gradient explosion", normAfter, ceiling)
	}
}

func TestPGO_RewardNormalisation_StatisticsStayFinite(t *testing.T) {
	p := NewPolicyGradientOptimizer(6)
	rng := rand.New(rand.NewSource(13))
	for i := 0; i < 120; i++ {
		state := make([]float64, 6)
		for j := range state {
			state[j] = rng.Float64()
		}
		p.Act(state)
		reward := rng.Float64() * 10 * float64(i%7+1)
		p.Observe(state, reward, 0.1, false)
	}
	t.Logf("  rewardMean after 120 spiky steps = %.6f", p.rewardMean)
	t.Logf("  rewardStd                        = %.6f", p.rewardStd)
	t.Logf("  NaN(mean)=%v  NaN(std)=%v", math.IsNaN(p.rewardMean), math.IsNaN(p.rewardStd))

	pass := !math.IsNaN(p.rewardMean) && !math.IsInf(p.rewardMean, 0) &&
		!math.IsNaN(p.rewardStd) && !math.IsInf(p.rewardStd, 0)
	record(t.Name(), pass, "Running reward normalisation is numerically stable",
		"reward_mean", fmt.Sprintf("%.6f", p.rewardMean),
		"reward_std", fmt.Sprintf("%.6f", p.rewardStd),
	)
	if !pass {
		t.Errorf("reward stats not finite: mean=%v std=%v", p.rewardMean, p.rewardStd)
	}
}

func TestPGO_ReplayPriority_HigherForHighRiskTransitions(t *testing.T) {
	state := makeVec(6, 0.5)
	next := makeVec(6, 0.6)
	p := NewPolicyGradientOptimizer(6)

	p.Act(state)
	p.Observe(next, 0.5, 0.05, false)
	lowRiskPriority := p.replay[len(p.replay)-1].priority

	p.Act(state)
	p.Observe(next, 0.5, 0.95, false)
	highRiskPriority := p.replay[len(p.replay)-1].priority

	t.Logf("  Priority @ risk=0.05 = %.6f", lowRiskPriority)
	t.Logf("  Priority @ risk=0.95 = %.6f  (want higher)", highRiskPriority)
	t.Logf("  Formula: |normalised_reward| + 2*risk")
	t.Logf("  Difference           = %.6f", highRiskPriority-lowRiskPriority)

	pass := highRiskPriority > lowRiskPriority
	record(t.Name(), pass, "Catastrophic experience is over-sampled during training",
		"priority_low_risk", fmt.Sprintf("%.6f", lowRiskPriority),
		"priority_high_risk", fmt.Sprintf("%.6f", highRiskPriority),
		"delta", fmt.Sprintf("%.6f", highRiskPriority-lowRiskPriority),
	)
	if !pass {
		t.Errorf("high-risk priority (%.4f) must exceed low-risk (%.4f)",
			highRiskPriority, lowRiskPriority)
	}
}

func TestPGO_NormaliseReward_FiniteForExtremeSpike(t *testing.T) {
	p := NewPolicyGradientOptimizer(6)
	for i := 0; i < 60; i++ {
		p.normalizeReward(1.0)
	}
	meanBefore := p.rewardMean
	stdBefore := p.rewardStd

	spike := p.normalizeReward(10000.0)

	t.Logf("  Running mean before spike = %.6f", meanBefore)
	t.Logf("  Running std  before spike = %.6f", stdBefore)
	t.Logf("  Spike reward  = 10000.0")
	t.Logf("  Normalised    = %.6f  (want finite)", spike)
	t.Logf("  Running mean after  spike = %.6f", p.rewardMean)
	t.Logf("  Running std  after  spike = %.6f", p.rewardStd)

	pass := !math.IsNaN(spike) && !math.IsInf(spike, 0)
	record(t.Name(), pass, "Reward normaliser survives extreme spikes without NaN",
		"mean_before", fmt.Sprintf("%.6f", meanBefore),
		"std_before", fmt.Sprintf("%.6f", stdBefore),
		"normalised_spike", fmt.Sprintf("%.6f", spike),
	)
	if !pass {
		t.Errorf("normalised reward not finite for extreme spike: %v", spike)
	}
}

// =============================================================================
// JSON REPORT GENERATOR — run last
// =============================================================================

func TestBrainReport(t *testing.T) {
	if len(allResults) == 0 {
		t.Skip("no results collected -- run together with other Test* functions")
	}

	passed, failed := 0, 0
	for _, r := range allResults {
		if r.Pass {
			passed++
		} else {
			failed++
		}
	}

	report := map[string]interface{}{
		"generated_at":  time.Now().Format(time.RFC3339),
		"total_tests":   len(allResults),
		"passed":        passed,
		"failed":        failed,
		"pass_rate_pct": fmt.Sprintf("%.1f%%", float64(passed)/float64(len(allResults))*100),
		"results":       allResults,
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	path := "brain_test_report.json"
	_ = os.WriteFile(path, data, 0644)

	t.Logf("")
	t.Logf("========= BRAIN TEST SUMMARY =========")
	t.Logf("  Total  : %d", len(allResults))
	t.Logf("  Passed : %d  (%.1f%%)", passed, float64(passed)/float64(len(allResults))*100)
	t.Logf("  Failed : %d", failed)
	t.Logf("  Report : %s", path)
	t.Logf("======================================")
	for _, r := range allResults {
		sym := "PASS"
		if !r.Pass {
			sym = "FAIL"
		}
		t.Logf("  [%s] %s", sym, r.Name)
	}
}
