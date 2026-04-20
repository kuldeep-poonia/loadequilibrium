// Minimal force-free topology: deterministic radial layout, colored by risk.
import { memo, useMemo } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import { riskZone, zoneColor } from "@/lib/format";
import type { GraphEdge } from "@/lib/types";

interface Node {
  id: string;
  x: number;
  y: number;
  risk: number;
}

const W = 600;
const H = 360;

const layout = (ids: string[]): Map<string, { x: number; y: number }> => {
  const map = new Map<string, { x: number; y: number }>();
  const n = ids.length;
  if (n === 0) return map;
  if (n === 1) {
    map.set(ids[0], { x: W / 2, y: H / 2 });
    return map;
  }
  const cx = W / 2;
  const cy = H / 2;
  const r = Math.min(W, H) * 0.38;
  ids.forEach((id, i) => {
    const a = (i / n) * Math.PI * 2 - Math.PI / 2;
    map.set(id, { x: cx + Math.cos(a) * r, y: cy + Math.sin(a) * r });
  });
  return map;
};

const toneFill = (risk: number): string => {
  const z = riskZone(risk);
  const t = zoneColor(z);
  return t === "crit"
    ? "hsl(var(--crit))"
    : t === "warn"
      ? "hsl(var(--warn))"
      : "hsl(var(--safe))";
};

export const TopologyGraph = memo(function TopologyGraph() {
  const tick = useTelemetry((s) => s.tick);
  const selectedId = useTelemetry((s) => s.selectedServiceId);
  const select = useTelemetry((s) => s.selectService);

  const { nodes, edges } = useMemo(() => {
    if (!tick) return { nodes: [] as Node[], edges: [] as GraphEdge[] };
    // Use bundle keys as authoritative service set; topology may add edges.
    const ids = Object.keys(tick.bundles ?? {});
    const pos = layout(ids);
    const ns: Node[] = ids.map((id) => {
      const p = pos.get(id) ?? { x: W / 2, y: H / 2 };
      const risk = tick.bundles[id]?.stability.collapse_risk ?? 0;
      return { id, x: p.x, y: p.y, risk };
    });
    const rawEdges = (tick.topology?.edges ?? []).filter(
      (e) => pos.has(e.source) && pos.has(e.target),
    );
    return { nodes: ns, edges: rawEdges };
  }, [tick]);

  if (nodes.length === 0) {
    return (
      <section className="panel flex flex-col h-full min-h-0">
        <header className="panel-header">
          <span className="led led-off" />
          <span>Service Topology</span>
        </header>
        <div className="flex-1 grid place-items-center text-[10px] text-muted-foreground font-mono uppercase tracking-widest">
          ── no topology data ──
        </div>
      </section>
    );
  }

  const posMap = new Map(nodes.map((n) => [n.id, n]));
  const critCount = nodes.filter((n) => n.risk >= 0.7).length;

  return (
    <section className="panel flex flex-col h-full min-h-0">
      <header className="panel-header justify-between">
        <div className="flex items-center gap-2">
          <span className={`led ${critCount > 0 ? "led-crit animate-pulse-dot" : "led-phosphor"}`} />
          <span>Service Topology</span>
        </div>
        <span className="font-mono normal-case tracking-wider text-[9px] text-muted-foreground tabular-nums">
          {nodes.length} NODES · {edges.length} EDGES
        </span>
      </header>
      <div className="flex-1 min-h-0 p-2 relative bg-surface-1/40 crt-wash">
        <svg
          viewBox={`0 0 ${W} ${H}`}
          className="w-full h-full"
          preserveAspectRatio="xMidYMid meet"
        >
          <defs>
            <pattern id="topo-grid" width="30" height="30" patternUnits="userSpaceOnUse">
              <path d="M 30 0 L 0 0 0 30" fill="none" stroke="hsl(var(--phosphor) / 0.18)" strokeWidth="0.5" />
            </pattern>
            <pattern id="topo-grid-fine" width="6" height="6" patternUnits="userSpaceOnUse">
              <path d="M 6 0 L 0 0 0 6" fill="none" stroke="hsl(var(--phosphor) / 0.06)" strokeWidth="0.3" />
            </pattern>
            <radialGradient id="topo-vignette" cx="50%" cy="50%" r="60%">
              <stop offset="50%" stopColor="hsl(var(--background))" stopOpacity="0" />
              <stop offset="100%" stopColor="hsl(var(--background))" stopOpacity="0.85" />
            </radialGradient>
            <radialGradient id="radar-sweep" cx="50%" cy="50%" r="50%">
              <stop offset="0%" stopColor="hsl(var(--phosphor))" stopOpacity="0.35" />
              <stop offset="40%" stopColor="hsl(var(--phosphor))" stopOpacity="0.12" />
              <stop offset="100%" stopColor="hsl(var(--phosphor))" stopOpacity="0" />
            </radialGradient>
            <marker
              id="arrow"
              viewBox="0 0 10 10"
              refX="9"
              refY="5"
              markerWidth="5"
              markerHeight="5"
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill="hsl(var(--phosphor-dim))" />
            </marker>
          </defs>
          <rect width={W} height={H} fill="url(#topo-grid-fine)" />
          <rect width={W} height={H} fill="url(#topo-grid)" />
          {/* Concentric reference rings + crosshair */}
          <g opacity="0.5">
            {[0.45, 0.32, 0.2, 0.1].map((f) => (
              <circle
                key={f}
                cx={W / 2}
                cy={H / 2}
                r={Math.min(W, H) * f}
                fill="none"
                stroke="hsl(var(--phosphor) / 0.3)"
                strokeWidth="0.5"
                strokeDasharray="3 5"
              />
            ))}
            <line x1={W / 2} y1="0" x2={W / 2} y2={H} stroke="hsl(var(--phosphor) / 0.25)" strokeWidth="0.5" strokeDasharray="2 4" />
            <line x1="0" y1={H / 2} x2={W} y2={H / 2} stroke="hsl(var(--phosphor) / 0.25)" strokeWidth="0.5" strokeDasharray="2 4" />
            {Array.from({ length: 36 }).map((_, i) => {
              const a = (i / 36) * Math.PI * 2;
              const r1 = Math.min(W, H) * 0.45;
              const r2 = r1 + (i % 3 === 0 ? 8 : 4);
              const x1 = W / 2 + Math.cos(a) * r1;
              const y1 = H / 2 + Math.sin(a) * r1;
              const x2 = W / 2 + Math.cos(a) * r2;
              const y2 = H / 2 + Math.sin(a) * r2;
              return <line key={i} x1={x1} y1={y1} x2={x2} y2={y2} stroke="hsl(var(--phosphor) / 0.5)" strokeWidth="0.5" />;
            })}
          </g>
          {/* Radar sweep */}
          <g style={{ transformOrigin: `${W / 2}px ${H / 2}px` }} className="animate-radar">
            <path
              d={`M ${W / 2} ${H / 2} L ${W / 2 + Math.min(W, H) * 0.5} ${H / 2} A ${Math.min(W, H) * 0.5} ${Math.min(W, H) * 0.5} 0 0 0 ${W / 2 + Math.cos(-Math.PI / 4) * Math.min(W, H) * 0.5} ${H / 2 + Math.sin(-Math.PI / 4) * Math.min(W, H) * 0.5} Z`}
              fill="url(#radar-sweep)"
            />
          </g>
          <rect width={W} height={H} fill="url(#topo-vignette)" />

          <g>
            {edges.map((e, i) => {
              const a = posMap.get(e.source);
              const b = posMap.get(e.target);
              if (!a || !b) return null;
              const maxRisk = Math.max(a.risk, b.risk);
              const stroke = maxRisk >= 0.7 ? "hsl(var(--crit))"
                : maxRisk >= 0.4 ? "hsl(var(--warn))"
                : "hsl(var(--border-strong))";
              return (
                <line
                  key={`${e.source}-${e.target}-${i}`}
                  x1={a.x} y1={a.y} x2={b.x} y2={b.y}
                  stroke={stroke}
                  strokeWidth={maxRisk >= 0.4 ? 1.5 : 1}
                  markerEnd="url(#arrow)"
                  opacity={maxRisk >= 0.4 ? 0.85 : 0.6}
                />
              );
            })}
          </g>
          <g>
            {nodes.map((n) => {
              const fill = toneFill(n.risk);
              const isSel = selectedId === n.id;
              const isCrit = n.risk >= 0.7;
              return (
                <g
                  key={n.id}
                  transform={`translate(${n.x}, ${n.y})`}
                  className="cursor-pointer"
                  onClick={() => select(n.id)}
                >
                  {isCrit && (
                    <circle r={16} fill={fill} opacity="0.2" className="animate-pulse-dot" />
                  )}
                  <circle
                    r={isSel ? 11 : 8}
                    fill={fill}
                    stroke={isSel ? "hsl(var(--foreground))" : "hsl(var(--background))"}
                    strokeWidth={2}
                    style={{ filter: `drop-shadow(0 0 ${isCrit ? 6 : 3}px ${fill})` }}
                  />
                  <text
                    y={-14}
                    textAnchor="middle"
                    fontSize={9}
                    fill={isSel ? "hsl(var(--foreground))" : "hsl(var(--muted-foreground))"}
                    className="font-mono select-none pointer-events-none uppercase tracking-wider"
                  >
                    {n.id}
                  </text>
                </g>
              );
            })}
          </g>
        </svg>
      </div>
    </section>
  );
});
