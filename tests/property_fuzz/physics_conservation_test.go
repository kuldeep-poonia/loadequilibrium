package layer1

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/loadequilibrium/loadequilibrium/internal/physics"
)

// ---------------------------------------------------------------
// L1-PHY-001 — Bounded state evolution (no divergence)
// AIM: FluidPlant.Step must never produce NaN, Inf, or unbounded Q
//      for any valid arrival rate input across 100 consecutive ticks.
// THRESHOLD: max |Q| <= 100 (physically: queue length is bounded)
//            0 NaN/Inf states
// ON EXCEED: Unbounded queue length → MPC diverges → actuator
//            receives unbounded command → system crash
// ---------------------------------------------------------------
func TestL1_PHY_001_BoundedStateEvolution(t *testing.T) {
	const seed int64 = 42
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 50000
	properties := gopter.NewProperties(params)

	var (
		worstQ     float64
		worstInput map[string]float64
		iterations int
		violations int
	)

	properties.Property("FluidPlant.Step produces bounded, finite state", prop.ForAll(
		func(arrivalRate float64) bool {
			iterations++
			plant := physics.NewFluidPlant(seed)
			plant.A = arrivalRate

			var maxQ float64
			for tick := 0; tick < 100; tick++ {
				plant.Step(1.0)
				snap := plant.Snapshot()

				// Check all state variables are finite.
				for key, val := range snap {
					if math.IsNaN(val) || math.IsInf(val, 0) {
						violations++
						t.Logf("VIOLATION: %s is %v at tick %d, arrival=%.4f", key, val, tick, arrivalRate)
						return false
					}
				}
				if plant.Q > maxQ {
					maxQ = plant.Q
				}
			}
			if maxQ > worstQ {
				worstQ = maxQ
				worstInput = map[string]float64{"arrival_rate": arrivalRate, "max_q": maxQ}
			}
			// The fluid plant has a stabilising term -StabilityGain*Q in the drift.
			// For bounded arrivals, Q must remain bounded. 100 is a generous physical bound.
			if maxQ > 100 {
				violations++
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 5.0), // arrival rates from idle to extreme overload
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-PHY-001", Layer: 1,
		Name:              "Bounded state evolution (no divergence)",
		Aim:               "FluidPlant.Step must never produce NaN/Inf/unbounded Q for any arrival rate over 100 ticks",
		Package:           "internal/physics",
		File:              "plant.go",
		FunctionUnderTest: "FluidPlant.Step + FluidPlant.Snapshot",
		Threshold:         L1Threshold{"max_queue_length", "<=", 100, "requests", "StabilityGain=0.42 ensures dissipative contraction: bounded input → bounded Q. 100 is generous; math says ~12 is steady-state max at arrival=5"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: worstQ, ActualUnit: "requests",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstQ, DurationMs: durationMs,
		},
		OnExceed: "Unbounded queue length → MPC operates on divergent state → actuator command has no physical bound → system crash or hardware damage",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("FluidPlant.Step across %d random arrival rates [0, 5], each run for 100 ticks", iterations),
			WhyThisThreshold:     "The plant has dissipative drift -0.42*Q; for bounded arrivals (max 5), steady-state Q ≈ (A-S)/0.42 ≈ 12. Threshold 100 gives 8× headroom for stochastic excursions",
			WhatHappensIfFails:   "Queue length diverges without bound, MPC and PID operate on meaningless state, all downstream control becomes unstable",
			IsDeterministic:      "Yes — gopter seed=42, FluidPlant seed=42, fully reproducible",
			HasEverFailed:        fmt.Sprintf("%d violations found in this run", violations),
			WorstCaseDescription: fmt.Sprintf("worst Q=%.4f at input %v", worstQ, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-PHY-001 FAILED: %d violations. Worst Q=%.4f (threshold=100). Input=%v\nFIX: inspect FluidPlant.Step in internal/physics/plant.go for missing dissipation or unbounded integration",
			violations, worstQ, worstInput)
	}
	t.Logf("L1-PHY-001 PASS: %d iterations, worst Q=%.4f", iterations, worstQ)
}

// ---------------------------------------------------------------
// L1-PHY-002 — Deterministic reproducibility with same seed
// AIM: Two FluidPlant instances with identical seed and inputs must
//      produce bit-identical state traces.
// THRESHOLD: max |Q_diff| <= 0 (exact bit equality)
// ON EXCEED: Stochastics break reproducibility → debugging impossible,
//            simulation replay worthless
// ---------------------------------------------------------------
func TestL1_PHY_002_DeterministicReproducibility(t *testing.T) {
	const seed int64 = 99
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 20000
	properties := gopter.NewProperties(params)

	var (
		worstDiff  float64
		worstInput map[string]float64
		iterations int
		violations int
	)

	const numTicks = 50

	properties.Property("identical seed produces identical trace", prop.ForAll(
		func(arrivalRate float64, plantSeed int64) bool {
			iterations++

			// Run 1.
			p1 := physics.NewFluidPlant(plantSeed)
			for tick := 0; tick < numTicks; tick++ {
				p1.A = arrivalRate
				p1.Step(1.0)
			}

			// Run 2 — same seed, same inputs.
			p2 := physics.NewFluidPlant(plantSeed)
			for tick := 0; tick < numTicks; tick++ {
				p2.A = arrivalRate
				p2.Step(1.0)
			}

			snap1 := p1.Snapshot()
			snap2 := p2.Snapshot()

			var maxDiff float64
			for key := range snap1 {
				d := math.Abs(snap1[key] - snap2[key])
				if d > maxDiff {
					maxDiff = d
				}
			}
			if maxDiff > worstDiff {
				worstDiff = maxDiff
				worstInput = map[string]float64{"arrival_rate": arrivalRate, "plant_seed": float64(plantSeed), "max_diff": maxDiff}
			}
			if maxDiff > 0 {
				violations++
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 3.0),
		gen.Int64Range(1, 1000000),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-PHY-002", Layer: 1,
		Name:              "Deterministic reproducibility",
		Aim:               "Two FluidPlant runs with identical seed+inputs must produce bit-identical state",
		Package:           "internal/physics",
		File:              "plant.go",
		FunctionUnderTest: "FluidPlant.Step (deterministic seed path)",
		Threshold:         L1Threshold{"max_state_diff", "==", 0, "dimensionless", "Same seed + same input = same output by construction of math/rand.NewSource"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: worstDiff, ActualUnit: "dimensionless",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstDiff, DurationMs: durationMs,
		},
		OnExceed: "Non-determinism breaks simulation replay, A/B testing, and offline debugging — root cause analysis becomes impossible",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("FluidPlant determinism across %d seed/arrival combinations, %d ticks each", iterations, numTicks),
			WhyThisThreshold:     "Threshold is exactly 0: Go's math/rand with same source produces identical sequence. Any nonzero diff means external state leaked into the computation",
			WhatHappensIfFails:   "Simulation replay produces different results → debugging and regression testing become unreliable",
			IsDeterministic:      "Yes — gopter seed=99, plant seeds generated deterministically",
			HasEverFailed:        fmt.Sprintf("%d violations found in this run", violations),
			WorstCaseDescription: fmt.Sprintf("worst diff=%.4e at input %v", worstDiff, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-PHY-002 FAILED: %d violations. Worst diff=%.4e (threshold=0). Input=%v\nFIX: inspect FluidPlant for non-deterministic state (e.g. time.Now, map iteration order)",
			violations, worstDiff, worstInput)
	}
	t.Logf("L1-PHY-002 PASS: %d iterations, worst diff=%.4e", iterations, worstDiff)
}

// ---------------------------------------------------------------
// L1-PHY-003 — Hazard non-negativity invariant
// AIM: FluidPlant.Z (hazard state) must never go negative.
//      The plant enforces Z >= 0 via clamp in updateHazard.
// THRESHOLD: min Z >= 0 across all ticks and inputs
// ON EXCEED: Negative hazard → service rate boost from degradation
//            → physics model inverted → MPC steers into collapse
// ---------------------------------------------------------------
func TestL1_PHY_003_HazardNonNegativity(t *testing.T) {
	const seed int64 = 1337
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 50000
	properties := gopter.NewProperties(params)

	var (
		worstZ     float64
		worstInput map[string]float64
		iterations int
		violations int
	)

	properties.Property("hazard Z >= 0 at every tick", prop.ForAll(
		func(arrivalRate float64) bool {
			iterations++
			plant := physics.NewFluidPlant(seed)

			for tick := 0; tick < 100; tick++ {
				plant.A = arrivalRate
				plant.Step(1.0)
				if plant.Z < 0 {
					violations++
					if plant.Z < worstZ {
						worstZ = plant.Z
						worstInput = map[string]float64{"arrival": arrivalRate, "tick": float64(tick), "z": plant.Z}
					}
					return false
				}
			}
			return true
		},
		gen.Float64Range(0.0, 5.0),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-PHY-003", Layer: 1,
		Name:              "Hazard non-negativity invariant",
		Aim:               "FluidPlant.Z (hazard) must stay >= 0 for all inputs at every tick",
		Package:           "internal/physics",
		File:              "plant.go",
		FunctionUnderTest: "FluidPlant.updateHazard (via Step)",
		Threshold:         L1Threshold{"min_hazard_z", ">=", 0, "dimensionless", "Hazard is a damage accumulator — negative hazard is physically meaningless (you cannot un-damage hardware)"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: worstZ, ActualUnit: "dimensionless",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstZ, DurationMs: durationMs,
		},
		OnExceed: "Negative hazard → BaseService/(1+α*q+β*Z) increases service rate → physics model rewards damage → MPC exploits this to steer into collapse",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("FluidPlant.Z non-negativity across %d random arrival rates, 100 ticks each", iterations),
			WhyThisThreshold:     "Z represents cumulative structural degradation. updateHazard clamps Z>=0 explicitly. If the clamp is removed or bypassed, Z goes negative under healing+drag",
			WhatHappensIfFails:   "Service rate formula S = BaseService/(1+α*Q+β*Z) produces S > BaseService when Z < 0 → degraded system appears faster than healthy one",
			IsDeterministic:      "Yes — gopter seed=1337, plant seed=1337",
			HasEverFailed:        fmt.Sprintf("%d violations found in this run", violations),
			WorstCaseDescription: fmt.Sprintf("worst Z=%.6f at input %v", worstZ, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-PHY-003 FAILED: %d violations. Worst Z=%.6f (threshold>=0). Input=%v\nFIX: ensure clamp Z>=0 in updateHazard in internal/physics/plant.go",
			violations, worstZ, worstInput)
	}
	t.Logf("L1-PHY-003 PASS: %d iterations, worst Z=%.6f (non-negative confirmed)", iterations, worstZ)
}

// ---------------------------------------------------------------
// L1-PHY-004 — Fuzz: FluidPlant.Step never panics or returns NaN/Inf
// AIM: No float64 input that reaches Step must cause panic, NaN, or Inf
// THRESHOLD: 0 panics, 0 NaN, 0 Inf across fuzz corpus
// ON EXCEED: production crash on unexpected sensor reading
// ---------------------------------------------------------------
func FuzzPlantStep(f *testing.F) {
	// Seed corpus — known edge cases.
	f.Add(0.0, 1.0)
	f.Add(1e308, 1.0)
	f.Add(-0.0, 1.0)
	f.Add(math.SmallestNonzeroFloat64, 0.001)
	f.Add(1000.0, 0.1)

	f.Fuzz(func(t *testing.T, arrivalRate, dt float64) {
		// Skip non-physical inputs.
		if dt <= 0 || dt > 100 || math.IsNaN(arrivalRate) || math.IsInf(arrivalRate, 0) {
			t.Skip()
		}
		plant := physics.NewFluidPlant(12345)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("L1-PHY-004 PANIC: arrival=%.4e dt=%.4e panic=%v\nFIX: add input validation in FluidPlant.Step",
					arrivalRate, dt, r)
			}
		}()
		plant.A = arrivalRate
		plant.Step(dt)

		snap := plant.Snapshot()
		for key, val := range snap {
			if math.IsNaN(val) {
				t.Fatalf("L1-PHY-004 NaN: %s at arrival=%.4e dt=%.4e\nFIX: add NaN guard after division in plant.Step", key, arrivalRate, dt)
			}
			if math.IsInf(val, 0) {
				t.Fatalf("L1-PHY-004 Inf: %s at arrival=%.4e dt=%.4e\nFIX: add overflow clamp in plant.Step", key, arrivalRate, dt)
			}
		}
	})
}
