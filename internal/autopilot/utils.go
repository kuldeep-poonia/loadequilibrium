package autopilot

import "math"

// clamp01 clamps x to [0, 1].
// Canonical definition — all other files reference this; do NOT redeclare.
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// norm maps x ∈ [0,∞) → [0,1) via log-compression. Returns 0 for x ≤ 0.
// Canonical definition — replaces per-file copies in instability_engine and
// decision_policy, and is functionally identical to the now-removed normPos
// in anomaly_classifier.
func norm(x float64) float64 {
	if x <= 0 {
		return 0
	}
	l := math.Log(1.0 + x)
	return l / (1.0 + l)
}

// pos clamps x to [0,∞). Canonical definition.
func pos(x float64) float64 {
	if x < 0 {
		return 0
	}
	return x
}

// boundedAgg computes a zero-preserving soft-maximum over a set of values in [0, 1].
//
// WHY THIS EXISTS (replaces softAgg / softBlend everywhere):
//   log-sum-exp returns ln(N) ≥ ln(4) ≈ 1.386 when all inputs are 0.
//   This caused phantom Cascade classification at idle (anomaly_classifier)
//   and a permanent "warning" instability score at zero load (instability_engine).
//
// CONTRACT:
//   - All inputs are expected in [0, 1].
//   - Returns exactly 0 when all inputs are 0.
//   - Dominated by the maximum; mean contributes as a secondary signal.
//   - Output is normalized to [0, 1) via x/(1+x).
//
// WEIGHTS: 70% max + 30% mean — preserves soft-max dominance semantics
// without the log-space floor artefact.
func boundedAgg(vals ...float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	maximum := 0.0
	sum := 0.0
	for _, v := range vals {
		if v > maximum {
			maximum = v
		}
		sum += v
	}
	if sum == 0 {
		return 0 // all inputs are zero → no signal
	}
	mean := sum / float64(len(vals))
	blend := 0.7*maximum + 0.3*mean
	return blend / (1.0 + blend)
}