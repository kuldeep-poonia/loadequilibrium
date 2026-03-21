package streaming

import (
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/reasoning"
	"github.com/loadequilibrium/loadequilibrium/internal/simulation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// SchemaVersion is bumped on any breaking TickPayload shape change.
const SchemaVersion uint32 = 3

type MessageType string

const (
	MsgTick MessageType = "tick"
	MsgPing MessageType = "ping"
)

// PredictionPoint is one point on a service's predicted utilisation curve.
type PredictionPoint struct {
	TickOffset int     `json:"t"`   // ticks from now
	Rho        float64 `json:"rho"` // predicted utilisation
	Lower95    float64 `json:"lo"`  // 95% CI lower bound
	Upper95    float64 `json:"hi"`  // 95% CI upper bound
}

// TickPayload is the full state broadcast on each engine tick.
type TickPayload struct {
	Type       MessageType                              `json:"type"`
	SequenceNo uint64                                   `json:"seq"`
	Timestamp  time.Time                                `json:"ts"`
	Schema     uint32                                   `json:"schema_version"`
	Bundles    map[string]*modelling.ServiceModelBundle `json:"bundles"`
	Topology   topology.GraphSnapshot                   `json:"topology"`
	TopoDiff   TopologyDiff                             `json:"topo_diff"`
	Objective  optimisation.ObjectiveScore              `json:"objective"`
	Directives map[string]optimisation.ControlDirective `json:"directives"`
	Events     []reasoning.Event                        `json:"events"`
	SimResult  *simulation.SimulationResult             `json:"sim_result,omitempty"`

	// Control-room intelligence overlay fields.
	DegradedServices     []string                       `json:"degraded_services,omitempty"`
	SaturationCountdowns map[string]float64             `json:"sat_countdowns,omitempty"`
	StabilityZones       map[string]string              `json:"stability_zones,omitempty"`
	PredictionHorizon    map[string]float64             `json:"prediction_horizon,omitempty"`

	// PredictionTimeline: per-service prediction curves for the next N ticks.
	PredictionTimeline map[string][]PredictionPoint `json:"prediction_timeline,omitempty"`

	// Runtime health metrics streamed to dashboard.
	TickHealthMs     float64 `json:"tick_health_ms"`
	DegradedFraction float64 `json:"degraded_fraction"`
	SafetyMode       bool    `json:"safety_mode"`
	JitterMs         float64 `json:"jitter_ms"`

	// RuntimeMetrics: per-stage rolling average latencies for the control-room
	// runtime health panel. Zero-allocation fixed struct — values are EWMA
	// smoothed over the last ~10 ticks (α=0.1).
	RuntimeMetrics RuntimeMetrics `json:"runtime_metrics"`

	// NetworkCouplingData: per-service equilibrium coupling data.
	NetworkCouplingData map[string]NetworkCouplingSnapshot `json:"network_coupling,omitempty"`

	// NetworkEquilibrium: system-level equilibrium state derived from coupled queue analysis.
	NetworkEquilibrium NetworkEquilibriumSnapshot `json:"network_equilibrium,omitempty"`

	// TopologySensitivity: structural sensitivity of the dependency graph to perturbations.
	TopologySensitivity TopologySensitivitySnapshot `json:"topology_sensitivity,omitempty"`

	// PriorityRiskQueue: ordered list of service IDs ranked by operational urgency.
	// Index 0 = most urgent. Each entry carries a composite risk score.
	// Dashboard uses this to drive attention ordering in the service list.
	PriorityRiskQueue []RiskQueueItem `json:"priority_risk_queue,omitempty"`

	// PressureHeatmap: maps each service to a normalised pressure intensity [0,1].
	PressureHeatmap map[string]float64 `json:"pressure_heatmap,omitempty"`

	// SimOverlay: simulation-derived overlay state for dashboard topology rendering.
	// Updated when a new simulation result arrives; stale when SimTickAge > 5.
	SimOverlay *SimOverlayState `json:"sim_overlay,omitempty"`

	// FixedPointEquilibrium: per-service steady-state ρ from the Gauss-Seidel fixed-point solver.
	FixedPointEquilibrium FixedPointSnapshot `json:"fixed_point_equilibrium,omitempty"`

	// ScenarioComparison: comparison of multiple simulation scenario outcomes.
	// Populated when the Monte-Carlo runner has produced ≥ 2 scenario results
	// this tick. Enables operators to compare best/worst/median trajectories.
	ScenarioComparison *ScenarioComparisonSnapshot `json:"scenario_comparison,omitempty"`

	// RiskTimeline: per-service predicted risk trajectory over the prediction horizon.
	// Each entry shows ρ and CollapseRisk at k ticks from now under current trends.
	// Enables "risk runway" reasoning: how many ticks until a service goes critical.
	RiskTimeline PredictiveRiskTimeline `json:"risk_timeline,omitempty"`

	// StabilityEnvelope: stable operating boundary from equilibrium analysis.
	// Tells operators whether the system is inside or outside its stability region.
	StabilityEnvelope StabilityEnvelopeSnapshot `json:"stability_envelope,omitempty"`
}

// ScenarioComparisonSnapshot represents a lightweight comparison across MC scenarios.
type ScenarioComparisonSnapshot struct {
	// ScenarioCount: number of scenarios aggregated into this comparison.
	ScenarioCount int `json:"scenario_count"`
	// BestCaseCollapse: minimum systemic collapse probability across scenarios.
	BestCaseCollapse float64 `json:"best_case_collapse"`
	// WorstCaseCollapse: maximum systemic collapse probability across scenarios.
	WorstCaseCollapse float64 `json:"worst_case_collapse"`
	// MedianSLAViolation: median SLA violation probability across all services and scenarios.
	MedianSLAViolation float64 `json:"median_sla_violation"`
	// StableScenarioFraction: fraction of scenarios where SystemStable==true.
	StableScenarioFraction float64 `json:"stable_scenario_fraction"`
	// RecoveryConvergenceRange: [min, max] recovery convergence times in ms across scenarios.
	RecoveryConvergenceMin float64 `json:"recovery_convergence_min_ms"`
	RecoveryConvergenceMax float64 `json:"recovery_convergence_max_ms"`
}

// FixedPointSnapshot is the JSON form of modelling.FixedPointResult.
type FixedPointSnapshot struct {
	EquilibriumRho          map[string]float64 `json:"equilibrium_rho,omitempty"`
	SystemicCollapseProb    float64            `json:"systemic_collapse_prob"`
	ConvergedIterations     int                `json:"converged_iterations"`
	Converged               bool               `json:"converged"`
	PerturbationSensitivity map[string]float64 `json:"perturbation_sensitivity,omitempty"`
	// ConvergenceRate: spectral radius estimate ρ(J). < 1 = stable equilibrium.
	ConvergenceRate float64 `json:"convergence_rate"`
	// StabilityMargin: 1 - ConvergenceRate. Positive = system returns to equilibrium.
	StabilityMargin float64 `json:"stability_margin"`
}

// RiskTimelinePoint is a single point on a service's predicted risk trajectory.
type RiskTimelinePoint struct {
	TickOffset   int     `json:"t"`
	Rho          float64 `json:"rho"`
	CollapseRisk float64 `json:"risk"`
}

// PredictiveRiskTimeline maps service IDs to their predicted risk runway.
type PredictiveRiskTimeline map[string][]RiskTimelinePoint

// StabilityEnvelopeSnapshot describes the safe operating boundary derived from
// the fixed-point equilibrium solver and perturbation sensitivity analysis.
type StabilityEnvelopeSnapshot struct {
	SafeSystemRhoMax       float64 `json:"safe_system_rho_max"`
	CurrentSystemRhoMean   float64 `json:"current_system_rho_mean"`
	EnvelopeHeadroom       float64 `json:"envelope_headroom"`
	MostVulnerableService  string  `json:"most_vulnerable_service,omitempty"`
	WorstPerturbationDelta float64 `json:"worst_perturbation_delta"`
}

// RiskQueueItem is a ranked entry in the priority risk queue.
type RiskQueueItem struct {
	ServiceID    string  `json:"service_id"`
	UrgencyScore float64 `json:"urgency_score"` // 0..1, higher = more urgent
	CollapseRisk float64 `json:"collapse_risk"`
	Rho          float64 `json:"rho"`
	IsKeystone   bool    `json:"is_keystone"`
	// PathCollapseProb: path-level collapse probability from network equilibrium solver.
	// Higher than local CollapseRisk when upstream pressure amplifies the risk.
	PathCollapseProb float64 `json:"path_collapse_prob"`
	// UrgencyClass: categorical urgency for dashboard badge colouring.
	// "critical" | "warning" | "elevated" | "nominal"
	UrgencyClass string `json:"urgency_class"`
}

// SimOverlayState carries per-service simulation overlay data for dashboard rendering.
type SimOverlayState struct {
	// CascadeFailureProbability: per-service empirical collapse probability from sim.
	CascadeFailureProbability map[string]float64 `json:"cascade_failure_prob,omitempty"`
	// P95QueueLen: per-service 95th-pct queue length at horizon end.
	P95QueueLen map[string]float64 `json:"p95_queue_len,omitempty"`
	// SaturationFrac: fraction of simulated time spent in near-saturation per service.
	SaturationFrac map[string]float64 `json:"saturation_frac,omitempty"`
	// SLAViolationProbability: per-service fraction of requests exceeding SLA threshold.
	// Derived from Monte-Carlo runs. P(latency > SLAThreshold) per service.
	SLAViolationProbability map[string]float64 `json:"sla_violation_prob,omitempty"`
	// HorizonMs: virtual-time horizon this overlay covers.
	HorizonMs float64 `json:"horizon_ms"`
	// SimTickAge: ticks since this result was computed. Stale overlays fade on dashboard.
	SimTickAge int `json:"sim_tick_age"`
}

// RuntimeMetrics carries per-stage EWMA latency data for dashboard rendering.
// All durations are in milliseconds. Zero value means the stage has not yet run.
type RuntimeMetrics struct {
	AvgPruneMs     float64 `json:"avg_prune_ms"`
	AvgWindowsMs   float64 `json:"avg_windows_ms"`
	AvgTopologyMs  float64 `json:"avg_topology_ms"`
	AvgCouplingMs  float64 `json:"avg_coupling_ms"`
	AvgModellingMs float64 `json:"avg_modelling_ms"`
	AvgOptimiseMs  float64 `json:"avg_optimise_ms"`
	AvgSimMs       float64 `json:"avg_sim_ms"`
	AvgReasoningMs float64 `json:"avg_reasoning_ms"`
	AvgBroadcastMs float64 `json:"avg_broadcast_ms"`
	TotalOverruns  uint64  `json:"total_overruns"`
	ConsecOverruns int     `json:"consec_overruns"`
	// PredictedCriticalMs: EWMA-predicted cost of critical stages for next tick.
	// When > 80% of TickDeadline, the engine tightens budgets proactively.
	PredictedCriticalMs float64 `json:"predicted_critical_ms"`
	// PredictedOverrun: true when EWMA trend projects critical stages will exceed 80% of deadline.
	PredictedOverrun bool `json:"predicted_overrun"`
	SafetyLevel     int  `json:"safety_level"` // graduated 0-3
}

// TopologySensitivitySnapshot is the JSON-friendly form of modelling.TopologySensitivity.
type TopologySensitivitySnapshot struct {
	SystemFragility       float64                    `json:"system_fragility"`
	MaxAmplificationPath  []string                   `json:"max_amplification_path,omitempty"`
	MaxAmplificationScore float64                    `json:"max_amplification_score"`
	KeystoneServices      []string                   `json:"keystone_services,omitempty"`
	ByService             map[string]ServiceSensSnap `json:"by_service,omitempty"`
}

// ServiceSensSnap is the per-service JSON form.
type ServiceSensSnap struct {
	PerturbationScore float64 `json:"perturbation_score"`
	DownstreamReach   int     `json:"downstream_reach"`
	UpstreamExposure  int     `json:"upstream_exposure"`
	IsKeystone        bool    `json:"is_keystone"`
}

// NetworkCouplingSnapshot is the JSON-friendly subset of modelling.NetworkCoupling.
type NetworkCouplingSnapshot struct {
	EffectivePressure        float64 `json:"effective_pressure"`
	PathSaturationRisk       float64 `json:"path_sat_risk"`
	CoupledArrivalRate       float64 `json:"coupled_arrival_rate"`
	PathEquilibriumRho       float64 `json:"path_equilibrium_rho"`
	SaturationPathLength     int     `json:"path_length"`
	CongestionFeedbackScore  float64 `json:"congestion_feedback"`
	PathSaturationHorizonSec float64 `json:"path_sat_horizon_sec"`
	PathCollapseProb         float64 `json:"path_collapse_prob"`
	// Steady-state queue distribution at equilibrium ρ.
	SteadyStateP0         float64 `json:"steady_state_p0"`
	SteadyStateMeanQueue  float64 `json:"steady_state_mean_queue"`
}

// NetworkEquilibriumSnapshot is the JSON-friendly form of modelling.NetworkEquilibriumState.
type NetworkEquilibriumSnapshot struct {
	SystemRhoMean         float64 `json:"system_rho_mean"`
	SystemRhoVariance     float64 `json:"system_rho_variance"`
	EquilibriumDelta      float64 `json:"equilibrium_delta"`
	IsConverging          bool    `json:"is_converging"`
	MaxCongestionFeedback float64 `json:"max_congestion_feedback"`
	CriticalServiceID     string  `json:"critical_service_id"`
	NetworkSaturationRisk float64 `json:"network_saturation_risk"`
}
