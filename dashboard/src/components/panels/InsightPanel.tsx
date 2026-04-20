// AI insight panel: turns raw numbers into actionable, human-readable claims.
import { memo, useMemo } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { collapseEta, pct, sortByRisk, trendLabel } from "@/lib/format";
import type { TickPayload } from "@/lib/types";

type Tone = "safe" | "warn" | "crit" | "info";

interface Insight {
  id: string;
  tone: Tone;
  title: string;
  detail: string;
  action?: string;
}

const buildInsights = (tick: TickPayload | null): Insight[] => {
  if (!tick) return [];
  const out: Insight[] = [];
  const bundles = sortByRisk(tick.bundles);

  // 1. Imminent collapse
  const imminent = bundles.find(
    (b) =>
      b.stability.predicted_collapse_ms > 0 &&
      b.stability.predicted_collapse_ms < 10_000,
  );
  if (imminent) {
    const eta = collapseEta(imminent.stability.predicted_collapse_ms);
    out.push({
      id: `collapse-${imminent.queue.service_id}`,
      tone: "crit",
      title: `${imminent.queue.service_id} will likely collapse in ${eta ?? "soon"}`,
      detail: `Risk ${pct(imminent.stability.collapse_risk)} · utilisation ${pct(imminent.queue.utilisation)} · trend ${trendLabel(imminent.queue.utilisation_trend)}`,
      action: "Reduce arrival rate or scale immediately",
    });
  }

  // 2. Cascade risk
  const cascade = tick.objective.cascade_failure_probability;
  const worstCascade = bundles[0];
  if (cascade >= 0.4 && worstCascade) {
    const amp = worstCascade.stability.cascade_amplification_score;
    const affected = Math.max(1, Math.round(amp));
    out.push({
      id: "cascade",
      tone: cascade >= 0.7 ? "crit" : "warn",
      title: `Failure may cascade to ~${affected} downstream service${affected === 1 ? "" : "s"}`,
      detail: `Cascade probability ${pct(cascade)} · originating at ${worstCascade.queue.service_id}`,
      action: "Isolate upstream traffic or activate sandbox",
    });
  }

  // 3. Stability trend
  const rising = bundles.filter(
    (b) => b.queue.utilisation_trend > 0.05 && b.stability.collapse_risk > 0.3,
  );
  if (rising.length >= 2) {
    out.push({
      id: "trend",
      tone: "warn",
      title: "System stability degrading",
      detail: `${rising.length} services trending toward saturation`,
      action: "Switch policy to conservative",
    });
  }

  // 4. Oscillation
  if (tick.objective.oscillation_risk >= 0.5) {
    out.push({
      id: "oscillation",
      tone: "warn",
      title: "Control loop oscillation detected",
      detail: `Oscillation risk ${pct(tick.objective.oscillation_risk)}`,
      action: "Dampen scaling or extend simulation horizon",
    });
  }

  // 5. Predicted P99
  if (tick.objective.predicted_p99_latency_ms > 250) {
    out.push({
      id: "p99",
      tone: tick.objective.predicted_p99_latency_ms > 500 ? "crit" : "warn",
      title: `P99 latency projected to ${Math.round(tick.objective.predicted_p99_latency_ms)}ms`,
      detail: "Tail latency budget likely to be breached",
      action: "Investigate slow service in detail panel",
    });
  }

  // 6. All clear
  if (out.length === 0) {
    out.push({
      id: "clear",
      tone: "safe",
      title: "System nominal",
      detail: `Composite score ${pct(Math.max(0, Math.min(1, tick.objective.composite_score)))} · all services within stability margin`,
    });
  }
  return out.slice(0, 6);
};

const toneClasses: Record<Tone, { bar: string; title: string }> = {
  crit: { bar: "bg-crit", title: "text-crit" },
  warn: { bar: "bg-warn", title: "text-warn" },
  safe: { bar: "bg-safe", title: "text-safe" },
  info: { bar: "bg-info", title: "text-info" },
};

export const InsightPanel = memo(function InsightPanel() {
  const tick = useTelemetry((s) => s.tick);
  const insights = useMemo(() => buildInsights(tick), [tick]);

  const critCount = insights.filter((i) => i.tone === "crit").length;
  return (
    <section className="panel flex flex-col h-full min-h-0">
      <header className="panel-header justify-between">
        <div className="flex items-center gap-2">
          <span className={`led ${critCount > 0 ? "led-crit animate-pulse-dot" : "led-phosphor"}`} />
          <span>Mission Insights</span>
        </div>
        <span className="font-mono normal-case tracking-wider text-[9px] text-muted-foreground">
          {insights.length} active{critCount > 0 ? ` · ${critCount} crit` : ""}
        </span>
      </header>
      <div className="flex-1 min-h-0 overflow-y-auto divide-y divide-border">
        {insights.map((i) => {
          const c = toneClasses[i.tone];
          return (
            <article key={i.id} className="flex gap-3 px-3 py-2.5 hover:bg-surface-2/40 transition-colors">
              <span className={`mt-1.5 h-2 w-2 rounded-full shrink-0 ${c.bar}`} />
              <div className="min-w-0 flex-1">
                <div className={`text-[13px] font-medium leading-snug ${c.title}`}>
                  {i.title}
                </div>
                <div className="text-[10px] text-muted-foreground mt-0.5 font-mono uppercase tracking-wider">
                  {i.detail}
                </div>
                {i.action && (
                  <div className="text-[11px] mt-1.5 text-foreground/90 flex items-start gap-1.5">
                    <span className="text-phosphor">▸</span>
                    <span>{i.action}</span>
                  </div>
                )}
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
});
