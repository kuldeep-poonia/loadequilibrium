'use client';

import React, { useEffect, useRef } from 'react';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { TacticalBox } from '@/components/ui/HUD';
import { motion } from 'framer-motion';
import type { Node, Edge } from '@/types/backend';

export function TopologyCanvas() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const { tick } = useTelemetryStore();
  const animationFrameRef = useRef<number>(0);
  const pulsePhaseRef = useRef(0);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const render = () => {
      const dpr = window.devicePixelRatio || 1;
      const rect = canvas.getBoundingClientRect();
      if (rect.width === 0) { animationFrameRef.current = requestAnimationFrame(render); return; }

      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      ctx.scale(dpr, dpr);

      const w = rect.width;
      const h = rect.height;
      ctx.clearRect(0, 0, w, h);

      // Grid
      ctx.strokeStyle = "rgba(0, 242, 255, 0.04)";
      ctx.lineWidth = 1;
      for (let x = 0; x < w; x += 30) { ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke(); }
      for (let y = 0; y < h; y += 30) { ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke(); }

      // Reticle
      ctx.strokeStyle = "rgba(0, 242, 255, 0.08)";
      ctx.beginPath(); ctx.moveTo(w / 2 - 15, h / 2); ctx.lineTo(w / 2 + 15, h / 2); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(w / 2, h / 2 - 15); ctx.lineTo(w / 2, h / 2 + 15); ctx.stroke();

      const nodes: Node[] = tick?.topology?.nodes || [];
      const edges: Edge[] = tick?.topology?.edges || [];
      const critPath = tick?.topology?.critical_path?.nodes || [];
      const heatmap = tick?.pressure_heatmap || {};
      const solverActive = Boolean(tick && !tick.safety_mode && (tick.tick_health_ms ?? 0) > 0);

      if (nodes.length === 0) {
        ctx.fillStyle = 'rgba(148,163,184,0.3)';
        ctx.font = "9px 'Orbitron'";
        ctx.textAlign = 'center';
        ctx.fillText('AWAITING TOPOLOGY DATA', w / 2, h / 2);
        animationFrameRef.current = requestAnimationFrame(render);
        return;
      }

      pulsePhaseRef.current = (pulsePhaseRef.current + 0.03) % (Math.PI * 2);

      // Position nodes in a circle
      const positions = new Map<string, { x: number; y: number }>();
      nodes.forEach((n, i) => {
        const angle = (i / nodes.length) * Math.PI * 2 - Math.PI / 2;
        const r = Math.min(w, h) * 0.34;
        positions.set(n.service_id, {
          x: w / 2 + Math.cos(angle) * r,
          y: h / 2 + Math.sin(angle) * r
        });
      });

      // Draw edges with weight-based opacity
      edges.forEach(e => {
        const s = positions.get(e.source);
        const t = positions.get(e.target);
        if (!s || !t) return;
        const alpha = 0.05 + Math.min(0.35, (e.weight || 0) * 0.4);
        const isCritEdge = critPath.includes(e.source) && critPath.includes(e.target);
        ctx.strokeStyle = isCritEdge ? `rgba(255, 51, 102, ${alpha + 0.15})` : `rgba(0, 242, 255, ${alpha})`;
        ctx.lineWidth = isCritEdge ? 2 : 1;
        ctx.beginPath(); ctx.moveTo(s.x, s.y); ctx.lineTo(t.x, t.y); ctx.stroke();

        // Packet propagation dots along edges
        if (e.call_rate > 0) {
          const t_prog = ((pulsePhaseRef.current + (e.weight || 0) * 3) % (Math.PI * 2)) / (Math.PI * 2);
          const px = s.x + (t.x - s.x) * t_prog;
          const py = s.y + (t.y - s.y) * t_prog;
          ctx.fillStyle = isCritEdge ? 'rgba(255, 51, 102, 0.6)' : 'rgba(0, 242, 255, 0.5)';
          ctx.beginPath();
          ctx.arc(px, py, 1.5, 0, Math.PI * 2);
          ctx.fill();
        }
      });

      // Draw nodes with load-based sizing
      nodes.forEach((n, index) => {
        const p = positions.get(n.service_id);
        if (!p) return;
        const isCrit = critPath.includes(n.service_id);
        const load = n.normalised_load || 0;
        const pressure = heatmap[n.service_id] || 0;
        const pulse = 0.5 + 0.5 * Math.sin(pulsePhaseRef.current * 1.8 + index * 0.55);

        // Node radius scales with load
        const baseRadius = 3;
        const maxRadius = 10;
        const radius = baseRadius + load * (maxRadius - baseRadius);

        // Color based on pressure
        const nodeColor = isCrit ? '#ff3366' : pressure > 0.7 ? '#ef4444' : pressure > 0.4 ? '#f59e0b' : '#00f2ff';

        // Glow
        ctx.shadowBlur = isCrit ? 18 : 6 + load * 12;
        ctx.shadowColor = nodeColor;

        // Subtle observatory pulse for active simulations
        ctx.strokeStyle = isCrit ? 'rgba(255, 51, 102, 0.34)' : `rgba(0, 242, 255, ${0.12 + pulse * 0.12 + load * 0.08})`;
        ctx.lineWidth = isCrit ? 1.3 : 1;
        ctx.globalAlpha = solverActive ? 0.18 + pulse * 0.2 + load * 0.08 : 0.12 + load * 0.06;
        ctx.beginPath();
        ctx.arc(p.x, p.y, radius + 3 + load * 4 + pulse * 2.4, 0, Math.PI * 2);
        ctx.stroke();
        ctx.globalAlpha = 1;

        // Outer ring for high-load nodes
        if (load > 0.7) {
          ctx.strokeStyle = nodeColor;
          ctx.lineWidth = 1;
          ctx.globalAlpha = 0.3 + 0.2 * Math.sin(pulsePhaseRef.current * 2);
          ctx.beginPath();
          ctx.arc(p.x, p.y, radius + 4, 0, Math.PI * 2);
          ctx.stroke();
          ctx.globalAlpha = 1;
        }

        ctx.fillStyle = nodeColor;
        ctx.beginPath();
        ctx.arc(p.x, p.y, radius, 0, Math.PI * 2);
        ctx.fill();

        ctx.fillStyle = 'rgba(255, 255, 255, 0.16)';
        ctx.globalAlpha = 0.16 + pulse * 0.12;
        ctx.beginPath();
        ctx.arc(p.x - radius * 0.18, p.y - radius * 0.18, Math.max(1.2, radius * 0.28), 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 1;

        ctx.shadowBlur = 0;

        // Label
        ctx.fillStyle = 'rgba(148, 163, 184, 0.7)';
        ctx.font = "7px 'Orbitron'";
        ctx.textAlign = 'center';
        const label = n.service_id.length > 10 ? n.service_id.substring(0, 10) : n.service_id;
        ctx.fillText(label.toUpperCase(), p.x, p.y + radius + 10);

        // Load percentage
        if (load > 0) {
          ctx.fillStyle = 'rgba(148, 163, 184, 0.4)';
          ctx.font = "6px monospace";
          ctx.fillText(`${(load * 100).toFixed(0)}%`, p.x, p.y + radius + 18);
        }
      });

      animationFrameRef.current = requestAnimationFrame(render);
    };

    render();
    return () => {
      if (animationFrameRef.current) cancelAnimationFrame(animationFrameRef.current);
    };
  }, [tick]);

  return <canvas ref={canvasRef} className="w-full h-full cursor-crosshair" />;
}

export function TopologyModule() {
  const { tick } = useTelemetryStore();
  const critPath = tick?.topology?.critical_path;
  const nodeCount = tick?.topology?.nodes?.length || 0;
  const edgeCount = tick?.topology?.edges?.length || 0;
  const eq = tick?.network_equilibrium;

  return (
    <div className="flex flex-col gap-3 h-full">
      <motion.div initial={{ opacity: 0, scale: 0.98 }} animate={{ opacity: 1, scale: 1 }} transition={{ duration: 0.4 }} className="flex-1 min-h-0 flex flex-col">
        <TacticalBox title="TOPOLOGY MESH" badge={nodeCount > 0 ? `${nodeCount}N/${edgeCount}E` : 'WARMING'} className="flex-1">
          <TopologyCanvas />
        </TacticalBox>
      </motion.div>

      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.3, delay: 0.1 }} className="shrink-0">
        <TacticalBox title="MESH ANALYSIS" status={critPath && critPath.cascade_risk > 0.5 ? 'critical' : 'nominal'}>
          <div className="grid grid-cols-4 gap-3">
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Nodes</span>
              <span className="text-lg font-bold text-cyan-400 font-data">{nodeCount}</span>
            </div>
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Edges</span>
              <span className="text-lg font-bold text-cyan-400 font-data">{edgeCount}</span>
            </div>
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">ρ Mean</span>
              <span className={`text-lg font-bold font-data ${(eq?.system_rho_mean || 0) > 0.8 ? 'text-red-400' : 'text-cyan-400'}`}>
                {((eq?.system_rho_mean || 0) * 100).toFixed(0)}%
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Conv</span>
              <span className={`text-lg font-bold font-data ${eq?.is_converging ? 'text-cyan-400' : 'text-amber-400'}`}>
                {eq?.is_converging ? 'YES' : 'NO'}
              </span>
            </div>
          </div>
          {critPath && critPath.nodes?.length > 0 && (
            <div className="mt-3">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest block mb-1">Critical Path ({(critPath.cascade_risk * 100).toFixed(0)}% risk)</span>
              <div className="flex flex-wrap gap-1">
                {critPath.nodes.map(n => (
                  <span key={n} className="px-1.5 py-0.5 bg-red-500/10 border border-red-500/20 text-red-400 text-[7px] font-hud uppercase">
                    {n}
                  </span>
                ))}
              </div>
            </div>
          )}
        </TacticalBox>
      </motion.div>
    </div>
  );
}
