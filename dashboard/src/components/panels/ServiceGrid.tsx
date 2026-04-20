import { memo, useMemo } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { collapseEta, pct, sortByRisk, zoneColor } from "@/lib/format";
import type { ServiceBundle } from "@/lib/types";

const zoneStyles = {
  safe: { dot: "bg-safe", ring: "ring-safe/30", text: "text-safe" },
  warn: { dot: "bg-warn", ring: "ring-warn/40", text: "text-warn" },
  crit: { dot: "bg-crit", ring: "ring-crit/50", text: "text-crit" },
} as const;

const Bar = ({ value, tone }: { value: number; tone: "safe" | "warn" | "crit" }) => {
  const w = `${Math.min(100, Math.max(0, value * 100))}%`;
  const bg = tone === "crit" ? "bg-crit" : tone === "warn" ? "bg-warn" : "bg-safe";
  return (
    <div className="h-1 w-full bg-surface-3 rounded-sm overflow-hidden">
      <div className={`h-full ${bg}`} style={{ width: w }} />
    </div>
  );
};

const ServiceCard = memo(function ServiceCard({
  bundle,
  selected,
  onSelect,
}: {
  bundle: ServiceBundle;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  const id = bundle.queue.service_id;
  const tone = zoneColor(bundle.stability.collapse_zone);
  const s = zoneStyles[tone];
  const eta = collapseEta(bundle.stability.predicted_collapse_ms);

  return (
    <button
      onClick={() => onSelect(id)}
      className={`text-left panel p-3 transition-colors hover:bg-surface-2 ${
        selected ? `ring-1 ${s.ring}` : ""
      }`}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`h-2 w-2 rounded-full shrink-0 ${s.dot}`} />
          <span className="text-sm font-medium truncate">{id}</span>
        </div>
        {eta && tone !== "safe" && (
          <span className={`text-[10px] font-mono ${s.text}`}>
            {eta}
          </span>
        )}
      </div>
      <div className="mt-2.5 space-y-2">
        <div>
          <div className="flex justify-between text-[10px] text-muted-foreground font-mono">
            <span>ρ utilisation</span>
            <span>{pct(bundle.queue.utilisation)}</span>
          </div>
          <Bar
            value={bundle.queue.utilisation}
            tone={
              bundle.queue.utilisation >= 0.9
                ? "crit"
                : bundle.queue.utilisation >= 0.75
                  ? "warn"
                  : "safe"
            }
          />
        </div>
        <div>
          <div className="flex justify-between text-[10px] text-muted-foreground font-mono">
            <span>collapse risk</span>
            <span className={s.text}>{pct(bundle.stability.collapse_risk)}</span>
          </div>
          <Bar value={bundle.stability.collapse_risk} tone={tone} />
        </div>
      </div>
    </button>
  );
});

export const ServiceGrid = memo(function ServiceGrid() {
  const bundles = useTelemetry((s) => s.tick?.bundles);
  const selectedId = useTelemetry((s) => s.selectedServiceId);
  const select = useTelemetry((s) => s.selectService);

  const sorted = useMemo(
    () => (bundles ? sortByRisk(bundles) : []),
    [bundles],
  );

  const critCount = sorted.filter((b) => b.stability.collapse_zone === "collapse").length;
  return (
    <section className="panel flex flex-col h-full min-h-0">
      <header className="panel-header justify-between">
        <div className="flex items-center gap-2">
          <span className={`led ${critCount > 0 ? "led-crit animate-pulse-dot" : sorted.length > 0 ? "led-phosphor" : "led-off"}`} />
          <span>Service Matrix</span>
        </div>
        <span className="font-mono normal-case tracking-wider text-[9px] text-muted-foreground tabular-nums">
          {sorted.length.toString().padStart(3, "0")} TRACKED
        </span>
      </header>
      <div className="flex-1 min-h-0 overflow-y-auto p-2">
        {sorted.length === 0 ? (
          <div className="text-[10px] text-muted-foreground text-center py-10 font-mono uppercase tracking-widest">
            ── no services reporting ──
          </div>
        ) : (
          <div className="grid gap-2 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]">
            {sorted.map((b) => (
              <ServiceCard
                key={b.queue.service_id}
                bundle={b}
                selected={selectedId === b.queue.service_id}
                onSelect={select}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
});
