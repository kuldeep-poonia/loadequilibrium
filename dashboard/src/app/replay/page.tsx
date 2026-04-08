'use client';

import React from 'react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export default function ReplayPage() {
  const { tick, triggerAction } = useTelemetryStore();
  const events = tick?.events || [];

  return (
    <div className="flex flex-col gap-4 h-full max-w-5xl mx-auto">
      <motion.div initial={{ opacity: 0, y: -10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }}>
        <TacticalBox title="REPLAY CONTROLS" badge="EVENT_LOG">
          <div className="flex gap-3">
            <button
              onClick={() => triggerAction('replay-burst')}
              className="px-4 py-2 bg-cyan-500/5 border border-cyan-500/20 hover:bg-cyan-500/10 text-[8px] font-hud tracking-widest text-cyan-400 uppercase transition-all"
            >
              TRIGGER REPLAY BURST
            </button>
          </div>
        </TacticalBox>
      </motion.div>

      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, delay: 0.05 }} className="flex-1 min-h-0 flex flex-col">
        <TacticalBox title="EVENT TIMELINE" badge={`${events.length} EVENTS`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1">
            {events.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO EVENT HISTORY...</div>
            ) : (
              events.map((e, i) => (
                <div key={i} className="flex items-start gap-3 p-2 bg-white/[0.02] border border-white/5 hover:border-cyan-500/15 transition-all">
                  <div className={`w-1.5 h-1.5 mt-1 rounded-full shrink-0 ${
                    e.severity === 'critical' ? 'bg-red-500' :
                    e.severity === 'warning' ? 'bg-amber-500' : 'bg-cyan-500/60'
                  }`} />
                  <div className="flex-1 min-w-0">
                    <div className="flex justify-between items-center">
                      <span className="text-[8px] font-hud text-cyan-400/70 uppercase tracking-wider">{e.category}</span>
                      {e.service_id && <span className="text-[7px] font-data text-slate-600">{e.service_id}</span>}
                    </div>
                    <div className="text-[9px] font-data text-slate-400 mt-0.5 truncate">{e.description}</div>
                    {e.evidence && (
                      <div className="flex gap-3 mt-1 text-[7px] font-data text-slate-600">
                        <span>ρ:{(e.evidence.utilisation * 100).toFixed(0)}%</span>
                        <span>risk:{(e.evidence.collapse_risk * 100).toFixed(0)}%</span>
                        <span>burst:{e.evidence.burst_factor?.toFixed(2)}</span>
                      </div>
                    )}
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
