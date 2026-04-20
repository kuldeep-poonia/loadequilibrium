// Per-service detail with sparkline trends drawn directly via SVG (no chart deps).
import { memo, useMemo } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import {
  collapseEta,
  fixed,
  ms,
  pct,
  trendLabel,
  zoneColor,
} from "@/lib/format";
import type { ServiceHistoryPoint } from "@/store/telemetryStore";

const SPARK_W = 240;
const SPARK_H = 40;

const Sparkline = ({
  points,
  selector,
  tone,
  domain,
}: {
  points: ServiceHistoryPoint[];
  selector: (p: ServiceHistoryPoint) => number;
  tone: "safe" | "warn" | "crit";
  domain?: [number, number];
}) => {
  if (points.length < 2) {
    return (
      <div className="h-10 grid place-items-center text-[10px] text-muted-foreground">
        gathering…
      </div>
    );
  }
  const vals = points.map(selector);
  const lo = domain ? domain[0] : Math.min(...vals);
  const hi = domain ? domain[1] : Math.max(...vals);
  const range = hi - lo || 1;
  const step = SPARK_W / (points.length - 1);
  const stroke =
    tone === "crit"
      ? "hsl(var(--crit))"
      : tone === "warn"
        ? "hsl(var(--warn))"
        : "hsl(var(--safe))";
  const d = vals
    .map((v, i) => {
      const x = i * step;
      const y = SPARK_H - ((v - lo) / range) * SPARK_H;
      return `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
  return (
    <svg viewBox={`0 0 ${SPARK_W} ${SPARK_H}`} className="w-full h-10">
      <path d={d} fill="none" stroke={stroke} strokeWidth={1.25} />
    </svg>
  );
};

const Row = ({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) => (
  <div className="flex items-baseline justify-between gap-3 text-xs">
    <span className="text-muted-foreground">{label}</span>
    <span className="font-mono text-foreground">
      {value}
      {hint && (
        <span className="text-muted-foreground ml-1.5">{hint}</span>
      )}
    </span>
  </div>
);

export const ServiceDetail = memo(function ServiceDetail() {
  const selectedId = useTelemetry((s) => s.selectedServiceId);
  const bundle = useTelemetry((s) =>
    selectedId ? s.tick?.bundles[selectedId] ?? null : null,
  );
  const directive = useTelemetry((s) =>
    selectedId ? s.tick?.directives[selectedId] ?? null : null,
  );
  const historyMap = useTelemetry((s) => s.history);
  const select = useTelemetry((s) => s.selectService);
  const history = useMemo(
    () => (selectedId ? historyMap[selectedId] ?? [] : []),
    [historyMap, selectedId],
  );

  const tone = useMemo(
    () => (bundle ? zoneColor(bundle.stability.collapse_zone) : "safe"),
    [bundle],
  );

  if (!selectedId) {
    return (
      <section className="panel flex flex-col h-full min-h-0">
        <header className="panel-header">
          <span className="led led-off" />
          <span>Inspect Channel</span>
        </header>
        <div className="flex-1 grid place-items-center text-[10px] text-muted-foreground px-4 text-center font-mono uppercase tracking-widest">
          ── select a service ──
        </div>
      </section>
    );
  }

  if (!bundle) {
    return (
      <section className="panel flex flex-col h-full min-h-0">
        <header className="panel-header justify-between">
          <div className="flex items-center gap-2">
            <span className="led led-off" />
            <span className="normal-case tracking-normal text-foreground">{selectedId}</span>
          </div>
          <button onClick={() => select(null)} className="text-[9px] uppercase tracking-widest font-mono text-muted-foreground hover:text-foreground">
            ✕ CLOSE
          </button>
        </header>
        <div className="flex-1 grid place-items-center text-[10px] text-muted-foreground font-mono uppercase tracking-widest">
          ── service offline ──
        </div>
      </section>
    );
  }

  const eta = collapseEta(bundle.stability.predicted_collapse_ms);
  const ledClass = tone === "crit" ? "led-crit animate-pulse-dot" : tone === "warn" ? "led-warn" : "led-safe";

  return (
    <section className="panel flex flex-col h-full min-h-0">
      <header className="panel-header justify-between">
        <div className="flex items-center gap-2">
          <span className={`led ${ledClass}`} />
          <span className="normal-case tracking-normal text-foreground text-[12px] font-medium">
            {selectedId}
          </span>
        </div>
        <button onClick={() => select(null)} className="text-[9px] uppercase tracking-widest font-mono text-muted-foreground hover:text-foreground">
          ✕ CLOSE
        </button>
      </header>
      <div className="flex-1 min-h-0 overflow-y-auto p-3 space-y-4">
        <div>
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-1">
            Utilisation (ρ)
          </div>
          <Sparkline
            points={history}
            selector={(p) => p.utilisation}
            tone={
              bundle.queue.utilisation >= 0.9
                ? "crit"
                : bundle.queue.utilisation >= 0.75
                  ? "warn"
                  : "safe"
            }
            domain={[0, Math.max(1, bundle.queue.utilisation * 1.05)]}
          />
        </div>
        <div>
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-1">
            Collapse risk
          </div>
          <Sparkline
            points={history}
            selector={(p) => p.collapse_risk}
            tone={tone}
            domain={[0, 1]}
          />
        </div>

        <div className="space-y-1.5 pt-1">
          <Row
            label="Zone"
            value={bundle.stability.collapse_zone.toUpperCase()}
          />
          <Row
            label="Predicted collapse"
            value={eta ?? "n/a"}
            hint={
              bundle.stability.predicted_collapse_ms > 0
                ? `(${ms(bundle.stability.predicted_collapse_ms)})`
                : undefined
            }
          />
          <Row
            label="Saturation horizon"
            value={
              bundle.queue.saturation_horizon > 0
                ? `${fixed(bundle.queue.saturation_horizon)}s`
                : "—"
            }
          />
          <Row label="Arrival rate" value={`${fixed(bundle.queue.arrival_rate)} req/s`} />
          <Row label="Service rate" value={`${fixed(bundle.queue.service_rate)} req/s`} />
          <Row
            label="Trend"
            value={trendLabel(bundle.queue.utilisation_trend)}
            hint={`(${fixed(bundle.queue.utilisation_trend, 3)}/s)`}
          />
          <Row
            label="Stability margin"
            value={pct(bundle.stability.stability_margin)}
          />
          <Row
            label="Oscillation risk"
            value={pct(bundle.stability.oscillation_risk)}
          />
          <Row
            label="Cascade amplification"
            value={fixed(bundle.stability.cascade_amplification_score, 2)}
          />
          <Row
            label="Confidence"
            value={pct(bundle.queue.confidence)}
          />
        </div>

        {directive && (
          <div className="pt-2 border-t border-border">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-2">
              Active directive
            </div>
            <div className="space-y-1.5">
              <Row
                label="State"
                value={directive.active ? "ACTIVE" : "idle"}
              />
              <Row
                label="Scale factor"
                value={`×${fixed(directive.scale_factor, 2)}`}
              />
              <Row
                label="Target ρ"
                value={pct(directive.target_utilisation)}
              />
              <Row
                label="Urgency"
                value={fixed(directive.cost_gradient, 3)}
              />
            </div>
          </div>
        )}
      </div>
    </section>
  );
});
