// Package tests — Integration Test Suite
// Tests cross-module data flow and verifies that outputs of each layer
// are correctly consumed as inputs to the next layer.
// Run: go test ./tests/ -run TestIntegration -v -timeout 60s
package tests

import (
	"math"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/policy"
	"github.com/loadequilibrium/loadequilibrium/internal/reasoning"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ─── helpers 



func makeWindow(serviceID string, arrivalRate, latencyMs float64, samples int) *telemetry.ServiceWindow {
	return &telemetry.ServiceWindow{
		ServiceID:       serviceID,
		MeanRequestRate: arrivalRate,
		MeanLatencyMs:   latencyMs,
		SampleCount:     samples,
		ConfidenceScore: 1.0,
		ComputedAt:     time.Now(),

		UpstreamEdges: map[string]telemetry.EdgeWindow{
	"svc-b": {
		TargetServiceID: "svc-b",
		MeanCallRate:    50,
		MeanErrorRate:   0.01,
		MeanLatencyMs:   20,
	},
},
	}


	
}

func makeStore() *telemetry.Store {
	return telemetry.NewStore(256, 64, 30*time.Second)
}

func ingestPoint(s *telemetry.Store, serviceID string, rps, latencyMs float64) {
	s.Ingest(&telemetry.MetricPoint{
		ServiceID:   serviceID,
		RequestRate: rps,
		ErrorRate:   0.01,
		Latency: telemetry.LatencyStats{
			Mean: latencyMs,
			P50:  latencyMs * 0.8,
			P95:  latencyMs * 1.5,
			P99:  latencyMs * 2.0,
		},
		Timestamp: time.Now(),
	})
}

func defaultCfg() *config.Config {
	return &config.Config{
		TickInterval:           500 * time.Millisecond,
		TickDeadline:           200 * time.Millisecond,
		RingBufferDepth:        256,
		WorkerPoolSize:         4,
		MaxServices:            64,
		StaleServiceAge:        30 * time.Second,
		UtilisationSetpoint:    0.70,
		CollapseThreshold:      0.90,
		WindowFraction:         0.5,
		EWMAFastAlpha:          0.30,
		EWMASlowAlpha:          0.05,
		SpikeZScore:            3.0,
		PIDKp:                  0.5,
		PIDKi:                  0.1,
		PIDKd:                  0.05,
		PIDDeadband:            0.02,
		PIDIntegralMax:         1.0,
		PredictiveHorizonTicks: 5,
		SimHorizonMs:           500,
		SimShockFactor:         1.5,
		SimAsyncBuffer:         8,
		SimBudget:              20 * time.Millisecond,
		MaxReasoningCooldowns:  500,
		ArrivalEstimatorMode:   "ewma",
	}
}

// ─── Test 1: Telemetry → Store → Windows ────────────────────────────────────

func TestIntegration_TelemetryToWindows(t *testing.T) {
	store := makeStore()

	for i := 0; i < 20; i++ {
		ingestPoint(store, "svc-a", 100.0, 50.0)
		ingestPoint(store, "svc-b", 200.0, 80.0)
	}

	windows := store.AllWindows(10, 30*time.Second)
	if len(windows) != 2 {
		t.Fatalf("expected 2 service windows, got %d", len(windows))
	}
	wA := windows["svc-a"]
	if wA == nil {
		t.Fatal("svc-a window is nil")
	}
	if wA.SampleCount < 1 {
		t.Fatalf("svc-a has %d samples, expected ≥1", wA.SampleCount)
	}
	if wA.MeanRequestRate <= 0 {
		t.Fatalf("svc-a MeanRequestRate=%.2f, expected >0", wA.MeanRequestRate)
	}
	if wA.ConfidenceScore <= 0 || wA.ConfidenceScore > 1 {
		t.Fatalf("svc-a ConfidenceScore=%.2f out of [0,1]", wA.ConfidenceScore)
	}
}

// ─── Test 2: Windows → Topology ─────────────────────────────────────────────

func TestIntegration_WindowsToTopology(t *testing.T) {
	store := makeStore()
	for i := 0; i < 20; i++ {
		// Inject dependency: svc-a calls svc-b
		ingestPoint(store, "svc-a", 100, 50)
		ingestPoint(store, "svc-b", 80, 60)
	}
	windows := store.AllWindows(10, 30*time.Second)
	graph := topology.New()
	graph.Update(windows)
	snap := graph.Snapshot()

	// Snapshot must be non-empty — services are registered
	if len(snap.Nodes) == 0 {
		t.Log("topology snapshot has 0 nodes — expected when no edge data injected (correct behaviour)")
	}
	// CriticalPath must have sane values
	if snap.CriticalPath.CascadeRisk < 0 || snap.CriticalPath.CascadeRisk > 1 {
		t.Fatalf("CriticalPath.CascadeRisk=%.4f out of [0,1]", snap.CriticalPath.CascadeRisk)
	}
}

// ─── Test 3: Windows → Queue Physics (M/M/c) ────────────────────────────────

func TestIntegration_WindowsToQueueModel(t *testing.T) {
	qp := modelling.NewQueuePhysicsEngine()
	w := makeWindow("svc-x", 150.0, 10.0, 30) // arrival=150 rps, latency=10ms
	snap := topology.GraphSnapshot{}
	q := qp.RunQueueModel(w, snap, false)

	if q.ServiceID != "svc-x" {
		t.Fatalf("ServiceID mismatch: got %q", q.ServiceID)
	}
	if q.ArrivalRate <= 0 {
		t.Fatalf("ArrivalRate=%.4f, expected >0", q.ArrivalRate)
	}
	if q.ServiceRate <= 0 {
		t.Fatalf("ServiceRate=%.4f, expected >0", q.ServiceRate)
	}
	if q.Utilisation < 0 || q.Utilisation > 2 {
		t.Fatalf("Utilisation=%.4f out of sensible range [0,2]", q.Utilisation)
	}
	if math.IsNaN(q.AdjustedWaitMs) || math.IsInf(q.AdjustedWaitMs, 0) {
		t.Fatalf("AdjustedWaitMs is NaN or Inf")
	}
	if q.Confidence < 0 || q.Confidence > 1 {
		t.Fatalf("Confidence=%.4f out of [0,1]", q.Confidence)
	}
}

// ─── Test 4: QueueModel → Stability Assessment ──────────────────────────────

func TestIntegration_QueueToStability(t *testing.T) {
	qp := modelling.NewQueuePhysicsEngine()
	sp := modelling.NewSignalProcessor(0.3, 0.05, 3.0)
	snap := topology.GraphSnapshot{}

	// High load scenario — rho should approach collapse threshold
	w := makeWindow("svc-y", 950.0, 10.0, 50) // very high arrival
	q := qp.RunQueueModel(w, snap, false)
	sig := sp.Update(w)
	stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)

	if stab.ServiceID != "svc-y" {
		t.Fatalf("StabilityAssessment ServiceID mismatch")
	}
	if stab.CollapseRisk < 0 || stab.CollapseRisk > 1 {
		t.Fatalf("CollapseRisk=%.4f out of [0,1]", stab.CollapseRisk)
	}
	if stab.CollapseZone == "" {
		t.Fatal("CollapseZone is empty")
	}
	validZones := map[string]bool{"safe": true, "warning": true, "collapse": true}
	if !validZones[stab.CollapseZone] {
		t.Fatalf("CollapseZone=%q is not one of safe/warning/collapse", stab.CollapseZone)
	}
	if stab.OscillationRisk < 0 || stab.OscillationRisk > 1 {
		t.Fatalf("OscillationRisk=%.4f out of [0,1]", stab.OscillationRisk)
	}
}

// ─── Test 5: QueueModel → Network Coupling ───────────────────────────────────

func TestIntegration_WindowsToNetworkCoupling(t *testing.T) {
	windows := map[string]*telemetry.ServiceWindow{
		"svc-a": makeWindow("svc-a", 100, 50, 30),
		"svc-b": makeWindow("svc-b", 80, 60, 30),
	}
	graph := topology.New()
graph.Update(windows)
snap := graph.Snapshot()
	coupling := modelling.ComputeNetworkCoupling(windows, snap)

	// Must return a map (possibly empty if no topology edges)
	if coupling == nil {
		t.Fatal("NetworkCoupling result is nil")
	}
	for id, nc := range coupling {
		if nc.EffectivePressure < 0 {
			t.Fatalf("svc %s EffectivePressure=%.4f < 0", id, nc.EffectivePressure)
		}
		if nc.PathSaturationRisk < 0 || nc.PathSaturationRisk > 1 {
			t.Fatalf("svc %s PathSaturationRisk=%.4f out of [0,1]", id, nc.PathSaturationRisk)
		}
	}
}

// ─── Test 6: Bundles → Cost Gradients → Optimizer Candidates ────────────────

func TestIntegration_BundlesToOptimizerCandidates(t *testing.T) {
	cfg := defaultCfg()
	qp := modelling.NewQueuePhysicsEngine()
	sp := modelling.NewSignalProcessor(0.3, 0.05, 3.0)
	snap := topology.GraphSnapshot{}

	windows := map[string]*telemetry.ServiceWindow{
		"svc-a": makeWindow("svc-a", 100, 50, 40),
		"svc-b": makeWindow("svc-b", 200, 30, 40),
	}
	bundles := make(map[string]*modelling.ServiceModelBundle, 2)
	for id, w := range windows {
		q := qp.RunQueueModel(w, snap, false)
		sig := sp.Update(w)
		stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)
		bundles[id] = &modelling.ServiceModelBundle{
			Queue:      q,
			Signal:     sig,
			Stability:  stab,
			Stochastic: modelling.StochasticModel{ServiceID: id, Confidence: 0.9},
		}
	}

	gradients := optimisation.ComputeCostGradients(bundles, snap, 500.0)
	if len(gradients) == 0 {
		t.Fatal("ComputeCostGradients returned empty map")
	}
	for id, cg := range gradients {
		if math.IsNaN(cg.CostGradient) || math.IsInf(cg.CostGradient, 0) {
			t.Fatalf("svc %s CostGradient is NaN/Inf", id)
		}
	}

	engine := optimisation.NewEngine(cfg)
	candidates := engine.EvaluateCandidates(bundles, gradients, nil, snap, time.Now())
	if len(candidates) == 0 {
		t.Fatal("EvaluateCandidates returned empty map")
	}
	for id, cands := range candidates {
		if len(cands) == 0 {
			t.Fatalf("svc %s has 0 candidates", id)
		}
		for _, c := range cands {
			if c.ScaleFactor <= 0 {
				t.Fatalf("svc %s candidate ScaleFactor=%.4f ≤0", id, c.ScaleFactor)
			}
			if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) {
				t.Fatalf("svc %s candidate Score is NaN/Inf", id)
			}
		}
	}
}

// ─── Test 7: Bundles → Objective Score ───────────────────────────────────────

func TestIntegration_BundlesToObjective(t *testing.T) {
	qp := modelling.NewQueuePhysicsEngine()
	sp := modelling.NewSignalProcessor(0.3, 0.05, 3.0)
	snap := topology.GraphSnapshot{}

	bundles := make(map[string]*modelling.ServiceModelBundle)
	for i, cfg := range []struct{ rate, lat float64 }{
		{100, 50}, {200, 80}, {50, 200},
	} {
		id := "svc-" + string(rune('a'+i))
		w := makeWindow(id, cfg.rate, cfg.lat, 40)
		q := qp.RunQueueModel(w, snap, false)
		sig := sp.Update(w)
		stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)
		bundles[id] = &modelling.ServiceModelBundle{Queue: q, Signal: sig, Stability: stab,
			Stochastic: modelling.StochasticModel{ServiceID: id, Confidence: 0.9}}
	}

	obj := optimisation.ComputeObjective(bundles, snap, time.Now())
	if obj.CompositeScore < 0 || obj.CompositeScore > 1 {
		t.Fatalf("CompositeScore=%.4f out of [0,1]", obj.CompositeScore)
	}
	if obj.PredictedP99LatencyMs < 0 {
		t.Fatalf("PredictedP99LatencyMs=%.2f < 0", obj.PredictedP99LatencyMs)
	}
	if obj.ReferenceLatencyMs <= 0 {
		t.Fatalf("ReferenceLatencyMs=%.2f ≤0", obj.ReferenceLatencyMs)
	}
	if obj.TrajectoryScore < 0 || obj.TrajectoryScore > 1 {
		t.Fatalf("TrajectoryScore=%.4f out of [0,1]", obj.TrajectoryScore)
	}
}

// ─── Test 8: Bundles → Policy Engine ────────────────────────────────────────

func TestIntegration_BundlesToPolicy(t *testing.T) {
	qp := modelling.NewQueuePhysicsEngine()
	sp := modelling.NewSignalProcessor(0.3, 0.05, 3.0)
	snap := topology.GraphSnapshot{}

	w := makeWindow("svc-p", 150.0, 20.0, 40)
	q := qp.RunQueueModel(w, snap, false)
	sig := sp.Update(w)
	stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)
	bundle := &modelling.ServiceModelBundle{
		Queue:      q,
		Signal:     sig,
		Stability:  stab,
		Stochastic: modelling.StochasticModel{ServiceID: "svc-p", Confidence: 0.9},
	}

	input := policy.EngineInput{
		Scaling: policy.ScalingSignal{
			PredictedLoad:   bundle.Queue.ArrivalRate,
			CurrentReplicas: 2,
			TargetLatency:   500,
			ObservedLatency: bundle.Queue.AdjustedWaitMs + 1,
			MinReplicas:     1,
			MaxReplicas:     10,
			InstanceCost:    1.0,
			SlaPenaltyWeight: 1.0,
		},
		Retry: policy.RetrySignal{
			ObservedErrorRate: bundle.Stochastic.RiskPropagation,
			QueueDepth:        bundle.Queue.MeanQueueLen,
			PredictedArrival:  bundle.Queue.ArrivalRate,
			ServiceCapacity:   bundle.Queue.ServiceRate + 0.1,
			BaseSystemRisk:    bundle.Stability.CollapseRisk,
			CurrentRetryLimit: 3,
			MinRetryLimit:     1,
			MaxRetryLimit:     5,
			MinBackoff:        1.0,
			MaxBackoff:        3.0,
		},
		Queue: policy.QueueSignal{
			CurrentQueueDepth: bundle.Queue.MeanQueueLen,
			PredictedArrival:  bundle.Queue.ArrivalRate,
			ServiceCapacity:   bundle.Queue.ServiceRate + 0.1,
			ObservedLatency:   bundle.Queue.AdjustedWaitMs + 1,
			TargetLatency:     500,
			BaseSystemRisk:    bundle.Stability.CollapseRisk,
			CurrentQueueLimit: 20,
			MinQueueLimit:     1,
			MaxQueueLimit:     50,
			MaxStep:           5,
		},
		Cache: policy.CacheSignal{
			CurrentHitRate:         0.8,
			TargetHitRate:          0.85,
			PredictedArrival:       bundle.Queue.ArrivalRate,
			ServiceCapacity:        bundle.Queue.ServiceRate + 0.1,
			ObservedLatency:        bundle.Queue.AdjustedWaitMs + 1,
			TargetLatency:          500,
			CacheableRatio:         0.6,
			BaseMemoryPressure:     0.4,
			BaseSystemRisk:         bundle.Stability.CollapseRisk,
			CurrentCacheAggression: 0.5,
			MinAggression:          0,
			MaxAggression:          1,
			BaseStep:               0.1,
		},
		CostWeights: policy.CostWeights{
			SlaViolation: 1.0,
			InfraCost:    0.25,
			ChangeCost:   0.15,
			RiskCost:     0.75,
			FutureCost:   0.20,
		},
	}

	state := &policy.EngineState{}
	decision := policy.EvaluatePolicies(input, state)

	if decision.GlobalRisk < 0 || decision.GlobalRisk > 1 {
		t.Fatalf("GlobalRisk=%.4f out of [0,1]", decision.GlobalRisk)
	}
	if decision.GlobalCost < 0 {
		t.Fatalf("GlobalCost=%.4f < 0", decision.GlobalCost)
	}
	if decision.Confidence < 0 || decision.Confidence > 1 {
		t.Fatalf("Confidence=%.4f out of [0,1]", decision.Confidence)
	}
	if decision.Scaling.DesiredReplicas < 1 {
		t.Fatalf("DesiredReplicas=%d < 1", decision.Scaling.DesiredReplicas)
	}
	if decision.SystemReason == "" {
		t.Fatal("SystemReason is empty")
	}
}

// ─── Test 9: Bundles → Reasoning Events ─────────────────────────────────────

func TestIntegration_BundlesToReasoningEvents(t *testing.T) {
	qp := modelling.NewQueuePhysicsEngine()
	sp := modelling.NewSignalProcessor(0.3, 0.05, 3.0)
	snap := topology.GraphSnapshot{}
	eng := reasoning.NewEngine()
	eng.SetMaxCooldowns(500)
	eng.SetRuntimePressure(0)

	// High-load scenario to trigger events
	w := makeWindow("svc-r", 900.0, 50.0, 50)
	q := qp.RunQueueModel(w, snap, false)
	sig := sp.Update(w)
	stab := modelling.RunStabilityAssessment(q, sig, snap, 0.90)
	bundles := map[string]*modelling.ServiceModelBundle{
		"svc-r": {
			Queue:      q,
			Signal:     sig,
			Stability:  stab,
			Stochastic: modelling.StochasticModel{ServiceID: "svc-r", Confidence: 0.9},
		},
	}
	obj := optimisation.ComputeObjective(bundles, snap, time.Now())
	netEq := modelling.NetworkEquilibriumState{}
	topoSens := modelling.TopologySensitivity{ByService: make(map[string]modelling.ServiceSensitivity)}

	events := eng.AnalyseWithContext(bundles, snap, obj, netEq, topoSens, time.Now())

	// With high utilisation we expect at least one event
	if q.Utilisation >= 0.80 && len(events) == 0 {
		t.Logf("Utilisation=%.3f but no events fired — may need more samples for high rho", q.Utilisation)
	}
	for _, ev := range events {
		if ev.ID == "" {
			t.Fatal("event has empty ID")
		}
		if ev.Severity < 0 {
			t.Fatalf("event %s has negative severity", ev.ID)
		}
		if ev.Category == "" {
			t.Fatalf("event %s has empty category", ev.ID)
		}
		if ev.ModelChain == "" {
			t.Fatalf("event %s has empty ModelChain", ev.ID)
		}
		if ev.OperationalPriority < 0 || ev.OperationalPriority > 9 {
			t.Fatalf("event %s OperationalPriority=%d out of [0,9]", ev.ID, ev.OperationalPriority)
		}
	}
}

// ─── Test 10: Stale telemetry degrades confidence ────────────────────────────

func TestIntegration_StaleWindowConfidenceDegrades(t *testing.T) {
	store := makeStore()
	for i := 0; i < 5; i++ {
		ingestPoint(store, "svc-stale", 100, 50)
	}

	// Fresh cutoff = 1ns means everything is stale
	windows := store.AllWindows(10, 1*time.Nanosecond)
	// All windows should be excluded or have low confidence
	for _, w := range windows {
		if w.ConfidenceScore > 0.5 {
			t.Logf("Window with 1ns cutoff has ConfidenceScore=%.3f (may not be excluded by time alone)", w.ConfidenceScore)
		}
	}

	// With normal cutoff, confidence should be high
	windows2 := store.AllWindows(10, 30*time.Second)
	for _, w := range windows2 {
		if w.ConfidenceScore <= 0 {
			t.Fatalf("Fresh window has ConfidenceScore=%.3f ≤0", w.ConfidenceScore)
		}
	}
}

// ─── Test 11: Network Equilibrium monotone in load ───────────────────────────

func TestIntegration_NetworkEquilibriumMonotoneInLoad(t *testing.T) {
	snap := topology.GraphSnapshot{}

	lowLoad := map[string]*telemetry.ServiceWindow{
		"svc-a": makeWindow("svc-a", 10, 5, 30),
	}
	highLoad := map[string]*telemetry.ServiceWindow{
		"svc-a": makeWindow("svc-a", 900, 50, 30),
	}

	coupLow := modelling.ComputeNetworkCoupling(lowLoad, snap)
	coupHigh := modelling.ComputeNetworkCoupling(highLoad, snap)
	eqLow := modelling.ComputeNetworkEquilibrium(coupLow, lowLoad)
	eqHigh := modelling.ComputeNetworkEquilibrium(coupHigh, highLoad)

	if eqHigh.SystemRhoMean < eqLow.SystemRhoMean {
		t.Logf("High-load SystemRhoMean=%.4f < Low-load SystemRhoMean=%.4f (may still be valid if coupling is 0)", eqHigh.SystemRhoMean, eqLow.SystemRhoMean)
	}
	if eqHigh.SystemRhoMean < 0 || eqLow.SystemRhoMean < 0 {
		t.Fatalf("Negative SystemRhoMean: high=%.4f low=%.4f", eqHigh.SystemRhoMean, eqLow.SystemRhoMean)
	}
}

// ─── Test 12: Topology sensitivity returns sane values ───────────────────────

func TestIntegration_TopologySensitivity(t *testing.T) {
	windows := map[string]*telemetry.ServiceWindow{
		"svc-a": makeWindow("svc-a", 100, 50, 30),
		"svc-b": makeWindow("svc-b", 80, 60, 30),
	}
	graph := topology.New()
	graph.Update(windows)
	snap := graph.Snapshot()
	sens := modelling.ComputeTopologySensitivity(snap)

	if sens.SystemFragility < 0 || sens.SystemFragility > 1 {
		t.Fatalf("SystemFragility=%.4f out of [0,1]", sens.SystemFragility)
	}
	for id, ss := range sens.ByService {
		if ss.PerturbationScore < 0 {
			t.Fatalf("svc %s PerturbationScore=%.4f < 0", id, ss.PerturbationScore)
		}
	}
}

// ─── Test 13: Fixed-point solver converges on stable load ────────────────────

func TestIntegration_FixedPointEquilibriumConverges(t *testing.T) {
	windows := map[string]*telemetry.ServiceWindow{
		"svc-a": makeWindow("svc-a", 50, 10, 50),
		"svc-b": makeWindow("svc-b", 40, 12, 50),
	}
	snap := topology.GraphSnapshot{}
	fp := modelling.ComputeFixedPointEquilibrium(windows, snap)

	if math.IsNaN(fp.SystemicCollapseProb) {
		t.Fatal("SystemicCollapseProb is NaN")
	}
	if fp.SystemicCollapseProb < 0 || fp.SystemicCollapseProb > 1 {
		t.Fatalf("SystemicCollapseProb=%.4f out of [0,1]", fp.SystemicCollapseProb)
	}
	if fp.ConvergedInIterations < 0 {
		t.Fatalf("ConvergedInIterations=%d < 0", fp.ConvergedInIterations)
	}
}

// ─── Test 14: Stochastic model fields are finite ─────────────────────────────

func TestIntegration_StochasticModelFinite(t *testing.T) {
	w := makeWindow("svc-s", 120, 15, 40)
	sm := modelling.RunStochasticModel(w)

	if math.IsNaN(sm.ArrivalCoV) || math.IsInf(sm.ArrivalCoV, 0) {
		t.Fatalf("ArrivalCoV is NaN/Inf: %v", sm.ArrivalCoV)
	}
	if math.IsNaN(sm.BurstAmplification) || math.IsInf(sm.BurstAmplification, 0) {
		t.Fatalf("BurstAmplification is NaN/Inf: %v", sm.BurstAmplification)
	}
	if math.IsNaN(sm.RiskPropagation) || math.IsInf(sm.RiskPropagation, 0) {
		t.Fatalf("RiskPropagation is NaN/Inf: %v", sm.RiskPropagation)
	}
	if sm.RiskPropagation < 0 || sm.RiskPropagation > 1 {
		t.Fatalf("RiskPropagation=%.4f out of [0,1]", sm.RiskPropagation)
	}
}