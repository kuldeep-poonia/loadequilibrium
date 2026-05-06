// Types derived directly from backend Go structs in internal/streaming/types.go,
// internal/reasoning/types.go, internal/simulation/types.go,
// internal/telemetry/types.go, internal/optimisation/types.go,
// internal/modelling/types.go, internal/topology/types.go

export type MessageType = "tick" | "ping";

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

export type PredictiveRiskTimeline = Record<string, RiskTimelinePoint[]>;

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

export interface Evidence {
  utilisation?: number;
  collapse_risk?: number;
  oscillation_risk?: number;
  queue_wait_ms?: number;
  saturation_sec?: number;
  burst_factor?: number;
  cascade_risk?: number;
  stability_margin?: number;
  composite_score?: number;
}

export interface ReasoningEvent {
  id: string;
  timestamp: string;
  service_id?: string;
  severity: number;
  category: string;
  description: string;
  recommendation?: string;
  evidence: Evidence;
  uncertainty_score: number;
  operational_priority: number;
  model_chain?: string;
}

export interface QueueModel {
  service_id: string;
  computed_at: string;
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
  saturation_horizon: number;
  confidence: number;
  upstream_pressure: number;
  network_saturation_horizon: number;
  hazard: number;
  reservoir: number;
}

export interface StochasticModel {
  service_id: string;
  computed_at: string;
  arrival_co_v: number;
  burst_amplification: number;
  risk_propagation: number;
  confidence: number;
}

export interface SignalState {
  service_id: string;
  computed_at: string;
  fast_ewma: number;
  slow_ewma: number;
  ewma_variance: number;
  spike_detected: boolean;
  change_point_detected: boolean;
  cusum_pos: number;
  cusum_neg: number;
}

export interface StabilityAssessment {
  service_id: string;
  computed_at: string;
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

export interface ServiceModelBundle {
  Queue: QueueModel;
  Stochastic: StochasticModel;
  Signal: SignalState;
  Stability: StabilityAssessment;
}

export interface TopologyNode {
  ServiceID: string;
  LastSeen: string;
  NormalisedLoad: number;
}

export interface TopologyEdge {
  Source: string;
  Target: string;
  CallRate: number;
  ErrorRate: number;
  LatencyMs: number;
  Weight: number;
  LastUpdated: string;
}

export interface CriticalPath {
  Nodes: string[];
  TotalWeight: number;
  CascadeRisk: number;
}

export interface GraphSnapshot {
  CapturedAt: string;
  Nodes: TopologyNode[];
  Edges: TopologyEdge[];
  CriticalPath: CriticalPath;
}

export interface ObjectiveScore {
  computed_at: string;
  predicted_p99_latency_ms: number;
  cascade_failure_probability: number;
  weighted_stability_margin: number;
  max_collapse_risk: number;
  oscillation_risk: number;
  composite_score: number;
  latency_weight: number;
  utilisation_weight: number;
  risk_weight: number;
  predictive_horizon: number;
  reference_latency_ms: number;
  trend_stability_margin: number;
  risk_acceleration: number;
  trajectory_score: number;
}

export interface ControlDirective {
  ServiceID: string;
  ComputedAt: string;
  ScaleFactor: number;
  TargetUtilisation: number;
  Error: number;
  PIDOutput: number;
  Active: boolean;
  StabilityMargin: number;
  HysteresisState: string;
  ActuationBound: number;
  PredictiveTarget: number;
  MPCPredictedRho: number;
  MPCOvershootRisk: boolean;
  MPCUnderactuationRisk: boolean;
  CostGradient: number;
  TrajectoryCostAvg: number;
  MaxTrajectoryCost: number;
  PlannerScaleFactor: number;
  PlannerConvergent: boolean;
  PlannerConvex: boolean;
  PlannerProbabilisticScore: number;
}

export interface ServiceOutcome {
  ServiceID: string;
  FinalQueueLen: number;
  PeakQueueLen: number;
  ThroughputRatio: number;
  MeanWaitMs: number;
  Saturated: boolean;
  PeakUtilisation: number;
  RecoveryTimeMs: number;
  QueueLenMean: number;
  QueueLenVariance: number;
  FinalHazard: number;
  FinalReservoir: number;
}

export interface SimulationMeta {
  WallTimeMs: number;
  BudgetUsedPct: number;
  EventsPerMs: number;
}

export interface SimulationResult {
  Services: Record<string, ServiceOutcome>;
  HorizonMs: number;
  CascadeTriggered: boolean;
  EventsProcessed: number;
  Meta: SimulationMeta;
  SystemStable: boolean;
  CollapseDetected: boolean;
  RecoveryConvergenceMs: number;
  DegradedServiceCount: number;
  CascadeFailureProbability: Record<string, number>;
  SLAViolationProbability: Record<string, number>;
}

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

export interface ServiceSensSnap {
  perturbation_score: number;
  downstream_reach: number;
  upstream_exposure: number;
  is_keystone: boolean;
}

export interface TopologySensitivitySnapshot {
  system_fragility: number;
  max_amplification_path: string[];
  max_amplification_score: number;
  keystone_services: string[];
  by_service: Record<string, ServiceSensSnap>;
}

export interface RiskQueueItem {
  service_id: string;
  urgency_score: number;
  collapse_risk: number;
  rho: number;
  is_keystone: boolean;
  path_collapse_prob: number;
  urgency_class: "critical" | "warning" | "elevated" | "nominal";
}

export interface SimOverlayState {
  cascade_failure_prob: Record<string, number>;
  p95_queue_len: Record<string, number>;
  saturation_frac: Record<string, number>;
  sla_violation_prob: Record<string, number>;
  horizon_ms: number;
  sim_tick_age: number;
}

export interface FixedPointSnapshot {
  equilibrium_rho: Record<string, number>;
  systemic_collapse_prob: number;
  converged_iterations: number;
  converged: boolean;
  perturbation_sensitivity: Record<string, number>;
  convergence_rate: number;
  stability_margin: number;
}

export interface ScenarioComparisonSnapshot {
  scenario_count: number;
  best_case_collapse: number;
  worst_case_collapse: number;
  median_sla_violation: number;
  stable_scenario_fraction: number;
  recovery_convergence_min_ms: number;
  recovery_convergence_max_ms: number;
}

export interface StabilityEnvelopeSnapshot {
  safe_system_rho_max: number;
  current_system_rho_mean: number;
  envelope_headroom: number;
  most_vulnerable_service: string;
  worst_perturbation_delta: number;
}

export interface TopologyDiff {
  added_nodes: string[];
  removed_nodes: string[];
  added_edges: string[];
  removed_edges: string[];
}

export interface TickPayload {
  type: MessageType;
  seq: number;
  ts: string;
  schema_version: number;
  bundles: Record<string, ServiceModelBundle>;
  topology: GraphSnapshot;
  topo_diff: TopologyDiff;
  objective: ObjectiveScore;
  directives: Record<string, ControlDirective>;
  events: ReasoningEvent[];
  sim_result?: SimulationResult;
  degraded_services?: string[];
  sat_countdowns?: Record<string, number>;
  stability_zones?: Record<string, string>;
  prediction_horizon?: Record<string, number>;
  prediction_timeline?: Record<string, PredictionPoint[]>;
  tick_health_ms: number;
  degraded_fraction: number;
  safety_mode: boolean;
  jitter_ms: number;
  runtime_metrics: RuntimeMetrics;
  control_plane: ControlPlaneState;
  network_coupling?: Record<string, NetworkCouplingSnapshot>;
  network_equilibrium?: NetworkEquilibriumSnapshot;
  topology_sensitivity?: TopologySensitivitySnapshot;
  priority_risk_queue?: RiskQueueItem[];
  pressure_heatmap?: Record<string, number>;
  sim_overlay?: SimOverlayState;
  fixed_point_equilibrium?: FixedPointSnapshot;
  scenario_comparison?: ScenarioComparisonSnapshot;
  risk_timeline?: PredictiveRiskTimeline;
  stability_envelope?: StabilityEnvelopeSnapshot;
}
