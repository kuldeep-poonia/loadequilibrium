"use client";

import { useMemo } from "react";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  LineChart,
  Line,
} from "recharts";
import type { TickPayload } from "@/types/tick";
import type { TelemetryPoint } from "@/hooks/useTelemetryHistory";
import { Card, EmptyState } from "@/components/ui";
import { ms, pct, rho } from "@/services/format";

import { fixed } from "@/services/format";

interface Props {
  tick: TickPayload;
  history: TelemetryPoint[];
}

interface ChartProps {
  data: TelemetryPoint[];
  dataKey: keyof TelemetryPoint;
  color: string;
  label: string;
  format?: (v: number) => string;
  domain?: [number, number];
}

function MiniChart({ data, dataKey, color, label, format, domain }: ChartProps) {
  return (
    <Card title={label}>
      <div className="px-2 pt-1 pb-2 h-[140px]">
        {data.length < 2 ? (
          <EmptyState message="Waiting for data…" />
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 4, right: 4, left: -30, bottom: 0 }}>
              <defs>
                <linearGradient id={`grad-${dataKey as string}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor={color} stopOpacity={0.3} />
                  <stop offset="95%" stopColor={color} stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="seq" hide />
              <YAxis
                domain={domain}
                tick={{ fill: "#64748b", fontSize: 10 }}
                tickFormatter={(v: number) => format ? format(v) : v.toFixed(2)}
              />
              <Tooltip
                contentStyle={{
                  background: "#10121a",
                  border: "1px solid #1e2236",
                  borderRadius: 6,
                  fontSize: 11,
                  color: "#e2e8f0",
                }}
                formatter={(v: unknown) => [format ? format(v as number) : (v as number).toFixed(3), label]}
                labelFormatter={() => ""}
              />
              <Area
                type="monotone"
                dataKey={dataKey as string}
                stroke={color}
                strokeWidth={1.5}
                fill={`url(#grad-${dataKey as string})`}
                dot={false}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>
    </Card>
  );
}

export function TelemetryPanel({ tick, history }: Props) {
  const rm = tick.runtime_metrics;
  const bundles = tick.bundles ?? {};
  const serviceIds = Object.keys(bundles);

  const stageData = useMemo(
    () => [
      { stage: "Prune", ms: rm?.avg_prune_ms ?? 0 },
      { stage: "Windows", ms: rm?.avg_windows_ms ?? 0 },
      { stage: "Topology", ms: rm?.avg_topology_ms ?? 0 },
      { stage: "Coupling", ms: rm?.avg_coupling_ms ?? 0 },
      { stage: "Modelling", ms: rm?.avg_modelling_ms ?? 0 },
      { stage: "Optimise", ms: rm?.avg_optimise_ms ?? 0 },
      { stage: "Sim", ms: rm?.avg_sim_ms ?? 0 },
      { stage: "Reasoning", ms: rm?.avg_reasoning_ms ?? 0 },
      { stage: "Broadcast", ms: rm?.avg_broadcast_ms ?? 0 },
    ],
    [rm]
  );

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      {/* Top row charts */}
      <div className="grid grid-cols-4 gap-3">
        <MiniChart
          data={history}
          dataKey="tickHealthMs"
          color="#4f8ef7"
          label="Tick Health (ms)"
          format={ms}
        />
        <MiniChart
          data={history}
          dataKey="degradedFraction"
          color="#f59e0b"
          label="Degraded Fraction"
          format={pct}
          domain={[0, 1]}
        />
        <MiniChart
          data={history}
          dataKey="compositeScore"
          color="#22c55e"
          label="Composite Score"
          domain={[0, 1]}
        />
        <MiniChart
          data={history}
          dataKey="maxCollapseRisk"
          color="#ef4444"
          label="Max Collapse Risk"
          format={pct}
          domain={[0, 1]}
        />
      </div>

      <div className="grid grid-cols-4 gap-3">
        <MiniChart
          data={history}
          dataKey="p99LatencyMs"
          color="#a78bfa"
          label="P99 Latency (ms)"
          format={ms}
        />
        <MiniChart
          data={history}
          dataKey="cascadeFailureProb"
          color="#f97316"
          label="Cascade Failure Prob"
          format={pct}
          domain={[0, 1]}
        />
        <MiniChart
          data={history}
          dataKey="oscillationRisk"
          color="#ec4899"
          label="Oscillation Risk"
          format={pct}
          domain={[0, 1]}
        />
        <MiniChart
          data={history}
          dataKey="jitterMs"
          color="#64748b"
          label="Jitter (ms)"
          format={ms}
        />
      </div>

      {/* Pipeline stage latencies */}
      <Card title="Pipeline Stage Latencies (EWMA ms)">
        <div className="px-4 py-3 h-[160px]">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={stageData} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
              <XAxis dataKey="stage" tick={{ fill: "#64748b", fontSize: 10 }} />
              <YAxis tick={{ fill: "#64748b", fontSize: 10 }} tickFormatter={(v: number) => `${v.toFixed(1)}`} />
              <Tooltip
                contentStyle={{ background: "#10121a", border: "1px solid #1e2236", borderRadius: 6, fontSize: 11, color: "#e2e8f0" }}
                formatter={(v: unknown) => [`${(v as number).toFixed(2)}ms`, "latency"]}
              />
              <Line type="monotone" dataKey="ms" stroke="#4f8ef7" strokeWidth={1.5} dot={{ r: 3, fill: "#4f8ef7" }} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </Card>

      {/* Per-service utilisation */}
      <Card title="Per-Service Utilisation (ρ)">
        <div className="p-4 flex flex-col gap-2 max-h-64 overflow-y-auto">
          {serviceIds.length === 0 ? (
            <EmptyState message="No service data" />
          ) : (
            serviceIds.map((id) => {
              const b = bundles[id];
              const u = b.Queue.utilisation;
              const barColor = u > 0.9 ? "bg-danger" : u > 0.7 ? "bg-warning" : "bg-success";
              return (
                <div key={id} className="flex items-center gap-3">
                  <span className="font-mono text-[10px] text-text-secondary w-40 truncate shrink-0">{id}</span>
                  <div className="flex-1 h-1.5 bg-surface-3 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full transition-all duration-300 ${barColor}`} style={{ width: `${Math.min(100, u * 100)}%` }} />
                  </div>
                  <span className="font-mono text-[10px] text-text-primary w-12 text-right">{rho(u)}</span>
                  <span className="font-mono text-[10px] text-text-tertiary w-16 text-right">{ms(b.Queue.mean_wait_ms)}</span>
                </div>
              );
            })
          )}
        </div>
      </Card>

      {/* Per-service signal states */}
      <Card title="Signal States">
        <div className="overflow-x-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="border-b border-surface-3">
                {["Service", "FastEWMA", "SlowEWMA", "Variance", "Spike", "ChangePoint"].map((h) => (
                  <th key={h} className="px-3 py-2 text-left text-text-tertiary font-semibold uppercase tracking-wider">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {serviceIds.map((id) => {
                const sig = bundles[id].Signal;
                return (
                  <tr key={id} className="border-b border-surface-2 hover:bg-surface-2 transition-colors">
                    <td className="px-3 py-1.5 text-text-primary">{id}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{fixed(sig.fast_ewma, 4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{fixed(sig.slow_ewma, 4)}</td>
                    <td className="px-3 py-1.5 text-text-secondary">{fixed(sig.ewma_variance, 6)}</td>
                    <td className="px-3 py-1.5">
                      {sig.spike_detected ? <span className="text-warning">⚡ YES</span> : <span className="text-text-tertiary">—</span>}
                    </td>
                    <td className="px-3 py-1.5">
                      {sig.change_point_detected ? <span className="text-brand">✦ YES</span> : <span className="text-text-tertiary">—</span>}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
