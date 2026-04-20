import { memo, useMemo } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { computeOverview, fixed, pct } from "@/lib/format";

const HealthGauge = ({ value }: { value: number }) => {
  const pctVal = Math.max(0, Math.min(1, value));
  const tone =
    pctVal >= 0.75 ? "safe" : pctVal >= 0.45 ? "warn" : "crit";
  const stroke =
    tone === "safe" ? "hsl(var(--safe))" :
    tone === "warn" ? "hsl(var(--warn))" : "hsl(var(--crit))";
  const r = 22;
  const c = 2 * Math.PI * r;
  const off = c * (1 - pctVal);
  return (
    <div className="relative h-14 w-14 shrink-0">
      <svg viewBox="0 0 60 60" className="h-full w-full -rotate-90">
        <circle cx="30" cy="30" r={r} fill="none" stroke="hsl(var(--surface-3))" strokeWidth="4" />
        <circle
          cx="30" cy="30" r={r} fill="none" stroke={stroke} strokeWidth="4"
          strokeDasharray={c} strokeDashoffset={off} strokeLinecap="round"
          style={{ filter: `drop-shadow(0 0 4px ${stroke})` }}
        />
      </svg>
      <div className="absolute inset-0 grid place-items-center">
        <span className={`text-xs readout font-medium ${
          tone === "safe" ? "text-safe" : tone === "warn" ? "text-warn" : "text-crit"
        }`}>
          {Math.round(pctVal * 100)}
        </span>
      </div>
    </div>
  );
};

interface StatProps {
  label: string;
  value: string;
  hint?: string;
  tone?: "safe" | "warn" | "crit" | "default";
  unit?: string;
}
const Stat = ({ label, value, hint, tone = "default", unit }: StatProps) => {
  const valueClass =
    tone === "crit" ? "text-crit glow-crit"
    : tone === "warn" ? "text-warn glow-warn"
    : tone === "safe" ? "text-safe"
    : "text-foreground";
  return (
    <div className="flex flex-col gap-0.5 px-4 py-2.5 min-w-[150px] flex-1">
      <div className="text-[9px] uppercase tracking-[0.2em] text-muted-foreground font-mono">
        {label}
      </div>
      <div className="flex items-baseline gap-1">
        <span className={`text-2xl readout leading-none ${valueClass}`}>
          {value}
        </span>
        {unit && (
          <span className="text-[10px] font-mono text-muted-foreground">
            {unit}
          </span>
        )}
      </div>
      {hint && (
        <div className="text-[10px] text-muted-foreground font-mono truncate">
          {hint}
        </div>
      )}
    </div>
  );
};

export const SystemOverview = memo(function SystemOverview() {
  const tick = useTelemetry((s) => s.tick);
  const stats = useMemo(() => computeOverview(tick), [tick]);

  const unstableTone =
    stats.unstablePct >= 0.4 ? "crit"
    : stats.unstablePct >= 0.15 ? "warn" : "safe";
  const riskTone =
    stats.maxCollapseRisk >= 0.7 ? "crit"
    : stats.maxCollapseRisk >= 0.4 ? "warn" : "safe";
  const healthTone =
    stats.health >= 0.75 ? "safe" : stats.health >= 0.45 ? "warn" : "crit";

  return (
    <div className="panel flex flex-wrap items-stretch divide-x divide-border overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-2.5 bg-surface-2/40">
        <HealthGauge value={stats.health} />
        <div className="leading-tight">
          <div className="text-[9px] uppercase tracking-[0.2em] text-muted-foreground font-mono">
            System Health
          </div>
          <div className={`text-base font-medium ${
            healthTone === "safe" ? "text-safe" :
            healthTone === "warn" ? "text-warn" : "text-crit"
          }`}>
            {healthTone === "safe" ? "NOMINAL" : healthTone === "warn" ? "DEGRADED" : "CRITICAL"}
          </div>
          <div className="text-[10px] text-muted-foreground font-mono mt-0.5">
            composite · {pct(stats.health)}
          </div>
        </div>
      </div>
      <Stat label="Services" value={String(stats.total)} unit="active" />
      <Stat
        label="Unstable"
        value={`${stats.unstable}`}
        hint={pct(stats.unstablePct)}
        tone={unstableTone}
      />
      <Stat
        label="Max Collapse Risk"
        value={pct(stats.maxCollapseRisk)}
        hint={stats.worstService ?? "—"}
        tone={riskTone}
      />
      <Stat
        label="Tick Health"
        value={tick ? fixed(tick.tick_health_ms) : "—"}
        unit={tick ? "ms" : undefined}
        hint={tick ? `jitter ${fixed(tick.jitter_ms)}ms` : undefined}
      />
    </div>
  );
});
