"use client";

import { useMemo } from "react";
import type { TickPayload } from "@/types/tick";
import { Card, Badge, StatRow, ProgressBar, Dot } from "@/components/ui";
import { pct, ms, fixed, rho } from "@/services/format";
import { api } from "@/services/api";
import { useMutation } from "@/hooks/useMutation";
import { Button, ErrorBanner } from "@/components/ui";

interface Props {
  tick: TickPayload;
}

function systemStatus(t: TickPayload): { label: string; variant: "success" | "warning" | "danger" | "critical" | "info" | "muted" } {
  if (t.safety_mode) return { label: "SAFETY MODE", variant: "critical" };
  if (t.degraded_fraction > 0.5) return { label: "DEGRADED", variant: "danger" };
  if (t.degraded_fraction > 0.1 || (t.runtime_metrics?.consec_overruns ?? 0) > 2)
    return { label: "DEGRADED", variant: "warning" };
  return { label: "NOMINAL", variant: "success" };
}

export function OverviewPanel({ tick }: Props) {
  const status = useMemo(() => systemStatus(tick), [tick]);
  const stepMutation = useMutation(api.runtimeStep);
  const toggleMutation = useMutation(
    (enabled: boolean) => api.toggleActuation({ enabled })
  );

  const cp = tick.control_plane;
  const obj = tick.objective;
  const rm = tick.runtime_metrics;
  const ne = tick.network_equilibrium;
  const se = tick.stability_envelope;

  const serviceIds = Object.keys(tick.bundles ?? {});

  return (
    <div className="grid grid-cols-3 gap-3 p-4 animate-fadein">
      {/* Status card */}
      <Card title="System Status" className="col-span-1">
        <div className="p-4 flex flex-col gap-3">
          <div className="flex items-center gap-3">
            <Dot
              className={
                status.variant === "success"
                  ? "bg-success"
                  : status.variant === "warning"
                  ? "bg-warning"
                  : "bg-danger"
              }
            />
            <Badge variant={status.variant} size="sm">{status.label}</Badge>
          </div>

          <StatRow label="Tick" value={<span className="font-mono">{cp.tick.toLocaleString()}</span>} />
          <StatRow label="Actuation" value={
            <Badge variant={cp.actuation_enabled ? "success" : "muted"}>
              {cp.actuation_enabled ? "ENABLED" : "DISABLED"}
            </Badge>
          } />
          <StatRow label="Policy preset" value={<span className="font-mono text-brand">{cp.policy_preset}</span>} />
          <StatRow label="Degraded fraction" value={pct(tick.degraded_fraction)} />
          <StatRow label="Safety mode" value={
            <Badge variant={tick.safety_mode ? "critical" : "muted"}>
              {tick.safety_mode ? "ON" : "OFF"}
            </Badge>
          } />
          <StatRow label="Safety level" value={<span className="font-mono">{rm?.safety_level ?? "—"}/3</span>} />

          <div className="flex gap-2 pt-1">
            <Button
              variant={cp.actuation_enabled ? "danger" : "primary"}
              loading={toggleMutation.loading}
              onClick={() => toggleMutation.mutate(!cp.actuation_enabled)}
            >
              {cp.actuation_enabled ? "Disable Actuation" : "Enable Actuation"}
            </Button>
            <Button
              variant="ghost"
              loading={stepMutation.loading}
              onClick={() => stepMutation.mutate(undefined)}
            >
              Step
            </Button>
          </div>
          {(toggleMutation.error ?? stepMutation.error) && (
            <ErrorBanner message={(toggleMutation.error ?? stepMutation.error)!} />
          )}
        </div>
      </Card>

      {/* Objective card */}
      <Card title="Objective Score" className="col-span-1">
        <div className="p-4 flex flex-col gap-3">
          <div className="flex items-end gap-2">
            <span className="text-3xl font-mono font-bold text-text-primary">
              {(obj?.composite_score ?? 0).toFixed(4)}
            </span>
            <span className="text-xs text-text-tertiary pb-1">composite</span>
          </div>
          <ProgressBar value={obj?.composite_score ?? 0} max={1} />
          <StatRow label="P99 latency" value={ms(obj?.predicted_p99_latency_ms ?? 0)} />
          <StatRow label="Cascade failure prob" value={pct(obj?.cascade_failure_probability ?? 0)} />
          <StatRow label="Max collapse risk" value={pct(obj?.max_collapse_risk ?? 0)} />
          <StatRow label="Oscillation risk" value={pct(obj?.oscillation_risk ?? 0)} />
          <StatRow label="Trajectory score" value={fixed(obj?.trajectory_score ?? 0)} />
          <StatRow label="Risk acceleration" value={fixed(obj?.risk_acceleration ?? 0)} />
        </div>
      </Card>

      {/* Runtime health card */}
      <Card title="Runtime Health" className="col-span-1">
        <div className="p-4 flex flex-col gap-3">
          <StatRow label="Tick health" value={<span className="font-mono">{ms(tick.tick_health_ms)}</span>} />
          <StatRow label="Jitter" value={<span className="font-mono">{ms(tick.jitter_ms)}</span>} />
          <StatRow label="Consec overruns" value={
            <Badge variant={(rm?.consec_overruns ?? 0) > 0 ? "warning" : "muted"}>
              {rm?.consec_overruns ?? 0}
            </Badge>
          } />
          <StatRow label="Total overruns" value={<span className="font-mono">{rm?.total_overruns?.toLocaleString() ?? "0"}</span>} />
          <StatRow label="Predicted overrun" value={
            rm?.predicted_overrun
              ? <Badge variant="warning">YES</Badge>
              : <Badge variant="muted">NO</Badge>
          } />
          <StatRow label="Predicted critical" value={ms(rm?.predicted_critical_ms ?? 0)} />
        </div>
      </Card>

      {/* Network equilibrium */}
      <Card title="Network Equilibrium" className="col-span-1">
        <div className="p-4 flex flex-col gap-3">
          {ne ? (
            <>
              <StatRow label="System ρ mean" value={<span className="font-mono">{rho(ne.system_rho_mean)}</span>} />
              <StatRow label="ρ variance" value={<span className="font-mono">{rho(ne.system_rho_variance)}</span>} />
              <StatRow label="Equilibrium Δ" value={<span className="font-mono">{ne.equilibrium_delta.toFixed(4)}</span>} />
              <StatRow label="Converging" value={<Badge variant={ne.is_converging ? "success" : "warning"}>{ne.is_converging ? "YES" : "NO"}</Badge>} />
              <StatRow label="Saturation risk" value={pct(ne.network_saturation_risk)} />
              {ne.critical_service_id && (
                <StatRow label="Critical service" value={<span className="font-mono text-danger text-[10px]">{ne.critical_service_id}</span>} />
              )}
            </>
          ) : (
            <span className="text-xs text-text-tertiary">No equilibrium data</span>
          )}
        </div>
      </Card>

      {/* Stability envelope */}
      <Card title="Stability Envelope" className="col-span-1">
        <div className="p-4 flex flex-col gap-3">
          {se ? (
            <>
              <StatRow label="Safe system ρ max" value={<span className="font-mono">{rho(se.safe_system_rho_max)}</span>} />
              <StatRow label="Current ρ mean" value={<span className="font-mono">{rho(se.current_system_rho_mean)}</span>} />
              <StatRow label="Headroom" value={
                <span className={`font-mono ${se.envelope_headroom < 0.05 ? "text-danger" : "text-success"}`}>
                  {rho(se.envelope_headroom)}
                </span>
              } />
              <StatRow label="Worst Δ" value={<span className="font-mono">{se.worst_perturbation_delta.toFixed(4)}</span>} />
              {se.most_vulnerable_service && (
                <StatRow label="Most vulnerable" value={<span className="font-mono text-warning text-[10px]">{se.most_vulnerable_service}</span>} />
              )}
            </>
          ) : (
            <span className="text-xs text-text-tertiary">No envelope data</span>
          )}
        </div>
      </Card>

      {/* Services summary */}
      <Card title="Active Services" className="col-span-1">
        <div className="p-4 flex flex-col gap-1.5 overflow-y-auto max-h-48">
          {serviceIds.length === 0 ? (
            <span className="text-xs text-text-tertiary">No service data</span>
          ) : (
            serviceIds.map((id) => {
              const b = tick.bundles[id];
              const zone = tick.stability_zones?.[id] ?? "safe";
              const urgency = tick.priority_risk_queue?.find((r) => r.service_id === id);
              return (
                <div key={id} className="flex items-center gap-2 py-1 border-b border-surface-2 last:border-0">
                  <Dot
                    className={
                      zone === "collapse" ? "bg-danger" : zone === "warning" ? "bg-warning" : "bg-success"
                    }
                  />
                  <span className="font-mono text-[10px] text-text-primary truncate flex-1">{id}</span>
                  <span className="font-mono text-[10px] text-text-secondary">{rho(b.Queue.utilisation)}</span>
                  {urgency?.urgency_class === "critical" && (
                    <Badge variant="critical" size="xs">CRIT</Badge>
                  )}
                </div>
              );
            })
          )}
        </div>
      </Card>

      {/* Forced windows */}
      {(cp.forced_simulation_until > 0 || cp.forced_sandbox_until > 0 || cp.forced_intelligence_until > 0) && (
        <Card title="Active Force Windows" className="col-span-3">
          <div className="p-4 flex gap-4">
            {cp.forced_simulation_until > 0 && (
              <div className="flex items-center gap-2">
                <Badge variant="info">Simulation forced</Badge>
                <span className="text-xs font-mono text-text-tertiary">until tick {cp.forced_simulation_until}</span>
              </div>
            )}
            {cp.forced_sandbox_until > 0 && (
              <div className="flex items-center gap-2">
                <Badge variant="warning">Sandbox forced</Badge>
                <span className="text-xs font-mono text-text-tertiary">until tick {cp.forced_sandbox_until}</span>
              </div>
            )}
            {cp.forced_intelligence_until > 0 && (
              <div className="flex items-center gap-2">
                <Badge variant="success">Intelligence rollout</Badge>
                <span className="text-xs font-mono text-text-tertiary">until tick {cp.forced_intelligence_until}</span>
              </div>
            )}
          </div>
        </Card>
      )}
    </div>
  );
}
