'use client';

import React from 'react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export function PolicyModule() {
  const { tick, triggerDomain } = useTelemetryStore();

  const directives = tick?.directives || {};
  const directiveList = Object.entries(directives).map(([svc, d]) => ({ svc, ...d }));
  const stability = tick?.stability_envelope;
  const fixedPt = tick?.fixed_point_equilibrium;
  const zones = tick?.stability_zones || {};

  const handlePolicyChange = async (preset: string) => {
    const res = await triggerDomain('policy/update', { preset });
    if (!res.ok) console.error('[policy] trigger failed:', res.error);
  };

  return (
    <div className="grid grid-cols-3 gap-4 h-full">
      {/* Left: Presets + Stability Envelope */}
      <motion.div initial={{ x: -10, opacity: 0 }} animate={{ x: 0, opacity: 1 }} transition={{ delay: 0.05 }} className="flex flex-col gap-4">
        <TacticalBox title="POLICY PRESETS" badge="CONTROL">
          <div className="flex flex-col gap-2">
            {['aggressive', 'balanced', 'conservative', 'defensive'].map((preset) => (
              <button
                key={preset}
                onClick={() => handlePolicyChange(preset)}
                className="px-3 py-2 bg-white/[0.02] border border-white/10 hover:border-cyan-500/30 hover:bg-cyan-500/5 text-left transition-all group"
              >
                <span className="text-[9px] font-hud tracking-widest text-slate-400 group-hover:text-cyan-400 uppercase">{preset}</span>
              </button>
            ))}
          </div>
        </TacticalBox>

        {stability && (
          <TacticalBox title="STABILITY ENVELOPE" status={stability.envelope_headroom < 0.1 ? 'critical' : stability.envelope_headroom < 0.3 ? 'warning' : 'nominal'}>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Safe ρ Max</span>
                <span className="text-[10px] font-data text-cyan-400">{(stability.safe_system_rho_max * 100).toFixed(0)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Current ρ</span>
                <span className="text-[10px] font-data text-slate-300">{(stability.current_system_rho_mean * 100).toFixed(0)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Headroom</span>
                <span className={`text-[10px] font-data font-bold ${stability.envelope_headroom < 0.1 ? 'text-red-400' : 'text-cyan-400'}`}>
                  {(stability.envelope_headroom * 100).toFixed(1)}%
                </span>
              </div>
              {stability.most_vulnerable_service && (
                <div className="flex justify-between">
                  <span className="text-[7px] font-hud text-slate-600 uppercase">Vulnerable</span>
                  <span className="text-[9px] font-data text-amber-400">{stability.most_vulnerable_service}</span>
                </div>
              )}
            </div>
          </TacticalBox>
        )}
      </motion.div>

      {/* Center + Right: Directives + Zones */}
      <motion.div className="col-span-2 flex flex-col gap-4" initial={{ y: 10, opacity: 0 }} animate={{ y: 0, opacity: 1 }} transition={{ delay: 0.1 }}>
        <TacticalBox title="ACTIVE DIRECTIVES" badge={`${directiveList.length} SVC`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1.5">
            {directiveList.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO ACTIVE DIRECTIVES...</div>
            ) : (
              directiveList.map((d) => (
                <div key={d.svc} className="p-2 bg-white/[0.02] border border-white/5 hover:border-cyan-500/15 transition-all">
                  <div className="flex justify-between items-center mb-1">
                    <span className="text-[9px] font-hud text-cyan-400 tracking-wider uppercase">{d.svc}</span>
                    <span className={`text-[7px] font-hud px-1 py-0.5 rounded-sm ${
                      zones[d.svc] === 'collapse' ? 'text-red-400 bg-red-500/10' :
                      zones[d.svc] === 'warning' ? 'text-amber-400 bg-amber-500/10' :
                      'text-cyan-400/60 bg-cyan-500/5'
                    }`}>{zones[d.svc] || 'stable'}</span>
                  </div>
                  <div className="grid grid-cols-4 gap-2 text-[8px] font-data text-slate-400">
                    <div>R:{d.target_replicas}</div>
                    <div>Retry:{(d.retry_budget * 100).toFixed(0)}%</div>
                    <div>Q:{d.queue_limit}</div>
                    <div>Cache:{d.cache_ttl_ms}ms</div>
                  </div>
                  {d.reason && <div className="text-[7px] text-slate-600 mt-1 truncate">{d.reason}</div>}
                </div>
              ))
            )}
          </div>
        </TacticalBox>

        {/* Fixed Point Convergence */}
        {fixedPt && (
          <TacticalBox title="FIXED POINT EQUILIBRIUM" status={fixedPt.converged ? 'nominal' : 'warning'}>
            <div className="grid grid-cols-3 gap-3">
              <div className="flex flex-col">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Converged</span>
                <span className={`text-[11px] font-data font-bold ${fixedPt.converged ? 'text-cyan-400' : 'text-amber-400'}`}>
                  {fixedPt.converged ? 'YES' : 'NO'}
                </span>
              </div>
              <div className="flex flex-col">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Collapse P</span>
                <span className="text-[11px] font-data font-bold text-slate-300">
                  {(fixedPt.systemic_collapse_prob * 100).toFixed(1)}%
                </span>
              </div>
              <div className="flex flex-col">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Spectral ρ</span>
                <span className={`text-[11px] font-data font-bold ${fixedPt.convergence_rate > 0.9 ? 'text-red-400' : 'text-cyan-400'}`}>
                  {fixedPt.convergence_rate?.toFixed(3) ?? '—'}
                </span>
              </div>
            </div>
          </TacticalBox>
        )}
      </motion.div>
    </div>
  );
}
