"use client";

import { useCallback, useMemo } from "react";
import type { TickPayload, ReasoningEvent } from "@/types/tick";
import { Card, Badge, Button, EmptyState } from "@/components/ui";
import { severityLabel, severityVariant, relativeTime, pct } from "@/services/format";
import { api } from "@/services/api";
import { useMutation } from "@/hooks/useMutation";

interface Props {
  tick: TickPayload;
}

function EventRow({ event, onAck }: { event: ReasoningEvent; onAck: (id: string) => void }) {
  const sev = severityLabel(event.severity);
  const variant = severityVariant(event.severity);
  const ev = event.evidence;
  return (
    <div className="flex flex-col gap-1.5 p-3 border-b border-surface-2 last:border-0 hover:bg-surface-2 transition-colors">
      <div className="flex items-start gap-2">
        <Badge variant={variant} size="xs">{sev}</Badge>
        {event.service_id && (
          <span className="font-mono text-[10px] text-brand truncate max-w-[140px]">{event.service_id}</span>
        )}
        <span className="text-[10px] text-text-tertiary ml-auto shrink-0">{relativeTime(event.timestamp)}</span>
      </div>
      <p className="text-xs text-text-primary leading-relaxed">{event.description}</p>
      {event.recommendation && (
        <p className="text-[10px] text-text-secondary italic">{event.recommendation}</p>
      )}
      <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] font-mono text-text-tertiary">
        {ev.utilisation !== undefined && ev.utilisation > 0 && <span>ρ {pct(ev.utilisation)}</span>}
        {ev.collapse_risk !== undefined && ev.collapse_risk > 0 && <span>collapse {pct(ev.collapse_risk)}</span>}
        {ev.cascade_risk !== undefined && ev.cascade_risk > 0 && <span>cascade {pct(ev.cascade_risk)}</span>}
        {ev.composite_score !== undefined && ev.composite_score > 0 && <span>score {ev.composite_score.toFixed(4)}</span>}
        {event.uncertainty_score > 0 && <span>uncertainty {pct(event.uncertainty_score)}</span>}
        {event.model_chain && <span className="text-brand/60">chain: {event.model_chain}</span>}
      </div>
      {event.id && (
        <div className="flex justify-end">
          <Button variant="ghost" size="sm" onClick={() => onAck(event.id)}>Acknowledge</Button>
        </div>
      )}
    </div>
  );
}

export function AnomalyPanel({ tick }: Props) {
  const events = useMemo(() => tick.events ?? [], [tick.events]);
  const ackMutation = useMutation((id: string) => api.alertAck({ alert_id: id }));

  const handleAck = useCallback((id: string) => { ackMutation.mutate(id); }, [ackMutation]);

  const critical = events.filter((e) => e.severity === 2);
  const warnings = events.filter((e) => e.severity === 1);
  const infos = events.filter((e) => e.severity === 0);

  const degraded = tick.degraded_services ?? [];
  const countdowns = tick.sat_countdowns ?? {};
  const heatmap = tick.pressure_heatmap ?? {};
  const zones = tick.stability_zones ?? {};

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      {/* Summary badges */}
      <div className="flex items-center gap-3">
        <Badge variant="critical">{critical.length} CRITICAL</Badge>
        <Badge variant="warning">{warnings.length} WARNING</Badge>
        <Badge variant="info">{infos.length} INFO</Badge>
        <span className="text-xs text-text-tertiary ml-2">
          {tick.control_plane.acknowledged_alert_count} acknowledged
        </span>
        {ackMutation.error && (
          <span className="text-xs text-danger ml-auto">{ackMutation.error}</span>
        )}
      </div>

      <div className="grid grid-cols-3 gap-3">
        {/* Event list */}
        <Card title="Events" className="col-span-2">
          <div className="overflow-y-auto max-h-[480px]">
            {events.length === 0 ? (
              <div className="p-4"><EmptyState message="No events" /></div>
            ) : (
              events.map((e) => (
                <EventRow key={e.id || e.timestamp} event={e} onAck={handleAck} />
              ))
            )}
          </div>
        </Card>

        <div className="flex flex-col gap-3">
          {/* Degraded services */}
          <Card title="Degraded Services">
            <div className="p-3 flex flex-col gap-1">
              {degraded.length === 0 ? (
                <EmptyState message="None" />
              ) : (
                degraded.map((svc) => (
                  <div key={svc} className="flex items-center gap-2 py-1 border-b border-surface-2 last:border-0">
                    <span className="w-2 h-2 rounded-full bg-danger shrink-0" />
                    <span className="font-mono text-[10px] text-text-primary truncate">{svc}</span>
                  </div>
                ))
              )}
            </div>
          </Card>

          {/* Saturation countdowns */}
          <Card title="Saturation Countdowns (s)">
            <div className="p-3 flex flex-col gap-1">
              {Object.keys(countdowns).length === 0 ? (
                <EmptyState message="None critical" />
              ) : (
                Object.entries(countdowns)
                  .sort(([, a], [, b]) => a - b)
                  .map(([svc, secs]) => (
                    <div key={svc} className="flex items-center gap-2 py-1 border-b border-surface-2 last:border-0">
                      <span className={`font-mono text-[10px] ${secs < 30 ? "text-danger" : secs < 120 ? "text-warning" : "text-text-secondary"}`}>
                        {secs.toFixed(0)}s
                      </span>
                      <span className="font-mono text-[10px] text-text-primary truncate flex-1">{svc}</span>
                    </div>
                  ))
              )}
            </div>
          </Card>

          {/* Stability zones */}
          <Card title="Stability Zones">
            <div className="p-3 flex flex-col gap-1 max-h-40 overflow-y-auto">
              {Object.keys(zones).length === 0 ? (
                <EmptyState message="No data" />
              ) : (
                Object.entries(zones).map(([svc, zone]) => (
                  <div key={svc} className="flex items-center gap-2 py-1 border-b border-surface-2 last:border-0">
                    <span className={`font-mono text-[10px] font-bold ${zone === "collapse" ? "text-danger" : zone === "warning" ? "text-warning" : "text-success"}`}>
                      {zone.toUpperCase()}
                    </span>
                    <span className="font-mono text-[10px] text-text-primary truncate">{svc}</span>
                  </div>
                ))
              )}
            </div>
          </Card>
        </div>
      </div>

      {/* Pressure heatmap */}
      {Object.keys(heatmap).length > 0 && (
        <Card title="Pressure Heatmap">
          <div className="p-3 grid grid-cols-2 gap-2 max-h-48 overflow-y-auto">
            {Object.entries(heatmap)
              .sort(([, a], [, b]) => b - a)
              .map(([svc, pressure]) => {
                const pctVal = Math.min(100, pressure * 100);
                const barColor = pctVal > 80 ? "bg-danger" : pctVal > 60 ? "bg-warning" : "bg-success";
                return (
                  <div key={svc} className="flex items-center gap-2">
                    <span className="font-mono text-[10px] text-text-secondary w-32 truncate shrink-0">{svc}</span>
                    <div className="flex-1 h-1.5 bg-surface-3 rounded-full overflow-hidden">
                      <div className={`h-full rounded-full transition-all duration-300 ${barColor}`} style={{ width: `${pctVal}%` }} />
                    </div>
                    <span className="font-mono text-[10px] text-text-primary w-10 text-right">{pressure.toFixed(3)}</span>
                  </div>
                );
              })}
          </div>
        </Card>
      )}

      {/* Network coupling anomalies */}
      {tick.network_coupling && Object.keys(tick.network_coupling).length > 0 && (
        <Card title="Network Coupling">
          <div className="overflow-x-auto">
            <table className="w-full text-[10px] font-mono">
              <thead>
                <tr className="border-b border-surface-3">
                  {["Service", "Eff Pressure", "Path Sat Risk", "Path Collapse", "Congestion FB", "Path Sat Horizon"].map((h) => (
                    <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {Object.entries(tick.network_coupling).map(([id, nc]) => (
                  <tr key={id} className="border-b border-surface-2 hover:bg-surface-2">
                    <td className="px-3 py-1.5 text-text-primary">{id}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{nc.effective_pressure.toFixed(4)}</td>
                    <td className={`px-3 py-1.5 ${nc.path_sat_risk > 0.7 ? "text-danger" : "text-text-secondary"}`}>{pct(nc.path_sat_risk)}</td>
                    <td className={`px-3 py-1.5 ${nc.path_collapse_prob > 0.5 ? "text-danger" : "text-text-secondary"}`}>{pct(nc.path_collapse_prob)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{nc.congestion_feedback.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{nc.path_sat_horizon_sec.toFixed(1)}s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}
