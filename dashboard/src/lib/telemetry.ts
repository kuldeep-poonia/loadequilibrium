/**
 * LOADEQUILIBRIUM TELEMETRY ENGINE (TS)
 * Normalizer: camelCase → snake_case + safe defaults for all TickPayload fields
 */

import { TickPayload } from "@/types/backend";

function toSnakeCase(key: string): string {
  return key
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1_$2")
    .replace(/([a-z0-9])([A-Z])/g, "$1_$2")
    .toLowerCase();
}

export function normalizeObject(value: any): any {
  if (Array.isArray(value)) {
    return value.map((item) => normalizeObject(item));
  }
  if (value === null || typeof value !== "object") {
    return value;
  }
  if (value instanceof Date) {
    return value;
  }

  const normalized: any = {};
  for (const [key, nestedValue] of Object.entries(value)) {
    const nextKey = /[A-Z]/.test(key) ? toSnakeCase(key) : key;
    normalized[nextKey] = normalizeObject(nestedValue);
  }
  return normalized;
}

export function normalizeTickPayload(rawTick: any): TickPayload | null {
  if (!rawTick) return null;
  const tick = normalizeObject(rawTick);

  // Identity
  tick.type = tick.type || "tick";
  tick.seq = tick.seq ?? tick.sequence_no ?? 0;
  tick.ts = tick.ts || tick.timestamp || new Date().toISOString();
  tick.schema_version = tick.schema_version ?? 0;

  // Core data — safe defaults
  tick.bundles = tick.bundles || {};
  tick.topology = tick.topology || { captured_at: '', nodes: [], edges: [], critical_path: { nodes: [], total_weight: 0, cascade_risk: 0 } };
  tick.topology.nodes = tick.topology.nodes || [];
  tick.topology.edges = tick.topology.edges || [];
  tick.topology.critical_path = tick.topology.critical_path || { nodes: [], total_weight: 0, cascade_risk: 0 };
  tick.objective = tick.objective || { composite_score: 0, cascade_failure_probability: 0, predicted_p99_latency_ms: 0, max_collapse_risk: 0, trajectory_score: 0 };
  tick.directives = tick.directives || {};
  tick.events = Array.isArray(tick.events) ? tick.events : [];

  // Overlay intelligence
  tick.degraded_services = Array.isArray(tick.degraded_services) ? tick.degraded_services : [];
  tick.sat_countdowns = tick.sat_countdowns || {};
  tick.stability_zones = tick.stability_zones || {};
  tick.prediction_horizon = tick.prediction_horizon || {};
  tick.prediction_timeline = tick.prediction_timeline || {};

  // Runtime health
  tick.tick_health_ms = tick.tick_health_ms ?? 0;
  tick.degraded_fraction = tick.degraded_fraction ?? 0;
  tick.safety_mode = tick.safety_mode ?? false;
  tick.jitter_ms = tick.jitter_ms ?? 0;
  tick.runtime_metrics = tick.runtime_metrics || {};

  // Network analysis
  tick.network_coupling = tick.network_coupling || {};
  tick.network_equilibrium = tick.network_equilibrium || { system_rho_mean: 0, system_rho_variance: 0, is_converging: false, network_saturation_risk: 0 };
  tick.topology_sensitivity = tick.topology_sensitivity || { system_fragility: 0, keystone_services: [], by_service: {} };

  // Risk
  tick.priority_risk_queue = Array.isArray(tick.priority_risk_queue) ? tick.priority_risk_queue : [];
  tick.pressure_heatmap = tick.pressure_heatmap || {};

  // Simulation
  tick.fixed_point_equilibrium = tick.fixed_point_equilibrium || { systemic_collapse_prob: 0, converged: false };
  tick.risk_timeline = tick.risk_timeline || {};
  tick.stability_envelope = tick.stability_envelope || { safe_system_rho_max: 0, current_system_rho_mean: 0, envelope_headroom: 0 };
  // sim_result, sim_overlay, scenario_comparison, topo_diff are optional — leave as-is

  return tick as TickPayload;
}
