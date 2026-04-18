'use client';

import React, { useEffect, useRef } from 'react';
import { useTelemetryStore } from '@/store/useTelemetryStore';
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
      ctx.strokeStyle = "rgba(255, 255, 255, 0.025)";
      ctx.lineWidth = 1;
      for (let x = 0; x < w; x += 30) { ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke(); }
      for (let y = 0; y < h; y += 30) { ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke(); }

      // Reticle
      ctx.strokeStyle = "rgba(255, 255, 255, 0.04)";
      ctx.beginPath(); ctx.moveTo(w / 2 - 15, h / 2); ctx.lineTo(w / 2 + 15, h / 2); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(w / 2, h / 2 - 15); ctx.lineTo(w / 2, h / 2 + 15); ctx.stroke();

      const nodes: Node[] = tick?.topology?.nodes || [];
      const edges: Edge[] = tick?.topology?.edges || [];
      const critPath = tick?.topology?.critical_path?.nodes || [];
      const heatmap = tick?.pressure_heatmap || {};
      const solverActive = Boolean(tick && !tick.safety_mode && (tick.tick_health_ms ?? 0) > 0);

      if (nodes.length === 0) {
        ctx.fillStyle = 'rgba(148,163,184,0.3)';
        ctx.font = "9px ui-monospace,SF Mono,Menlo,monospace";
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
        ctx.strokeStyle = isCritEdge ? `rgba(255, 69, 58, ${alpha + 0.15})` : `rgba(0, 212, 255, ${alpha})`;
        ctx.lineWidth = isCritEdge ? 2 : 1;
        ctx.beginPath(); ctx.moveTo(s.x, s.y); ctx.lineTo(t.x, t.y); ctx.stroke();

        // Packet propagation dots along edges
        if (e.call_rate > 0) {
          const t_prog = ((pulsePhaseRef.current + (e.weight || 0) * 3) % (Math.PI * 2)) / (Math.PI * 2);
          const px = s.x + (t.x - s.x) * t_prog;
          const py = s.y + (t.y - s.y) * t_prog;
          ctx.fillStyle = isCritEdge ? 'rgba(255, 69, 58, 0.6)' : 'rgba(0, 212, 255, 0.5)';
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
        const nodeColor = isCrit ? '#FF453A' : pressure > 0.7 ? '#ef4444' : pressure > 0.4 ? '#f59e0b' : '#00D4FF';

        // Glow
        ctx.shadowBlur = isCrit ? 18 : 6 + load * 12;
        ctx.shadowColor = nodeColor;

        // Subtle observatory pulse for active simulations
        ctx.strokeStyle = isCrit ? 'rgba(255, 69, 58, 0.34)' : `rgba(0, 212, 255, ${0.12 + pulse * 0.12 + load * 0.08})`;
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
        ctx.font = "7px ui-monospace,SF Mono,Menlo,monospace";
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
