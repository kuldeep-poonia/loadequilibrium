package layer1

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
)

// ---------------------------------------------------------------
// L1-NET-001 — NetworkField finite-state invariant (no NaN/Inf)
// AIM: After any number of steps with any valid edge configuration,
//      all cell densities must remain finite (no NaN, no Inf).
//      TotalMass and TotalVariation must also be finite.
// THRESHOLD: 0 NaN/Inf states across all iterations
// ON EXCEED: PDE solver diverges → density field becomes NaN →
//            all network-coupled routing produces garbage
// ---------------------------------------------------------------
func TestL1_NET_001_NetworkFieldFiniteState(t *testing.T) {
	const seed int64 = 99887
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 10000
	properties := gopter.NewProperties(params)

	var (
		worstInput interface{}
		iterations int
		violations int
	)

	properties.Property("NetworkField states always finite", prop.ForAll(
		func(nCells int, initRho, noiseAmp, serviceRate float64) bool {
			iterations++
			if nCells < 3 {
				nCells = 3
			}

			nf := modelling.NewNetworkField()
			nf.RNG = modelling.RNG{}

			edge := &modelling.EdgeField{
				Cells:       make([]modelling.Cell, nCells),
				Dx:          1.0 / float64(nCells),
				ServiceRate: serviceRate,
				NoiseAmp:    noiseAmp,
				SourceGain:  0,
			}
			for i := range edge.Cells {
				edge.Cells[i].Rho = initRho
			}
			nf.Edges["test-edge"] = edge

			for step := 0; step < 30; step++ {
				nf.Step()

				mass := nf.TotalMass()
				tv := nf.TotalVariation()

				if math.IsNaN(mass) || math.IsInf(mass, 0) ||
					math.IsNaN(tv) || math.IsInf(tv, 0) {
					violations++
					worstInput = map[string]interface{}{
						"n_cells": nCells, "init_rho": initRho,
						"noise_amp": noiseAmp, "svc_rate": serviceRate,
						"step": step, "mass": mass, "tv": tv,
					}
					return false
				}

				for i, c := range edge.Cells {
					if math.IsNaN(c.Rho) || math.IsInf(c.Rho, 0) {
						violations++
						worstInput = map[string]interface{}{
							"n_cells": nCells, "init_rho": initRho,
							"noise_amp": noiseAmp, "svc_rate": serviceRate,
							"step": step, "cell_idx": i, "rho": c.Rho,
						}
						return false
					}
				}
			}
			return true
		},
		gen.IntRange(3, 50),
		gen.Float64Range(0.0, 1.0),
		gen.Float64Range(0.0, 0.05),
		gen.Float64Range(0.0, 0.5),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-NET-001", Layer: 1,
		Name:              "NetworkField finite-state invariant",
		Aim:               "All cell densities, TotalMass, and TotalVariation must remain finite (no NaN/Inf) for any configuration",
		Package:           "internal/modelling",
		File:              "network_field.go",
		FunctionUnderTest: "NetworkField.Step + TotalMass + TotalVariation",
		Threshold:         L1Threshold{"nan_inf_count", "==", 0, "count", "NaN/Inf in density field = PDE solver divergence. Any single occurrence corrupts the entire network model"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: float64(violations), DurationMs: durationMs,
		},
		OnExceed: "PDE solver produces NaN/Inf → density field becomes garbage → all network-coupled load propagation and routing produce meaningless results",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("NetworkField finite-state across %d random configs (nCells, initRho, noiseAmp, serviceRate), 30 steps each", iterations),
			WhyThisThreshold:     "The solver uses Godunov flux, MUSCL limiters, and explicit [0,1] clamping. NaN/Inf can only appear from division by zero in dt() or unguarded floating point operations — any occurrence is a code defect",
			WhatHappensIfFails:   "Density field contains NaN/Inf → all downstream consumers (topology, routing, network coupling) produce garbage → system-wide control failure",
			IsDeterministic:      "Yes — gopter seed=99887, deterministic RNG",
			HasEverFailed:        fmt.Sprintf("%d violations", violations),
			WorstCaseDescription: fmt.Sprintf("NaN/Inf at %v", worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-NET-001 FAILED: %d violations. Input=%v\nFIX: check for division by zero in dt() and unguarded operations in fluxStep/sourceStep",
			violations, worstInput)
	}
	t.Logf("L1-NET-001 PASS: %d iterations, all states finite", iterations)
}

// ---------------------------------------------------------------
// L1-NET-002 — NetworkField density always in [0, 1]
// AIM: Every cell's Rho must stay in [0, 1] after any number of steps.
//      The solver enforces this via explicit clamping in fluxStep.
// THRESHOLD: 0 out-of-range cells
// ON EXCEED: Density > 1 or < 0 is physically meaningless →
//            Flux/Supply/Demand functions produce wrong values
// ---------------------------------------------------------------
func TestL1_NET_002_NetworkFieldDensityBounds(t *testing.T) {
	const seed int64 = 77777
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 10000
	properties := gopter.NewProperties(params)

	var (
		worstOOB  float64
		worstInput interface{}
		iterations int
		violations int
	)

	properties.Property("all cell densities in [0, 1]", prop.ForAll(
		func(nCells int, initRho, noiseAmp float64) bool {
			iterations++
			if nCells < 3 {
				nCells = 3
			}

			nf := modelling.NewNetworkField()

			edge := &modelling.EdgeField{
				Cells:       make([]modelling.Cell, nCells),
				Dx:          1.0 / float64(nCells),
				ServiceRate: 0.1,
				NoiseAmp:    noiseAmp,
				SourceGain:  0.3,
			}
			for i := range edge.Cells {
				edge.Cells[i].Rho = initRho
			}
			nf.Edges["test-edge"] = edge

			for step := 0; step < 30; step++ {
				nf.Step()

				// Check all cells after each step.
				for i, c := range edge.Cells {
					oob := 0.0
					if c.Rho < 0 {
						oob = -c.Rho
					} else if c.Rho > 1.0 {
						oob = c.Rho - 1.0
					}
					if oob > 0 {
						violations++
						if oob > worstOOB {
							worstOOB = oob
							worstInput = map[string]interface{}{
								"n_cells": nCells, "init_rho": initRho, "noise_amp": noiseAmp,
								"step": step, "cell_idx": i, "rho": c.Rho,
							}
						}
						return false
					}
				}
			}
			return true
		},
		gen.IntRange(5, 40),
		gen.Float64Range(0.0, 1.0),
		gen.Float64Range(0.0, 0.05),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-NET-002", Layer: 1,
		Name:              "NetworkField density bounds [0, 1]",
		Aim:               "Every cell Rho must remain in [0, 1] after any number of steps",
		Package:           "internal/modelling",
		File:              "network_field.go",
		FunctionUnderTest: "NetworkField.Step (fluxStep clamping)",
		Threshold:         L1Threshold{"density_oob_count", "==", 0, "count", "fluxStep has explicit clamp: if Rho < 0 → 0; if Rho > 1 → 1. Any OOB means clamp bypassed or applied in wrong order"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstInput, WorstCaseOutput: worstOOB, DurationMs: durationMs,
		},
		OnExceed: "Density outside [0,1] → Flux(ρ)=ρ(1-ρ) produces negative values → Godunov flux sign inverts → solver diverges catastrophically",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("Cell density bounds across %d random field configs, 30 steps each", iterations),
			WhyThisThreshold:     "fluxStep clamps every cell to [0,1] after update. Any OOB is a code defect, not numeric noise",
			WhatHappensIfFails:   "Flux function Flux(ρ)=ρ(1-ρ) produces wrong sign when ρ∉[0,1]; GodunovFlux comparison logic breaks; solver produces garbage",
			IsDeterministic:      "Yes — gopter seed=77777",
			HasEverFailed:        fmt.Sprintf("%d violations, worst OOB=%.6f", violations, worstOOB),
			WorstCaseDescription: fmt.Sprintf("worst OOB=%.6f at %v", worstOOB, worstInput),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-NET-002 FAILED: %d violations. Worst OOB=%.6f. Input=%v\nFIX: check cell clamping in fluxStep in network_field.go",
			violations, worstOOB, worstInput)
	}
	t.Logf("L1-NET-002 PASS: %d iterations, all densities in [0, 1]", iterations)
}

// ---------------------------------------------------------------
// L1-NET-003 — NetworkField never panics for any edge configuration
// AIM: NetworkField.Step must never panic for any valid edge config
// THRESHOLD: 0 panics
// ON EXCEED: Production crash in PDE solver
// ---------------------------------------------------------------
func FuzzNetworkFieldStep(f *testing.F) {
	f.Add(5, 0.5, 0.01, 0.1, 0.3)
	f.Add(1, 0.0, 0.0, 0.0, 0.0)
	f.Add(100, 1.0, 0.05, 0.5, 1.0)
	f.Add(3, 0.99, 0.1, 0.01, 0.01)

	f.Fuzz(func(t *testing.T, nCells int, initRho, noiseAmp, serviceRate, sourceGain float64) {
		if nCells < 1 || nCells > 200 {
			t.Skip()
		}
		if math.IsNaN(initRho) || math.IsInf(initRho, 0) {
			t.Skip()
		}

		nf := modelling.NewNetworkField()

		edge := &modelling.EdgeField{
			Cells:       make([]modelling.Cell, nCells),
			Dx:          1.0 / float64(nCells),
			ServiceRate: serviceRate,
			NoiseAmp:    noiseAmp,
			SourceGain:  sourceGain,
		}
		for i := range edge.Cells {
			edge.Cells[i].Rho = math.Max(0, math.Min(initRho, 1.0))
		}
		nf.Edges["fuzz-edge"] = edge

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("L1-NET-003 PANIC: nCells=%d initRho=%.4e noiseAmp=%.4e svcRate=%.4e srcGain=%.4e panic=%v\nFIX: add guard clauses in NetworkField.Step/fluxStep",
					nCells, initRho, noiseAmp, serviceRate, sourceGain, r)
			}
		}()

		for step := 0; step < 10; step++ {
			nf.Step()
		}

		// Verify total mass is finite.
		mass := nf.TotalMass()
		if math.IsNaN(mass) || math.IsInf(mass, 0) {
			t.Fatalf("L1-NET-003 NaN/Inf mass: nCells=%d initRho=%.4e\nFIX: check for division by zero in dt() or fluxStep",
				nCells, initRho)
		}
	})
}