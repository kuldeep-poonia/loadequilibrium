'use client';

import React from 'react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export function SimulationModule() {
  const { tick, triggerDomain } = useTelemetryStore();
  const scenario = tick?.scenario_comparison;
  const predTimeline = tick?.prediction_timeline || {};
  const riskTimeline = tick?.risk_timeline || {};

  const predServices = Object.entries(predTimeline).slice(0, 6);
  const riskServices = Object.entries(riskTimeline).slice(0, 6);

  const handleSim = async (action: string) => {
    const res = await triggerDomain('simulation/control', { action });
    if (!res.ok) console.error('[sim] trigger failed:', res.error);
  };

  return (
    <div className="grid h-full min-h-0 min-w-0 gap-3 xl:grid-cols-[220px_minmax(0,1fr)_280px]">
      {/* Left: Controls + Scenario Comparison */}
      <motion.div initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25 }} className="flex min-h-0 min-w-0 flex-col gap-3">
        <TacticalBox title="SIM CONTROL">
          <div className="flex min-w-0 flex-col gap-2">
            <button onClick={() => handleSim('start')} className="px-3 py-2 bg-cyan-500/5 border border-cyan-500/20 hover:bg-cyan-500/10 text-[8px] font-hud tracking-widest text-cyan-400 uppercase transition-all">
              START SCENARIO
            </button>
            <button onClick={() => handleSim('reset')} className="px-3 py-2 bg-white/[0.02] border border-white/10 hover:bg-white/5 text-[8px] font-hud tracking-widest text-slate-400 uppercase transition-all">
              RESET BRANCH
            </button>
          </div>
        </TacticalBox>

        {scenario && (
          <TacticalBox title="SCENARIOS" badge={`${scenario.scenario_count} RUNS`} className="min-h-0 flex-1">
            <div className="min-w-0 space-y-2">
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Best Collapse</span>
                <span className="text-[10px] font-data text-cyan-400">{(scenario.best_case_collapse * 100).toFixed(1)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Worst Collapse</span>
                <span className="text-[10px] font-data text-red-400">{(scenario.worst_case_collapse * 100).toFixed(1)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Med SLA Viol</span>
                <span className="text-[10px] font-data text-amber-400">{(scenario.median_sla_violation * 100).toFixed(1)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Stable Frac</span>
                <span className="text-[10px] font-data text-slate-300">{(scenario.stable_scenario_fraction * 100).toFixed(0)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[7px] font-hud text-slate-600 uppercase">Recovery</span>
                <span className="text-[9px] font-data text-slate-400">
                  {scenario.recovery_convergence_min_ms?.toFixed(0)}–{scenario.recovery_convergence_max_ms?.toFixed(0)}ms
                </span>
              </div>
            </div>
          </TacticalBox>
        )}
      </motion.div>

      {/* Center: Prediction Timeline */}
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, delay: 0.05 }} className="flex min-h-0 min-w-0 flex-col">
        <TacticalBox title="PREDICTIONS" badge={`${predServices.length} SVC`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-3">
            {predServices.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO PREDICTION DATA...</div>
            ) : (
              predServices.map(([svc, points]) => (
                <div key={svc} className="min-w-0">
                  <span className="block min-w-0 break-all text-[8px] font-hud text-cyan-400/70 uppercase tracking-wider">{svc}</span>
                  <div className="flex items-end gap-0.5 h-8 mt-1">
                    {(points || []).map((pt, i) => {
                      const h = Math.min(100, (pt.rho || 0) * 100);
                      return (
                        <div key={i} className="flex-1 flex flex-col items-center justify-end" title={`t+${pt.t}: ρ=${(pt.rho*100).toFixed(0)}%`}>
                          <div
                            className={`w-full rounded-t-[1px] transition-all ${h > 85 ? 'bg-red-500' : h > 60 ? 'bg-amber-500' : 'bg-cyan-500/60'}`}
                            style={{ height: `${h}%` }}
                          />
                        </div>
                      );
                    })}
                  </div>
                </div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>

      {/* Right: Risk Runway */}
      <motion.div initial={{ opacity: 0, x: 10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25, delay: 0.1 }} className="flex min-h-0 min-w-0 flex-col">
        <TacticalBox title="RISK RUNWAY" className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-3">
            {riskServices.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO RISK TIMELINE...</div>
            ) : (
              riskServices.map(([svc, points]) => (
                <div key={svc} className="min-w-0">
                  <span className="block min-w-0 break-all text-[8px] font-hud text-amber-400/70 uppercase tracking-wider">{svc}</span>
                  <div className="flex items-end gap-0.5 h-8 mt-1">
                    {(points || []).map((pt, i) => {
                      const risk = (pt.risk || 0) * 100;
                      return (
                        <div key={i} className="flex-1 flex flex-col items-center justify-end" title={`t+${pt.t}: risk=${risk.toFixed(0)}%`}>
                          <div
                            className={`w-full rounded-t-[1px] ${risk > 70 ? 'bg-red-500' : risk > 30 ? 'bg-amber-500' : 'bg-cyan-500/40'}`}
                            style={{ height: `${Math.min(100, risk)}%` }}
                          />
                        </div>
                      );
                    })}
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
