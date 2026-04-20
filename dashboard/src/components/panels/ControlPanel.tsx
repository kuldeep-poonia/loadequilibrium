import { memo, useState } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { api, apiConfigured } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import type { PolicyPreset } from "@/lib/types";

const PRESETS: PolicyPreset[] = ["conservative", "balanced", "aggressive"];
const SCENARIOS = ["burst", "spike", "drift", "outage"] as const;

const useApiGuard = () => {
  const { toast } = useToast();
  return (fn: () => Promise<unknown>, label: string) => async () => {
    if (!apiConfigured()) {
      toast({
        title: "API not configured",
        description: "Set VITE_API_URL to enable controls.",
        variant: "destructive",
      });
      return;
    }
    try {
      await fn();
      toast({ title: label, description: "Sent" });
    } catch (err) {
      toast({
        title: `${label} failed`,
        description: err instanceof Error ? err.message : String(err),
        variant: "destructive",
      });
    }
  };
};

export const ControlPanel = memo(function ControlPanel() {
  const cp = useTelemetry((s) => s.tick?.control_plane);
  const guard = useApiGuard();
  const [scenario, setScenario] =
    useState<(typeof SCENARIOS)[number]>("burst");
  const [duration, setDuration] = useState(20);
  const [busy, setBusy] = useState<string | null>(null);

  const wrap =
    (key: string, run: () => Promise<unknown>, label: string) =>
    async () => {
      setBusy(key);
      try {
        await guard(run, label)();
      } finally {
        setBusy(null);
      }
    };

  const actuationOn = cp?.actuation_enabled ?? false;
  const policy = (cp?.policy_preset ?? "balanced") as string;

  return (
    <section className="panel flex flex-col h-full min-h-0">
      <header className="panel-header justify-between">
        <div className="flex items-center gap-2">
          <span className={`led ${actuationOn ? "led-safe animate-pulse-dot" : "led-off"}`} />
          <span>Command Console</span>
        </div>
        <span className="font-mono normal-case tracking-wider text-[9px] text-muted-foreground">
          {actuationOn ? "ARMED" : "SAFE"}
        </span>
      </header>
      <div className="flex-1 min-h-0 overflow-y-auto p-3 space-y-4">
        <div>
          <div className="text-[9px] uppercase tracking-[0.2em] text-muted-foreground mb-1.5 font-mono">
            Actuation
          </div>
          <button
            onClick={wrap("toggle", () => api.toggleActuation(), "Actuation toggled")}
            disabled={busy === "toggle"}
            className={`mc-button w-full justify-center ${actuationOn ? "mc-button-active" : ""}`}
          >
            {actuationOn ? "◉ ENABLED · click to disarm" : "○ DISABLED · click to arm"}
          </button>
        </div>

        <div>
          <div className="text-[9px] uppercase tracking-[0.2em] text-muted-foreground mb-1.5 font-mono">
            Policy Preset
          </div>
          <div className="grid grid-cols-3 gap-1">
            {PRESETS.map((p) => {
              const active = policy === p;
              return (
                <button
                  key={p}
                  onClick={wrap(`p-${p}`, () => api.updatePolicy(p), `Policy → ${p}`)}
                  disabled={busy === `p-${p}`}
                  className={`mc-button text-[10px] ${active ? "mc-button-active" : ""}`}
                >
                  {p}
                </button>
              );
            })}
          </div>
        </div>

        <div>
          <div className="text-[9px] uppercase tracking-[0.2em] text-muted-foreground mb-1.5 font-mono">
            Chaos Injection
          </div>
          <div className="flex gap-1.5 mb-2">
            <select
              value={scenario}
              onChange={(e) => setScenario(e.target.value as (typeof SCENARIOS)[number])}
              className="flex-1 bg-surface-2 border border-border rounded-sm px-2 py-1.5 text-[11px] font-mono uppercase text-foreground focus:outline-none focus:ring-1 focus:ring-phosphor"
            >
              {SCENARIOS.map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
            <input
              type="number"
              min={1}
              max={500}
              value={duration}
              onChange={(e) => setDuration(Math.max(1, Number(e.target.value) || 1))}
              className="w-20 bg-surface-2 border border-border rounded-sm px-2 py-1.5 text-[11px] readout text-foreground focus:outline-none focus:ring-1 focus:ring-phosphor"
            />
          </div>
          <button
            onClick={wrap("chaos", () => api.chaosRun(scenario, duration), `Chaos: ${scenario} (${duration}t)`)}
            disabled={busy === "chaos"}
            className="mc-button mc-button-warn w-full justify-center"
          >
            ▲ INJECT SCENARIO
          </button>
        </div>

        {cp && (
          <div className="pt-3 border-t border-border space-y-1">
            <div className="text-[9px] uppercase tracking-[0.2em] text-muted-foreground mb-1.5 font-mono">
              Telemetry
            </div>
            <div className="flex justify-between text-[10px] font-mono">
              <span className="text-muted-foreground uppercase">tick</span>
              <span className="text-foreground tabular-nums">{cp.tick.toString().padStart(6, "0")}</span>
            </div>
            <div className="flex justify-between text-[10px] font-mono">
              <span className="text-muted-foreground uppercase">acked</span>
              <span className="text-foreground tabular-nums">{cp.acknowledged_alert_count}</span>
            </div>
            {cp.forced_sandbox_until > cp.tick && (
              <div className="flex justify-between text-[10px] font-mono text-warn">
                <span className="uppercase">sandbox until</span>
                <span className="tabular-nums">{cp.forced_sandbox_until}</span>
              </div>
            )}
            {cp.forced_simulation_until > cp.tick && (
              <div className="flex justify-between text-[10px] font-mono text-warn">
                <span className="uppercase">sim forced</span>
                <span className="tabular-nums">{cp.forced_simulation_until}</span>
              </div>
            )}
          </div>
        )}
      </div>
    </section>
  );
});
