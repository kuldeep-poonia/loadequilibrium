"use client";

import { useMemo } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ReferenceLine,
} from "recharts";
import type { TickPayload } from "@/types/tick";
import { Card, Badge, EmptyState } from "@/components/ui";
import { pct, ms, rho, fixed } from "@/services/format";

interface Props {
  tick: TickPayload;
}

export function AutopilotPanel({ tick }: Props) {
  const bundles = useMemo(() => tick.bundles ?? {}, [tick.bundles]);
  const directives = useMemo(() => tick.directives ?? {}, [tick.directives]);
  const serviceIds = Object.keys(bundles);

  const directiveData = useMemo(
    () =>
      Object.entries(directives)
        .filter(([, d]) => d.Active)
        .map(([id, d]) => ({
          id,
          scale: d.ScaleFactor,
          pid: d.PIDOutput,
          mpcRho: d.MPCPredictedRho,
          gradient: d.CostGradient,
          trajAvg: d.TrajectoryCostAvg,
          overshoot: d.MPCOvershootRisk,
          underact: d.MPCUnderactuationRisk,
          hysteresis: d.HysteresisState,
          plannerConv: d.PlannerConvergent,
          plannerConvex: d.PlannerConvex,
        })),
    [directives]
  );

  const scaleChartData = useMemo(
    () =>
      directiveData.map((d) => ({
        id: d.id.slice(0, 14),
        scale: +d.scale.toFixed(3),
      })),
    [directiveData]
  );

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      {/* Control directives overview */}
      <div className="grid grid-cols-2 gap-3">
        <Card title="Active Control Directives" className="col-span-1">
          {directiveData.length === 0 ? (
            <div className="p-4"><EmptyState message="No active directives" /></div>
          ) : (
            <div className="px-4 py-2 h-[180px]">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={scaleChartData} margin={{ top: 4, right: 4, left: -20, bottom: 20 }}>
                  <XAxis dataKey="id" tick={{ fill: "#64748b", fontSize: 9 }} angle={-30} textAnchor="end" />
                  <YAxis tick={{ fill: "#64748b", fontSize: 10 }} />
                  <Tooltip
                    contentStyle={{ background: "#10121a", border: "1px solid #1e2236", borderRadius: 6, fontSize: 11, color: "#e2e8f0" }}
                    formatter={(v: unknown) => [`${(v as number).toFixed(3)}×`, "scale factor"]}
                  />
                  <ReferenceLine y={1} stroke="#4f8ef7" strokeDasharray="4 2" strokeWidth={1} />
                  <Bar dataKey="scale" fill="#4f8ef7" radius={[2, 2, 0, 0]} isAnimationActive={false} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}
        </Card>

        <Card title="MPC Risk Flags" className="col-span-1">
          <div className="p-4 flex flex-col gap-1.5 max-h-[200px] overflow-y-auto">
            {directiveData.length === 0 ? (
              <EmptyState message="No directive data" />
            ) : (
              directiveData.map((d) => (
                <div key={d.id} className="flex items-center gap-2 py-1 border-b border-surface-2 last:border-0">
                  <span className="font-mono text-[10px] text-text-primary truncate w-36">{d.id}</span>
                  <Badge variant={d.overshoot ? "warning" : "muted"} size="xs">OVR</Badge>
                  <Badge variant={d.underact ? "danger" : "muted"} size="xs">UND</Badge>
                  <Badge variant={d.plannerConv ? "success" : "warning"} size="xs">CNV</Badge>
                  <Badge variant={d.plannerConvex ? "success" : "muted"} size="xs">CVX</Badge>
                  <span className="font-mono text-[10px] text-text-tertiary ml-auto">{d.hysteresis}</span>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>

      {/* Full directive table */}
      <Card title="Directive Details">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "Scale", "Target ρ", "PID Out", "MPC ρ", "Cost ∇", "Traj Avg", "Max Traj", "Actn Bound", "Hysteresis"].map((h) => (
                  <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Object.entries(directives).length === 0 ? (
                <tr><td colSpan={10} className="px-3 py-4 text-center text-text-tertiary">No directives</td></tr>
              ) : (
                Object.entries(directives).map(([id, d]) => (
                  <tr key={id} className={`border-b border-surface-2 hover:bg-surface-2 transition-colors ${!d.Active ? "opacity-40" : ""}`}>
                    <td className="px-3 py-1.5 text-text-primary">{id}</td>
                    <td className="px-3 py-1.5 text-brand">{d.ScaleFactor.toFixed(4)}×</td>
                    <td className="px-3 py-1.5 text-text-secondary">{rho(d.TargetUtilisation)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.PIDOutput.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{rho(d.MPCPredictedRho)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.CostGradient.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.TrajectoryCostAvg.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.MaxTrajectoryCost.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{d.ActuationBound.toFixed(4)}</td>
                    <td className="px-3 py-1.5 text-text-tertiary">{d.HysteresisState}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Per-service stability assessments */}
      <Card title="Stability Assessments">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "Margin", "Collapse Risk", "Oscillation", "Zone", "Unstable", "Trend Margin", "dRisk/dt", "Predicted Collapse"].map((h) => (
                  <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {serviceIds.length === 0 ? (
                <tr><td colSpan={9} className="px-3 py-4 text-center text-text-tertiary">No data</td></tr>
              ) : (
                serviceIds.map((id) => {
                  const s = bundles[id].Stability;
                  const zoneColor = s.collapse_zone === "collapse" ? "text-danger" : s.collapse_zone === "warning" ? "text-warning" : "text-success";
                  return (
                    <tr key={id} className="border-b border-surface-2 hover:bg-surface-2 transition-colors">
                      <td className="px-3 py-1.5 text-text-primary">{id}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{fixed(s.stability_margin)}</td>
                      <td className={`px-3 py-1.5 ${s.collapse_risk > 0.7 ? "text-danger" : "text-text-secondary"}`}>{pct(s.collapse_risk)}</td>
                      <td className="px-3 py-1.5 text-text-secondary">{pct(s.oscillation_risk)}</td>
                      <td className={`px-3 py-1.5 font-bold ${zoneColor}`}>{(s.collapse_zone ?? "unknown").toUpperCase()}</td>
                      <td className="px-3 py-1.5">{s.is_unstable ? <span className="text-danger">YES</span> : <span className="text-text-tertiary">—</span>}</td>
                      <td className={`px-3 py-1.5 ${s.trend_adjusted_margin < 0 ? "text-danger" : "text-text-secondary"}`}>{fixed(s.trend_adjusted_margin)}</td>
                      <td className={`px-3 py-1.5 ${s.stability_derivative > 0 ? "text-warning" : "text-success"}`}>{fixed(s.stability_derivative, 4)}</td>
                      <td className="px-3 py-1.5 text-text-tertiary">{s.predicted_collapse_ms > 0 ? ms(s.predicted_collapse_ms) : "—"}</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Priority risk queue */}
      {tick.priority_risk_queue && tick.priority_risk_queue.length > 0 && (
        <Card title="Priority Risk Queue">
          <div className="p-4 flex flex-col gap-1.5">
            {tick.priority_risk_queue.map((item, idx) => (
              <div key={item.service_id} className="flex items-center gap-3 py-1.5 border-b border-surface-2 last:border-0">
                <span className="text-text-tertiary font-mono text-[10px] w-4 shrink-0">{idx + 1}</span>
                <Badge variant={
                  item.urgency_class === "critical" ? "critical" :
                  item.urgency_class === "warning" ? "warning" :
                  item.urgency_class === "elevated" ? "info" : "muted"
                } size="xs">{item.urgency_class.toUpperCase()}</Badge>
                <span className="font-mono text-[10px] text-text-primary flex-1 truncate">{item.service_id}</span>
                {item.is_keystone && <Badge variant="warning" size="xs">KEY</Badge>}
                <span className="font-mono text-[10px] text-text-tertiary">ρ {rho(item.rho)}</span>
                <span className="font-mono text-[10px] text-text-tertiary">risk {pct(item.collapse_risk)}</span>
                <span className="font-mono text-[10px] text-text-tertiary">urgency {item.urgency_score.toFixed(3)}</span>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}
