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
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ---------------------------------------------------------------
// L1-MOD-001 — Stability assessment CollapseZone classification
// AIM: RunStabilityAssessment must classify CollapseZone correctly:
//      "safe" when effectiveRho < warningBoundary
//      "warning" when warningBoundary <= effectiveRho < collapseThreshold
//      "collapse" when effectiveRho >= collapseThreshold
// THRESHOLD: 0 misclassifications
// ON EXCEED: Wrong zone classification → controller applies wrong
//            damping strategy → either under-damps (crash) or
//            over-damps (wasted capacity)
// ---------------------------------------------------------------
func TestL1_MOD_001_StabilityZoneClassification(t *testing.T) {
	const seed int64 = 31415
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 30000
	properties := gopter.NewProperties(params)

	var (
		worstCase  interface{}
		iterations int
		violations int
	)

	const collapseThreshold = 0.90

	properties.Property("CollapseZone classification is correct", prop.ForAll(
		func(rho, hazard, reservoir float64) bool {
			iterations++

			q := modelling.QueueModel{
				ServiceID:        "test-svc",
				Utilisation:      rho,
				UtilisationTrend: 0,
				Hazard:           hazard,
				Reservoir:        reservoir,
			}
			sig := modelling.SignalState{
				ServiceID: "test-svc",
				FastEWMA:  rho,
				SlowEWMA:  rho,
			}
			topo := topology.GraphSnapshot{
				Nodes: []topology.Node{{ServiceID: "test-svc", NormalisedLoad: 1.0}},
			}

			sa := modelling.RunStabilityAssessment(q, sig, topo, collapseThreshold)

			// Compute effectiveRho as the function does internally.
			effectiveRho := rho + hazard*0.2 + reservoir*0.1
			warningBoundary := collapseThreshold * 0.83

			var expectedZone string
			switch {
			case effectiveRho >= collapseThreshold:
				expectedZone = "collapse"
			case effectiveRho >= warningBoundary:
				expectedZone = "warning"
			default:
				expectedZone = "safe"
			}

			if sa.CollapseZone != expectedZone {
				violations++
				worstCase = map[string]interface{}{
					"rho": rho, "hazard": hazard, "reservoir": reservoir,
					"effectiveRho": effectiveRho, "expected": expectedZone, "got": sa.CollapseZone,
				}
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 1.5),   // rho
		gen.Float64Range(0.0, 0.5),   // hazard
		gen.Float64Range(0.0, 1.0),   // reservoir
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-MOD-001", Layer: 1,
		Name:              "Stability zone classification correctness",
		Aim:               "RunStabilityAssessment must classify CollapseZone correctly for any (ρ, hazard, reservoir)",
		Package:           "internal/modelling",
		File:              "stability.go",
		FunctionUnderTest: "RunStabilityAssessment",
		Threshold:         L1Threshold{"misclassification_count", "==", 0, "count", "Zone boundaries: safe < 0.83*threshold < warning < threshold < collapse. Any misclassification sends controller into wrong mode"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstCase, WorstCaseOutput: float64(violations), DurationMs: durationMs,
		},
		OnExceed: "Controller receives wrong zone → 'collapse' zone triggers max damping; wrong zone = either overreaction (capacity waste) or underreaction (congestion crash)",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("RunStabilityAssessment zone classification across %d random (ρ, hazard, reservoir) triples", iterations),
			WhyThisThreshold:     "Zone boundaries are exact: effectiveRho = ρ + hazard*0.2 + reservoir*0.1 checked against threshold*0.83 and threshold. Any misclassification is a bug",
			WhatHappensIfFails:   "Controller applies wrong damping strategy; collapse zone underreaction causes congestion crash; safe zone overreaction wastes capacity",
			IsDeterministic:      "Yes — gopter seed=31415",
			HasEverFailed:        fmt.Sprintf("%d violations", violations),
			WorstCaseDescription: fmt.Sprintf("misclassification at %v", worstCase),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-MOD-001 FAILED: %d misclassifications. Case=%v\nFIX: check zone boundary logic in RunStabilityAssessment in internal/modelling/stability.go",
			violations, worstCase)
	}
	t.Logf("L1-MOD-001 PASS: %d iterations, 0 misclassifications", iterations)
}

// ---------------------------------------------------------------
// L1-MOD-002 — StabilityMargin is exactly 1 - effectiveRho
// AIM: StabilityMargin must equal 1 - (rho + hazard*0.2 + reservoir*0.1)
//      before upstream pressure adjustments
// THRESHOLD: |error| <= 1e-10 (float64 arithmetic tolerance)
// ON EXCEED: Stability margin miscalculated → controller bases
//            damping decisions on wrong distance-to-saturation
// ---------------------------------------------------------------
func TestL1_MOD_002_StabilityMarginFormula(t *testing.T) {
	const seed int64 = 54321
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 30000
	properties := gopter.NewProperties(params)

	var (
		worstErr   float64
		worstCase  interface{}
		iterations int
		violations int
	)

	properties.Property("StabilityMargin = 1 - effectiveRho (no upstream pressure)", prop.ForAll(
		func(rho, hazard, reservoir float64) bool {
			iterations++

			q := modelling.QueueModel{
				ServiceID:        "test-svc",
				Utilisation:      rho,
				UtilisationTrend: 0,
				Hazard:           hazard,
				Reservoir:        reservoir,
				UpstreamPressure: 0, // zero upstream to avoid adjustment
			}
			sig := modelling.SignalState{
				ServiceID: "test-svc",
				FastEWMA:  rho,
				SlowEWMA:  rho,
			}
			topo := topology.GraphSnapshot{
				Nodes: []topology.Node{{ServiceID: "test-svc", NormalisedLoad: 1.0}},
			}

			sa := modelling.RunStabilityAssessment(q, sig, topo, 0.90)

			// Expected: 1 - (rho + hazard*0.2 + reservoir*0.1)
			effectiveRho := rho + hazard*0.2 + reservoir*0.1
			expected := 1.0 - effectiveRho

			err := math.Abs(sa.StabilityMargin - expected)
			if err > worstErr {
				worstErr = err
				worstCase = map[string]float64{
					"rho": rho, "hazard": hazard, "reservoir": reservoir,
					"expected": expected, "got": sa.StabilityMargin, "err": err,
				}
			}
			if err > 1e-10 {
				violations++
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 1.5),
		gen.Float64Range(0.0, 0.3),
		gen.Float64Range(0.0, 0.5),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-MOD-002", Layer: 1,
		Name:              "Stability margin formula correctness",
		Aim:               "StabilityMargin must equal 1 - (ρ + hazard*0.2 + reservoir*0.1) when no upstream pressure",
		Package:           "internal/modelling",
		File:              "stability.go",
		FunctionUnderTest: "RunStabilityAssessment",
		Threshold:         L1Threshold{"margin_formula_error", "<=", 1e-10, "dimensionless", "Direct algebraic identity: margin = 1 - effectiveRho. 1e-10 covers float64 rounding"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: worstErr, ActualUnit: "dimensionless",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstCase, WorstCaseOutput: worstErr, DurationMs: durationMs,
		},
		OnExceed: "StabilityMargin miscalculated → Engine.RunControl uses wrong distance-to-saturation → PID tunes deadband incorrectly → control oscillation or stiction",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("StabilityMargin formula across %d random (ρ, hazard, reservoir) inputs with 0 upstream pressure", iterations),
			WhyThisThreshold:     "margin = 1 - effectiveRho is a single subtraction; 1e-10 is generous for float64. Any larger error means the formula changed or has a bug",
			WhatHappensIfFails:   "Controller bases all decisions on distance-to-saturation. Wrong margin → wrong deadband → either chattering (too tight) or unresponsive (too loose)",
			IsDeterministic:      "Yes — gopter seed=54321",
			HasEverFailed:        fmt.Sprintf("%d violations, worst error=%.4e", violations, worstErr),
			WorstCaseDescription: fmt.Sprintf("worst err=%.4e at %v", worstErr, worstCase),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-MOD-002 FAILED: %d violations. Worst err=%.4e. Case=%v\nFIX: check StabilityMargin computation in RunStabilityAssessment",
			violations, worstErr, worstCase)
	}
	t.Logf("L1-MOD-002 PASS: %d iterations, worst formula error=%.4e", iterations, worstErr)
}

// ---------------------------------------------------------------
// L1-MOD-003 — CollapseRisk monotonicity with ρ
// AIM: Higher effective ρ must always produce higher CollapseRisk.
//      The sigmoid function is monotonically increasing.
// THRESHOLD: 0 monotonicity violations
// ON EXCEED: Stability model claims higher load is safer → controller
//            reduces capacity when it should increase it
// ---------------------------------------------------------------
func TestL1_MOD_003_CollapseRiskMonotonicity(t *testing.T) {
	const seed int64 = 27182
	start := time.Now()
	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 30000
	properties := gopter.NewProperties(params)

	var (
		worstReg  float64
		worstCase interface{}
		iterations int
		violations int
	)

	properties.Property("CollapseRisk is monotonically increasing with ρ", prop.ForAll(
		func(rhoLow, rhoDelta float64) bool {
			iterations++
			rhoHigh := rhoLow + rhoDelta
			if rhoHigh > 2.0 {
				rhoHigh = 2.0
			}

			topo := topology.GraphSnapshot{
				Nodes: []topology.Node{{ServiceID: "test-svc", NormalisedLoad: 1.0}},
			}
			sigBase := modelling.SignalState{ServiceID: "test-svc"}

			// Low ρ assessment.
			qLow := modelling.QueueModel{
				ServiceID: "test-svc", Utilisation: rhoLow,
			}
			saLow := modelling.RunStabilityAssessment(qLow, sigBase, topo, 0.90)

			// High ρ assessment.
			qHigh := modelling.QueueModel{
				ServiceID: "test-svc", Utilisation: rhoHigh,
			}
			saHigh := modelling.RunStabilityAssessment(qHigh, sigBase, topo, 0.90)

			regression := saLow.CollapseRisk - saHigh.CollapseRisk
			if regression > worstReg {
				worstReg = regression
				worstCase = map[string]float64{
					"rho_low": rhoLow, "rho_high": rhoHigh,
					"risk_low": saLow.CollapseRisk, "risk_high": saHigh.CollapseRisk,
				}
			}
			if regression > 1e-12 {
				violations++
				return false
			}
			return true
		},
		gen.Float64Range(0.0, 1.5),
		gen.Float64Range(0.001, 0.5),
	))

	properties.TestingRun(t)

	passed := violations == 0
	durationMs := time.Since(start).Milliseconds()

	writeL1Result(L1Record{
		TestID: "L1-MOD-003", Layer: 1,
		Name:              "CollapseRisk monotonicity with utilisation",
		Aim:               "Higher effective ρ must always produce equal or higher CollapseRisk",
		Package:           "internal/modelling",
		File:              "stability.go",
		FunctionUnderTest: "RunStabilityAssessment",
		Threshold:         L1Threshold{"monotonicity_violation_count", "==", 0, "count", "Sigmoid is monotonically increasing; any regression means cascade amplification or upstream pressure logic inverted a sign"},
		Result: L1ResultData{
			Status: l1Pass(passed), ActualValue: float64(violations), ActualUnit: "count",
			IterationsRun: iterations, Seed: seed,
			WorstCaseInput: worstCase, WorstCaseOutput: worstReg, DurationMs: durationMs,
		},
		OnExceed: "CollapseRisk decreases with load → stability model claims higher load is safer → controller reduces capacity when it should increase it → congestion crash",
		Questions: L1Questions{
			WhatWasTested:        fmt.Sprintf("CollapseRisk monotonicity across %d random (ρ_low, ρ_high) pairs", iterations),
			WhyThisThreshold:     "CollapseRisk = sigmoid((effectiveRho - threshold*0.95) / 0.04). Sigmoid is strictly monotone increasing. Any regression is a formula error, not numeric noise",
			WhatHappensIfFails:   "Stability model gives inverted advice; autopilot reduces capacity under increasing load; guaranteed congestion crash",
			IsDeterministic:      "Yes — gopter seed=27182",
			HasEverFailed:        fmt.Sprintf("%d violations, worst regression=%.4e", violations, worstReg),
			WorstCaseDescription: fmt.Sprintf("worst regression=%.4e at %v", worstReg, worstCase),
		},
		RunAt: l1Now(), GoVersion: l1GoVer(),
	})

	if !passed {
		t.Fatalf("L1-MOD-003 FAILED: %d violations. Worst regression=%.4e. Case=%v\nFIX: check CollapseRisk sigmoid and cascade amplification logic in stability.go",
			violations, worstReg, worstCase)
	}
	t.Logf("L1-MOD-003 PASS: %d iterations, 0 monotonicity violations", iterations)
}