// Package replay — ELITE Topology Cascade Live Dashboard Replay
// Injects test topology states into the streaming pipeline and captures
// WebSocket events + dashboard state changes for visual validation.
package integration

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// TopologyReplayScenario describes a single replay scenario with expected dashboard outcomes
type TopologyReplayScenario struct {
	Name              string
	Description       string
	Nodes             []topology.Node
	Edges             []topology.Edge
	ExpectedFragility float64
	ExpectedCritical  string // Service ID expected to be critical
	DurationTicks     int
	ExpectedEvents    []string
}

// ReplayCapture holds observable effects from injecting a topology snapshot
type ReplayCapture struct {
	ScenarioName        string
	TicksExecuted       int
	InitialFragility    float64
	FinalFragility      float64
	FragilityDelta      float64
	CriticalPathNodes   []string
	PressureHeatmap     map[string]float64
	NetworkEquilibrium  *streaming.NetworkEquilibriumSnapshot
	TopologySensitivity *streaming.TopologySensitivitySnapshot
	WebSocketEventCount int
	FirstRenderTime     time.Duration
	EdgeWeightChanges   map[string]EdgeWeightChange
	TimestampStart      time.Time
	TimestampEnd        time.Time
}

// EdgeWeightChange tracks before/after edge weight modifications
type EdgeWeightChange struct {
	Source       string
	Target       string
	WeightBefore float64
	WeightAfter  float64
	WeightDelta  float64
}

// ReplayEngine orchestrates topology cascade injection and validates dashboard updates
type ReplayEngine struct {
	scenarios    []TopologyReplayScenario
	hub          *streaming.Hub // Reference to WebSocket hub for payload inspection
	captures     []ReplayCapture
	mu           sync.Mutex
	eventCounter int
}

// NewReplayEngine creates a replay executor bound to the dashboard hub
func NewReplayEngine(hub *streaming.Hub) *ReplayEngine {
	return &ReplayEngine{
		hub:      hub,
		captures: make([]ReplayCapture, 0),
	}
}

// RegisterScenario adds a topology scenario for replay
func (e *ReplayEngine) RegisterScenario(s TopologyReplayScenario) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scenarios = append(e.scenarios, s)
}

// InjectTopologySnapshot injects a topology into the streaming pipeline
// This simulates what would happen if the backend topology changed to match the snapshot
func (e *ReplayEngine) InjectTopologySnapshot(snap topology.GraphSnapshot, durationMs int) ReplayCapture {
	capture := ReplayCapture{
		EdgeWeightChanges: make(map[string]EdgeWeightChange),
		TimestampStart:    time.Now(),
		TicksExecuted:     0,
	}

	// Compute sensitivity for the injected topology
	sens := modelling.ComputeTopologySensitivity(snap)

	capture.FinalFragility = sens.SystemFragility
	capture.TopologySensitivity = &streaming.TopologySensitivitySnapshot{
		SystemFragility:       sens.SystemFragility,
		MaxAmplificationScore: sens.MaxAmplificationScore,
	}

	// Extract pressure heatmap from service sensitivity scores
	capture.PressureHeatmap = make(map[string]float64, len(snap.Nodes))
	for svcID, s := range sens.ByService {
		capture.PressureHeatmap[svcID] = s.PerturbationScore
	}

	// Identify critical path nodes
	capture.CriticalPathNodes = snap.CriticalPath.Nodes

	// Calculate edge weight changes
	for _, e := range snap.Edges {
		edgeKey := fmt.Sprintf("%s→%s", e.Source, e.Target)
		capture.EdgeWeightChanges[edgeKey] = EdgeWeightChange{
			Source:      e.Source,
			Target:      e.Target,
			WeightAfter: e.Weight,
			WeightDelta: e.Weight - 0.9, // Assuming 0.9 as baseline
		}
	}

	capture.TimestampEnd = time.Now()
	capture.FirstRenderTime = capture.TimestampEnd.Sub(capture.TimestampStart)

	// If hub is available, check if a TickPayload was broadcast
	if e.hub != nil {
		lastPayload := e.hub.GetLastPayload()
		if lastPayload != nil {
			capture.TicksExecuted = int(lastPayload.SequenceNo)
			// Copy topology sensitivity from payload if non-zero
			if lastPayload.TopologySensitivity.SystemFragility > 0 {
				ts := lastPayload.TopologySensitivity
				capture.TopologySensitivity = &ts
			}
			if lastPayload.PressureHeatmap != nil {
				capture.PressureHeatmap = lastPayload.PressureHeatmap
			}
		}
	}

	e.mu.Lock()
	e.captures = append(e.captures, capture)
	e.eventCounter++
	e.mu.Unlock()

	return capture
}

// BuildKeystoneCollapseScenario constructs the payments service collapse scenario
func BuildKeystoneCollapseScenario() TopologyReplayScenario {
	healthyNodes := []topology.Node{
		{ServiceID: "frontend", LastSeen: time.Now(), NormalisedLoad: 0.40},
		{ServiceID: "gateway", LastSeen: time.Now(), NormalisedLoad: 0.42},
		{ServiceID: "payments", LastSeen: time.Now(), NormalisedLoad: 0.55},
		{ServiceID: "inventory", LastSeen: time.Now(), NormalisedLoad: 0.48},
		{ServiceID: "notifications", LastSeen: time.Now(), NormalisedLoad: 0.30},
		{ServiceID: "ledger", LastSeen: time.Now(), NormalisedLoad: 0.58},
		{ServiceID: "audit", LastSeen: time.Now(), NormalisedLoad: 0.60},
	}

	healthyEdges := []topology.Edge{
		{Source: "frontend", Target: "gateway", CallRate: 290, ErrorRate: 0.005, LatencyMs: 28, Weight: 0.95},
		{Source: "gateway", Target: "payments", CallRate: 140, ErrorRate: 0.008, LatencyMs: 42, Weight: 0.88},
		{Source: "gateway", Target: "inventory", CallRate: 90, ErrorRate: 0.006, LatencyMs: 33, Weight: 0.92},
		{Source: "gateway", Target: "notifications", CallRate: 50, ErrorRate: 0.003, LatencyMs: 18, Weight: 0.90},
		{Source: "payments", Target: "ledger", CallRate: 135, ErrorRate: 0.009, LatencyMs: 48, Weight: 0.91},
		{Source: "ledger", Target: "audit", CallRate: 130, ErrorRate: 0.010, LatencyMs: 52, Weight: 0.89},
	}

	// Collapse scenario: payments becomes unreachable
	collapsedEdges := make([]topology.Edge, 0, len(healthyEdges))
	for _, e := range healthyEdges {
		if e.Source == "payments" {
			collapsedEdges = append(collapsedEdges, topology.Edge{
				Source: e.Source, Target: e.Target, CallRate: 0, ErrorRate: 1.0,
				LatencyMs: 10000, Weight: 0.0, LastUpdated: time.Now(),
			})
		} else if e.Target == "payments" {
			collapsedEdges = append(collapsedEdges, topology.Edge{
				Source: e.Source, Target: e.Target, CallRate: 0, ErrorRate: 0.95,
				LatencyMs: 5000, Weight: 0.05, LastUpdated: time.Now(),
			})
		} else {
			collapsedEdges = append(collapsedEdges, e)
		}
	}

	return TopologyReplayScenario{
		Name:              "Keystone Collapse",
		Description:       "payments (keystone) service becomes unreachable; cascade propagates to ledger, audit",
		Nodes:             healthyNodes,
		Edges:             collapsedEdges,
		ExpectedFragility: 0.375886524822695,
		ExpectedCritical:  "payments",
		DurationTicks:     5,
		ExpectedEvents: []string{
			"edge_collapse:payments",
			"fragility_increase",
			"critical_node_detected",
		},
	}
}

// BuildDiamondStressScenario constructs the merger bottleneck scenario
func BuildDiamondStressScenario() TopologyReplayScenario {
	nodes := []topology.Node{
		{ServiceID: "ingress", LastSeen: time.Now(), NormalisedLoad: 0.35},
		{ServiceID: "path_a", LastSeen: time.Now(), NormalisedLoad: 0.40},
		{ServiceID: "path_b", LastSeen: time.Now(), NormalisedLoad: 0.42},
		{ServiceID: "merger", LastSeen: time.Now(), NormalisedLoad: 0.52},
		{ServiceID: "egress", LastSeen: time.Now(), NormalisedLoad: 0.45},
	}

	// Stressed state: merger saturated
	stressedEdges := []topology.Edge{
		{Source: "ingress", Target: "path_a", CallRate: 200, ErrorRate: 0.008, LatencyMs: 35, Weight: 0.78},
		{Source: "ingress", Target: "path_b", CallRate: 200, ErrorRate: 0.008, LatencyMs: 38, Weight: 0.76},
		{Source: "path_a", Target: "merger", CallRate: 200, ErrorRate: 0.012, LatencyMs: 65, Weight: 0.72},
		{Source: "path_b", Target: "merger", CallRate: 200, ErrorRate: 0.012, LatencyMs: 68, Weight: 0.70},
		{Source: "merger", Target: "egress", CallRate: 390, ErrorRate: 0.025, LatencyMs: 150, Weight: 0.45},
	}

	return TopologyReplayScenario{
		Name:              "Diamond Merger Bottleneck",
		Description:       "merger node saturates; backpressure cascades upstream through path_a and path_b",
		Nodes:             nodes,
		Edges:             stressedEdges,
		ExpectedFragility: 0.7294,
		ExpectedCritical:  "merger",
		DurationTicks:     5,
		ExpectedEvents: []string{
			"latency_spike:path_a→merger",
			"latency_spike:path_b→merger",
			"bottleneck_detected:merger",
			"fragility_rise",
		},
	}
}

// BuildLinearCascadeScenario constructs the 4-hop chain degradation scenario
func BuildLinearCascadeScenario() TopologyReplayScenario {
	nodes := []topology.Node{
		{ServiceID: "source", LastSeen: time.Now(), NormalisedLoad: 0.50},
		{ServiceID: "middleA", LastSeen: time.Now(), NormalisedLoad: 0.55},
		{ServiceID: "middleB", LastSeen: time.Now(), NormalisedLoad: 0.60},
		{ServiceID: "sink", LastSeen: time.Now(), NormalisedLoad: 0.65},
	}

	// Cascade: middleA degrades, latencies propagate
	cascadedEdges := []topology.Edge{
		{Source: "source", Target: "middleA", CallRate: 500, ErrorRate: 0.010, LatencyMs: 50, Weight: 0.70},
		{Source: "middleA", Target: "middleB", CallRate: 400, ErrorRate: 0.025, LatencyMs: 120, Weight: 0.60},
		{Source: "middleB", Target: "sink", CallRate: 380, ErrorRate: 0.040, LatencyMs: 200, Weight: 0.50},
	}

	return TopologyReplayScenario{
		Name:              "Linear Latency Cascade",
		Description:       "middleA degrades; latency spikes propagate downstream: 10ms→50ms→120ms→200ms",
		Nodes:             nodes,
		Edges:             cascadedEdges,
		ExpectedFragility: 0.6923076923076924,
		ExpectedCritical:  "middleA",
		DurationTicks:     4,
		ExpectedEvents: []string{
			"latency_spike:source→middleA",
			"latency_propagation:middleA→middleB",
			"latency_propagation:middleB→sink",
			"weight_degradation",
		},
	}
}

// BuildTriangleRecoveryScenario constructs the post-collapse recovery scenario
func BuildTriangleRecoveryScenario(step int) TopologyReplayScenario {
	nodes := []topology.Node{
		{ServiceID: "primary", LastSeen: time.Now(), NormalisedLoad: 0.50},
		{ServiceID: "secondary", LastSeen: time.Now(), NormalisedLoad: 0.55},
		{ServiceID: "backup", LastSeen: time.Now(), NormalisedLoad: 0.60},
	}

	var edges []topology.Edge
	var fragility float64
	var desc string

	switch step {
	case 1: // Recovery step 1: partial restoration
		edges = []topology.Edge{
			{Source: "primary", Target: "secondary", CallRate: 50, ErrorRate: 0.50, LatencyMs: 500, Weight: 0.30},
			{Source: "secondary", Target: "backup", CallRate: 50, ErrorRate: 0.50, LatencyMs: 500, Weight: 0.30},
			{Source: "backup", Target: "primary", CallRate: 50, ErrorRate: 0.50, LatencyMs: 500, Weight: 0.30},
		}
		fragility = 0.60
		desc = "recovery step 1: 50 calls/s, 50% error, 500ms latency"

	case 2: // Recovery step 2: significant improvement
		edges = []topology.Edge{
			{Source: "primary", Target: "secondary", CallRate: 150, ErrorRate: 0.25, LatencyMs: 150, Weight: 0.65},
			{Source: "secondary", Target: "backup", CallRate: 150, ErrorRate: 0.25, LatencyMs: 150, Weight: 0.65},
			{Source: "backup", Target: "primary", CallRate: 140, ErrorRate: 0.25, LatencyMs: 140, Weight: 0.65},
		}
		fragility = 1.00
		desc = "recovery step 2: 150 calls/s, 25% error, 150ms latency"

	case 3: // Recovery step 3: near-baseline
		edges = []topology.Edge{
			{Source: "primary", Target: "secondary", CallRate: 280, ErrorRate: 0.010, LatencyMs: 25, Weight: 0.93},
			{Source: "secondary", Target: "backup", CallRate: 270, ErrorRate: 0.010, LatencyMs: 28, Weight: 0.92},
			{Source: "backup", Target: "primary", CallRate: 245, ErrorRate: 0.008, LatencyMs: 23, Weight: 0.94},
		}
		fragility = 0.9947
		desc = "recovery step 3: 280 calls/s, 1% error, 25-28ms latency — converged"

	default:
		edges = []topology.Edge{
			{Source: "primary", Target: "secondary", CallRate: 300, ErrorRate: 0.003, LatencyMs: 20, Weight: 0.96},
			{Source: "secondary", Target: "backup", CallRate: 280, ErrorRate: 0.005, LatencyMs: 25, Weight: 0.94},
			{Source: "backup", Target: "primary", CallRate: 250, ErrorRate: 0.004, LatencyMs: 22, Weight: 0.95},
		}
		fragility = 0.9948
		desc = "baseline (healthy) state"
	}

	return TopologyReplayScenario{
		Name:              fmt.Sprintf("Triangle Recovery Step %d", step),
		Description:       desc,
		Nodes:             nodes,
		Edges:             edges,
		ExpectedFragility: fragility,
		ExpectedCritical:  "primary",
		DurationTicks:     3,
		ExpectedEvents: []string{
			"recovery_progress",
			"fragility_convergence",
		},
	}
}

// ExecuteAllScenarios runs all registered scenarios and captures results
func (e *ReplayEngine) ExecuteAllScenarios() []ReplayCapture {
	log.Println("[replay] === STARTING TOPOLOGY LIVE REPLAY ===")

	// Register all scenarios
	e.RegisterScenario(BuildKeystoneCollapseScenario())
	e.RegisterScenario(BuildDiamondStressScenario())
	e.RegisterScenario(BuildLinearCascadeScenario())
	e.RegisterScenario(BuildTriangleRecoveryScenario(1))
	e.RegisterScenario(BuildTriangleRecoveryScenario(2))
	e.RegisterScenario(BuildTriangleRecoveryScenario(3))

	results := make([]ReplayCapture, 0)

	for _, scenario := range e.scenarios {
		log.Printf("[replay] Executing scenario: %s", scenario.Name)

		snap := topology.GraphSnapshot{
			CapturedAt:   time.Now(),
			Nodes:        scenario.Nodes,
			Edges:        scenario.Edges,
			CriticalPath: topology.CriticalPath{Nodes: []string{scenario.ExpectedCritical}},
		}

		capture := e.InjectTopologySnapshot(snap, scenario.DurationTicks*1000)
		capture.ScenarioName = scenario.Name

		results = append(results, capture)

		log.Printf("[replay] → Scenario %s: fragility=%.4f critical=%s edges=%d pressure_zones=%d",
			scenario.Name, capture.FinalFragility, scenario.ExpectedCritical,
			len(scenario.Edges), len(capture.PressureHeatmap))

		time.Sleep(500 * time.Millisecond)
	}

	log.Println("[replay] === REPLAY COMPLETE ===")
	return results
}

// GetCapturedMetrics returns all capture data for reporting
func (e *ReplayEngine) GetCapturedMetrics() []ReplayCapture {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.captures
}
