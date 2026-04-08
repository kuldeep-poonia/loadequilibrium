'use client';

import React from 'react';
import { TacticalBox, StatusTube } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export function RuntimeModule() {
  const { tick } = useTelemetryStore();
  const rm = tick?.runtime_metrics;
  const safetyMode = tick?.safety_mode ?? false;
  const tickHealthMs = tick?.tick_health_ms ?? 0;
  const degradedFrac = tick?.degraded_fraction ?? 0;
  const jitterMs = tick?.jitter_ms ?? 0;

  const stages = rm ? [
    { label: 'Prune', ms: rm.avg_prune_ms },
    { label: 'Windows', ms: rm.avg_windows_ms },
    { label: 'Topology', ms: rm.avg_topology_ms },
    { label: 'Coupling', ms: rm.avg_coupling_ms },
    { label: 'Modelling', ms: rm.avg_modelling_ms },
    { label: 'Optimise', ms: rm.avg_optimise_ms },
    { label: 'Sim', ms: rm.avg_sim_ms },
    { label: 'Reasoning', ms: rm.avg_reasoning_ms },
    { label: 'Broadcast', ms: rm.avg_broadcast_ms },
  ] : [];

  const totalMs = stages.reduce((s, st) => s + (st.ms || 0), 0);

  return (
    <div className="flex flex-col gap-4 h-full">
      {/* Runtime Health KPIs */}
      <motion.div initial={{ opacity: 0, y: -10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }}>
        <TacticalBox
          title="RUNTIME HEALTH"
          badge={safetyMode ? 'SAFETY_ON' : 'NOMINAL'}
          status={safetyMode ? 'critical' : rm?.predicted_overrun ? 'warning' : 'nominal'}
        >
          <div className="grid grid-cols-4 gap-4">
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Tick Latency</span>
              <span className={`text-lg font-data font-bold tabular-nums ${tickHealthMs > 100 ? 'text-red-400' : 'text-cyan-400'}`}>
                {tickHealthMs.toFixed(1)}<span className="text-[8px] text-slate-500 ml-0.5">ms</span>
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Degraded</span>
              <span className={`text-lg font-data font-bold tabular-nums ${degradedFrac > 0.2 ? 'text-amber-400' : 'text-slate-300'}`}>
                {(degradedFrac * 100).toFixed(0)}%
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Jitter</span>
              <span className="text-lg font-data font-bold tabular-nums text-slate-300">
                {jitterMs.toFixed(1)}<span className="text-[8px] text-slate-500 ml-0.5">ms</span>
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-[7px] font-hud text-slate-600 uppercase tracking-widest">Safety Lvl</span>
              <span className={`text-lg font-data font-bold tabular-nums ${(rm?.safety_level ?? 0) > 1 ? 'text-amber-400' : 'text-cyan-400'}`}>
                {rm?.safety_level ?? 0}
              </span>
            </div>
          </div>
        </TacticalBox>
      </motion.div>

      {/* Per-Stage Pipeline Waterfall */}
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, delay: 0.05 }} className="flex-1 min-h-0">
        <TacticalBox
          title="PIPELINE STAGES"
          badge={`${totalMs.toFixed(1)}ms TOTAL`}
          status={rm?.predicted_overrun ? 'warning' : 'nominal'}
          className="h-full"
        >
          {stages.length === 0 ? (
            <div className="text-[9px] text-slate-600 font-data animate-pulse">AWAITING RUNTIME TELEMETRY...</div>
          ) : (
            <div className="space-y-2">
              {stages.map((st) => {
                const pct = totalMs > 0 ? (st.ms / totalMs) * 100 : 0;
                return (
                  <div key={st.label} className="flex items-center gap-3">
                    <span className="text-[8px] font-hud text-slate-500 uppercase tracking-widest w-16 shrink-0">{st.label}</span>
                    <div className="flex-1 h-2 bg-slate-900 rounded-sm overflow-hidden">
                      <div
                        className={`h-full transition-all duration-500 ${st.ms > 20 ? 'bg-amber-500' : 'bg-cyan-500/70'}`}
                        style={{ width: `${Math.min(100, pct)}%` }}
                      />
                    </div>
                    <span className="text-[9px] font-data text-slate-400 tabular-nums w-12 text-right">{(st.ms || 0).toFixed(1)}ms</span>
                  </div>
                );
              })}
              {/* Overrun indicator */}
              {rm?.total_overruns ? (
                <div className="mt-2 flex items-center gap-2">
                  <div className="w-1.5 h-1.5 bg-amber-500 rounded-full animate-pulse" />
                  <span className="text-[8px] font-hud text-amber-400 tracking-widest">
                    {rm.total_overruns} TOTAL OVERRUNS ({rm.consec_overruns} CONSECUTIVE)
                  </span>
                </div>
              ) : null}
            </div>
          )}
        </TacticalBox>
      </motion.div>
    </div>
  );
}
