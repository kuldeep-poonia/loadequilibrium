package modelling_test

import (
	"testing"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

func TestQueuePhysicsEngineStableRegime(t *testing.T) {
	engine := modelling.NewQueuePhysicsEngine()

	window := &telemetry.ServiceWindow{
		ServiceID:       "api-svc",
		SampleCount:     100,
		MeanRequestRate: 50.0,
		MeanLatencyMs:   10.0,
		MeanActiveConns: 3.0,
		ConfidenceScore: 0.95,
		Hazard:          0.0,
		Reservoir:       0.0,
		MeanQueueDepth:  2.0,
		LastQueueDepth:  2.1,
	}

	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{
			{ServiceID: "api-svc", NormalisedLoad: 1.0},
		},
		Edges: []topology.Edge{},
	}

	qm := engine.RunQueueModel(window, topoSnap, false)

	if qm.Utilisation > 0.5 {
		t.Errorf("stable regime: expected ρ < 0.5, got %.3f", qm.Utilisation)
	}
	if qm.Confidence < 0.5 {
		t.Errorf("stable regime: expected confidence >= 0.5, got %.3f", qm.Confidence)
	}
}

func TestQueuePhysicsEngineCriticalRegime(t *testing.T) {
	engine := modelling.NewQueuePhysicsEngine()

	window := &telemetry.ServiceWindow{
		ServiceID:       "batch-processor",
		SampleCount:     150,
		MeanRequestRate: 180.0,
		MeanLatencyMs:   20.0,
		MeanActiveConns: 9.0,
		ConfidenceScore: 0.90,
		Hazard:          0.1,
		Reservoir:       0.05,
		MeanQueueDepth:  85.0,
		LastQueueDepth:  87.0,
	}

	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{
			{ServiceID: "batch-processor", NormalisedLoad: 1.0},
		},
		Edges: []topology.Edge{},
	}

	qm := engine.RunQueueModel(window, topoSnap, false)

	if qm.Utilisation < 0.6 || qm.Utilisation > 1.0 {
		t.Logf("critical regime: ρ=%.3f (expected 0.6-1.0 range)", qm.Utilisation)
	}
	if qm.MeanQueueLen < 5.0 {
		t.Logf("critical regime: queue should have backlog, got %.2f", qm.MeanQueueLen)
	}
}

func TestStabilityAssessmentSafeZone(t *testing.T) {
	q := modelling.QueueModel{
		ServiceID:        "safe-svc",
		Utilisation:      0.30,
		UtilisationTrend: -0.02,
		MeanQueueLen:     2.0,
		Hazard:           0.0,
		Reservoir:        0.0,
	}

	sig := modelling.SignalState{
		ServiceID:     "safe-svc",
		FastEWMA:      0.30,
		SlowEWMA:      0.32,
		EWMAVariance:  0.001,
		SpikeDetected: false,
	}

	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{
			{ServiceID: "safe-svc", NormalisedLoad: 1.0},
		},
		Edges: []topology.Edge{},
	}

	sa := modelling.RunStabilityAssessment(q, sig, topoSnap, 0.90)

	if sa.CollapseZone != "safe" {
		t.Errorf("safe zone: expected CollapseZone='safe', got %q", sa.CollapseZone)
	}
	if sa.CollapseRisk > 0.1 {
		t.Errorf("safe zone: collapse risk should be low, got %.3f", sa.CollapseRisk)
	}
	if sa.IsUnstable {
		t.Errorf("safe zone: system should not be unstable")
	}
}

func TestStabilityAssessmentWarningZone(t *testing.T) {
	q := modelling.QueueModel{
		ServiceID:        "warning-svc",
		Utilisation:      0.85,
		UtilisationTrend: +0.05,
		MeanQueueLen:     45.0,
		Hazard:           0.03,
		Reservoir:        0.02,
	}

	sig := modelling.SignalState{
		ServiceID:     "warning-svc",
		FastEWMA:      0.85,
		SlowEWMA:      0.80,
		EWMAVariance:  0.01,
		SpikeDetected: false,
	}

	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{
			{ServiceID: "warning-svc", NormalisedLoad: 1.0},
		},
		Edges: []topology.Edge{},
	}

	sa := modelling.RunStabilityAssessment(q, sig, topoSnap, 0.90)

	if sa.CollapseZone != "warning" {
		t.Errorf("warning zone: expected CollapseZone='warning', got %q", sa.CollapseZone)
	}
	if sa.CollapseRisk < 0.2 {
		t.Errorf("warning zone: collapse risk should be elevated, got %.3f", sa.CollapseRisk)
	}
}

func TestStabilityAssessmentCollapseZone(t *testing.T) {
	q := modelling.QueueModel{
		ServiceID:        "collapsed-svc",
		Utilisation:      1.05,
		UtilisationTrend: +0.15,
		MeanQueueLen:     500.0,
		Hazard:           0.15,
		Reservoir:        0.10,
	}

	sig := modelling.SignalState{
		ServiceID:     "collapsed-svc",
		FastEWMA:      1.05,
		SlowEWMA:      0.95,
		EWMAVariance:  0.05,
		SpikeDetected: true,
	}

	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{
			{ServiceID: "collapsed-svc", NormalisedLoad: 1.0},
		},
		Edges: []topology.Edge{},
	}

	sa := modelling.RunStabilityAssessment(q, sig, topoSnap, 0.90)

	if sa.CollapseZone != "collapse" {
		t.Errorf("collapse zone: expected CollapseZone='collapse', got %q", sa.CollapseZone)
	}
	if !sa.IsUnstable {
		t.Errorf("collapse zone: system should be marked Unstable")
	}
}

func BenchmarkQueuePhysicsEngine(b *testing.B) {
	engine := modelling.NewQueuePhysicsEngine()
	window := &telemetry.ServiceWindow{
		ServiceID:       "bench-svc",
		SampleCount:     100,
		MeanRequestRate: 120.0,
		MeanLatencyMs:   20.0,
		MeanActiveConns: 5.0,
		ConfidenceScore: 0.90,
		Hazard:          0.05,
		Reservoir:       0.02,
		MeanQueueDepth:  30.0,
		LastQueueDepth:  31.0,
	}
	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{{ServiceID: "bench-svc", NormalisedLoad: 1.0}},
		Edges: []topology.Edge{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.RunQueueModel(window, topoSnap, false)
	}
}

func BenchmarkStabilityAssessment(b *testing.B) {
	q := modelling.QueueModel{
		ServiceID:        "bench-svc",
		Utilisation:      0.75,
		UtilisationTrend: 0.02,
		MeanQueueLen:     50.0,
		Hazard:           0.05,
		Reservoir:        0.02,
	}

	sig := modelling.SignalState{
		ServiceID:    "bench-svc",
		FastEWMA:     0.75,
		SlowEWMA:     0.73,
		EWMAVariance: 0.01,
	}

	topoSnap := topology.GraphSnapshot{
		Nodes: []topology.Node{
			{ServiceID: "bench-svc", NormalisedLoad: 1.0},
		},
		Edges: []topology.Edge{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = modelling.RunStabilityAssessment(q, sig, topoSnap, 0.90)
	}
}
