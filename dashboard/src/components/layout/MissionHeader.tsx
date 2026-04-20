import { memo, useEffect, useState } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { ConnectionBadge } from "./ConnectionBadge";

const fmtClock = (d: Date): string =>
  d.toISOString().split("T")[1]?.slice(0, 8) ?? "--:--:--";

const fmtDate = (d: Date): string =>
  d.toISOString().split("T")[0]?.replace(/-/g, ".") ?? "----.--.--";

const HeaderCell = ({
  label,
  value,
  tone = "default",
  className = "",
  glow = false,
}: {
  label: string;
  value: React.ReactNode;
  tone?: "default" | "phosphor" | "amber" | "warn" | "crit";
  className?: string;
  glow?: boolean;
}) => {
  const valTone =
    tone === "phosphor" ? "text-phosphor"
    : tone === "amber" ? "text-amber"
    : tone === "warn" ? "text-warn"
    : tone === "crit" ? "text-crit"
    : "text-foreground";
  const glowClass =
    glow && tone === "phosphor" ? "glow-phosphor"
    : glow && tone === "amber" ? "glow-amber"
    : glow && tone === "warn" ? "glow-warn"
    : glow && tone === "crit" ? "glow-crit" : "";
  return (
    <div className={`flex flex-col justify-center px-3 border-r border-border-strong/60 ${className}`}>
      <div className="text-[8px] font-mono uppercase tracking-[0.25em] text-muted-foreground/80 leading-none">
        {label}
      </div>
      <div className={`text-[13px] readout leading-tight mt-0.5 ${valTone} ${glowClass}`}>
        {value}
      </div>
    </div>
  );
};

export const MissionHeader = memo(function MissionHeader() {
  const [now, setNow] = useState(() => new Date());
  const cp = useTelemetry((s) => s.tick?.control_plane);
  const safetyMode = useTelemetry((s) => s.tick?.safety_mode ?? false);
  const status = useTelemetry((s) => s.status);

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const armed = cp?.actuation_enabled ?? false;

  return (
    <header className="relative border-b-2 border-border-strong">
      <div className="h-[3px] hazard-stripe opacity-60" />

      <div className="relative chrome-strip">
        <div className="scanlines absolute inset-0 pointer-events-none opacity-50" />
        <div className="relative flex items-stretch h-14">

          <div className="flex items-center gap-3 px-4 border-r-2 border-border-strong bg-surface-1/60 min-w-[280px]">
            <div className="flex flex-col gap-1">
              <span className="grommet" />
              <span className="grommet" />
            </div>
            <div className="leading-none">
              <div className="flex items-center gap-2">
                <span className="led led-phosphor animate-pulse-dot" />
                <span className="text-[13px] font-medium tracking-[0.3em] uppercase text-phosphor glow-phosphor">
                  CONTROL PLANE
                </span>
              </div>
              <div className="text-[9px] font-mono uppercase tracking-[0.3em] text-muted-foreground mt-1.5">
                MC-1 ▸ TELEMETRY ◂ OPS
              </div>
            </div>
            <div className="flex flex-col gap-1 ml-auto">
              <span className="grommet" />
              <span className="grommet" />
            </div>
          </div>

          <HeaderCell label="MODE" tone={safetyMode ? "warn" : "phosphor"} glow value={safetyMode ? "SAFETY" : "NOMINAL"} className="min-w-[100px]" />
          <HeaderCell label="ACTUATION" tone={armed ? "phosphor" : "amber"} glow value={armed ? "● ARMED" : "○ SAFE"} className="min-w-[110px]" />
          <HeaderCell label="POLICY" tone="amber" value={(cp?.policy_preset ?? "----").toUpperCase()} className="min-w-[130px]" />
          <HeaderCell label="TICK" value={cp ? cp.tick.toString().padStart(6, "0") : "------"} className="min-w-[100px]" />
          <HeaderCell label="ACK" value={(cp?.acknowledged_alert_count ?? 0).toString().padStart(3, "0")} className="min-w-[80px]" />

          <div className="flex-1 border-r border-border-strong/60" />

          <div className="flex items-center px-4 border-r border-border-strong/60">
            <ConnectionBadge />
          </div>

          <HeaderCell
            label="UTC TIME"
            tone="phosphor"
            glow
            value={
              <span>
                {fmtClock(now)}
                <span className="text-muted-foreground ml-2 text-[10px]">
                  {fmtDate(now)}
                </span>
              </span>
            }
            className="min-w-[200px] border-r-0"
          />

          <div className="flex flex-col justify-center gap-1 px-3 border-l-2 border-border-strong bg-surface-1/60">
            <span className="grommet" />
            <span className="grommet" />
          </div>
        </div>
      </div>

      <div className="relative bg-surface-1/80 border-t border-border-strong h-6 flex items-stretch overflow-hidden">
        <div className="flex items-center gap-2 px-3 border-r border-border-strong bg-phosphor-deep/50">
          <span className={`led ${status === "open" ? "led-phosphor animate-pulse-dot" : "led-warn animate-pulse-dot"}`} />
          <span className="text-[9px] font-mono uppercase tracking-[0.25em] text-phosphor">UPLINK</span>
        </div>
        <div className="flex-1 overflow-hidden flex items-center">
          <div className="flex items-center gap-8 whitespace-nowrap text-[10px] font-mono uppercase tracking-[0.2em] text-muted-foreground/80 px-4 animate-ticker">
            {Array.from({ length: 2 }).map((_, k) => (
              <span key={k} className="flex items-center gap-8">
                <span className="text-phosphor">▸</span><span>SYSTEM TELEMETRY ACTIVE</span>
                <span className="text-amber">◆</span><span>ALL CHANNELS NOMINAL</span>
                <span className="text-phosphor">▸</span><span>STABILITY MARGIN OPTIMAL</span>
                <span className="text-amber">◆</span><span>POLICY ENGINE ENGAGED</span>
                <span className="text-phosphor">▸</span><span>MONITORING {cp?.acknowledged_alert_count ?? 0} EVENTS</span>
                <span className="text-amber">◆</span><span>LINK QUALITY {status === "open" ? "100%" : "WAITING"}</span>
              </span>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-2 px-3 border-l border-border-strong">
          <span className="text-[9px] font-mono uppercase tracking-[0.25em] text-muted-foreground">REV 1.0.0</span>
        </div>
      </div>
    </header>
  );
});
