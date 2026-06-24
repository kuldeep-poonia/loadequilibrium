package intelligence

import (
	"math"
	"math/rand"
	"testing"
)

func TestNumericStabilityAudit(t *testing.T) {
	opt := NewPolicyGradientOptimizer(10)
	
	for ep := 0; ep < 10; ep++ {
		state := make([]float64, 10)
		for i := 0; i < 128; i++ {
			for j := range state {
				state[j] = rand.NormFloat64()
			}
			// Force extreme rewards to test numeric stability limits
			extremeReward := math.Pow(10, float64(i%5)) * rand.NormFloat64()
			extremeRisk := math.Abs(rand.NormFloat64() * 10)
			
			opt.Act(state)
			done := (i == 127)
			opt.Observe(state, extremeReward, extremeRisk, done)
		}
	}

	t.Logf("\n=== NUMERIC STABILITY AUDIT ===")
	t.Logf("NaN Count        : %d", opt.NaNCount)
	t.Logf("Inf Count        : %d", opt.InfCount)
	t.Logf("Grad Explosions  : %d", opt.GradExplosions)
	t.Logf("Total Weight Norm: %.4f", opt.TotalWeightNorm())
}
