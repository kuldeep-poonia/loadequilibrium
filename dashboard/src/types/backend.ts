// =============================================================================
// backend.ts — Full TickPayload contract matching streaming/types.go SchemaV3
// =============================================================================

export type MessageType = "tick" | "ping";

// ── Topology ─────────────────────────────────────────────────────────────────

export interface Node {
  service_id: string;
  normalised_load: number;
  last_seen?: string;
}

export interface Edge {
  source: string;
  target: string;
  weight: number;
  call_rate: number;
  error_rate: number;
  latency_ms: number;
  last_updated?: string;
}

export interface CriticalPath {
  nodes: string[];
  total_weight: number;
  cascade_risk: number;
}

export interface Topology {
  captured_at: string;
  nodes: Node[];
  edges: Edge[];
  critical_path: CriticalPath;
}

// ── Topology Diff ────────────────────────────────────────────────────────────

export interface TopologyDiff {
  schema: number;
  added_nodes?: Node[];
  removed_nodes?: string[];
  updated_nodes?: Node[];
  added_edges?: Edge[];
  removed_edges?: string[];
  updated_edges?: Edge[];
  is_full: boolean;
}

// ── Queue & Stability per Service ────────────────────────────────────────────

export interface QueueModel {
  service_id: string;
  arrival_rate: number;
  service_rate: number;
  concurrency: number;
  utilisation: number;
  mean_queue_len: number;
  mean_wait_ms: number;
  mean_sojourn_ms: number;
  burst_factor: number;
  adjusted_wait_ms: number;
  utilisation_trend: number;
  confidence: number;
  upstream_pressure: number;
  hazard: number;
  reservoir: number;
  last_p99_latency_ms: number;
}

export interface StabilityModel {
  service_id: string;
  stability_margin: number;
  collapse_risk: number;
  oscillation_risk: number;
  feedback_gain: number;
  is_unstable: boolean;
  predicted_collapse_ms: number;
  cascade_amplification_score: number;
  collapse_zone: "safe" | "warning" | "collapse";
  trend_adjusted_margin: number;
  stability_derivative: number;
}

export interface StochasticModel {
  arrival_co_v: number;
  burst_amplification: number;
  risk_propagation: number;
  confidence: number;
}

export interface SignalModel {
  fast_ewma: number;
  slow_ewma: number;
  ewma_variance: number;
  cusum_pos: number;
  cusum_neg: number;
  spike_detected: boolean;
}

export interface ServiceBundle {
  queue: QueueModel;
  stability: StabilityModel;
  stochastic: StochasticModel;
  signal: SignalModel;
}

// ── Objective ────────────────────────────────────────────────────────────────

export interface ObjectiveScore {
  composite_score: number;
  max_collapse_risk: number;
  cascade_failure_probability: number;
  predicted_p99_latency_ms: number;
  oscillation_risk: number;
  risk_acceleration: number;
  trajectory_score: number;
  latency_weight: number;
  utilisation_weight: number;
  risk_weight: number;
  reference_latency_ms: number;
  trend_stability_margin: number;
}

// ── Directives ───────────────────────────────────────────────────────────────

export interface ControlDirective {
  service_id?: string;
  computed_at?: string;
  scale_factor: number;
  target_utilisation: number;
  error: number;
  pid_output: number;
  active: boolean;
  stability_margin: number;
  hysteresis_state?: string;
  actuation_bound?: number;
  predictive_target?: number;
  mpc_predicted_rho?: number;
  mpc_overshoot_risk?: boolean;
  mpc_underactuation_risk?: boolean;
  cost_gradient?: number;
  trajectory_cost_avg?: number;
  max_trajectory_cost?: number;
  planner_scale_factor?: number;
  planner_convergent?: boolean;
  planner_convex?: boolean;
  planner_probabilistic_score?: number;
}

// ── Events ───────────────────────────────────────────────────────────────────

export interface EventEvidence {
  utilisation: number;
  collapse_risk: number;
  oscillation_risk: number;
  queue_wait_ms: number;
  saturation_sec: number;
  burst_factor: number;
  cascade_risk: number;
  stability_margin: number;
  composite_score: number;
}

export interface Event {
  id?: string;
  timestamp: string;
  category: string;
  description: string;
  severity: string;
  service_id?: string;
  evidence?: EventEvidence;
  uncertainty_score?: number;
}

// ── Risk Queue ───────────────────────────────────────────────────────────────

export interface RiskItem {
  service_id: string;
  urgency_score: number;
  collapse_risk: number;
  rho: number;
  is_keystone: boolean;
  path_collapse_prob: number;
  urgency_class: "critical" | "warning" | "elevated" | "nominal";
}

// ── Prediction & Risk Timelines ──────────────────────────────────────────────

export interface PredictionPoint {
  t: number;
  rho: number;
  lo: number;
  hi: number;
}

export interface RiskTimelinePoint {
  t: number;
  rho: number;
  risk: number;
}

// ── Runtime Metrics ──────────────────────────────────────────────────────────

export interface RuntimeMetrics {
  avg_prune_ms: number;
  avg_windows_ms: number;
  avg_topology_ms: number;
  avg_coupling_ms: number;
  avg_modelling_ms: number;
  avg_optimise_ms: number;
  avg_sim_ms: number;
  avg_reasoning_ms: number;
  avg_broadcast_ms: number;
  total_overruns: number;
  consec_overruns: number;
  predicted_critical_ms: number;
  predicted_overrun: boolean;
  safety_level: number;
}

export interface ControlPlaneState {
  tick: number;
  actuation_enabled: boolean;
  policy_preset: string;
  forced_sandbox_until: number;
  forced_simulation_until: number;
  forced_intelligence_until: number;
  simulation_reset_pending: boolean;
  acknowledged_alert_count: number;
}

// ── Network Coupling & Equilibrium ───────────────────────────────────────────

export interface NetworkCouplingSnapshot {
  effective_pressure: number;
  path_sat_risk: number;
  coupled_arrival_rate: number;
  path_equilibrium_rho: number;
  path_length: number;
  congestion_feedback: number;
  path_sat_horizon_sec: number;
  path_collapse_prob: number;
  steady_state_p0: number;
  steady_state_mean_queue: number;
}

export interface NetworkEquilibriumSnapshot {
  system_rho_mean: number;
  system_rho_variance: number;
  equilibrium_delta: number;
  is_converging: boolean;
  max_congestion_feedback: number;
  critical_service_id: string;
  network_saturation_risk: number;
}

// ── Topology Sensitivity ─────────────────────────────────────────────────────

export interface ServiceSensitivity {
  perturbation_score: number;
  downstream_reach: number;
  upstream_exposure: number;
  is_keystone: boolean;
}

export interface TopologySensitivity {
  system_fragility: number;
  max_amplification_path: string[];
  max_amplification_score: number;
  keystone_services: string[];
  by_service: Record<string, ServiceSensitivity>;
}

// ── Fixed Point Equilibrium ──────────────────────────────────────────────────

export interface FixedPointSnapshot {
  equilibrium_rho: Record<string, number>;
  systemic_collapse_prob: number;
  converged_iterations: number;
  converged: boolean;
  perturbation_sensitivity: Record<string, number>;
  convergence_rate: number;
  stability_margin: number;
}

// ── Simulation ───────────────────────────────────────────────────────────────

export interface QueueDistribution {
  mean_queue_len: number;
  var_queue_len: number;
  p95_queue_len: number;
  saturation_frac: number;
  utilisation_at_end: number;
}

export interface SimulationResult {
  system_stable: boolean;
  recovery_convergence_ms: number;
  cascade_failure_probability: Record<string, number>;
  queue_distribution_at_horizon: Record<string, QueueDistribution>;
  sla_violation_probability: Record<string, number>;
}

export interface SimOverlayState {
  cascade_failure_prob: Record<string, number>;
  p95_queue_len: Record<string, number>;
  saturation_frac: Record<string, number>;
  sla_violation_prob: Record<string, number>;
  horizon_ms: number;
  sim_tick_age: number;
}

export interface ScenarioComparison {
  scenario_count: number;
  best_case_collapse: number;
  worst_case_collapse: number;
  median_sla_violation: number;
  stable_scenario_fraction: number;
  recovery_convergence_min_ms: number;
  recovery_convergence_max_ms: number;
}

// ── Stability Envelope ───────────────────────────────────────────────────────

export interface StabilityEnvelope {
  safe_system_rho_max: number;
  current_system_rho_mean: number;
  envelope_headroom: number;
  most_vulnerable_service: string;
  worst_perturbation_delta: number;
}

// ── Full TickPayload ─────────────────────────────────────────────────────────

export interface TickPayload {
  type: MessageType;
  seq: number;
  ts: string;
  schema_version: number;

  // Core per-service data
  bundles: Record<string, ServiceBundle>;
  topology: Topology;
  topo_diff?: TopologyDiff;
  objective: ObjectiveScore;
  directives: Record<string, ControlDirective>;
  events: Event[];
  sim_result?: SimulationResult;

  // Control-room intelligence overlay
  degraded_services: string[];
  sat_countdowns: Record<string, number>;
  stability_zones: Record<string, string>;
  prediction_horizon: Record<string, number>;
  prediction_timeline: Record<string, PredictionPoint[]>;

  // Runtime health
  tick_health_ms: number;
  degraded_fraction: number;
  safety_mode: boolean;
  jitter_ms: number;
  runtime_metrics: RuntimeMetrics;
  control_plane: ControlPlaneState;

  // Network analysis
  network_coupling: Record<string, NetworkCouplingSnapshot>;
  network_equilibrium: NetworkEquilibriumSnapshot;
  topology_sensitivity: TopologySensitivity;

  // Risk
  priority_risk_queue: RiskItem[];
  pressure_heatmap: Record<string, number>;

  // Simulation overlays
  sim_overlay?: SimOverlayState;
  fixed_point_equilibrium: FixedPointSnapshot;
  scenario_comparison?: ScenarioComparison;
  risk_timeline: Record<string, RiskTimelinePoint[]>;
  stability_envelope: StabilityEnvelope;
}
