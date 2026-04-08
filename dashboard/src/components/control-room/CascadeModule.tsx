'use client';

import React from 'react';
import { TacticalBox, StatusTube } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export function CascadeModule() {
  const { tick } = useTelemetryStore();
  const cascadeRisk = tick?.objective?.cascade_failure_probability || 0;
  const sensitivity = tick?.topology_sensitivity;
  const pressure = tick?.pressure_heatmap || {};
  const riskTimeline = tick?.risk_timeline || {};
  const coupling = tick?.network_coupling || {};
  const envelope = tick?.stability_envelope;

  const pressureList = Object.entries(pressure)
    .map(([svc, val]) => ({ svc, val }))
    .sort((a, b) => b.val - a.val)
    .slice(0, 12);

  const couplingList = Object.entries(coupling)
    .map(([svc, c]) => ({ svc, ...c }))
    .sort((a, b) => b.path_collapse_prob - a.path_collapse_prob)
    .slice(0, 8);

  return (
    <div className="grid grid-cols-3 gap-4 h-full">
      {/* Left: Cascade KPIs */}
      <motion.div initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25 }} className="flex flex-col gap-4">
        <TacticalBox
          title="CASCADE RISK"
          status={cascadeRisk > 0.6 ? 'critical' : cascadeRisk > 0.3 ? 'warning' : 'nominal'}
        >
          <div className="space-y-3">
            <div className="text-center">
              <div className={`text-3xl font-data font-bold tabular-nums ${cascadeRisk > 0.6 ? 'text-red-400' : cascadeRisk > 0.3 ? 'text-amber-400' : 'text-cyan-400'}`}>
                {(cascadeRisk * 100).toFixed(1)}%
              </div>
              <div className="text-[7px] font-hud text-slate-600 uppercase tracking-widest mt-1">Global Failure Probability</div>
            </div>
            <StatusTube label="Cascade Intensity" value="" percent={cascadeRisk} status={cascadeRisk > 0.6 ? 'critical' : cascadeRisk > 0.3 ? 'warning' : 'nominal'} />
          </div>
        </TacticalBox>

        {sensitivity && (
          <TacticalBox title="TOPOLOGY SENSITIVITY" status={sensitivity.system_fragility > 0.7 ? 'critical' : 'nominal'}>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Fragility</span>
                <span className={`text-[10px] font-data font-bold ${sensitivity.system_fragility > 0.6 ? 'text-red-400' : 'text-slate-300'}`}>
                  {(sensitivity.system_fragility * 100).toFixed(0)}%
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Max Amplification</span>
                <span className="text-[10px] font-data text-slate-300">{sensitivity.max_amplification_score?.toFixed(2) ?? '—'}</span>
              </div>
              {sensitivity.keystone_services?.length > 0 && (
                <div>
                  <span className="text-[7px] font-hud text-slate-600 uppercase block mb-1">Keystones</span>
                  <div className="flex flex-wrap gap-1">
                    {sensitivity.keystone_services.map(ks => (
                      <span key={ks} className="px-1.5 py-0.5 bg-red-500/10 border border-red-500/20 text-red-400 text-[7px] font-hud">{ks}</span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </TacticalBox>
        )}
      </motion.div>

      {/* Center: Pressure Heatmap */}
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, delay: 0.05 }} className="flex flex-col gap-4">
        <TacticalBox title="PRESSURE HEATMAP" badge={`${pressureList.length} SVC`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1">
            {pressureList.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO PRESSURE DATA...</div>
            ) : (
              pressureList.map(({ svc, val }) => (
                <div key={svc} className="flex items-center gap-2">
                  <span className="text-[8px] font-hud text-slate-500 w-20 truncate uppercase">{svc}</span>
                  <div className="flex-1 h-1.5 bg-slate-900 rounded-sm overflow-hidden">
                    <div
                      className={`h-full transition-all duration-500 ${val > 0.7 ? 'bg-red-500' : val > 0.4 ? 'bg-amber-500' : 'bg-cyan-500/60'}`}
                      style={{ width: `${Math.min(100, val * 100)}%` }}
                    />
                  </div>
                  <span className="text-[8px] font-data text-slate-500 w-8 text-right tabular-nums">{(val * 100).toFixed(0)}%</span>
                </div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>

      {/* Right: Network Coupling */}
      <motion.div initial={{ opacity: 0, x: 10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25, delay: 0.1 }} className="flex flex-col gap-4">
        <TacticalBox title="NETWORK COUPLING" className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1.5">
            {couplingList.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">AWAITING COUPLING DATA...</div>
            ) : (
              couplingList.map(({ svc, path_collapse_prob, effective_pressure, path_length, congestion_feedback }) => (
                <div key={svc} className="p-2 bg-white/[0.02] border border-white/5 hover:border-cyan-500/15 transition-all">
                  <div className="flex justify-between items-center mb-1">
                    <span className="text-[8px] font-hud text-cyan-400 tracking-wider uppercase">{svc}</span>
                    <span className={`text-[8px] font-data font-bold ${path_collapse_prob > 0.5 ? 'text-red-400' : 'text-slate-400'}`}>
                      {(path_collapse_prob * 100).toFixed(0)}% path-fail
                    </span>
                  </div>
                  <div className="grid grid-cols-3 gap-1 text-[7px] font-data text-slate-500">
                    <span>P:{effective_pressure?.toFixed(2)}</span>
                    <span>L:{path_length}</span>
                    <span>C:{congestion_feedback?.toFixed(2)}</span>
                  </div>
                </div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>
    </div>
  );
}
