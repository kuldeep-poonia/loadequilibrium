"use client";

import { useState } from "react";
import type { TickPayload } from "@/types/tick";
import { Card, Badge, Button, StatRow, EmptyState, ErrorBanner } from "@/components/ui";
import { ms, pct } from "@/services/format";
import { api } from "@/services/api";
import { useMutation } from "@/hooks/useMutation";

interface Props {
  tick: TickPayload;
}

export function SimulationPanel({ tick }: Props) {
  const [simDuration, setSimDuration] = useState("10");
  const [chaosDuration, setChaosDuration] = useState("30");
  const [chaosServiceId, setChaosServiceId] = useState("");
  const [chaosReqFactor, setChaosReqFactor] = useState("2.5");
  const [chaosLatFactor, setChaosLatFactor] = useState("1.6");
  const [replayDuration, setReplayDuration] = useState("20");
  const [replayService, setReplayService] = useState("");
  const [replayFactor, setReplayFactor] = useState("2.0");
  const [sandboxType, setSandboxType] = useState("experiment");
  const [sandboxDuration, setSandboxDuration] = useState("10");

  const simMutation = useMutation((action: string) =>
    api.simulationControl({ action: action as "run" | "stop" | "reset", duration_ticks: +simDuration })
  );
  const chaosMutation = useMutation(() =>
    api.chaosRun({
      service_id: chaosServiceId || undefined,
      duration_ticks: +chaosDuration,
      request_factor: +chaosReqFactor,
      latency_factor: +chaosLatFactor,
    })
  );
  const replayMutation = useMutation(() =>
    api.replayBurst({
      service_id: replayService || undefined,
      duration_ticks: +replayDuration,
      factor: +replayFactor,
    })
  );
  const sandboxMutation = useMutation(() =>
    api.sandboxTrigger({ type: sandboxType, duration_ticks: +sandboxDuration })
  );

  const cp = tick.control_plane;
  const simResult = tick.sim_result;
  const overlay = tick.sim_overlay;
  const comparison = tick.scenario_comparison;
  const fp = tick.fixed_point_equilibrium;

  const isSimActive = cp.forced_simulation_until > 0 && cp.forced_simulation_until > cp.tick;
  const isSandboxActive = cp.forced_sandbox_until > 0 && cp.forced_sandbox_until > cp.tick;

  return (
    <div className="p-4 flex flex-col gap-3 animate-fadein overflow-y-auto">
      <div className="grid grid-cols-2 gap-3">
        {/* Simulation control */}
        <Card title="Simulation Control">
          <div className="p-4 flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Badge variant={isSimActive ? "success" : "muted"}>
                {isSimActive ? `ACTIVE until tick ${cp.forced_simulation_until}` : "IDLE"}
              </Badge>
              {cp.simulation_reset_pending && <Badge variant="warning">RESET PENDING</Badge>}
            </div>
            <div className="flex items-center gap-2">
              <label className="text-xs text-text-tertiary w-24 shrink-0">Duration ticks</label>
              <input
                type="number"
                value={simDuration}
                onChange={(e) => setSimDuration(e.target.value)}
                className="w-20 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary"
                min="1"
                max="600"
              />
            </div>
            <div className="flex gap-2">
              <Button variant="primary" loading={simMutation.loading} onClick={() => simMutation.mutate("run")}>Start</Button>
              <Button variant="secondary" loading={simMutation.loading} onClick={() => simMutation.mutate("stop")}>Stop</Button>
              <Button variant="ghost" loading={simMutation.loading} onClick={() => simMutation.mutate("reset")}>Reset</Button>
            </div>
            {simMutation.error && <ErrorBanner message={simMutation.error} />}
          </div>
        </Card>

        {/* Sandbox */}
        <Card title="Sandbox Experiment">
          <div className="p-4 flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Badge variant={isSandboxActive ? "warning" : "muted"}>
                {isSandboxActive ? `ACTIVE until tick ${cp.forced_sandbox_until}` : "IDLE"}
              </Badge>
            </div>
            <div className="flex items-center gap-2">
              <label className="text-xs text-text-tertiary w-24 shrink-0">Type</label>
              <input
                value={sandboxType}
                onChange={(e) => setSandboxType(e.target.value)}
                className="flex-1 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary"
              />
            </div>
            <div className="flex items-center gap-2">
              <label className="text-xs text-text-tertiary w-24 shrink-0">Duration ticks</label>
              <input
                type="number"
                value={sandboxDuration}
                onChange={(e) => setSandboxDuration(e.target.value)}
                className="w-20 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary"
              />
            </div>
            <Button variant="warning" loading={sandboxMutation.loading} onClick={() => sandboxMutation.mutate(undefined)}>Trigger Sandbox</Button>
            {sandboxMutation.error && <ErrorBanner message={sandboxMutation.error} />}
            {sandboxMutation.data && <span className="text-xs text-success">Scheduled until tick {sandboxMutation.data.until_tick}</span>}
          </div>
        </Card>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {/* Chaos run */}
        <Card title="Chaos Injection">
          <div className="p-4 flex flex-col gap-3">
            {[
              { label: "Target service", value: chaosServiceId, set: setChaosServiceId, placeholder: "* (all)" },
            ].map(({ label, value, set, placeholder }) => (
              <div key={label} className="flex items-center gap-2">
                <label className="text-xs text-text-tertiary w-28 shrink-0">{label}</label>
                <input value={value} onChange={(e) => set(e.target.value)} placeholder={placeholder}
                  className="flex-1 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary placeholder-text-tertiary" />
              </div>
            ))}
            {[
              { label: "Duration ticks", value: chaosDuration, set: setChaosDuration },
              { label: "Request factor", value: chaosReqFactor, set: setChaosReqFactor },
              { label: "Latency factor", value: chaosLatFactor, set: setChaosLatFactor },
            ].map(({ label, value, set }) => (
              <div key={label} className="flex items-center gap-2">
                <label className="text-xs text-text-tertiary w-28 shrink-0">{label}</label>
                <input type="number" value={value} onChange={(e) => set(e.target.value)}
                  className="w-24 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary" />
              </div>
            ))}
            <Button variant="danger" loading={chaosMutation.loading} onClick={() => chaosMutation.mutate(undefined)}>
              Inject Chaos
            </Button>
            {chaosMutation.error && <ErrorBanner message={chaosMutation.error} />}
            {chaosMutation.data && (
              <div className="text-xs text-text-secondary font-mono">
                Scheduled ticks {chaosMutation.data.start_tick}–{chaosMutation.data.until_tick}
                {" "} req×{chaosMutation.data.request_factor} lat×{chaosMutation.data.latency_factor}
              </div>
            )}
          </div>
        </Card>

        {/* Replay burst */}
        <Card title="Replay Burst">
          <div className="p-4 flex flex-col gap-3">
            {[
              { label: "Target service", value: replayService, set: setReplayService, placeholder: "* (all)" },
            ].map(({ label, value, set, placeholder }) => (
              <div key={label} className="flex items-center gap-2">
                <label className="text-xs text-text-tertiary w-28 shrink-0">{label}</label>
                <input value={value} onChange={(e) => set(e.target.value)} placeholder={placeholder}
                  className="flex-1 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary placeholder-text-tertiary" />
              </div>
            ))}
            {[
              { label: "Duration ticks", value: replayDuration, set: setReplayDuration },
              { label: "Max factor", value: replayFactor, set: setReplayFactor },
            ].map(({ label, value, set }) => (
              <div key={label} className="flex items-center gap-2">
                <label className="text-xs text-text-tertiary w-28 shrink-0">{label}</label>
                <input type="number" value={value} onChange={(e) => set(e.target.value)}
                  className="w-24 px-2 py-1 rounded border border-surface-4 bg-surface-2 text-xs font-mono text-text-primary" />
              </div>
            ))}
            <Button variant="warning" loading={replayMutation.loading} onClick={() => replayMutation.mutate(undefined)}>
              Replay Burst
            </Button>
            {replayMutation.error && <ErrorBanner message={replayMutation.error} />}
            {replayMutation.data && (
              <div className="text-xs text-text-secondary font-mono">
                Scheduled ticks {replayMutation.data.start_tick}–{replayMutation.data.until_tick} ×{replayMutation.data.factor}
              </div>
            )}
          </div>
        </Card>
      </div>

      {/* Last simulation result */}
      {simResult ? (
        <Card title="Last Simulation Result">
          <div className="p-4 grid grid-cols-3 gap-4">
            <div className="flex flex-col gap-2">
              <StatRow label="Horizon" value={ms(simResult.HorizonMs)} />
              <StatRow label="System stable" value={<Badge variant={simResult.SystemStable ? "success" : "danger"}>{simResult.SystemStable ? "YES" : "NO"}</Badge>} />
              <StatRow label="Collapse detected" value={<Badge variant={simResult.CollapseDetected ? "critical" : "muted"}>{simResult.CollapseDetected ? "YES" : "NO"}</Badge>} />
              <StatRow label="Cascade triggered" value={<Badge variant={simResult.CascadeTriggered ? "danger" : "muted"}>{simResult.CascadeTriggered ? "YES" : "NO"}</Badge>} />
              <StatRow label="Degraded services" value={simResult.DegradedServiceCount} />
              <StatRow label="Recovery convergence" value={ms(simResult.RecoveryConvergenceMs)} />
              <StatRow label="Events processed" value={simResult.EventsProcessed.toLocaleString()} />
              <StatRow label="Wall time" value={ms(simResult.Meta.WallTimeMs)} />
              <StatRow label="Budget used" value={pct(simResult.Meta.BudgetUsedPct / 100)} />
              <StatRow label="Events/ms" value={simResult.Meta.EventsPerMs.toFixed(1)} />
            </div>
            <div className="col-span-2 overflow-x-auto">
              <table className="w-full text-[10px] font-mono">
                <thead>
                  <tr className="border-b border-surface-3">
                    {["Service", "Util", "Peak Util", "Q Len", "Peak Q", "Mean Wait", "Throughput", "Saturated", "Recovery"].map((h) => (
                      <th key={h} className="px-2 py-1.5 text-left text-text-tertiary font-semibold uppercase tracking-wider whitespace-nowrap">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {Object.entries(simResult.Services).map(([id, svc]) => (
                    <tr key={id} className="border-b border-surface-2 hover:bg-surface-2">
                      <td className="px-2 py-1 text-text-primary">{id}</td>
                      <td className={`px-2 py-1 ${svc.PeakUtilisation > 0.9 ? "text-danger" : "text-text-secondary"}`}>{pct(svc.PeakUtilisation)}</td>
                      <td className="px-2 py-1 text-text-secondary">{pct(svc.PeakUtilisation)}</td>
                      <td className="px-2 py-1 text-text-secondary">{svc.FinalQueueLen}</td>
                      <td className="px-2 py-1 text-text-secondary">{svc.PeakQueueLen}</td>
                      <td className="px-2 py-1 text-text-secondary">{ms(svc.MeanWaitMs)}</td>
                      <td className="px-2 py-1 text-text-secondary">{pct(svc.ThroughputRatio)}</td>
                      <td className="px-2 py-1">{svc.Saturated ? <span className="text-danger">YES</span> : <span className="text-text-tertiary">—</span>}</td>
                      <td className="px-2 py-1 text-text-secondary">{svc.RecoveryTimeMs > 0 ? ms(svc.RecoveryTimeMs) : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </Card>
      ) : (
        <Card title="Simulation Result">
          <div className="p-4"><EmptyState message="No simulation result yet — trigger a run above" /></div>
        </Card>
      )}

      {/* Scenario comparison */}
      {comparison && (
        <Card title="Scenario Comparison">
          <div className="p-4 grid grid-cols-4 gap-4">
            <StatRow label="Scenarios" value={comparison.scenario_count} />
            <StatRow label="Best case collapse" value={pct(comparison.best_case_collapse)} />
            <StatRow label="Worst case collapse" value={pct(comparison.worst_case_collapse)} />
            <StatRow label="Median SLA violation" value={pct(comparison.median_sla_violation)} />
            <StatRow label="Stable fraction" value={pct(comparison.stable_scenario_fraction)} />
            <StatRow label="Recovery min" value={ms(comparison.recovery_convergence_min_ms)} />
            <StatRow label="Recovery max" value={ms(comparison.recovery_convergence_max_ms)} />
          </div>
        </Card>
      )}

      {/* Fixed-point equilibrium */}
      {fp && (
        <Card title="Fixed-Point Equilibrium">
          <div className="p-4 grid grid-cols-2 gap-4">
            <div>
              <StatRow label="Systemic collapse prob" value={pct(fp.systemic_collapse_prob)} />
              <StatRow label="Converged" value={<Badge variant={fp.converged ? "success" : "warning"}>{fp.converged ? "YES" : "NO"}</Badge>} />
              <StatRow label="Iterations" value={fp.converged_iterations} />
              <StatRow label="Convergence rate ρ(J)" value={fp.convergence_rate.toFixed(4)} />
              <StatRow label="Stability margin" value={fp.stability_margin.toFixed(4)} />
            </div>
            {fp.equilibrium_rho && (
              <div className="flex flex-col gap-1 overflow-y-auto max-h-32">
                {Object.entries(fp.equilibrium_rho).map(([id, r]) => (
                  <div key={id} className="flex items-center gap-2 text-[10px] font-mono">
                    <span className="text-text-primary truncate flex-1">{id}</span>
                    <span className={r > 0.9 ? "text-danger" : r > 0.7 ? "text-warning" : "text-success"}>{r.toFixed(4)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>
      )}

      {/* Sim overlay */}
      {overlay && (
        <Card title={`Simulation Overlay (age: ${overlay.sim_tick_age} ticks, horizon: ${ms(overlay.horizon_ms)})`}>
          <div className="overflow-x-auto">
            <table className="w-full text-[10px] font-mono">
              <thead>
                <tr className="border-b border-surface-3">
                  {["Service", "Cascade Fail Prob", "P95 Queue Len", "Saturation Frac", "SLA Violation Prob"].map((h) => (
                    <th key={h} className="px-3 py-1.5 text-left text-text-tertiary font-semibold">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {Object.keys(overlay.cascade_failure_prob ?? {}).map((id) => (
                  <tr key={id} className="border-b border-surface-2 hover:bg-surface-2">
                    <td className="px-3 py-1 text-text-primary">{id}</td>
                    <td className="px-3 py-1 text-text-secondary">{pct(overlay.cascade_failure_prob?.[id] ?? 0)}</td>
                    <td className="px-3 py-1 text-text-secondary">{(overlay.p95_queue_len?.[id] ?? 0).toFixed(1)}</td>
                    <td className="px-3 py-1 text-text-secondary">{pct(overlay.saturation_frac?.[id] ?? 0)}</td>
                    <td className="px-3 py-1 text-text-secondary">{pct(overlay.sla_violation_prob?.[id] ?? 0)}</td>
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
