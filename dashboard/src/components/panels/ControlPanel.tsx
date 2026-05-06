"use client";

import {
  ResponsiveContainer,
  ScatterChart,
  Scatter,
  XAxis,
  YAxis,
  Tooltip,
  BarChart,
  Bar,
  Cell,
} from "recharts";
import type { TickPayload } from "@/types/tick";
import { Card, StatRow, EmptyState } from "@/components/ui";
import { pct, ms } from "@/services/format";

interface Props {
  tick: TickPayload;
}

export function ControlPanel({ tick }: Props) {
  const bundles = tick.bundles ?? {};
  const directives = tick.directives ?? {};
  const serviceIds = Object.keys(bundles);

  const feedbackData = serviceIds.map((id) => {
    const b = bundles[id];
    const d = directives[id];
    return {
      id,
      rho: b.Queue.utilisation,
      target: d?.TargetUtilisation ?? 0,
      error: d?.Error ?? 0,
      scale: d?.ScaleFactor ?? 1,
      pid: d?.PIDOutput ?? 0,
      cascadeAmp: b.Stability.cascade_amplification_score,
      feedbackGain: b.Stability.feedback_gain,
      hazard: b.Queue.hazard,
      reservoir: b.Queue.reservoir,
    };
  });

  const scatterData = feedbackData.map((d) => ({ rho: +d.rho.toFixed(3), error: +d.error.toFixed(4), id: d.id }));

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      <div className="grid grid-cols-2 gap-3">
        {/* Feedback loop scatter: ρ vs control error */}
        <Card title="ρ vs Control Error">
          <div className="px-2 pt-1 pb-2 h-[200px]">
            {scatterData.length === 0 ? (
              <EmptyState message="No data" />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <ScatterChart margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                  <XAxis dataKey="rho" name="ρ" tick={{ fill: "#64748b", fontSize: 10 }} label={{ value: "ρ", position: "insideBottom", offset: -2, fill: "#64748b", fontSize: 10 }} />
                  <YAxis dataKey="error" name="error" tick={{ fill: "#64748b", fontSize: 10 }} />
                  <Tooltip
                    cursor={{ strokeDasharray: "3 3" }}
                    contentStyle={{ background: "#10121a", border: "1px solid #1e2236", borderRadius: 6, fontSize: 11, color: "#e2e8f0" }}
                    formatter={(v: unknown, name: string) => [(v as number).toFixed(4), name]}
                  />
                  <Scatter data={scatterData} fill="#4f8ef7" isAnimationActive={false} />
                </ScatterChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>

        {/* Scale factor bar chart */}
        <Card title="Scale Factors">
          <div className="px-2 pt-1 pb-2 h-[200px]">
            {feedbackData.length === 0 ? (
              <EmptyState message="No directives" />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={feedbackData.map((d) => ({ id: d.id.slice(0, 12), scale: d.scale }))} margin={{ top: 4, right: 4, left: -20, bottom: 20 }}>
                  <XAxis dataKey="id" tick={{ fill: "#64748b", fontSize: 9 }} angle={-25} textAnchor="end" />
                  <YAxis tick={{ fill: "#64748b", fontSize: 10 }} domain={[0, "auto"]} />
                  <Tooltip contentStyle={{ background: "#10121a", border: "1px solid #1e2236", borderRadius: 6, fontSize: 11, color: "#e2e8f0" }} formatter={(v: unknown) => [(v as number).toFixed(4), "scale"]} />
                  {feedbackData.map((d) => (
                    <Cell key={d.id} fill={d.scale > 1.2 ? "#ef4444" : d.scale < 0.8 ? "#f59e0b" : "#4f8ef7"} />
                  ))}
                  <Bar dataKey="scale" isAnimationActive={false}>
                    {feedbackData.map((d) => (
                      <Cell key={d.id} fill={d.scale > 1.5 ? "#ef4444" : d.scale > 1.1 ? "#f59e0b" : "#4f8ef7"} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>
      </div>

      {/* Actuator state table */}
      <Card title="Actuator & Control States">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "ρ", "Target", "Error", "Scale", "PID", "Cascade Amp", "FB Gain", "Hazard", "Reservoir", "Arrival Rate", "Service Rate", "Mean Wait"].map((h) => (
                  <th key={h} className="px-2 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {feedbackData.length === 0 ? (
                <tr><td colSpan={13} className="px-3 py-4 text-center text-text-tertiary">No data</td></tr>
              ) : (
                feedbackData.map((d) => {
                  const q = bundles[d.id].Queue;
                  return (
                    <tr key={d.id} className="border-b border-surface-2 hover:bg-surface-2">
                      <td className="px-2 py-1.5 text-text-primary">{d.id}</td>
                      <td className={`px-2 py-1.5 ${d.rho > 0.9 ? "text-danger" : d.rho > 0.7 ? "text-warning" : "text-text-secondary"}`}>{d.rho.toFixed(4)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{d.target.toFixed(4)}</td>
                      <td className={`px-2 py-1.5 ${Math.abs(d.error) > 0.1 ? "text-warning" : "text-text-secondary"}`}>{d.error.toFixed(4)}</td>
                      <td className={`px-2 py-1.5 ${d.scale > 1.1 ? "text-brand" : "text-text-secondary"}`}>{d.scale.toFixed(4)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{d.pid.toFixed(4)}</td>
                      <td className={`px-2 py-1.5 ${d.cascadeAmp > 1 ? "text-warning" : "text-text-secondary"}`}>{d.cascadeAmp.toFixed(4)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{d.feedbackGain.toFixed(4)}</td>
                      <td className={`px-2 py-1.5 ${d.hazard > 0.7 ? "text-danger" : "text-text-secondary"}`}>{d.hazard.toFixed(4)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{d.reservoir.toFixed(4)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{q.arrival_rate.toFixed(2)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{q.service_rate.toFixed(2)}</td>
                      <td className="px-2 py-1.5 text-text-secondary">{ms(q.mean_wait_ms)}</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Network coupling details */}
      {tick.network_coupling && (
        <Card title="Network Coupling Details">
          <div className="p-4 grid grid-cols-2 gap-4">
            {Object.entries(tick.network_coupling).map(([id, nc]) => (
              <div key={id} className="flex flex-col gap-1.5 p-3 rounded border border-surface-3 bg-surface-2">
                <span className="font-mono text-xs text-brand font-semibold">{id}</span>
                <StatRow label="Eff pressure" value={nc.effective_pressure.toFixed(4)} />
                <StatRow label="Coupled arrival rate" value={nc.coupled_arrival_rate.toFixed(4)} />
                <StatRow label="Path eq ρ" value={nc.path_equilibrium_rho.toFixed(4)} />
                <StatRow label="Path sat risk" value={pct(nc.path_sat_risk)} />
                <StatRow label="Path collapse prob" value={pct(nc.path_collapse_prob)} />
                <StatRow label="Congestion feedback" value={nc.congestion_feedback.toFixed(4)} />
                <StatRow label="Path horizon" value={`${nc.path_sat_horizon_sec.toFixed(1)}s`} />
                <StatRow label="Steady-state P0" value={nc.steady_state_p0.toFixed(4)} />
                <StatRow label="Mean queue (eq)" value={nc.steady_state_mean_queue.toFixed(2)} />
              </div>
            ))}
          </div>
        </Card>
      )}

      {/* Prediction horizon */}
      {tick.prediction_horizon && Object.keys(tick.prediction_horizon).length > 0 && (
        <Card title="Prediction Horizon (s to saturation)">
          <div className="p-4 flex flex-col gap-1.5">
            {Object.entries(tick.prediction_horizon)
              .sort(([, a], [, b]) => a - b)
              .map(([id, secs]) => (
                <div key={id} className="flex items-center gap-3">
                  <span className={`font-mono text-xs ${secs < 60 ? "text-danger" : secs < 300 ? "text-warning" : "text-text-secondary"}`}>
                    {secs.toFixed(0)}s
                  </span>
                  <span className="font-mono text-[10px] text-text-primary">{id}</span>
                </div>
              ))}
          </div>
        </Card>
      )}
    </div>
  );
}
