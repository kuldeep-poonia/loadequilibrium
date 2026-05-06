"use client";

import { useState } from "react";
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  ReferenceArea,
} from "recharts";
import type { TickPayload } from "@/types/tick";
import { Card, Badge, Button, EmptyState, ErrorBanner } from "@/components/ui";
import { pct, ms } from "@/services/format";
import { api } from "@/services/api";
import { useMutation } from "@/hooks/useMutation";

interface Props {
  tick: TickPayload;
}

const CHART_COLORS = ["#4f8ef7", "#22c55e", "#f59e0b", "#ef4444", "#a78bfa", "#ec4899", "#14b8a6"];

export function IntelligencePanel({ tick }: Props) {
  const [rolloutDuration, setRolloutDuration] = useState("10");
  const rolloutMutation = useMutation(() =>
    api.intelligenceRollout({ duration_ticks: +rolloutDuration })
  );

  const cp = tick.control_plane;
  const bundles = tick.bundles ?? {};
  const serviceIds = Object.keys(bundles);
  const predTimeline = tick.prediction_timeline ?? {};
  const riskTimeline = tick.risk_timeline ?? {};

  const isRolloutActive = cp.forced_intelligence_until > 0 && cp.forced_intelligence_until > cp.tick;

  // Merge prediction timeline into chart format
  const allTOffsets = new Set<number>();
  Object.values(predTimeline).forEach((pts) => pts.forEach((p) => allTOffsets.add(p.t)));
  const sortedOffsets = Array.from(allTOffsets).sort((a, b) => a - b);

  const predChartData = sortedOffsets.map((t) => {
    const row: Record<string, number> = { t };
    serviceIds.forEach((id) => {
      const pts = predTimeline[id] ?? [];
      const pt = pts.find((p) => p.t === t);
      if (pt) {
        row[`${id}_rho`] = pt.rho;
        row[`${id}_lo`] = pt.lo;
        row[`${id}_hi`] = pt.hi;
      }
    });
    return row;
  });

  // Risk timeline chart data
  const allRiskOffsets = new Set<number>();
  Object.values(riskTimeline).forEach((pts) => pts.forEach((p) => allRiskOffsets.add(p.t)));
  const sortedRiskOffsets = Array.from(allRiskOffsets).sort((a, b) => a - b);

  const riskChartData = sortedRiskOffsets.map((t) => {
    const row: Record<string, number> = { t };
    Object.entries(riskTimeline).forEach(([id, pts]) => {
      const pt = pts.find((p) => p.t === t);
      if (pt) {
        row[`${id}_risk`] = pt.risk;
        row[`${id}_rho`] = pt.rho;
      }
    });
    return row;
  });

  const shownServices = serviceIds.slice(0, CHART_COLORS.length);

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      {/* Rollout trigger */}
      <Card title="Intelligence Rollout">
        <div className="p-4 flex flex-col gap-3">
          <div className="flex items-center gap-2">
            <Badge variant={isRolloutActive ? "success" : "muted"}>
              {isRolloutActive ? `ACTIVE until tick ${cp.forced_intelligence_until}` : "IDLE"}
            </Badge>
          </div>
          <div className="flex items-center gap-3">
            <label className="text-xs text-text-tertiary">Duration ticks</label>
            <input
              type="number"
              value={rolloutDuration}
              onChange={(e) => setRolloutDuration(e.target.value)}
              className="w-20 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary"
              min="1" max="120"
            />
            <Button variant="primary" loading={rolloutMutation.loading} onClick={() => rolloutMutation.mutate(undefined)}>
              Force Rollout
            </Button>
          </div>
          {rolloutMutation.error && <ErrorBanner message={rolloutMutation.error} />}
          {rolloutMutation.data && (
            <span className="text-xs text-success font-mono">
              Scheduled until tick {rolloutMutation.data.until_tick}
            </span>
          )}
        </div>
      </Card>

      {/* Prediction timeline chart */}
      <Card title="Prediction Timeline (ρ horizon)">
        <div className="px-2 pt-1 pb-2 h-[220px]">
          {predChartData.length < 2 ? (
            <EmptyState message="No prediction timeline data" />
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={predChartData} margin={{ top: 4, right: 16, left: -20, bottom: 0 }}>
                <XAxis dataKey="t" tick={{ fill: "#64748b", fontSize: 10 }} label={{ value: "ticks from now", position: "insideBottomRight", offset: -4, fill: "#64748b", fontSize: 10 }} />
                <YAxis tick={{ fill: "#64748b", fontSize: 10 }} domain={[0, 1]} tickFormatter={(v: number) => v.toFixed(1)} />
                <Tooltip
                  contentStyle={{ background: "#10121a", border: "1px solid #1e2236", borderRadius: 6, fontSize: 11, color: "#e2e8f0" }}
                  formatter={(v: unknown) => [(v as number).toFixed(4), ""]}
                />
                <ReferenceArea y1={0.9} y2={1} fill="#ef4444" fillOpacity={0.08} />
                <Legend wrapperStyle={{ fontSize: 10, color: "#64748b" }} />
                {shownServices.map((id, i) => (
                  <Line
                    key={id}
                    type="monotone"
                    dataKey={`${id}_rho`}
                    stroke={CHART_COLORS[i]}
                    strokeWidth={1.5}
                    dot={false}
                    name={id}
                    isAnimationActive={false}
                  />
                ))}
              </LineChart>
            </ResponsiveContainer>
          )}
        </div>
      </Card>

      {/* Risk timeline chart */}
      <Card title="Risk Timeline (collapse risk runway)">
        <div className="px-2 pt-1 pb-2 h-[220px]">
          {riskChartData.length < 2 ? (
            <EmptyState message="No risk timeline data" />
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={riskChartData} margin={{ top: 4, right: 16, left: -20, bottom: 0 }}>
                <XAxis dataKey="t" tick={{ fill: "#64748b", fontSize: 10 }} label={{ value: "ticks from now", position: "insideBottomRight", offset: -4, fill: "#64748b", fontSize: 10 }} />
                <YAxis tick={{ fill: "#64748b", fontSize: 10 }} domain={[0, 1]} tickFormatter={(v: number) => pct(v, 0)} />
                <Tooltip
                  contentStyle={{ background: "#10121a", border: "1px solid #1e2236", borderRadius: 6, fontSize: 11, color: "#e2e8f0" }}
                  formatter={(v: unknown) => [pct(v as number), ""]}
                />
                <ReferenceArea y1={0.7} y2={1} fill="#ef4444" fillOpacity={0.08} />
                <Legend wrapperStyle={{ fontSize: 10, color: "#64748b" }} />
                {Object.keys(riskTimeline).slice(0, CHART_COLORS.length).map((id, i) => (
                  <Line
                    key={id}
                    type="monotone"
                    dataKey={`${id}_risk`}
                    stroke={CHART_COLORS[i]}
                    strokeWidth={1.5}
                    dot={false}
                    name={id}
                    isAnimationActive={false}
                  />
                ))}
              </LineChart>
            </ResponsiveContainer>
          )}
        </div>
      </Card>

      {/* Stochastic model insights */}
      <Card title="Stochastic Model — Burst & Propagation">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "Arrival CoV", "Burst Amplification", "Risk Propagation", "Confidence"].map((h) => (
                  <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {serviceIds.length === 0 ? (
                <tr><td colSpan={5} className="px-3 py-4 text-center text-text-tertiary">No data</td></tr>
              ) : (
                serviceIds.map((id) => {
                  const s = bundles[id].Stochastic;
                  return (
                    <tr key={id} className="border-b border-surface-2 hover:bg-surface-2">
                      <td className="px-3 py-1.5 text-text-primary">{id}</td>
                      <td className={`px-3 py-1.5 ${s.arrival_co_v > 1 ? "text-warning" : "text-text-secondary"}`}>{s.arrival_co_v.toFixed(4)}</td>
                      <td className={`px-3 py-1.5 ${s.burst_amplification > 2 ? "text-danger" : "text-text-secondary"}`}>{s.burst_amplification.toFixed(4)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{pct(s.risk_propagation)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{pct(s.confidence)}</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Queue model summary */}
      <Card title="Queue Model Summary">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "ρ", "ρ Trend", "Mean Queue", "Adj Wait", "Burst Factor", "Upstream Pressure", "Confidence", "Signal Quality"].map((h) => (
                  <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {serviceIds.length === 0 ? (
                <tr><td colSpan={9} className="px-3 py-4 text-center text-text-tertiary">No data</td></tr>
              ) : (
                serviceIds.map((id) => {
                  const q = bundles[id].Queue;
                  return (
                    <tr key={id} className="border-b border-surface-2 hover:bg-surface-2">
                      <td className="px-3 py-1.5 text-text-primary">{id}</td>
                      <td className={`px-3 py-1.5 ${q.utilisation > 0.9 ? "text-danger" : q.utilisation > 0.7 ? "text-warning" : "text-text-secondary"}`}>{q.utilisation.toFixed(4)}</td>
                      <td className={`px-3 py-1.5 ${q.utilisation_trend > 0 ? "text-warning" : "text-success"}`}>{q.utilisation_trend > 0 ? "↑" : "↓"} {Math.abs(q.utilisation_trend).toFixed(4)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{q.mean_queue_len.toFixed(2)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{ms(q.adjusted_wait_ms)}</td>
                      <td className={`px-3 py-1.5 ${q.burst_factor > 2 ? "text-warning" : "text-text-secondary"}`}>{q.burst_factor.toFixed(3)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{pct(q.upstream_pressure)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{pct(q.confidence)}</td>
                      <td className="px-3 py-1.5 text-text-tertiary">—</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
