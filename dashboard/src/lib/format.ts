// Pure helpers: convert raw backend numbers into human-readable insights.
import type { CollapseZone, ServiceBundle, TickPayload } from "./types";

export const pct = (v: number, digits = 0): string =>
  `${(v * 100).toFixed(digits)}%`;

export const fixed = (v: number, digits = 1): string =>
  Number.isFinite(v) ? v.toFixed(digits) : "—";

export const ms = (v: number): string => {
  if (!Number.isFinite(v) || v < 0) return "—";
  if (v < 1) return "<1ms";
  if (v < 1000) return `${Math.round(v)}ms`;
  return `${(v / 1000).toFixed(1)}s`;
};

export const collapseEta = (predictedMs: number): string | null => {
  if (!Number.isFinite(predictedMs) || predictedMs < 0) return null;
  if (predictedMs < 1000) return "<1s";
  if (predictedMs < 60_000) return `~${Math.round(predictedMs / 1000)}s`;
  return `~${Math.round(predictedMs / 60_000)}m`;
};

export const zoneColor = (
  z: CollapseZone,
): "safe" | "warn" | "crit" => {
  if (z === "collapse") return "crit";
  if (z === "warning") return "warn";
  return "safe";
};

export const riskZone = (risk: number): CollapseZone => {
  if (risk >= 0.7) return "collapse";
  if (risk >= 0.4) return "warning";
  return "safe";
};

export const trendLabel = (trend: number): string => {
  if (trend > 0.05) return "rising";
  if (trend < -0.05) return "falling";
  return "steady";
};

export interface OverviewStats {
  total: number;
  unstable: number;
  unstablePct: number;
  maxCollapseRisk: number;
  health: number; // 0..1
  worstService: string | null;
}

export const computeOverview = (
  tick: TickPayload | null,
): OverviewStats => {
  if (!tick) {
    return {
      total: 0,
      unstable: 0,
      unstablePct: 0,
      maxCollapseRisk: 0,
      health: 1,
      worstService: null,
    };
  }
  const bundles = Object.values(tick.bundles ?? {});
  const total = bundles.length;
  let unstable = 0;
  let maxRisk = 0;
  let worst: string | null = null;
  for (const b of bundles) {
    if (b.stability.is_unstable) unstable++;
    if (b.stability.collapse_risk > maxRisk) {
      maxRisk = b.stability.collapse_risk;
      worst = b.stability.service_id;
    }
  }
  // Composite health: 1 - weighted combination; clamp.
  const obj = tick.objective;
  const health = clamp(
    1 -
      0.5 * obj.max_collapse_risk -
      0.3 * obj.cascade_failure_probability -
      0.2 * (tick.degraded_fraction ?? 0),
    0,
    1,
  );
  return {
    total,
    unstable,
    unstablePct: total > 0 ? unstable / total : 0,
    maxCollapseRisk: maxRisk,
    health,
    worstService: worst,
  };
};

export const clamp = (v: number, lo: number, hi: number): number =>
  Math.min(hi, Math.max(lo, v));

export const sortByRisk = (
  bundles: Record<string, ServiceBundle>,
): ServiceBundle[] =>
  Object.values(bundles).sort(
    (a, b) => b.stability.collapse_risk - a.stability.collapse_risk,
  );
