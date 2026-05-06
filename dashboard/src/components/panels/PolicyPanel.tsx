"use client";

import { useState } from "react";
import type { TickPayload } from "@/types/tick";
import type { PolicyPreset } from "@/types/api";
import { Card, Badge, Button, StatRow, EmptyState, ErrorBanner } from "@/components/ui";
import { api } from "@/services/api";
import { useMutation } from "@/hooks/useMutation";
import { pct, ms } from "@/services/format";

interface Props {
  tick: TickPayload;
}

const PRESETS: PolicyPreset[] = ["balanced", "aggressive", "conservative", "safe", "performance"];

export function PolicyPanel({ tick }: Props) {
  const [selectedPreset, setSelectedPreset] = useState<PolicyPreset>("balanced");
  const policyMutation = useMutation((preset: PolicyPreset) => api.updatePolicy({ preset }));

  const cp = tick.control_plane;
  const directives = tick.directives ?? {};
  const obj = tick.objective;
  const topo = tick.topology;

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      <div className="grid grid-cols-3 gap-3">
        {/* Policy preset */}
        <Card title="Policy Preset" className="col-span-1">
          <div className="p-4 flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <span className="text-xs text-text-tertiary">Active preset</span>
              <Badge variant="info">{cp.policy_preset}</Badge>
            </div>
            <div className="flex flex-col gap-1.5">
              {PRESETS.map((p) => (
                <button
                  key={p}
                  onClick={() => setSelectedPreset(p)}
                  className={`text-left px-3 py-1.5 rounded text-xs font-mono transition-colors ${
                    selectedPreset === p
                      ? "bg-brand/15 text-brand border border-brand/30"
                      : "text-text-secondary hover:bg-surface-2 border border-transparent"
                  }`}
                >
                  {p}
                  {p === cp.policy_preset && <span className="ml-2 text-[10px] text-text-tertiary">(active)</span>}
                </button>
              ))}
            </div>
            <Button
              variant="primary"
              loading={policyMutation.loading}
              onClick={() => policyMutation.mutate(selectedPreset)}
            >
              Apply {selectedPreset}
            </Button>
            {policyMutation.error && <ErrorBanner message={policyMutation.error} />}
            {policyMutation.data && (
              <span className="text-xs text-success">Applied: {policyMutation.data.preset}</span>
            )}
          </div>
        </Card>

        {/* Objective weights */}
        <Card title="Objective Weights" className="col-span-1">
          <div className="p-4 flex flex-col gap-3">
            <StatRow label="Latency weight" value={obj?.latency_weight?.toFixed(4) ?? "—"} />
            <StatRow label="Utilisation weight" value={obj?.utilisation_weight?.toFixed(4) ?? "—"} />
            <StatRow label="Risk weight" value={obj?.risk_weight?.toFixed(4) ?? "—"} />
            <StatRow label="Predictive horizon" value={obj?.predictive_horizon ?? "—"} />
            <StatRow label="Reference latency" value={ms(obj?.reference_latency_ms ?? 0)} />
            <StatRow label="Trend stability margin" value={(obj?.trend_stability_margin ?? 0).toFixed(4)} />
          </div>
        </Card>

        {/* Critical path */}
        <Card title="Critical Path" className="col-span-1">
          <div className="p-4 flex flex-col gap-3">
            {topo?.CriticalPath?.Nodes?.length ? (
              <>
                <div className="flex flex-wrap gap-1">
                  {topo.CriticalPath.Nodes.map((n, i) => (
                    <span key={n}>
                      <span className="font-mono text-[10px] text-brand">{n}</span>
                      {i < topo.CriticalPath.Nodes.length - 1 && (
                        <span className="text-text-tertiary mx-1">→</span>
                      )}
                    </span>
                  ))}
                </div>
                <StatRow label="Total weight" value={topo.CriticalPath.TotalWeight.toFixed(4)} />
                <StatRow label="Cascade risk" value={pct(topo.CriticalPath.CascadeRisk)} />
              </>
            ) : (
              <EmptyState message="No critical path data" />
            )}
          </div>
        </Card>
      </div>

      {/* Active directives summary */}
      <Card title="Active Control Directives">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "Active", "Scale", "Target ρ", "Error", "PID Out", "Stability Margin", "Predictive Target", "Planner Score"].map((h) => (
                  <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Object.keys(directives).length === 0 ? (
                <tr><td colSpan={9} className="px-3 py-4 text-center text-text-tertiary">No directives</td></tr>
              ) : (
                Object.entries(directives).map(([id, d]) => (
                  <tr key={id} className={`border-b border-surface-2 hover:bg-surface-2 ${!d.Active ? "opacity-40" : ""}`}>
                    <td className="px-3 py-1.5 text-text-primary">{id}</td>
                    <td className="px-3 py-1.5">{d.Active ? <Badge variant="success" size="xs">ON</Badge> : <Badge variant="muted" size="xs">OFF</Badge>}</td>
                    <td className="px-3 py-1.5 text-brand">{d.ScaleFactor.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.TargetUtilisation.toFixed(4)}</td>
                    <td className={`px-3 py-1.5 ${Math.abs(d.Error) > 0.1 ? "text-warning" : "text-text-secondary"}`}>{d.Error.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.PIDOutput.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.StabilityMargin.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.PredictiveTarget.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.PlannerProbabilisticScore.toFixed(4)}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Topology sensitivity */}
      {tick.topology_sensitivity && (
        <Card title="Topology Sensitivity">
          <div className="p-4 grid grid-cols-2 gap-4">
            <div className="flex flex-col gap-2">
              <StatRow label="System fragility" value={tick.topology_sensitivity.system_fragility.toFixed(4)} />
              <StatRow label="Max amplification score" value={tick.topology_sensitivity.max_amplification_score.toFixed(4)} />
              {tick.topology_sensitivity.keystone_services?.length > 0 && (
                <StatRow label="Keystone services" value={
                  <span className="font-mono text-warning text-[10px]">
                    {tick.topology_sensitivity.keystone_services.join(", ")}
                  </span>
                } />
              )}
              {tick.topology_sensitivity.max_amplification_path?.length > 0 && (
                <StatRow label="Max amp path" value={
                  <span className="font-mono text-[10px] text-brand">
                    {tick.topology_sensitivity.max_amplification_path.join(" → ")}
                  </span>
                } />
              )}
            </div>
            <div className="flex flex-col gap-1 overflow-y-auto max-h-48">
              {Object.entries(tick.topology_sensitivity.by_service ?? {}).map(([id, s]) => (
                <div key={id} className="flex items-center gap-2 py-1 border-b border-surface-2 last:border-0 text-[10px] font-mono">
                  <span className="text-text-primary truncate flex-1">{id}</span>
                  {s.is_keystone && <Badge variant="warning" size="xs">KEY</Badge>}
                  <span className="text-text-tertiary">perturb {s.perturbation_score.toFixed(3)}</span>
                  <span className="text-text-tertiary">↓{s.downstream_reach}</span>
                </div>
              ))}
            </div>
          </div>
        </Card>
      )}
    </div>
  );
}
