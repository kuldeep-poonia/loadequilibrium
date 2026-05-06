package layer2_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/intelligence"
)

// ---------------------------------------------------------------
// L2-SAFETY-001 — SafetyConstraintProjector envelope bounds
// AIM: For ANY input action (including extreme values), Project()
//      must return actions within the integratedEnvelope bounds:
//        upper = 3.2 + 0.5*tanh(load) - 1.1*risk
//        lower = -2.6 - 0.6*tanh(load)
// THRESHOLD: 0 out-of-bound outputs
// ON EXCEED: CRITICAL — unsafe action reaches actuator
// ---------------------------------------------------------------
func TestL2_SAFETY_001_ProjectorEnvelopeBounds(t *testing.T) {
	start := time.Now()
	const actDim = 4
	const N = 50000

	proj := intelligence.NewSafetyConstraintProjector(actDim)

	var violations int
	var worstViolation interface{}

	for i := 0; i < N; i++ {
		// Systematically vary risk, capacity pressure, and action magnitude.
		risk := float64(i%10) * 0.1          // 0..0.9
		capPress := float64(i%5) * 0.2       // 0..0.8
		actionScale := 10.0 * float64(i%20)  // 0..190

		action := make([]float64, actDim)
		prev := make([]float64, actDim)
		state := []float64{1.0, 2.0, 0.5, 0.3}
		stability := []float64{0.5, 0.5, 0.5, 0.5}

		for j := range action {
			// Alternate positive and negative extreme values.
			if j%2 == 0 {
				action[j] = actionScale
			} else {
				action[j] = -actionScale
			}
			prev[j] = 0
		}

		in := intelligence.SafetyInput{
			Action:        action,
			PrevAction:    prev,
			State:         state,
			StabilityVec:  stability,
			Risk:          risk,
			HazardProxy:   risk * 0.5,
			CapacityPress: capPress,
			SLAWeight:     1.0,
		}

		out := proj.Project(in)

		// Compute the exact integratedEnvelope bounds for this input.
		load := vecNormL2(state)
		for j, v := range out.Action {
			upper := 3.2 + 0.5*math.Tanh(load) - 1.1*risk
			lower := -2.6 - 0.6*math.Tanh(load)

			if v > upper+1e-9 || v < lower-1e-9 {
				violations++
				if worstViolation == nil {
					worstViolation = map[string]interface{}{
						"iter": i, "dim": j, "value": v,
						"upper": upper, "lower": lower,
						"risk": risk, "capPress": capPress,
						"input_action": action[j],
					}
				}
			}
		}
	}

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-SAFETY-001", Layer: 2,
		Name:              "SafetyConstraintProjector envelope bounds",
		Aim:               "Project() output must be within integratedEnvelope [lo, up] for any input",
		PackagesInvolved:  []string{"internal/intelligence"},
		FunctionUnderTest: "SafetyConstraintProjector.Project → integratedEnvelope",
		Threshold:         L2Threshold{"envelope_violations", "==", 0, "count", "CRITICAL — no action may exceed the safety envelope"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(violations),
			ActualUnit: "count", SampleCount: N * actDim,
			WorstCaseInput: worstViolation, DurationMs: durationMs,
		},
		OnExceed: "CRITICAL: Action exceeds physical safety envelope → actuator operates in unsafe regime → hardware damage risk",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d inputs × %d dimensions = %d bound checks with action magnitudes up to ±190", N, actDim, N*actDim),
			WhyThisThreshold:     "integratedEnvelope is the hard safety clamp. The gradient descent may not converge, but the envelope MUST clamp",
			WhatHappensIfFails:   "Action outside envelope → actuator commanded beyond physical limits → mechanical/electrical failure",
			HowInterfaceVerified: "Compute exact upper/lower per dimension from integratedEnvelope formula, check every output element",
			HasEverFailed:        fmt.Sprintf("%d violations in this run", violations),
			WorstCaseDescription: fmt.Sprintf("violation: %v", worstViolation),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-SAFETY-001 FAILED: %d envelope violations\nFirst: %v\nFIX: check integratedEnvelope clamping in safety_constraint_projector.go",
			violations, worstViolation)
	}
	t.Logf("L2-SAFETY-001 PASS: %d × %d = %d bound checks, 0 violations", N, actDim, N*actDim)
}

// ---------------------------------------------------------------
// L2-SAFETY-002 — Projector reduces violation for unsafe inputs
// AIM: When an unsafe input is projected, the output ViolationNorm
//      must be less than the input violation (the projector actually
//      corrects the action, not ignores it).
// THRESHOLD: correction_rate >= 99% (some may already be safe)
// ON EXCEED: Projector returns input unchanged → no safety correction
// ---------------------------------------------------------------
func TestL2_SAFETY_002_ProjectorReducesViolation(t *testing.T) {
	start := time.Now()
	const actDim = 4
	const N = 10000

	proj := intelligence.NewSafetyConstraintProjector(actDim)

	var corrected, total int
	var worstInput interface{}
	worstRatio := 0.0

	for i := 0; i < N; i++ {
		risk := float64(i%8) * 0.1
		action := make([]float64, actDim)
		prev := make([]float64, actDim)

		// Create action that significantly violates envelope.
		for j := range action {
			action[j] = 20.0 + float64(i%30) // always > upper bound (~3.7)
		}

		in := intelligence.SafetyInput{
			Action:        action,
			PrevAction:    prev,
			State:         []float64{1, 1, 1, 1},
			StabilityVec:  []float64{1, 0, 0, 0},
			Risk:          risk,
			HazardProxy:   0.3,
			CapacityPress: 0.3,
			SLAWeight:     1.0,
		}

		out := proj.Project(in)

		// Measure input violation vs output violation.
		inputNorm := vecNormL2(action)
		outputNorm := vecNormL2(out.Action)

		total++
		if outputNorm < inputNorm-1e-9 {
			corrected++
		} else {
			ratio := outputNorm / (inputNorm + 1e-12)
			if ratio > worstRatio {
				worstRatio = ratio
				worstInput = map[string]interface{}{
					"iter": i, "input_norm": inputNorm, "output_norm": outputNorm,
					"ratio": ratio, "risk": risk,
				}
			}
		}
	}

	correctionRate := float64(corrected) / float64(total) * 100
	passed := correctionRate >= 99.0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-SAFETY-002", Layer: 2,
		Name:              "SafetyConstraintProjector correction effectiveness",
		Aim:               "Projector must reduce action magnitude for >99% of unsafe inputs",
		PackagesInvolved:  []string{"internal/intelligence"},
		FunctionUnderTest: "SafetyConstraintProjector.Project",
		Threshold:         L2Threshold{"correction_rate_pct", ">=", 99, "percent", "Projector that doesn't correct is a no-op — defeats safety purpose"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: correctionRate,
			ActualUnit: "percent", SampleCount: total,
			WorstCaseInput: worstInput, DurationMs: durationMs,
		},
		OnExceed: "Projector passes unsafe actions through unchanged → safety layer is decorative, not functional",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d inputs with action magnitude 20+ (far beyond envelope ~3.7)", N),
			WhyThisThreshold:     "Input at 20+ vs envelope at ~3.7 → projector must reduce. 99% allows tiny tolerance for edge cases where gradient is trapped",
			WhatHappensIfFails:   "Safety projector passes dangerous actions unchanged → actuator operates in unsafe region → damage",
			HowInterfaceVerified: "Compare ‖input action‖ vs ‖projected action‖, verify output norm < input norm",
			HasEverFailed:        fmt.Sprintf("correction rate=%.2f%%, worst ratio=%.4f", correctionRate, worstRatio),
			WorstCaseDescription: fmt.Sprintf("worst uncorrected: %v", worstInput),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-SAFETY-002 FAILED: correction rate=%.2f%% (threshold 99%%)\n%v",
			correctionRate, worstInput)
	}
	t.Logf("L2-SAFETY-002 PASS: %.2f%% correction rate (%d/%d corrected)", correctionRate, corrected, total)
}

// ---------------------------------------------------------------
// L2-SAFETY-003 — Projector output finite (no NaN/Inf)
// AIM: Project() must never return NaN/Inf in Action, ViolationNorm,
//      or ConstraintCost, regardless of input.
// THRESHOLD: 0 NaN/Inf
// ON EXCEED: CRITICAL — NaN action sent to actuator → undefined behavior
// ---------------------------------------------------------------
func TestL2_SAFETY_003_ProjectorOutputFinite(t *testing.T) {
	start := time.Now()
	const actDim = 4
	const N = 20000

	proj := intelligence.NewSafetyConstraintProjector(actDim)

	var nanCount int
	var worstNaN interface{}

	for i := 0; i < N; i++ {
		// Adversarial: extreme values, edge cases.
		action := make([]float64, actDim)
		prev := make([]float64, actDim)
		for j := range action {
			action[j] = float64((i*137+j*31)%1000) - 500 // range [-500, 499]
			prev[j] = float64((i*43+j*17)%200) - 100
		}

		risk := float64(i%11) * 0.1
		capPress := float64(i%6) * 0.2

		in := intelligence.SafetyInput{
			Action:        action,
			PrevAction:    prev,
			State:         []float64{float64(i % 10), float64(i%7) * 0.5, 0.1, 0.2},
			StabilityVec:  []float64{0.5, 0.3, 0.1, 0.1},
			Risk:          risk,
			HazardProxy:   risk * 0.8,
			CapacityPress: capPress,
			SLAWeight:     1.0 + float64(i%3)*0.5,
		}

		out := proj.Project(in)

		for j, v := range out.Action {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				nanCount++
				if worstNaN == nil {
					worstNaN = map[string]interface{}{
						"iter": i, "dim": j, "value": v, "risk": risk,
					}
				}
			}
		}
		if math.IsNaN(out.ViolationNorm) || math.IsInf(out.ViolationNorm, 0) {
			nanCount++
		}
		if math.IsNaN(out.ConstraintCost) || math.IsInf(out.ConstraintCost, 0) {
			nanCount++
		}
	}

	passed := nanCount == 0
	durationMs := time.Since(start).Milliseconds()

	writeL2Result(L2Record{
		TestID: "L2-SAFETY-003", Layer: 2,
		Name:              "SafetyConstraintProjector output finite",
		Aim:               "Project() must never return NaN/Inf in Action, ViolationNorm, or ConstraintCost",
		PackagesInvolved:  []string{"internal/intelligence"},
		FunctionUnderTest: "SafetyConstraintProjector.Project",
		Threshold:         L2Threshold{"nan_inf_count", "==", 0, "count", "CRITICAL — NaN in action vector → undefined actuator behavior"},
		Result: L2ResultData{
			Status: l2Pass(passed), ActualValue: float64(nanCount),
			ActualUnit: "count", SampleCount: N,
			WorstCaseInput: worstNaN, DurationMs: durationMs,
		},
		OnExceed: "CRITICAL: NaN/Inf in projected action → actuator receives undefined command → system crash or hardware damage",
		Questions: L2Questions{
			WhatWasTested:        fmt.Sprintf("%d adversarial inputs with action range [-500, 499], risk [0, 1], capPress [0, 1]", N),
			WhyThisThreshold:     "NaN/Inf propagates through the entire actuator chain — any single occurrence is catastrophic",
			WhatHappensIfFails:   "NaN action → actuator produces undefined behavior → system enters unknown state",
			HowInterfaceVerified: "Check all output fields (Action[], ViolationNorm, ConstraintCost) for IsNaN/IsInf",
			HasEverFailed:        fmt.Sprintf("%d NaN/Inf in this run", nanCount),
			WorstCaseDescription: fmt.Sprintf("NaN at %v", worstNaN),
		},
		RunAt: l2Now(), GoVersion: l2GoVer(),
	})

	if !passed {
		t.Fatalf("L2-SAFETY-003 FAILED: %d NaN/Inf values\nFirst: %v",
			nanCount, worstNaN)
	}
	t.Logf("L2-SAFETY-003 PASS: %d adversarial inputs, 0 NaN/Inf", N)
}

// ---------------------------------------------------------------
// Helper: vecNormL2 computes L2 norm of a float64 slice.
// ---------------------------------------------------------------
func vecNormL2(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s)
}
