import { memo, useMemo } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { api, apiConfigured } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import type { Event } from "@/lib/types";

const sevTone = (severity: string): string =>
  severity === "critical"
    ? "border-l-crit text-crit"
    : "border-l-warn text-warn";

const fmtTime = (isoString: string): string => {
  try {
    const d = new Date(isoString);
    return d.toLocaleTimeString([], { hour12: false });
  } catch {
    return isoString;
  }
};

export const AlertPanel = memo(function AlertPanel() {
  const rawEvents = useTelemetry((s) => s.tick?.events);
  const acked = useTelemetry((s) => s.ackedEventIds);
  const ackEvent = useTelemetry((s) => s.ackEvent);
  const events = useMemo(
    () => (rawEvents ?? []).filter((e) => !acked.has(e.id ?? "")),
    [rawEvents, acked],
  );
  const { toast } = useToast();

  const handleAck = async (e: Event) => {
    if (!e.id) return;
    ackEvent(e.id); // optimistic
    if (!apiConfigured()) return;
    try {
      await api.ackAlert(e.id);
    } catch (err) {
      toast({
        title: "Acknowledge failed",
        description: err instanceof Error ? err.message : String(err),
        variant: "destructive",
      });
    }
  };

  const critCount = events.filter((e) => e.severity === "critical").length;
  return (
    <section className="panel flex flex-col h-full min-h-0">
      <header className="panel-header justify-between">
        <div className="flex items-center gap-2">
          <span className={`led ${critCount > 0 ? "led-crit animate-pulse-dot" : events.length > 0 ? "led-warn" : "led-off"}`} />
          <span>Alert Log</span>
        </div>
        <span className="font-mono normal-case tracking-wider text-[9px] text-muted-foreground tabular-nums">
          {events.length.toString().padStart(3, "0")} OPEN
        </span>
      </header>
      <div className="flex-1 min-h-0 overflow-y-auto">
        {events.length === 0 ? (
          <div className="px-3 py-8 text-[11px] text-muted-foreground text-center font-mono uppercase tracking-widest">
            ── no active alerts ──
          </div>
        ) : (
          <ul className="divide-y divide-border">
            {events.map((e) => (
              <li
                key={e.id}
                className={`pl-3 pr-2 py-2 border-l-2 hover:bg-surface-2/40 transition-colors ${sevTone(e.severity)}`}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-[9px] uppercase tracking-widest font-mono">
                      <span className={e.severity === "critical" ? "text-crit" : "text-warn"}>
                        {e.severity === "critical" ? "● CRIT" : "● WARN"}
                      </span>
                      <span className="text-muted-foreground">{e.category}</span>
                      <span className="text-muted-foreground/70 ml-auto">
                        {fmtTime(e.timestamp)}
                      </span>
                    </div>
                    <div className="text-[12px] text-foreground mt-1 leading-snug">
                      {e.description}
                    </div>
                    {e.recommendation && (
                      <div className="text-[10px] text-muted-foreground mt-1 flex items-start gap-1">
                        <span className="text-phosphor">▸</span>
                        <span>{e.recommendation}</span>
                      </div>
                    )}
                  </div>
                  <button
                    onClick={() => handleAck(e)}
                    className="shrink-0 text-[9px] uppercase tracking-widest font-mono px-2 py-1 rounded-sm border border-border hover:border-border-strong hover:bg-surface-3 text-muted-foreground hover:text-foreground transition-colors"
                  >
                    ACK
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
});
