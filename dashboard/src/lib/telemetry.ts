/**
 * LOADEQUILIBRIUM TELEMETRY ENGINE (TS)
 * Normalizer: camelCase -> snake_case + safe defaults for TickPayload fields.
 */

import { TickPayload } from '@/types/backend';

type MutableRecord = Record<string, unknown>;

function toSnakeCase(key: string): string {
  return key
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1_$2')
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .toLowerCase();
}

function asRecord(value: unknown): MutableRecord {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as MutableRecord : {};
}

function normaliseSeverity(value: unknown) {
  if (typeof value === 'number') {
    if (value >= 2) return 'critical';
    if (value >= 1) return 'warning';
    return 'info';
  }

  if (typeof value === 'string') {
    const lower = value.toLowerCase();
    if (lower === 'crit' || lower === 'critical') return 'critical';
    if (lower === 'warn' || lower === 'warning') return 'warning';
    if (lower === 'info' || lower === 'informational') return 'info';
    return lower;
  }

  return 'info';
}

export function normalizeObject(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => normalizeObject(item));
  }
  if (value === null || typeof value !== 'object') {
    return value;
  }
  if (value instanceof Date) {
    return value;
  }

  const normalized: MutableRecord = {};
  for (const [key, nestedValue] of Object.entries(value)) {
    const nextKey = /[A-Z]/.test(key) ? toSnakeCase(key) : key;
    normalized[nextKey] = normalizeObject(nestedValue);
  }
  return normalized;
}

export function normalizeTickPayload(rawTick: unknown): TickPayload | null {
  if (!rawTick || typeof rawTick !== 'object') return null;
  const tick = normalizeObject(rawTick) as MutableRecord;

  tick.type = typeof tick.type === 'string' ? tick.type : 'tick';
  tick.seq = Number(tick.seq ?? tick.sequence_no ?? 0);
  tick.ts = typeof tick.ts === 'string' ? tick.ts : String(tick.timestamp || new Date().toISOString());
  tick.schema_version = Number(tick.schema_version ?? 0);

  tick.bundles = asRecord(tick.bundles);

  const topology = asRecord(tick.topology);
  topology.captured_at = typeof topology.captured_at === 'string' ? topology.captured_at : '';
  topology.nodes = Array.isArray(topology.nodes) ? topology.nodes : [];
  topology.edges = Array.isArray(topology.edges) ? topology.edges : [];
  topology.critical_path = Object.keys(asRecord(topology.critical_path)).length
    ? topology.critical_path
    : { nodes: [], total_weight: 0, cascade_risk: 0 };
  tick.topology = topology;

  tick.objective = Object.keys(asRecord(tick.objective)).length
    ? tick.objective
    : {
        composite_score: 0,
        cascade_failure_probability: 0,
        predicted_p99_latency_ms: 0,
        max_collapse_risk: 0,
        trajectory_score: 0,
      };
  tick.directives = asRecord(tick.directives);
  tick.events = Array.isArray(tick.events)
    ? tick.events.map((event) => {
        const normalizedEvent = asRecord(event);
        normalizedEvent.severity = normaliseSeverity(normalizedEvent.severity);
        return normalizedEvent;
      })
    : [];

  tick.degraded_services = Array.isArray(tick.degraded_services) ? tick.degraded_services : [];
  tick.sat_countdowns = asRecord(tick.sat_countdowns);
  tick.stability_zones = asRecord(tick.stability_zones);
  tick.prediction_horizon = asRecord(tick.prediction_horizon);
  tick.prediction_timeline = asRecord(tick.prediction_timeline);

  tick.tick_health_ms = Number(tick.tick_health_ms ?? 0);
  tick.degraded_fraction = Number(tick.degraded_fraction ?? 0);
  tick.safety_mode = Boolean(tick.safety_mode ?? false);
  tick.jitter_ms = Number(tick.jitter_ms ?? 0);
  tick.runtime_metrics = asRecord(tick.runtime_metrics);
  tick.control_plane = Object.keys(asRecord(tick.control_plane)).length
    ? tick.control_plane
    : {
        tick: tick.seq,
        actuation_enabled: true,
        policy_preset: 'balanced',
        forced_sandbox_until: 0,
        forced_simulation_until: 0,
        forced_intelligence_until: 0,
        simulation_reset_pending: false,
        acknowledged_alert_count: 0,
      };

  tick.network_coupling = asRecord(tick.network_coupling);
  tick.network_equilibrium = Object.keys(asRecord(tick.network_equilibrium)).length
    ? tick.network_equilibrium
    : { system_rho_mean: 0, system_rho_variance: 0, is_converging: false, network_saturation_risk: 0 };
  tick.topology_sensitivity = Object.keys(asRecord(tick.topology_sensitivity)).length
    ? tick.topology_sensitivity
    : { system_fragility: 0, keystone_services: [], by_service: {} };

  tick.priority_risk_queue = Array.isArray(tick.priority_risk_queue) ? tick.priority_risk_queue : [];
  tick.pressure_heatmap = asRecord(tick.pressure_heatmap);

  tick.fixed_point_equilibrium = Object.keys(asRecord(tick.fixed_point_equilibrium)).length
    ? tick.fixed_point_equilibrium
    : { systemic_collapse_prob: 0, converged: false };
  tick.risk_timeline = asRecord(tick.risk_timeline);
  tick.stability_envelope = Object.keys(asRecord(tick.stability_envelope)).length
    ? tick.stability_envelope
    : { safe_system_rho_max: 0, current_system_rho_mean: 0, envelope_headroom: 0 };

  return tick as unknown as TickPayload;
}
