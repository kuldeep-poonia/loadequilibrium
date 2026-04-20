import { memo } from "react";
import { useTelemetry } from "@/store/telemetryStore";

const dotClass = (s: string): string => {
  switch (s) {
    case "open":
      return "led-safe animate-pulse-dot";
    case "connecting":
      return "led-warn animate-pulse-dot";
    case "error":
    case "closed":
      return "led-crit";
    default:
      return "led-off";
  }
};

const label = (s: string): string => {
  switch (s) {
    case "open":
      return "LIVE";
    case "connecting":
      return "LINK…";
    case "closed":
      return "OFFLINE";
    case "error":
      return "ERROR";
    default:
      return "STANDBY";
  }
};

const labelTone = (s: string): string => {
  switch (s) {
    case "open":
      return "text-safe";
    case "connecting":
      return "text-warn";
    case "error":
    case "closed":
      return "text-crit";
    default:
      return "text-muted-foreground";
  }
};

export const ConnectionBadge = memo(function ConnectionBadge() {
  const status = useTelemetry((s) => s.status);
  const seq = useTelemetry((s) => s.lastSeq);
  return (
    <div className="flex items-center gap-2">
      <span className={`led ${dotClass(status)}`} />
      <div className="leading-tight">
        <div
          className={`text-[10px] font-mono uppercase tracking-widest ${labelTone(status)}`}
        >
          {label(status)}
        </div>
        {seq >= 0 && status === "open" && (
          <div className="text-[9px] font-mono text-muted-foreground tabular-nums">
            seq {seq.toString().padStart(6, "0")}
          </div>
        )}
      </div>
    </div>
  );
});
