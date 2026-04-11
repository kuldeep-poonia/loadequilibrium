'use client';

import React from 'react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export function SandboxModule() {
  const { tick, triggerDomain } = useTelemetryStore();
  const simResult = tick?.sim_result;
  const simOverlay = tick?.sim_overlay;

  const handleTrigger = async (type: string) => {
    const res = await triggerDomain('sandbox/trigger', { type });
    if (!res.ok) console.error('[sandbox] trigger failed:', res.error);
  };

  const cascadeProbs = simResult?.cascade_failure_probability || {};
  const slaViols = simResult?.sla_violation_probability || {};
  const queueDists = simResult?.queue_distribution_at_horizon || {};
  const cascadeList = Object.entries(cascadeProbs).sort((a, b) => b[1] - a[1]).slice(0, 10);
  const slaList = Object.entries(slaViols).sort((a, b) => b[1] - a[1]).slice(0, 10);

  return (
    <div className="grid grid-cols-3 gap-4 h-full">
      {/* Left: Controls + Status */}
      <motion.div initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25 }} className="flex flex-col gap-4">
        <TacticalBox title="SANDBOX CONTROLS" badge="ISOLATED">
          <div className="flex flex-col gap-2">
            {['structural_break', 'latency_spike', 'cascade_inject', 'traffic_surge'].map((type, index) => (
              <motion.button
                key={type}
                onClick={() => handleTrigger(type)}
                whileHover={{ y: -1, scale: 1.005 }}
                whileTap={{ y: 1, scale: 0.99 }}
                transition={{ type: 'spring', stiffness: 350, damping: 25 }}
                className="neo-control px-4 py-3 border-red-500/15 hover:border-red-500/30 text-left group rounded-[14px]"
              >
                <span className="text-[9px] font-hud tracking-[0.22em] text-red-300/80 group-hover:text-red-300 uppercase">{type.replace(/_/g, ' ')}</span>
              </motion.button>
            ))}
          </div>
        </TacticalBox>

        {simResult && (
          <TacticalBox title="SIM STATUS" status={simResult.system_stable ? 'nominal' : 'critical'}>
            <div className="space-y-3 pt-1">
              <div className="flex items-center justify-between gap-2">
                <span className="text-[8px] font-hud text-slate-500 uppercase tracking-[0.2em]">STABLE</span>
                <span className={`text-[11px] font-data font-bold ${simResult.system_stable ? 'text-cyan-300' : 'text-red-300'}`}>
                  {simResult.system_stable ? 'YES' : 'NO'}
                </span>
              </div>
              <div className="flex items-center justify-between gap-2">
                <span className="text-[8px] font-hud text-slate-500 uppercase tracking-[0.2em]">RECOVERY</span>
                <span className="text-[11px] font-data text-slate-300">{simResult.recovery_convergence_ms?.toFixed(0) ?? '—'}ms</span>
              </div>
            </div>
          </TacticalBox>
        )}

        {simOverlay && (
          <TacticalBox title="SIM OVERLAY" badge={`AGE:${simOverlay.sim_tick_age}`} status={simOverlay.sim_tick_age > 5 ? 'warning' : 'nominal'}>
            <div className="flex items-center justify-between gap-2 pt-1">
              <span className="text-[8px] font-hud text-slate-500 uppercase tracking-[0.2em]">HORIZON</span>
              <span className="text-[11px] font-data text-slate-300">{simOverlay.horizon_ms?.toFixed(0) ?? '—'}ms</span>
            </div>
          </TacticalBox>
        )}
      </motion.div>

      {/* Center: Cascade Failure Probabilities */}
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, delay: 0.05 }} className="flex flex-col">
        <TacticalBox title="CASCADE FAILURE PROBS" className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1">
            {cascadeList.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO SIM RESULTS YET...</div>
            ) : (
              cascadeList.map(([svc, prob]) => (
                <div key={svc} className="flex items-center gap-2">
                  <span className="text-[8px] font-hud text-slate-500 w-20 truncate uppercase">{svc}</span>
                  <div className="flex-1 h-1.5 bg-slate-900 rounded-sm overflow-hidden">
                    <div className={`h-full ${prob > 0.5 ? 'bg-red-500' : prob > 0.2 ? 'bg-amber-500' : 'bg-cyan-500/60'}`} style={{ width: `${Math.min(100, prob * 100)}%` }} />
                  </div>
                  <span className="text-[8px] font-data text-slate-500 w-8 text-right tabular-nums">{(prob * 100).toFixed(0)}%</span>
                </div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>

      {/* Right: SLA Violations */}
      <motion.div initial={{ opacity: 0, x: 10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25, delay: 0.1 }} className="flex flex-col">
        <TacticalBox title="SLA VIOLATION PROBS" className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1">
            {slaList.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO SLA DATA...</div>
            ) : (
              slaList.map(([svc, prob]) => (
                <div key={svc} className="flex items-center gap-2">
                  <span className="text-[8px] font-hud text-slate-500 w-20 truncate uppercase">{svc}</span>
                  <div className="flex-1 h-1.5 bg-slate-900 rounded-sm overflow-hidden">
                    <div className={`h-full ${prob > 0.3 ? 'bg-red-500' : 'bg-amber-500/60'}`} style={{ width: `${Math.min(100, prob * 100)}%` }} />
                  </div>
                  <span className="text-[8px] font-data text-slate-500 w-8 text-right tabular-nums">{(prob * 100).toFixed(0)}%</span>
                </div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>
    </div>
  );
}
