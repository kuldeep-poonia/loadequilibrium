package optimisation_test

import (
	"math"
	"testing"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

func TestMPCTrajectoryEvaluation(t *testing.T) {
	mpc := optimisation.NewMPCHorizonEval(10, 2.0, 0.70)

	bundle := &modelling.ServiceModelBundle{
		Queue: modelling.QueueModel{
			ServiceID:        "api-svc",
			Utilisation:      0.80,
			UtilisationTrend: 0.02,
			MeanQueueLen:     100.0,
		},
		Signal: modelling.SignalState{
			FastEWMA:     0.80,
			SlowEWMA:     0.78,
			EWMAVariance: 0.01,
		},
	}

	pidOutput := -0.3
	currentScale := 1.2

	result := mpc.Evaluate(bundle, pidOutput, currentScale)

	if math.IsNaN(result.AdjustedScaleFactor) || math.IsInf(result.AdjustedScaleFactor, 0) {
		t.Errorf("MPC: scale factor diverged to %v", result.AdjustedScaleFactor)
	}

	if result.MaxTrajectoryCost < 0 || result.MaxTrajectoryCost > 1.0 {
		t.Errorf("MPC: trajectory cost out of bounds: %.3f", result.MaxTrajectoryCost)
	}
}

func TestMPCEdgeCases(t *testing.T) {
	mpc := optimisation.NewMPCHorizonEval(10, 2.0, 0.70)

	testCases := []struct {
		name        string
		utilisation float64
		trend       float64
	}{
		{"idle", 0.01, 0.0},
		{"stable", 0.50, 0.0},
		{"approaching_collapse", 0.95, 0.05},
		{"overloaded", 1.05, 0.1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := &modelling.ServiceModelBundle{
				Queue: modelling.QueueModel{
					ServiceID:        tc.name,
					Utilisation:      tc.utilisation,
					UtilisationTrend: tc.trend,
					MeanQueueLen:     50.0,
				},
				Signal: modelling.SignalState{
					FastEWMA:     tc.utilisation,
					SlowEWMA:     tc.utilisation - 0.01,
					EWMAVariance: 0.001,
				},
			}

			result := mpc.Evaluate(bundle, -0.15, 1.0)

			if math.IsNaN(result.AdjustedScaleFactor) {
				t.Errorf("%s: scale factor is NaN", tc.name)
			}
		})
	}
}

func BenchmarkMPCEvaluation(b *testing.B) {
	mpc := optimisation.NewMPCHorizonEval(10, 2.0, 0.70)

	bundle := &modelling.ServiceModelBundle{
		Queue: modelling.QueueModel{
			Utilisation:      0.75,
			UtilisationTrend: 0.02,
			MeanQueueLen:     50.0,
		},
		Signal: modelling.SignalState{
			FastEWMA:     0.75,
			SlowEWMA:     0.73,
			EWMAVariance: 0.01,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mpc.Evaluate(bundle, -0.15, 1.0)
	}
}
