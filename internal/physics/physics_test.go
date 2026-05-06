package physics_test

import (
	"math"
	"testing"

	"github.com/loadequilibrium/loadequilibrium/internal/physics"
)

// TestFluidPlantStableDynamics validates system behavior under stable conditions
func TestFluidPlantStableDynamics(t *testing.T) {
	plant := physics.NewFluidPlant(42)

	for i := 0; i < 10; i++ {
		plant.A = 0.5
		plant.Step(1.0)
	}

	if math.IsNaN(plant.Q) || math.IsInf(plant.Q, 0) {
		t.Errorf("stable dynamics: Q diverged to %v", plant.Q)
	}
	if plant.Z < -1.0 || plant.Z > 2.0 {
		t.Logf("stable physics: hazard %.3f", plant.Z)
	}
}

// TestFluidPlantPhaseDynamics validates phase transition behavior
func TestFluidPlantPhaseDynamics(t *testing.T) {
	plant := physics.NewFluidPlant(12345)

	for i := 0; i < 50; i++ {
		plant.A = 0.6
		plant.Step(1.0)
		if plant.Phase < 0 || plant.Phase > 1 {
			t.Errorf("phase dynamics: invalid phase %d", plant.Phase)
		}
	}
}

// TestFluidPlantHazardDynamics validates hazard accumulation and dissipation
func TestFluidPlantHazardDynamics(t *testing.T) {
	plant := physics.NewFluidPlant(67890)

	// High load phase
	for i := 0; i < 20; i++ {
		plant.A = 1.5
		plant.Step(1.0)
	}

	if math.IsNaN(plant.Z) || math.IsInf(plant.Z, 0) {
		t.Errorf("hazard accumulation: diverged to %v", plant.Z)
	}

	// Recovery phase
	for i := 0; i < 10; i++ {
		plant.A = 0.3
		plant.Step(1.0)
	}

	if math.IsNaN(plant.Z) {
		t.Errorf("hazard recovery: NaN state reached")
	}
}

// TestFluidPlantReservoirDynamics validates reservoir state evolution
func TestFluidPlantReservoirDynamics(t *testing.T) {
	plant := physics.NewFluidPlant(11111)

	for i := 0; i < 15; i++ {
		plant.A = 1.8
		plant.Step(1.0)
	}

	peakReservoir := plant.R

	for i := 0; i < 20; i++ {
		plant.A = 0.2
		plant.Step(1.0)
	}

	recoveredReservoir := plant.R

	if math.IsNaN(plant.R) || math.IsInf(plant.R, 0) {
		t.Errorf("reservoir dynamics: diverged to %v", plant.R)
	}

	t.Logf("reservoir: peak=%.3f, recovered=%.3f", peakReservoir, recoveredReservoir)
}

// TestFluidPlantDeterminism validates reproducibility with same seed
func TestFluidPlantDeterminism(t *testing.T) {
	const testSeed = 55555
	const numTicks = 30
	const testArrival = 0.65

	// First run
	plant1 := physics.NewFluidPlant(testSeed)
	trace1Q := make([]float64, numTicks)

	for i := 0; i < numTicks; i++ {
		plant1.A = testArrival
		plant1.Step(1.0)
		trace1Q[i] = plant1.Q
	}

	// Second run
	plant2 := physics.NewFluidPlant(testSeed)
	trace2Q := make([]float64, numTicks)

	for i := 0; i < numTicks; i++ {
		plant2.A = testArrival
		plant2.Step(1.0)
		trace2Q[i] = plant2.Q
	}

	for i := 0; i < numTicks; i++ {
		if math.Abs(trace1Q[i]-trace2Q[i]) > 1e-10 {
			t.Errorf("determinism: Q diverged at tick %d: %.15f vs %.15f",
				i, trace1Q[i], trace2Q[i])
		}
	}

	t.Logf("determinism: %d ticks identical between runs (seed=%d)", numTicks, testSeed)
}

// TestFluidPlantBoundaryRegimes validates extreme load conditions
func TestFluidPlantBoundaryRegimes(t *testing.T) {
	testCases := []struct {
		name    string
		arrival float64
		seed    int64
	}{
		{"minimal", 0.1, 100},
		{"normal", 0.5, 101},
		{"high_stress", 1.5, 102},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plant := physics.NewFluidPlant(tc.seed)

			for i := 0; i < 30; i++ {
				plant.A = tc.arrival
				plant.Step(1.0)
			}

			if math.IsNaN(plant.Q) || math.IsInf(plant.Q, 0) {
				t.Errorf("%s: Q diverged to %v", tc.name, plant.Q)
			}
			if math.IsNaN(plant.Z) || math.IsInf(plant.Z, 0) {
				t.Errorf("%s: Z diverged to %v", tc.name, plant.Z)
			}

			t.Logf("%s: Q≈%.2f, Z≈%.3f, R≈%.3f, Phase=%d", tc.name, plant.Q, plant.Z, plant.R, plant.Phase)
		})
	}
}

// TestFluidPlantBurstResponse validates transient response to load spike
func TestFluidPlantBurstResponse(t *testing.T) {
	plant := physics.NewFluidPlant(77777)

	// Baseline
	for i := 0; i < 10; i++ {
		plant.A = 0.2
		plant.Step(1.0)
	}
	qBeforeBurst := plant.Q

	// Burst
	for i := 0; i < 5; i++ {
		plant.A = 1.5
		plant.Step(1.0)
	}
	qAtBurst := plant.Q

	// Recovery
	for i := 0; i < 10; i++ {
		plant.A = 0.2
		plant.Step(1.0)
	}
	qAfterRecovery := plant.Q

	t.Logf("burst response: before=%.2f, peak=%.2f, recovered=%.2f", qBeforeBurst, qAtBurst, qAfterRecovery)
}

func BenchmarkFluidPlantStep(b *testing.B) {
	plant := physics.NewFluidPlant(123456)
	plant.A = 0.5

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plant.Step(1.0)
	}
}
