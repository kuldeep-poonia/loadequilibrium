package layer4

// FILE: tests/layer4_scenario_replay/L4_REP_003_sensitivity_stability_test.go
//
// Tests:   L4-REP-003, L4-REP-003b
// Package: github.com/loadequilibrium/loadequilibrium/internal/modelling
//          github.com/loadequilibrium/loadequilibrium/internal/integration
// Real functions used:
//   modelling.ComputeTopologySensitivity(snap topology.GraphSnapshot) TopologySensitivity
//   TopologySensitivity.SystemFragility       float64
//   TopologySensitivity.ByService             map[string]ServiceSensitivity
//   ServiceSensitivity.PerturbationScore      float64
//   ServiceSensitivity.IsKeystone             bool
//   ServiceSensitivity.DownstreamReach        int
//   ServiceSensitivity.UpstreamExposure       int
//   integration.NewReplayEngine(hub) *ReplayEngine
//   (*ReplayEngine).InjectTopologySnapshot(snap, durationMs) ReplayCapture
//   ReplayCapture.FinalFragility              float64
//   ReplayCapture.PressureHeatmap             map[string]float64
//   ReplayCapture.EdgeWeightChanges           map[string]EdgeWeightChange
//   EdgeWeightChange.WeightAfter              float64
//   EdgeWeightChange.WeightBefore             float64
//
// RUN: go test ./tests/layer4_scenario_replay/ -run TestL4_REP_003 -count=1 -timeout=300s -v

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
// L4-REP-003 — ComputeTopologySensitivity is stable across 100 identical calls
//
// AIM:   Call ComputeTopologySensitivity 100 times on the same GraphSnapshot.
//        Every call must return identical SystemFragility and per-service
//        PerturbationScore. This is a stronger check than L4-REP-001 because
//        it exercises a single function in complete isolation.
//
// THRESHOLD: zero deviations across 100 calls (max delta == 0)
// ON EXCEED: The function is not a pure function — it has hidden mutable state
//            (package-level variable, random number, time.Now() dependency).
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_003_SensitivityComputationStable100Calls(t *testing.T) {
	start := time.Now()

	const runs = 100
	const epsilon = 1e-12 // stricter than L4-REP-001 — single isolated function

	// Use the keystone collapse topology — 7 nodes, 6 edges, richest sensitivity signal.
	keystone := integration.BuildKeystoneCollapseScenario()
	snap := topology.GraphSnapshot{
		CapturedAt:   time.Now(), // fixed — should not affect computation
		Nodes:        keystone.Nodes,
		Edges:        keystone.Edges,
		CriticalPath: topology.CriticalPath{Nodes: []string{keystone.ExpectedCritical}},
	}

	// Run 0: capture reference.
	ref := modelling.ComputeTopologySensitivity(snap)
	refFragility := ref.SystemFragility
	refAmpScore := ref.MaxAmplificationScore
	refByService := make(map[string]float64, len(ref.ByService))
	for svcID, ss := range ref.ByService {
		refByService[svcID] = ss.PerturbationScore
	}

	t.Logf("L4-REP-003 [run 0] fragility=%.12f max_amp=%.12f services=%d",
		refFragility, refAmpScore, len(refByService))

	type deviation struct {
		runIdx int
		field  string
		ref    float64
		got    float64
		delta  float64
	}
	var deviations []deviation

	for runIdx := 1; runIdx < runs; runIdx++ {
		// Re-use the same snapshot — CapturedAt is intentionally stable.
		s := modelling.ComputeTopologySensitivity(snap)

		// Check SystemFragility.
		if d := math.Abs(s.SystemFragility - refFragility); d > epsilon {
			deviations = append(deviations, deviation{runIdx, "SystemFragility", refFragility, s.SystemFragility, d})
		}
		// Check MaxAmplificationScore.
		if d := math.Abs(s.MaxAmplificationScore - refAmpScore); d > epsilon {
			deviations = append(deviations, deviation{runIdx, "MaxAmplificationScore", refAmpScore, s.MaxAmplificationScore, d})
		}
		// Check every per-service PerturbationScore.
		for svcID, refScore := range refByService {
			ss, ok := s.ByService[svcID]
			if !ok {
				deviations = append(deviations, deviation{runIdx, "service." + svcID + ".present", 1, 0, 1})
				continue
			}
			if d := math.Abs(ss.PerturbationScore - refScore); d > epsilon {
				deviations = append(deviations, deviation{runIdx, "service." + svcID + ".PerturbationScore", refScore, ss.PerturbationScore, d})
			}
		}
	}

	var worstDelta float64
	for _, d := range deviations {
		if d.delta > worstDelta {
			worstDelta = d.delta
		}
	}

	passed := len(deviations) == 0
	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	if len(deviations) > 0 {
		for _, d := range deviations {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"run=%d field=%s ref=%.12f got=%.12f delta=%.2e",
				d.runIdx, d.field, d.ref, d.got, d.delta,
			))
		}
	} else {
		errMsgs = append(errMsgs, fmt.Sprintf(
			"%d calls all produced identical output (worst_delta=0)", runs,
		))
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-003",
		Layer:  4,
		Name:   "ComputeTopologySensitivity stability across 100 identical calls",
		Aim: fmt.Sprintf(
			"ComputeTopologySensitivity called %d times on identical GraphSnapshot must produce zero deviations",
			runs,
		),
		PackagesInvolved: []string{"internal/modelling", "internal/integration"},
		FunctionsTested:  []string{"modelling.ComputeTopologySensitivity"},
		GoldenFile:       "N/A — comparison is internal across runs",
		Threshold: L4Threshold{
			Metric:    "max_output_deviation",
			Operator:  "==",
			Value:     0,
			Unit:      "dimensionless",
			Rationale: "ComputeTopologySensitivity takes no time/random inputs — it must be a pure function",
		},
		Result: L4ResultData{
			Status:        l4Status(passed),
			ActualValue:   worstDelta,
			ActualUnit:    "fragility_delta",
			RunsAttempted: runs,
			RunsIdentical: runs - len(deviations),
			FieldsOutside: len(deviations),
			WorstDeltaPct: worstDelta * 100,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "ComputeTopologySensitivity has hidden state (global variable, map iteration order, time.Now()) " +
			"→ dashboard shows different fragility scores on each orchestrator tick for the same topology",
		Questions: L4Questions{
			WhatWasTested: fmt.Sprintf(
				"ComputeTopologySensitivity called %d times on same GraphSnapshot (keystone collapse: 7 nodes, 6 edges)",
				runs,
			),
			WhyThisThreshold:     "1e-12 is below float64 arithmetic rounding — any deviation is a state side-effect, not precision",
			WhatHappensIfFails:   "Orchestrator tick produces different fragility values on consecutive calls → dashboard flickers → operator can't trust readings",
			HowDeterminismVerified: "Direct float64 comparison with 1e-12 epsilon across 100 calls",
			IsGoldenFileFrozen:   "N/A",
			HowToUpdateGolden:    "N/A",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L4-REP-003 FAILED: %d deviations across %d calls. Worst delta=%.2e (threshold=1e-12)\n"+
				"First: %v\n"+
				"FIX: Remove any mutable state from ComputeTopologySensitivity in internal/modelling/topology_sensitivity.go.\n"+
				"     Common causes: map iteration used as ordered computation, package-level cache with write-without-lock,\n"+
				"     or runtime.NumCPU()-dependent parallelism that produces different summation order.",
			len(deviations), runs, worstDelta,
			func() string {
				if len(deviations) > 0 {
					d := deviations[0]
					return fmt.Sprintf("run=%d field=%s delta=%.2e", d.runIdx, d.field, d.delta)
				}
				return ""
			}(),
		)
	}
	t.Logf("L4-REP-003 PASS | %d calls, all identical | worst_delta=0", runs)
}

// ─────────────────────────────────────────────────────────────────────────────
// L4-REP-003b — InjectTopologySnapshot capture fidelity
//
// AIM:   After InjectTopologySnapshot(snap, durationMs), the returned
//        ReplayCapture.FinalFragility must equal the value returned by
//        ComputeTopologySensitivity(snap).SystemFragility.
//        The ReplayCapture.PressureHeatmap must contain every service ID
//        present in snap.Nodes with a valid float64 value in [0, ∞).
//        The ReplayCapture.EdgeWeightChanges must contain one entry per edge.
//
// THRESHOLD: fragility_delta == 0, missing_services == 0, missing_edges == 0
// ON EXCEED: InjectTopologySnapshot returns a capture that does not faithfully
//            reflect the injected topology — replay tests would compare against
//            wrong reference values.
// ─────────────────────────────────────────────────────────────────────────────
func TestL4_REP_003b_InjectTopologySnapshotCaptureFidelity(t *testing.T) {
	start := time.Now()

	scenarios := []struct {
		name     string
		scenario integration.TopologyReplayScenario
	}{
		{"keystone_collapse", integration.BuildKeystoneCollapseScenario()},
		{"diamond_stress", integration.BuildDiamondStressScenario()},
		{"linear_cascade", integration.BuildLinearCascadeScenario()},
		{"recovery_step_3", integration.BuildTriangleRecoveryScenario(3)},
	}

	hub := streaming.NewHub()
	replay := integration.NewReplayEngine(hub)

	type checkResult struct {
		name           string
		fragilityDelta float64
		missingServices []string
		missingEdges    []string
		invalidPressure []string
		passed         bool
	}

	allChecks := make([]checkResult, 0, len(scenarios))
	overallPassed := true

	for _, s := range scenarios {
		snap := topology.GraphSnapshot{
			CapturedAt:   time.Now(),
			Nodes:        s.scenario.Nodes,
			Edges:        s.scenario.Edges,
			CriticalPath: topology.CriticalPath{Nodes: []string{s.scenario.ExpectedCritical}},
		}

		// Ground truth: direct computation.
		expectedSens := modelling.ComputeTopologySensitivity(snap)
		expectedFragility := expectedSens.SystemFragility

		// Call InjectTopologySnapshot — this is the function under test.
		capture := replay.InjectTopologySnapshot(snap, 3000)

		cr := checkResult{name: s.name}

		// Check 1: FinalFragility must match direct computation exactly.
		cr.fragilityDelta = math.Abs(capture.FinalFragility - expectedFragility)

		// Check 2: PressureHeatmap must contain every node.
		for _, node := range s.scenario.Nodes {
			if _, ok := capture.PressureHeatmap[node.ServiceID]; !ok {
				cr.missingServices = append(cr.missingServices, node.ServiceID)
			}
		}
		// Check 3: All pressure values must be finite and non-negative.
		for svcID, pressure := range capture.PressureHeatmap {
			if math.IsNaN(pressure) || math.IsInf(pressure, 0) || pressure < 0 {
				cr.invalidPressure = append(cr.invalidPressure, fmt.Sprintf("%s=%.4f", svcID, pressure))
			}
		}
		// Check 4: EdgeWeightChanges must contain one entry per edge.
		for _, edge := range s.scenario.Edges {
			edgeKey := fmt.Sprintf("%s→%s", edge.Source, edge.Target)
			if _, ok := capture.EdgeWeightChanges[edgeKey]; !ok {
				cr.missingEdges = append(cr.missingEdges, edgeKey)
			}
		}

		cr.passed = cr.fragilityDelta < 1e-9 &&
			len(cr.missingServices) == 0 &&
			len(cr.missingEdges) == 0 &&
			len(cr.invalidPressure) == 0

		if !cr.passed {
			overallPassed = false
		}

		status := "PASS"
		if !cr.passed {
			status = "FAIL"
		}
		t.Logf("L4-REP-003b [%s]: %s | fragility_delta=%.2e missing_svc=%d missing_edges=%d bad_pressure=%d",
			s.name, status, cr.fragilityDelta,
			len(cr.missingServices), len(cr.missingEdges), len(cr.invalidPressure),
		)

		allChecks = append(allChecks, cr)
	}

	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	for _, c := range allChecks {
		if !c.passed {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: fragility_delta=%.2e missing_svc=%v missing_edges=%v bad_pressure=%v",
				c.name, c.fragilityDelta, c.missingServices, c.missingEdges, c.invalidPressure))
		}
	}
	if len(errMsgs) == 0 {
		errMsgs = append(errMsgs, fmt.Sprintf("all %d scenarios: capture fidelity verified", len(scenarios)))
	}

	// Worst fragility delta across scenarios.
	var worstFragilityDelta float64
	for _, c := range allChecks {
		if c.fragilityDelta > worstFragilityDelta {
			worstFragilityDelta = c.fragilityDelta
		}
	}

	writeL4Result(L4Record{
		TestID: "L4-REP-003b",
		Layer:  4,
		Name:   "InjectTopologySnapshot capture fidelity",
		Aim: "ReplayCapture.FinalFragility must equal ComputeTopologySensitivity output; " +
			"PressureHeatmap must contain all nodes with valid values; EdgeWeightChanges must cover all edges",
		PackagesInvolved: []string{"internal/integration", "internal/modelling"},
		FunctionsTested: []string{
			"(*ReplayEngine).InjectTopologySnapshot",
			"ReplayCapture.FinalFragility",
			"ReplayCapture.PressureHeatmap",
			"ReplayCapture.EdgeWeightChanges",
		},
		GoldenFile: "N/A",
		Threshold: L4Threshold{
			Metric:    "fragility_delta",
			Operator:  "<",
			Value:     1e-9,
			Unit:      "dimensionless",
			Rationale: "InjectTopologySnapshot must faithfully report ComputeTopologySensitivity output",
		},
		Result: L4ResultData{
			Status:        l4Status(overallPassed),
			ActualValue:   worstFragilityDelta,
			ActualUnit:    "fragility_delta",
			RunsAttempted: len(scenarios),
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Layer 4 regression tests compare against ReplayCapture values — if those values do not " +
			"faithfully reflect the topology, golden files are built on wrong reference data",
		Questions: L4Questions{
			WhatWasTested: fmt.Sprintf(
				"InjectTopologySnapshot on %d scenarios; capture fields compared to direct ComputeTopologySensitivity output",
				len(scenarios),
			),
			WhyThisThreshold:     "1e-9 — the values should be identical since InjectTopologySnapshot calls ComputeTopologySensitivity internally",
			WhatHappensIfFails:   "Golden files written from wrong values — regression detection broken from the start",
			HowDeterminismVerified: "Direct comparison of capture.FinalFragility against ComputeTopologySensitivity(same snap)",
			IsGoldenFileFrozen:   "N/A",
			HowToUpdateGolden:    "N/A",
		},
		RunAt:     l4Now(),
		GoVersion: l4GoVer(),
	})

	if !overallPassed {
		t.Fatalf(
			"L4-REP-003b FAILED: capture fidelity issues detected.\n%v\n"+
				"FIX: InjectTopologySnapshot in internal/integration/replay.go must set\n"+
				"     capture.FinalFragility = sens.SystemFragility directly from ComputeTopologySensitivity output.\n"+
				"     PressureHeatmap must include ALL node IDs, not just those with high scores.\n"+
				"     EdgeWeightChanges must be keyed as 'Source→Target' for every edge in the snapshot.",
			errMsgs,
		)
	}
	t.Logf("L4-REP-003b PASS | %d scenarios, all captures faithful", len(scenarios))
}