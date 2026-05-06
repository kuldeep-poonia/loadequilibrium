"use client";

import { useMemo } from "react";
import type { TickPayload, ReasoningEvent } from "@/types/tick";
import { Card, Badge, EmptyState } from "@/components/ui";
import { severityLabel, severityVariant, relativeTime, pct } from "@/services/format";

interface Props {
  tick: TickPayload;
  history: TickPayload[];
}

function EventLogRow({ event }: { event: ReasoningEvent }) {
  return (
    <div className="flex items-start gap-3 py-2 px-3 border-b border-surface-2 last:border-0 font-mono text-[10px] hover:bg-surface-2 transition-colors">
      <span className="text-text-tertiary shrink-0 w-16">{relativeTime(event.timestamp)}</span>
      <Badge variant={severityVariant(event.severity)} size="xs">{severityLabel(event.severity)}</Badge>
      {event.service_id && (
        <span className="text-brand shrink-0 w-28 truncate">{event.service_id}</span>
      )}
      <span className="text-text-primary flex-1">{event.description}</span>
      {event.evidence?.composite_score !== undefined && event.evidence.composite_score > 0 && (
        <span className="text-text-tertiary shrink-0">{event.evidence.composite_score.toFixed(4)}</span>
      )}
    </div>
  );
}

export function LogsPanel({ tick }: Props) {
  const events = useMemo(() => tick.events ?? [], [tick.events]);
  const topology = tick.topology;
  const rm = tick.runtime_metrics;
  const cp = tick.control_plane;

  const criticalEvents = events.filter((e) => e.severity === 2);
  const warnEvents = events.filter((e) => e.severity === 1);
  const infoEvents = events.filter((e) => e.severity === 0);

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      {/* Event log */}
      <Card title={`Event Log (${events.length} events)`}>
        <div className="overflow-y-auto max-h-[300px]">
          {events.length === 0 ? (
            <div className="p-4"><EmptyState message="No events this tick" /></div>
          ) : (
            events.map((e) => <EventLogRow key={`${e.id}-${e.timestamp}`} event={e} />)
          )}
        </div>
      </Card>

      <div className="grid grid-cols-3 gap-3">
        {/* Critical events summary */}
        <Card title="Critical Events">
          <div className="overflow-y-auto max-h-40">
            {criticalEvents.length === 0 ? (
              <div className="p-3"><EmptyState message="None" /></div>
            ) : (
              criticalEvents.map((e) => (
                <div key={e.id} className="px-3 py-1.5 border-b border-surface-2 last:border-0 text-[10px] font-mono text-danger">
                  {e.service_id ? `[${e.service_id}] ` : ""}{e.description.slice(0, 80)}
                </div>
              ))
            )}
          </div>
        </Card>

        <Card title="Warnings">
          <div className="overflow-y-auto max-h-40">
            {warnEvents.length === 0 ? (
              <div className="p-3"><EmptyState message="None" /></div>
            ) : (
              warnEvents.map((e) => (
                <div key={e.id} className="px-3 py-1.5 border-b border-surface-2 last:border-0 text-[10px] font-mono text-warning">
                  {e.service_id ? `[${e.service_id}] ` : ""}{e.description.slice(0, 80)}
                </div>
              ))
            )}
          </div>
        </Card>

        <Card title="Info Events">
          <div className="overflow-y-auto max-h-40">
            {infoEvents.length === 0 ? (
              <div className="p-3"><EmptyState message="None" /></div>
            ) : (
              infoEvents.map((e) => (
                <div key={e.id} className="px-3 py-1.5 border-b border-surface-2 last:border-0 text-[10px] font-mono text-text-secondary">
                  {e.service_id ? `[${e.service_id}] ` : ""}{e.description.slice(0, 80)}
                </div>
              ))
            )}
          </div>
        </Card>
      </div>

      {/* Topology audit */}
      <Card title="Topology Audit">
        <div className="p-4 grid grid-cols-2 gap-4">
          <div>
            <span className="text-xs text-text-tertiary uppercase tracking-wider block mb-2">Nodes ({topology?.Nodes?.length ?? 0})</span>
            <div className="flex flex-col gap-0.5 max-h-36 overflow-y-auto font-mono text-[10px]">
              {(topology?.Nodes ?? []).map((n) => (
                <div key={n.ServiceID} className="flex items-center gap-2 py-0.5">
                  <span className="text-text-primary">{n.ServiceID}</span>
                  <span className="text-text-tertiary ml-auto">{(n.NormalisedLoad * 100).toFixed(1)}%</span>
                </div>
              ))}
            </div>
          </div>
          <div>
            <span className="text-xs text-text-tertiary uppercase tracking-wider block mb-2">Edges ({topology?.Edges?.length ?? 0})</span>
            <div className="flex flex-col gap-0.5 max-h-36 overflow-y-auto font-mono text-[10px]">
              {(topology?.Edges ?? []).map((e, i) => (
                <div key={i} className="flex items-center gap-1 py-0.5">
                  <span className="text-text-primary">{e.Source}</span>
                  <span className="text-text-tertiary">→</span>
                  <span className="text-brand">{e.Target}</span>
                  <span className="text-text-tertiary ml-auto">{pct(e.ErrorRate, 0)} err</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </Card>

      {/* Runtime metrics audit */}
      <Card title="Runtime Audit — Control Plane State">
        <div className="p-4 grid grid-cols-2 gap-4">
          <div className="font-mono text-[10px] flex flex-col gap-1">
            <div className="grid grid-cols-2 gap-x-4">
              <span className="text-text-tertiary">tick</span><span className="text-text-primary">{cp.tick}</span>
              <span className="text-text-tertiary">policy</span><span className="text-brand">{cp.policy_preset}</span>
              <span className="text-text-tertiary">actuation</span><span className={cp.actuation_enabled ? "text-success" : "text-muted"}>{cp.actuation_enabled ? "enabled" : "disabled"}</span>
              <span className="text-text-tertiary">acked alerts</span><span className="text-text-primary">{cp.acknowledged_alert_count}</span>
              <span className="text-text-tertiary">sim reset pending</span><span className={cp.simulation_reset_pending ? "text-warning" : "text-text-tertiary"}>{String(cp.simulation_reset_pending)}</span>
              <span className="text-text-tertiary">safety mode</span><span className={tick.safety_mode ? "text-danger" : "text-text-tertiary"}>{String(tick.safety_mode)}</span>
              <span className="text-text-tertiary">safety level</span><span className="text-text-primary">{rm?.safety_level ?? 0}/3</span>
              <span className="text-text-tertiary">forced sim until</span><span className="text-text-secondary">{cp.forced_simulation_until || "—"}</span>
              <span className="text-text-tertiary">forced sandbox until</span><span className="text-text-secondary">{cp.forced_sandbox_until || "—"}</span>
              <span className="text-text-tertiary">forced intel until</span><span className="text-text-secondary">{cp.forced_intelligence_until || "—"}</span>
            </div>
          </div>
          <div className="font-mono text-[10px] flex flex-col gap-1">
            <span className="text-text-tertiary uppercase tracking-wider block mb-1">Pipeline EWMA (ms)</span>
            {[
              ["prune", rm?.avg_prune_ms],
              ["windows", rm?.avg_windows_ms],
              ["topology", rm?.avg_topology_ms],
              ["coupling", rm?.avg_coupling_ms],
              ["modelling", rm?.avg_modelling_ms],
              ["optimise", rm?.avg_optimise_ms],
              ["sim", rm?.avg_sim_ms],
              ["reasoning", rm?.avg_reasoning_ms],
              ["broadcast", rm?.avg_broadcast_ms],
            ].map(([label, val]) => (
              <div key={label as string} className="grid grid-cols-2 gap-x-4">
                <span className="text-text-tertiary">{label as string}</span>
                <span className="text-text-secondary">{(val as number ?? 0).toFixed(3)}ms</span>
              </div>
            ))}
          </div>
        </div>
      </Card>

      {/* Model chain audit */}
      {events.some((e) => e.model_chain) && (
        <Card title="Decision Model Chains">
          <div className="p-4 flex flex-col gap-2">
            {events.filter((e) => e.model_chain).map((e) => (
              <div key={e.id} className="flex flex-col gap-1 p-2 rounded border border-surface-3 bg-surface-2">
                <div className="flex items-center gap-2">
                  <Badge variant={severityVariant(e.severity)} size="xs">{severityLabel(e.severity)}</Badge>
                  {e.service_id && <span className="font-mono text-[10px] text-brand">{e.service_id}</span>}
                </div>
                <span className="font-mono text-[10px] text-brand/70">{e.model_chain}</span>
                <span className="text-[10px] text-text-secondary">{e.description}</span>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}
