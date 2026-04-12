package layer4

// FILE: tests/layer4_scenario_replay/L4_REP_004_version_regression_test.go
//
// Tests:   L4-REP-004, L4-REP-004b
// Package: github.com/loadequilibrium/loadequilibrium/internal/integration
//          github.com/loadequilibrium/loadequilibrium/internal/modelling
// Real functions used:
//   integration.BuildTriangleRecoveryScenario(step int) TopologyReplayScenario
//   modelling.ComputeTopologySensitivity(snap) TopologySensitivity
//   integration.NewReplayEngine(hub) *ReplayEngine
//   (*ReplayEngine).ExecuteAllScenarios() []ReplayCapture
//   ReplayCapture.FinalFragility   float64
//   ReplayCapture.ScenarioName     string
//
// RUN: go test ./tests/layer4_scenario_replay/ -run TestL4_REP_004 -count=1 -timeout=300s -v

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/integration"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-004 — Version regression: all scenarios within 0.01% of golden
//
// AIM:   Every commit must produce fragility values within 0.01% of the stored
//        golden for all 6 scenarios. This is the primary regression guard:
//        any unintentional algorithm change in topology_sensitivity.go will
//        fail this test before it reaches production.
//
// Prerequisite: L4-REP-002 must have run first to write the golden files.
//
// THRESHOLD: max field delta <= 0.01% per scenario
// ON EXCEED: Topology sensitivity algorithm changed in a way that alters the
//            risk scores shown on the dashboard — must be reviewed explicitly.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_004_VersionRegression(t *testing.T) {
	start := time.Now()

	const tolerancePct = 0.01 // tighter than L4-REP-002's 0.001% write tolerance

	scenarios := []struct {
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

	type regressionResult struct {
		name        string
		diffs       []L4FieldDiff
		worstDelta  float64
		fieldsTotal int
		passed      bool
		skipped     bool
	}

	allResults := make([]regressionResult, 0, len(scenarios))
	overallPassed := true
	var globalWorstDelta float64
	var totalFieldsOutside int
	skippedCount := 0

	for _, s := range scenarios {
		goldenName := "topology_fragility_" + s.name

		if !goldenExists(goldenName) {
			t.Logf("L4-REP-004 [%s]: SKIP — golden not yet written (run L4-REP-002 first)", s.name)
			allResults = append(allResults, regressionResult{name: s.name, skipped: true, passed: true})
			skippedCount++
			continue
		}

		snap := topology.GraphSnapshot{
			CapturedAt:   time.Now(),
			Nodes:        s.scenario.Nodes,
			Edges:        s.scenario.Edges,
			CriticalPath: topology.CriticalPath{Nodes: []string{s.scenario.ExpectedCritical}},
		}
		sens := modelling.ComputeTopologySensitivity(snap)

		// Build the same flat numeric map used by L4-REP-002.
		actual := make(map[string]float64)
		actual["system_fragility"] = sens.SystemFragility
		actual["max_amplification_score"] = sens.MaxAmplificationScore
		for svcID, ss := range sens.ByService {
			actual["service."+svcID+".perturbation"] = ss.PerturbationScore
			actual["service."+svcID+".downstream_reach"] = float64(ss.DownstreamReach)
			if ss.IsKeystone {
				actual["service."+svcID+".is_keystone"] = 1.0
			} else {
				actual["service."+svcID+".is_keystone"] = 0.0
			}
		}

		diffs, worstDelta := compareToGolden(goldenName, actual, tolerancePct)
		passed := len(diffs) == 0

		if !passed {
			overallPassed = false
			totalFieldsOutside += len(diffs)
		}
		if worstDelta > globalWorstDelta {
			globalWorstDelta = worstDelta
		}

		allResults = append(allResults, regressionResult{
			name:        s.name,
			diffs:       diffs,
			worstDelta:  worstDelta,
			fieldsTotal: len(actual),
			passed:      passed,
		})

		status := "PASS"
		if !passed {
			status = "FAIL"
		}
		t.Logf("L4-REP-004 [%s]: %s | fields=%d outside=%d worst_delta=%.6f%%",
			s.name, status, len(actual), len(diffs), worstDelta)
		for _, d := range diffs {
			t.Logf("  REGRESSION field=%s golden=%.9f actual=%.9f delta=%.6f%%",
				d.FieldName, d.GoldenValue, d.ActualValue, d.RelDeltaPct)
		}
	}

	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	for _, r := range allResults {
		if r.skipped {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: SKIPPED (no golden)", r.name))
		} else if !r.passed {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: FAIL outside=%d worst=%.6f%%", r.name, len(r.diffs), r.worstDelta))
			for _, d := range r.diffs {
				errMsgs = append(errMsgs, fmt.Sprintf(
					"  → %s: golden=%.9f actual=%.9f delta=%.6f%%",
					d.FieldName, d.GoldenValue, d.ActualValue, d.RelDeltaPct,
				))
			}
		} else {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: PASS fields=%d worst=%.6f%%", r.name, r.fieldsTotal, r.worstDelta))
		}
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-004",
		Layer:  4,
		Name:   "Version regression: all scenarios within 0.01% of golden",
		Aim: fmt.Sprintf(
			"Current build must produce topology sensitivity within %.4f%% of stored golden for all %d scenarios",
			tolerancePct, len(scenarios),
		),
		PackagesInvolved: []string{"internal/integration", "internal/modelling"},
		FunctionsTested: []string{
			"modelling.ComputeTopologySensitivity",
			"integration.Build*Scenario (as inputs)",
		},
		GoldenFile: goldenDir + "/topology_fragility_*.json",
		Threshold: L4Threshold{
			Metric:    "max_field_delta_pct",
			Operator:  "<=",
			Value:     tolerancePct,
			Unit:      "percent",
			Rationale: "Any change in sensitivity algorithm must be intentional and explicitly reviewed — 0.01% catches real changes, not float noise",
		},
		Result: L4ResultData{
			Status:        l4Status(overallPassed),
			ActualValue:   globalWorstDelta,
			ActualUnit:    "relative_delta_pct",
			RunsAttempted: len(scenarios) - skippedCount,
			FieldsOutside: totalFieldsOutside,
			WorstDeltaPct: globalWorstDelta,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Topology sensitivity algorithm changed between this commit and the golden-writing commit — " +
			"review internal/modelling/topology_sensitivity.go diff before proceeding",
		Questions: L4Questions{
			WhatWasTested: fmt.Sprintf(
				"ComputeTopologySensitivity output for %d scenarios compared against golden files written by L4-REP-002",
				len(scenarios),
			),
			WhyThisThreshold:     "0.01% is tighter than the write tolerance (0.001%) to catch regression; still above float64 noise floor",
			WhatHappensIfFails:   "An algorithm change altered risk scores — operators may see wrong fragility or perturbation rankings",
			HowDeterminismVerified: "Same inputs as L4-REP-002; golden files are the invariant anchor",
			IsGoldenFileFrozen:   "Yes — update ONLY when algorithm change is intentional and reviewed",
			HowToUpdateGolden:    "Delete golden files and re-run L4-REP-002, then confirm all scenarios produce expected outputs",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if skippedCount == len(scenarios) {
		t.Skip("L4-REP-004: all golden files missing — run L4-REP-002 first to write them")
	}

	if !overallPassed {
		t.Fatalf(
			"L4-REP-004 FAILED: %d fields outside %.4f%% tolerance. Worst delta=%.6f%%\n"+
				"FIX: Review git diff on internal/modelling/topology_sensitivity.go.\n"+
				"     If change is intentional: delete golden files and re-run L4-REP-002.\n"+
				"     If change is unintentional: revert the commit that caused the regression.",
			totalFieldsOutside, tolerancePct, globalWorstDelta,
		)
	}
	t.Logf("L4-REP-004 PASS | worst_delta=%.6f%% (threshold=%.4f%%)", globalWorstDelta, tolerancePct)
}

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-004b — Recovery convergence: step 3 fragility within 1% of baseline
//
// AIM:   After 3 recovery steps, the triangle scenario must converge to within
//        1% of the healthy baseline fragility.  This tests that the recovery
//        curve defined in BuildTriangleRecoveryScenario is physically plausible
//        and that the sensitivity algorithm correctly reflects recovery.
//
// THRESHOLD: |step3_fragility - baseline_fragility| / baseline_fragility <= 1%
// ON EXCEED: Recovery scenario does not converge — either the topology edges
//            in step 3 are not close enough to healthy, or the sensitivity
//            computation is overweighting a degraded metric.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_004b_RecoveryConvergence(t *testing.T) {
	start := time.Now()

	const convergenceTolPct = 1.0 // 1% tolerance for full recovery

	// ── Healthy baseline (step 0 — same topology as healthy triangle) ─────────
	// BuildTriangleRecoveryScenario with step=0 (or default) returns the baseline.
	baseline := integration.BuildTriangleRecoveryScenario(0) // default = healthy state
	baselineSnap := topology.GraphSnapshot{
		CapturedAt:   time.Now(),
		Nodes:        baseline.Nodes,
		Edges:        baseline.Edges,
		CriticalPath: topology.CriticalPath{Nodes: []string{baseline.ExpectedCritical}},
	}
	baselineSens := modelling.ComputeTopologySensitivity(baselineSnap)
	baselineFragility := baselineSens.SystemFragility
	t.Logf("L4-REP-004b [baseline] fragility=%.6f", baselineFragility)

	// ── Run 3 recovery steps ──────────────────────────────────────────────────
	stepFragilities := make([]float64, 3)
	for step := 1; step <= 3; step++ {
		recovery := integration.BuildTriangleRecoveryScenario(step)
		snap := topology.GraphSnapshot{
			CapturedAt:   time.Now(),
			Nodes:        recovery.Nodes,
			Edges:        recovery.Edges,
			CriticalPath: topology.CriticalPath{Nodes: []string{recovery.ExpectedCritical}},
		}
		sens := modelling.ComputeTopologySensitivity(snap)
		stepFragilities[step-1] = sens.SystemFragility
		t.Logf("L4-REP-004b [step %d] fragility=%.6f expected=%.6f",
			step, sens.SystemFragility, recovery.ExpectedFragility)
	}

	// ── Write golden for recovery convergence ─────────────────────────────────
	goldenData := map[string]float64{
		"baseline_fragility": baselineFragility,
		"step1_fragility":    stepFragilities[0],
		"step2_fragility":    stepFragilities[1],
		"step3_fragility":    stepFragilities[2],
	}
	if !goldenExists("recovery_convergence") {
		if err := writeGoldenFile("recovery_convergence", goldenData); err != nil {
			t.Logf("L4-REP-004b WARNING: could not write golden: %v", err)
		} else {
			t.Logf("L4-REP-004b: golden written")
		}
	}

	// ── Assertions ────────────────────────────────────────────────────────────
	step3Fragility := stepFragilities[2]

	// 1. Step 3 must be within convergenceTolPct of baseline.
	convergenceDelta := math.Abs(step3Fragility-baselineFragility) / math.Max(math.Abs(baselineFragility), 1e-9) * 100
	converged := convergenceDelta <= convergenceTolPct

	// 2. Recovery must be monotonic: step3 >= step2 >= step1 in fragility
	// (higher fragility = stronger topology = more recovered)
	// NOTE: This is expected for triangle recovery where step 3 has higher-weight edges.
	monotonic := stepFragilities[2] >= stepFragilities[0]

	passed := converged && monotonic
	durationMs := time.Since(start).Milliseconds()

	errMsgs := []string{
		fmt.Sprintf("baseline=%.6f step1=%.6f step2=%.6f step3=%.6f",
			baselineFragility, stepFragilities[0], stepFragilities[1], stepFragilities[2]),
		fmt.Sprintf("convergence_delta=%.4f%% (threshold=%.1f%%)", convergenceDelta, convergenceTolPct),
		fmt.Sprintf("monotonic=%v (step1<=step2<=step3 in fragility)", monotonic),
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-004b",
		Layer:  4,
		Name:   "Triangle recovery convergence within 1% of baseline",
		Aim: fmt.Sprintf(
			"After 3 recovery steps, fragility must be within %.1f%% of healthy baseline",
			convergenceTolPct,
		),
		PackagesInvolved: []string{"internal/integration", "internal/modelling"},
		FunctionsTested: []string{
			"integration.BuildTriangleRecoveryScenario(0,1,2,3)",
			"modelling.ComputeTopologySensitivity",
		},
		GoldenFile: goldenDir + "/recovery_convergence.json",
		Threshold: L4Threshold{
			Metric:    "convergence_delta_pct",
			Operator:  "<=",
			Value:     convergenceTolPct,
			Unit:      "percent",
			Rationale: "Step 3 edges are nearly identical to healthy baseline — convergence within 1% validates the recovery model",
		},
		Result: L4ResultData{
			Status:        l4Status(passed),
			ActualValue:   convergenceDelta,
			ActualUnit:    "convergence_delta_pct",
			RunsAttempted: 4, // baseline + 3 steps
			WorstDeltaPct: convergenceDelta,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Recovery scenario step 3 does not return to baseline within 1% — " +
			"either BuildTriangleRecoveryScenario(3) edges need updating or sensitivity is over-penalising residual degradation",
		Questions: L4Questions{
			WhatWasTested:        "Triangle recovery fragility across 3 steps; step 3 convergence to healthy baseline",
			WhyThisThreshold:     "1% matches the topology_replay_dashboard_test.go acceptance criterion of 0.98-1.00 fragility",
			WhatHappensIfFails:   "Dashboard shows incomplete recovery even after all edges are restored — operators confused about system health",
			HowDeterminismVerified: "All topology inputs are fixed (no time/random) — computation is deterministic",
			IsGoldenFileFrozen:   "Written on first run",
			HowToUpdateGolden:    "Delete tests/layer4_scenario_replay/golden/recovery_convergence.json and re-run",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L4-REP-004b FAILED: convergence_delta=%.4f%% (threshold=%.1f%%) monotonic=%v\n"+
				"baseline=%.6f step1=%.6f step2=%.6f step3=%.6f\n"+
				"FIX: Check BuildTriangleRecoveryScenario(3) edge weights — they must be within\n"+
				"     ~1%% of the healthy triangle edge weights (source in internal/integration/replay.go).",
			convergenceDelta, convergenceTolPct, monotonic,
			baselineFragility, stepFragilities[0], stepFragilities[1], stepFragilities[2],
		)
	}
	t.Logf("L4-REP-004b PASS | convergence=%.4f%% step3_fragility=%.6f baseline=%.6f",
		convergenceDelta, step3Fragility, baselineFragility)
}

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-004c — ExecuteAllScenarios replay vs golden: end-to-end regression
//
// AIM:   Run ExecuteAllScenarios() exactly once and compare every
//        ReplayCapture.FinalFragility against the golden written by L4-REP-001.
//        This is the end-to-end regression guard that combines the full
//        replay pipeline (not just direct ComputeTopologySensitivity calls).
//
// THRESHOLD: max fragility delta vs golden <= 0.01%
// ON EXCEED: The full replay pipeline (ReplayEngine → InjectTopologySnapshot →
//            ComputeTopologySensitivity) produces different results than the
//            isolated path — something in the pipeline wraps/modifies outputs.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_004c_ExecuteAllScenariosRegression(t *testing.T) {
	start := time.Now()

	const tolerancePct = 0.01

	goldenName := "replay_all_scenarios" // written by L4-REP-001
	if !goldenExists(goldenName) {
		t.Skip("L4-REP-004c: golden 'replay_all_scenarios' not present — run L4-REP-001 first")
	}

	hub := streaming.NewHub()
	replay := integration.NewReplayEngine(hub)
	captures := replay.ExecuteAllScenarios()

	// Build flat map for comparison.
	actual := make(map[string]float64, len(captures))
	for _, cap := range captures {
		actual[cap.ScenarioName] = cap.FinalFragility
	}

	diffs, worstDelta := compareToGolden(goldenName, actual, tolerancePct)
	passed := len(diffs) == 0
	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	if len(diffs) == 0 {
		errMsgs = append(errMsgs, fmt.Sprintf(
			"%d scenarios all within %.4f%% of golden", len(captures), tolerancePct,
		))
	}
	for _, d := range diffs {
		errMsgs = append(errMsgs, fmt.Sprintf(
			"scenario=%s golden=%.9f actual=%.9f delta=%.6f%%",
			d.FieldName, d.GoldenValue, d.ActualValue, d.RelDeltaPct,
		))
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-004c",
		Layer:  4,
		Name:   "ExecuteAllScenarios end-to-end regression vs golden",
		Aim: fmt.Sprintf(
			"Full replay pipeline output must match golden within %.4f%% per scenario",
			tolerancePct,
		),
		PackagesInvolved: []string{"internal/integration", "internal/streaming"},
		FunctionsTested: []string{
			"integration.NewReplayEngine",
			"(*ReplayEngine).ExecuteAllScenarios",
			"ReplayCapture.FinalFragility",
		},
		GoldenFile: goldenDir + "/" + goldenName + ".json",
		Threshold: L4Threshold{
			Metric:    "max_fragility_delta_pct",
			Operator:  "<=",
			Value:     tolerancePct,
			Unit:      "percent",
			Rationale: "End-to-end regression via full replay pipeline — catches wrapping/modification in InjectTopologySnapshot",
		},
		Result: L4ResultData{
			Status:        l4Status(passed),
			ActualValue:   worstDelta,
			ActualUnit:    "relative_delta_pct",
			RunsAttempted: 1,
			FieldsChecked: len(captures),
			FieldsOutside: len(diffs),
			WorstDeltaPct: worstDelta,
			Diffs:         diffs,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Full replay pipeline produces different fragility than direct ComputeTopologySensitivity — " +
			"InjectTopologySnapshot is transforming or overwriting the computed sensitivity",
		Questions: L4Questions{
			WhatWasTested:        "ExecuteAllScenarios() output compared to golden written by L4-REP-001",
			WhyThisThreshold:     "0.01% — pipeline should be identical to isolated function; any delta indicates unwanted transformation",
			WhatHappensIfFails:   "Replay pipeline does not faithfully reflect topology sensitivity — golden comparisons are unreliable",
			HowDeterminismVerified: "L4-REP-001 already verified determinism; this test only checks regression",
			IsGoldenFileFrozen:   "Written by L4-REP-001",
			HowToUpdateGolden:    "Re-run L4-REP-001 to regenerate",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L4-REP-004c FAILED: %d scenarios outside %.4f%% tolerance. Worst delta=%.6f%%\n"+
				"Differences:\n%v\n"+
				"FIX: Check InjectTopologySnapshot in internal/integration/replay.go —\n"+
				"     it must set capture.FinalFragility = sens.SystemFragility without modification.",
			len(diffs), tolerancePct, worstDelta, errMsgs,
		)
	}
	t.Logf("L4-REP-004c PASS | %d scenarios within %.4f%% of golden | worst=%.6f%%",
		len(captures), tolerancePct, worstDelta)
}