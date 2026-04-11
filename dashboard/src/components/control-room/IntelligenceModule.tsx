'use client';

import React from 'react';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { TacticalBox } from '@/components/ui/HUD';
import { motion } from 'framer-motion';

export function IntelligenceModule() {
  const { tick, triggerAction, triggerDomain } = useTelemetryStore();

  const events = tick?.events || [];
  const riskQueue = tick?.priority_risk_queue || [];
  const degradedSvcs = tick?.degraded_services || [];

  const handleAction = async (action: string) => {
    await triggerAction(action);
  };

  const handleRollout = async () => {
    const res = await triggerDomain('intelligence/rollout');
    if (!res.ok) console.error('[intelligence] rollout failed:', res.error);
  };

  return (
    <div className="flex flex-col gap-3 h-full">
      {/* Tactical Command */}
      <motion.div initial={{ x: -10, opacity: 0 }} animate={{ x: 0, opacity: 1 }} transition={{ duration: 0.2 }}>
        <TacticalBox title="TACTICAL COMMAND" badge="ACTUATOR">
          <div className="grid grid-cols-2 gap-2">
            <motion.button 
              onClick={() => handleAction('toggle')} 
              whileHover={{ y: -1, scale: 1.005 }}
              whileTap={{ y: 1, scale: 0.99 }}
              transition={{ type: 'spring', stiffness: 350, damping: 25 }}
              className="neo-control neo-control--cyan py-3 font-hud text-[9px] tracking-[0.22em] uppercase rounded-[14px] text-center"
            >
              Toggle Node
            </motion.button>
            <motion.button 
              onClick={() => handleAction('chaos-run')} 
              whileHover={{ y: -1, scale: 1.005 }}
              whileTap={{ y: 1, scale: 0.99 }}
              transition={{ type: 'spring', stiffness: 350, damping: 25 }}
              className="neo-control neo-control--red py-3 font-hud text-[9px] tracking-[0.22em] uppercase rounded-[14px] text-center"
            >
              Inject Chaos
            </motion.button>
            <motion.button 
              onClick={() => handleAction('replay-burst')} 
              whileHover={{ y: -1, scale: 1.005 }}
              whileTap={{ y: 1, scale: 0.99 }}
              transition={{ type: 'spring', stiffness: 350, damping: 25 }}
              className="neo-control py-3 font-hud text-[9px] tracking-[0.22em] uppercase rounded-[14px] text-center text-slate-300"
            >
              Replay Burst
            </motion.button>
            <motion.button 
              onClick={() => handleRollout()} 
              whileHover={{ y: -1, scale: 1.005 }}
              whileTap={{ y: 1, scale: 0.99 }}
              transition={{ type: 'spring', stiffness: 350, damping: 25 }}
              className="neo-control neo-control--amber py-3 font-hud text-[9px] tracking-[0.22em] uppercase rounded-[14px] text-center"
            >
              RL Rollout
            </motion.button>
          </div>
        </TacticalBox>
      </motion.div>

      {/* Risk Queue */}
      <motion.div initial={{ x: 10, opacity: 0 }} animate={{ x: 0, opacity: 1 }} transition={{ duration: 0.2, delay: 0.05 }} className="flex-1 min-h-0 flex flex-col">
        <TacticalBox title="RISK QUEUE" badge={`${riskQueue.length} ITEMS`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1.5">
            {riskQueue.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data p-1">NO CRITICAL RISKS</div>
            ) : (
              riskQueue.slice(0, 8).map((r, i) => (
                <motion.div 
                  key={i} 
                  whileHover={{ x: 1 }}
                  transition={{ type: 'spring', stiffness: 350, damping: 25 }}
                  className="neo-control flex items-center gap-2 p-2 rounded-[12px]"
                >
                  <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                    r.urgency_class === 'critical' ? 'bg-red-500 animate-pulse' :
                    r.urgency_class === 'warning' ? 'bg-amber-500' :
                    r.urgency_class === 'elevated' ? 'bg-amber-500/60' : 'bg-slate-600'
                  }`} />
                  <span className="text-[8px] text-cyan-400 font-hud uppercase tracking-wider flex-1 truncate">{r.service_id}</span>
                  <span className={`text-[8px] font-data tabular-nums ${r.collapse_risk > 0.5 ? 'text-red-400' : 'text-slate-400'}`}>
                    {(r.collapse_risk * 100).toFixed(0)}%
                  </span>
                  {r.is_keystone && <span className="text-[6px] font-hud text-amber-400 bg-amber-500/10 px-1 py-0.5 rounded-sm">KEY</span>}
                </motion.div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>

      {/* Events */}
      <motion.div initial={{ y: 10, opacity: 0 }} animate={{ y: 0, opacity: 1 }} transition={{ duration: 0.2, delay: 0.1 }} className="flex-1 min-h-0 flex flex-col">
        <TacticalBox title="SYSTEM EVENTS" badge={`${events.length} EVT`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1.5">
            {events.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data p-1 animate-pulse">AWAITING EVENTS...</div>
            ) : (
              events.slice(0, 8).map((e, i) => (
                <motion.div 
                  key={i} 
                  whileHover={{ x: 2 }}
                  transition={{ type: 'spring', stiffness: 350, damping: 25 }}
                  className="neo-control p-2 rounded-[12px] border-l-2 border-white/5"
                >
                  <div className="flex justify-between items-center">
                    <span className={`text-[7px] font-hud uppercase tracking-wider ${
                      e.severity === 'critical' ? 'text-red-400' :
                      e.severity === 'warning' ? 'text-amber-400' : 'text-cyan-400/70'
                    }`}>{e.category}</span>
                    {e.service_id && <span className="text-[7px] font-data text-slate-600">{e.service_id}</span>}
                  </div>
                  <span className="text-[8px] text-slate-400 font-data leading-tight block mt-0.5">{e.description}</span>
                </motion.div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>
    </div>
  );
}