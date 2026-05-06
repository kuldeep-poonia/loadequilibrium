package layer4

// FILE: tests/layer4_scenario_replay/L4_REP_001_replay_engine_determinism_test.go
//
// Tests:   L4-REP-001
// Package: github.com/loadequilibrium/loadequilibrium/internal/integration
// Real functions used:
//   integration.NewReplayEngine(hub *streaming.Hub) *ReplayEngine
//   (*ReplayEngine).ExecuteAllScenarios() []ReplayCapture
//   ReplayCapture.FinalFragility      float64
//   ReplayCapture.PressureHeatmap     map[string]float64
//   ReplayCapture.CriticalPathNodes   []string
//   ReplayCapture.ScenarioName        string
//   streaming.NewHub() *Hub
//
// RUN: go test ./tests/layer4_scenario_replay/ -run TestL4_REP_001 -count=1 -timeout=300s -v
//
// DETERMINISM CONTRACT:
//   ExecuteAllScenarios() must produce bit-identical FinalFragility values
//   across all runs given the same topology inputs.  It calls
//   modelling.ComputeTopologySensitivity() which is a pure function of the
//   GraphSnapshot — therefore it MUST be deterministic.
//   If any run produces a different fragility value, a non-deterministic
//   code path exists (e.g. map iteration order used in a computation).

import (
	"fmt"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/integration"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
)

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-001 — ExecuteAllScenarios is deterministic across 20 runs
//
// AIM:   Run ExecuteAllScenarios() 20 times. Each run must produce the same
//        FinalFragility per scenario as run 0. Any deviation > 1e-9 means
//        the sensitivity computation is non-deterministic.
//
// THRESHOLD: max fragility deviation across all runs == 0 (within 1e-9)
// ON EXCEED: Replay-based regression tests are unreliable because the same
//            code produces different outputs on different runs — golden files
//            become meaningless and CI can produce false failures/passes.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_001_ExecuteAllScenariosDeterminism(t *testing.T) {
	start := time.Now()

	const (
		runs          = 20
		epsilon       = 1e-9 // fragility must be bit-stable at float64 precision
		scenarioCount = 6    // keystone, diamond, linear, recovery×3
	)

	// ── Run 0: establish reference ────────────────────────────────────────────
	hub0 := streaming.NewHub()
	replay0 := integration.NewReplayEngine(hub0)
	reference := replay0.ExecuteAllScenarios()

	if len(reference) != scenarioCount {
		t.Fatalf("L4-REP-001: expected %d scenarios from ExecuteAllScenarios(), got %d",
			scenarioCount, len(reference))
	}

	// Build reference map: ScenarioName → FinalFragility
	refFragility := make(map[string]float64, len(reference))
	refChecksum := make(map[string]string, len(reference))
	for _, cap := range reference {
		refFragility[cap.ScenarioName] = cap.FinalFragility
		// Checksum includes PressureHeatmap values too — catches map ordering issues.
		chk := checksumOf(map[string]interface{}{
			"fragility": cap.FinalFragility,
			"pressure":  cap.PressureHeatmap,
		})
		refChecksum[cap.ScenarioName] = chk
		t.Logf("L4-REP-001 [run 0] scenario=%q fragility=%.9f checksum=%s",
			cap.ScenarioName, cap.FinalFragility, chk[:12])
	}

	// ── Runs 1-19: verify identical output ───────────────────────────────────
	type deviation struct {
		runIdx       int
		scenario     string
		refFragility float64
		gotFragility float64
		delta        float64
	}
	var deviations []deviation

	for runIdx := 1; runIdx < runs; runIdx++ {
		hub := streaming.NewHub()
		replay := integration.NewReplayEngine(hub)
		captures := replay.ExecuteAllScenarios()

		if len(captures) != scenarioCount {
			t.Errorf("L4-REP-001 run %d: expected %d scenarios, got %d", runIdx, scenarioCount, len(captures))
			continue
		}

		for _, cap := range captures {
			ref, ok := refFragility[cap.ScenarioName]
			if !ok {
				t.Errorf("L4-REP-001 run %d: scenario %q not seen in run 0", runIdx, cap.ScenarioName)
				continue
			}
			delta := absFloat64(cap.FinalFragility - ref)
			if delta > epsilon {
				deviations = append(deviations, deviation{
					runIdx:       runIdx,
					scenario:     cap.ScenarioName,
					refFragility: ref,
					gotFragility: cap.FinalFragility,
					delta:        delta,
				})
			}

			// Also verify PressureHeatmap checksum is stable.
			chk := checksumOf(map[string]interface{}{
				"fragility": cap.FinalFragility,
				"pressure":  cap.PressureHeatmap,
			})
			if chk != refChecksum[cap.ScenarioName] {
				t.Errorf("L4-REP-001 run %d scenario=%q: PressureHeatmap checksum changed (run0=%s thisRun=%s)",
					runIdx, cap.ScenarioName, refChecksum[cap.ScenarioName][:12], chk[:12])
			}
		}
	}

	// ── Worst delta across all runs ───────────────────────────────────────────
	var worstDelta float64
	for _, d := range deviations {
		if d.delta > worstDelta {
			worstDelta = d.delta
		}
	}

	passed := len(deviations) == 0
	durationMs := time.Since(start).Milliseconds()

	// ── Write golden for other tests to consume ───────────────────────────────
	// Only written if not already present — preserves first-run as reference.
	if !goldenExists("replay_all_scenarios") {
		golden := make(map[string]float64, len(reference))
		for _, cap := range reference {
			golden[cap.ScenarioName] = cap.FinalFragility
		}
		if err := writeGoldenFile("replay_all_scenarios", golden); err != nil {
			t.Logf("L4-REP-001 WARNING: could not write golden file: %v", err)
		} else {
			t.Logf("L4-REP-001: golden file written to %s/replay_all_scenarios.json", goldenDir)
		}
	}

	// ── Build answer map for results ──────────────────────────────────────────
	actualMap := make(map[string]float64, len(refFragility))
	for k, v := range refFragility {
		actualMap[k] = v
	}

	var errMsgs []string
	for _, d := range deviations {
		errMsgs = append(errMsgs, fmt.Sprintf(
			"run=%d scenario=%q ref=%.9f got=%.9f delta=%.2e",
			d.runIdx, d.scenario, d.refFragility, d.gotFragility, d.delta,
		))
	}
	if len(errMsgs) == 0 {
		errMsgs = append(errMsgs, fmt.Sprintf(
			"%d runs × %d scenarios = %d checks all identical",
			runs, scenarioCount, runs*scenarioCount,
		))
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-001",
		Layer:  4,
		Name:   "ExecuteAllScenarios determinism across 20 runs",
		Aim: fmt.Sprintf(
			"ReplayEngine.ExecuteAllScenarios() must produce identical FinalFragility for all %d scenarios across %d runs",
			scenarioCount, runs,
		),
		PackagesInvolved: []string{"internal/integration", "internal/streaming"},
		FunctionsTested: []string{
			"integration.NewReplayEngine",
			"(*ReplayEngine).ExecuteAllScenarios",
			"integration.BuildKeystoneCollapseScenario",
			"integration.BuildDiamondStressScenario",
			"integration.BuildLinearCascadeScenario",
			"integration.BuildTriangleRecoveryScenario",
		},
		GoldenFile: goldenDir + "/replay_all_scenarios.json",
		Threshold: L4Threshold{
			Metric:    "max_fragility_deviation",
			Operator:  "<=",
			Value:     epsilon,
			Unit:      "dimensionless",
			Rationale: "ComputeTopologySensitivity is a pure function of GraphSnapshot — any deviation indicates non-determinism",
		},
		Result: L4ResultData{
			Status:            l4Status(passed),
			ActualValue:       worstDelta,
			ActualUnit:        "fragility_delta",
			RunsAttempted:     runs,
			RunsIdentical:     runs - len(deviations),
			FieldsChecked:     runs * scenarioCount,
			FieldsInTolerance: runs*scenarioCount - len(deviations),
			FieldsOutside:     len(deviations),
			WorstDeltaPct:     worstDelta * 100,
			DurationMs:        durationMs,
			ErrorMessages:     errMsgs,
		},
		OnExceed: "Non-deterministic sensitivity computation means replay golden files cannot be trusted — " +
			"CI will produce random pass/fail results for the same commit",
		Questions: L4Questions{
			WhatWasTested: fmt.Sprintf(
				"ExecuteAllScenarios() called %d times; FinalFragility and PressureHeatmap checksum compared per scenario per run",
				runs,
			),
			WhyThisThreshold:     "1e-9 is float64 arithmetic noise floor — any larger deviation is a real non-determinism bug, not precision",
			WhatHappensIfFails:   "Golden file written from run 0 will mismatch run 1 — regression tests produce false alarms every CI build",
			HowDeterminismVerified: "SHA256 of (fragility, pressure_heatmap) per scenario compared across all runs",
			IsGoldenFileFrozen:   "Written on first run if absent; never overwritten automatically",
			HowToUpdateGolden:    "Delete tests/layer4_scenario_replay/golden/replay_all_scenarios.json and re-run",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L4-REP-001 FAILED: %d deviations across %d runs. Worst delta=%.2e (threshold=1e-9)\n"+
				"First violation: %v\n"+
				"FIX: Find where ComputeTopologySensitivity iterates a map without a deterministic ordering.\n"+
				"     Look for range over map[string]* in internal/modelling/topology_sensitivity.go\n"+
				"     Replace with sorted key iteration using sort.Strings(keys).",
			len(deviations), runs, worstDelta, func() string {
				if len(deviations) > 0 {
					return fmt.Sprintf("run=%d scenario=%q delta=%.2e", deviations[0].runIdx, deviations[0].scenario, deviations[0].delta)
				}
				return ""
			}(),
		)
	}

	t.Logf("L4-REP-001 PASS | %d runs × %d scenarios, all identical | worst_delta=%.2e",
		runs, scenarioCount, worstDelta)
}