package layer4

// FILE: tests/layer4_scenario_replay/L4_REP_002_topology_fragility_golden_test.go
//
// Tests:   L4-REP-002
// Package: github.com/loadequilibrium/loadequilibrium/internal/integration
//          github.com/loadequilibrium/loadequilibrium/internal/modelling
// Real functions used:
//   integration.BuildKeystoneCollapseScenario() TopologyReplayScenario
//   integration.BuildDiamondStressScenario() TopologyReplayScenario
//   integration.BuildLinearCascadeScenario() TopologyReplayScenario
//   integration.BuildTriangleRecoveryScenario(step int) TopologyReplayScenario
//   modelling.ComputeTopologySensitivity(snap topology.GraphSnapshot) TopologySensitivity
//   TopologySensitivity.SystemFragility      float64
//   TopologySensitivity.MaxAmplificationScore float64
//   TopologySensitivity.ByService            map[string]ServiceSensitivity
//   ServiceSensitivity.PerturbationScore     float64
//   ServiceSensitivity.DownstreamReach       int
//   ServiceSensitivity.IsKeystone            bool
//   TopologyReplayScenario.ExpectedFragility float64
//   TopologyReplayScenario.Name              string
//   TopologyReplayScenario.Nodes             []topology.Node
//   TopologyReplayScenario.Edges             []topology.Edge
//
// RUN: go test ./tests/layer4_scenario_replay/ -run TestL4_REP_002 -count=1 -timeout=120s -v

import (
	"fmt"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/integration"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-002 — Topology sensitivity golden file: all 6 scenarios
//
// AIM:   For each of the 6 topology scenarios, run ComputeTopologySensitivity
//        and compare output against a stored golden file.
//        First run: writes the golden. Subsequent runs: verify within 0.001%.
//        Validates that no code change silently altered topology sensitivity.
//
// THRESHOLD: max relative delta per field <= 0.001%
// ON EXCEED: A code change to topology_sensitivity.go silently altered the
//            fragility or perturbation scores that the dashboard displays —
//            operators see wrong risk numbers without being notified.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_002_TopologyFragilityGolden(t *testing.T) {
	start := time.Now()

	const tolerancePct = 0.001 // 0.001% tolerance — tighter than float32 noise

	// ── Define all 6 scenarios using real builder functions ──────────────────
	type scenarioEntry struct {
		name     string
		scenario integration.TopologyReplayScenario
	}

	scenarios := []scenarioEntry{
		{"keystone_collapse", integration.BuildKeystoneCollapseScenario()},
		{"diamond_stress", integration.BuildDiamondStressScenario()},
		{"linear_cascade", integration.BuildLinearCascadeScenario()},
		{"recovery_step_1", integration.BuildTriangleRecoveryScenario(1)},
		{"recovery_step_2", integration.BuildTriangleRecoveryScenario(2)},
		{"recovery_step_3", integration.BuildTriangleRecoveryScenario(3)},
	}

	// ── Run sensitivity computation for each scenario ─────────────────────────
	type scenarioResult struct {
		name        string
		fragility   float64
		maxAmpScore float64
		byService   map[string]float64 // serviceID → perturbationScore
		snap        topology.GraphSnapshot
		sens        modelling.TopologySensitivity
	}
	results := make([]scenarioResult, 0, len(scenarios))

	for _, s := range scenarios {
		snap := topology.GraphSnapshot{
			CapturedAt:   time.Now(),
			Nodes:        s.scenario.Nodes,
			Edges:        s.scenario.Edges,
			CriticalPath: topology.CriticalPath{Nodes: []string{s.scenario.ExpectedCritical}},
		}
		sens := modelling.ComputeTopologySensitivity(snap)

		// Build flat numeric map for golden comparison.
		numericFields := make(map[string]float64)
		numericFields["system_fragility"] = sens.SystemFragility
		numericFields["max_amplification_score"] = sens.MaxAmplificationScore
		for svcID, ss := range sens.ByService {
			numericFields["service."+svcID+".perturbation"] = ss.PerturbationScore
			numericFields["service."+svcID+".downstream_reach"] = float64(ss.DownstreamReach)
			if ss.IsKeystone {
				numericFields["service."+svcID+".is_keystone"] = 1.0
			} else {
				numericFields["service."+svcID+".is_keystone"] = 0.0
			}
		}

		results = append(results, scenarioResult{
			name:        s.name,
			fragility:   sens.SystemFragility,
			maxAmpScore: sens.MaxAmplificationScore,
			byService:   numericFields,
			snap:        snap,
			sens:        sens,
		})

		t.Logf("L4-REP-002 [%s] fragility=%.6f max_amp=%.6f expected_fragility=%.6f",
			s.name, sens.SystemFragility, sens.MaxAmplificationScore, s.scenario.ExpectedFragility)
	}

	// ── Golden write / compare per scenario ───────────────────────────────────
	type scenarioCompareResult struct {
		name        string
		diffs       []L4FieldDiff
		worstDelta  float64
		fieldsTotal int
		passed      bool
		firstRun    bool
	}

	allComparisons := make([]scenarioCompareResult, 0, len(results))
	overallPassed := true
	var totalFieldsOutside int
	var globalWorstDelta float64

	for _, res := range results {
		goldenName := "topology_fragility_" + res.name

		if !goldenExists(goldenName) {
			// First run: write golden.
			if err := writeGoldenFile(goldenName, res.byService); err != nil {
				t.Logf("L4-REP-002 WARNING [%s]: could not write golden: %v", res.name, err)
			} else {
				t.Logf("L4-REP-002 [%s]: golden written to %s/%s.json", res.name, goldenDir, goldenName)
			}
			allComparisons = append(allComparisons, scenarioCompareResult{
				name:        res.name,
				fieldsTotal: len(res.byService),
				passed:      true,
				firstRun:    true,
			})
			continue
		}

		diffs, worstDelta := compareToGolden(goldenName, res.byService, tolerancePct)
		passed := len(diffs) == 0
		if !passed {
			overallPassed = false
			totalFieldsOutside += len(diffs)
		}
		if worstDelta > globalWorstDelta {
			globalWorstDelta = worstDelta
		}

		allComparisons = append(allComparisons, scenarioCompareResult{
			name:        res.name,
			diffs:       diffs,
			worstDelta:  worstDelta,
			fieldsTotal: len(res.byService),
			passed:      passed,
		})

		status := "PASS"
		if !passed {
			status = "FAIL"
		}
		t.Logf("L4-REP-002 [%s]: %s — fields=%d outside_tolerance=%d worst_delta=%.6f%%",
			res.name, status, len(res.byService), len(diffs), worstDelta)

		for _, d := range diffs {
			t.Logf("  DIFF field=%s golden=%.9f actual=%.9f delta=%.6f%%",
				d.FieldName, d.GoldenValue, d.ActualValue, d.RelDeltaPct)
		}
	}

	// ── Build summary for results file ────────────────────────────────────────
	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	for _, c := range allComparisons {
		if c.firstRun {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: GOLDEN_WRITTEN (first run)", c.name))
		} else if !c.passed {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"%s: FAIL fields_outside=%d worst_delta=%.6f%%",
				c.name, len(c.diffs), c.worstDelta,
			))
			for _, d := range c.diffs {
				errMsgs = append(errMsgs, fmt.Sprintf(
					"  → %s: golden=%.9f actual=%.9f delta=%.6f%%",
					d.FieldName, d.GoldenValue, d.ActualValue, d.RelDeltaPct,
				))
			}
		} else {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"%s: PASS fields=%d all within %.4f%%", c.name, c.fieldsTotal, tolerancePct,
			))
		}
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-002",
		Layer:  4,
		Name:   "Topology sensitivity golden file comparison — 6 scenarios",
		Aim: fmt.Sprintf(
			"ComputeTopologySensitivity output for all 6 scenarios must match golden files within %.4f%%",
			tolerancePct,
		),
		PackagesInvolved: []string{"internal/integration", "internal/modelling", "internal/topology"},
		FunctionsTested: []string{
			"integration.BuildKeystoneCollapseScenario",
			"integration.BuildDiamondStressScenario",
			"integration.BuildLinearCascadeScenario",
			"integration.BuildTriangleRecoveryScenario",
			"modelling.ComputeTopologySensitivity",
		},
		GoldenFile: goldenDir + "/topology_fragility_*.json",
		Threshold: L4Threshold{
			Metric:    "max_field_relative_delta_pct",
			Operator:  "<=",
			Value:     tolerancePct,
			Unit:      "percent",
			Rationale: "Any change in fragility/perturbation scores changes what operators see in the dashboard risk panel",
		},
		Result: L4ResultData{
			Status:        l4Status(overallPassed),
			ActualValue:   globalWorstDelta,
			ActualUnit:    "relative_delta_pct",
			RunsAttempted: 1,
			FieldsOutside: totalFieldsOutside,
			WorstDeltaPct: globalWorstDelta,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "A code change in topology_sensitivity.go silently altered the SystemFragility or " +
			"PerturbationScore values that the dashboard renders — operators see wrong risk information",
		Questions: L4Questions{
			WhatWasTested: fmt.Sprintf(
				"ComputeTopologySensitivity on 6 topology snapshots: %s",
				func() string {
					names := ""
					for i, s := range scenarios {
						if i > 0 {
							names += ", "
						}
						names += s.name
					}
					return names
				}(),
			),
			WhyThisThreshold:     "0.001% tolerance covers float64 arithmetic variance; larger change is a real behavioral difference that changes dashboard values",
			WhatHappensIfFails:   "Dashboard fragility scores and service perturbation rankings differ from expected — operators may prioritise wrong services",
			HowDeterminismVerified: "Golden written on first run from actual runtime output; compared on every subsequent run",
			IsGoldenFileFrozen:   "Yes — update only when the topology sensitivity algorithm is intentionally changed",
			HowToUpdateGolden:    "Delete tests/layer4_scenario_replay/golden/topology_fragility_*.json files and re-run test",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !overallPassed {
		t.Fatalf(
			"L4-REP-002 FAILED: %d fields outside %.4f%% tolerance. Worst delta=%.6f%%\n"+
				"FIX: Check what changed in internal/modelling/topology_sensitivity.go.\n"+
				"     If change is intentional, delete the golden files listed above and re-run.\n"+
				"     If unintentional, revert the change that caused the deviation.",
			totalFieldsOutside, tolerancePct, globalWorstDelta,
		)
	}
	t.Logf("L4-REP-002 PASS | %d scenarios, all fields within %.4f%% | worst=%.6f%%",
		len(scenarios), tolerancePct, globalWorstDelta)
}

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-002b — Expected fragility values match builders' own declarations
//
// AIM:   Each builder function declares ExpectedFragility. The actual computed
//        value from ComputeTopologySensitivity must be within 5% of that declared
//        expectation. This tests that the builders are internally consistent.
//
// THRESHOLD: |actual - expected| / expected <= 5%
// ON EXCEED: The scenario builder declares wrong expected values — test setup
//            is misleading and the L4-REP-002 golden baseline is questionable.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_002b_ExpectedFragilityMatchesBuilders(t *testing.T) {
	start := time.Now()

	type check struct {
		name     string
		expected float64
		actual   float64
		deltaPct float64
		passed   bool
	}

	const tolerancePct = 5.0 // 5% — intentionally loose; builder values are approximate

	entries := []struct {
		name     string
		scenario integration.TopologyReplayScenario
	}{
		{"keystone_collapse", integration.BuildKeystoneCollapseScenario()},
		{"diamond_stress", integration.BuildDiamondStressScenario()},
		{"linear_cascade", integration.BuildLinearCascadeScenario()},
		{"recovery_step_1", integration.BuildTriangleRecoveryScenario(1)},
		{"recovery_step_2", integration.BuildTriangleRecoveryScenario(2)},
		{"recovery_step_3", integration.BuildTriangleRecoveryScenario(3)},
	}

	checks := make([]check, 0, len(entries))
	overallPassed := true
	var worstDelta float64

	for _, e := range entries {
		snap := topology.GraphSnapshot{
			CapturedAt:   time.Now(),
			Nodes:        e.scenario.Nodes,
			Edges:        e.scenario.Edges,
			CriticalPath: topology.CriticalPath{Nodes: []string{e.scenario.ExpectedCritical}},
		}
		sens := modelling.ComputeTopologySensitivity(snap)
		actual := sens.SystemFragility
		expected := e.scenario.ExpectedFragility

		deltaPct := 0.0
		if expected != 0 {
			deltaPct = absFloat64(actual-expected) / absFloat64(expected) * 100
		} else if actual != 0 {
			deltaPct = 100
		}
		within := deltaPct <= tolerancePct
		if !within {
			overallPassed = false
		}
		if deltaPct > worstDelta {
			worstDelta = deltaPct
		}

		checks = append(checks, check{
			name:     e.name,
			expected: expected,
			actual:   actual,
			deltaPct: deltaPct,
			passed:   within,
		})

		status := "PASS"
		if !within {
			status = "FAIL"
		}
		t.Logf("L4-REP-002b [%s]: %s | expected=%.4f actual=%.6f delta=%.2f%%",
			e.name, status, expected, actual, deltaPct)
	}

	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	for _, c := range checks {
		if !c.passed {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"%s: expected=%.4f actual=%.6f delta=%.2f%% (threshold %.1f%%)",
				c.name, c.expected, c.actual, c.deltaPct, tolerancePct,
			))
		} else {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"%s: OK delta=%.2f%%", c.name, c.deltaPct,
			))
		}
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-002b",
		Layer:  4,
		Name:   "Builder ExpectedFragility values match ComputeTopologySensitivity output",
		Aim: fmt.Sprintf(
			"Each scenario builder's ExpectedFragility must be within %.1f%% of actual ComputeTopologySensitivity output",
			tolerancePct,
		),
		PackagesInvolved: []string{"internal/integration", "internal/modelling"},
		FunctionsTested: []string{
			"integration.BuildKeystoneCollapseScenario (ExpectedFragility field)",
			"modelling.ComputeTopologySensitivity (SystemFragility output)",
		},
		GoldenFile: "N/A — no golden needed, threshold is from builder declarations",
		Threshold: L4Threshold{
			Metric:    "fragility_delta_pct",
			Operator:  "<=",
			Value:     tolerancePct,
			Unit:      "percent",
			Rationale: "5% tolerance — builder expected values are approximate documentation; exact match not required but large deviation means stale docs",
		},
		Result: L4ResultData{
			Status:        l4Status(overallPassed),
			ActualValue:   worstDelta,
			ActualUnit:    "worst_delta_pct",
			RunsAttempted: 1,
			FieldsChecked: len(checks),
			WorstDeltaPct: worstDelta,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Scenario builders declare fragility values that no longer match computation — " +
			"documentation and test expectations diverged from reality",
		Questions: L4Questions{
			WhatWasTested:        "ExpectedFragility in each of the 6 builder functions vs actual ComputeTopologySensitivity output",
			WhyThisThreshold:     "5% — these are declared approximations; exact match is not the contract, but >5% means the algorithm changed significantly",
			WhatHappensIfFails:   "Test documentation claims wrong fragility values — operators briefed on wrong expected behaviour",
			HowDeterminismVerified: "Single run — cross-run determinism verified in L4-REP-001",
			IsGoldenFileFrozen:   "N/A",
			HowToUpdateGolden:    "Update ExpectedFragility in each builder function in internal/integration/replay.go",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !overallPassed {
		t.Fatalf(
			"L4-REP-002b FAILED: worst delta=%.2f%% (threshold=%.1f%%)\n"+
				"FIX: Update ExpectedFragility in integration.Build*Scenario() functions in internal/integration/replay.go\n"+
				"     to match actual ComputeTopologySensitivity output. Do NOT change the threshold.\n%v",
			worstDelta, tolerancePct, errMsgs,
		)
	}
	t.Logf("L4-REP-002b PASS | worst_delta=%.2f%% (threshold=%.1f%%)", worstDelta, tolerancePct)
}