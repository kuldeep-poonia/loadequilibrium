'use client';

import React from 'react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

export default function ActuatorPage() {
  const { tick, triggerAction } = useTelemetryStore();
  const directives = tick?.directives || {};
  const directiveList = Object.entries(directives).map(([svc, d]) => ({ svc, ...d }));

  return (
    <div className="flex flex-col gap-4 h-full max-w-5xl mx-auto">
      <motion.div initial={{ opacity: 0, y: -10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }}>
        <TacticalBox title="ACTUATOR CONTROLS" badge="CONTROL_PLANE">
          <div className="flex gap-3">
            <motion.button
              onClick={() => triggerAction('toggle')}
              whileHover={{ y: -1, scale: 1.005 }}
              whileTap={{ y: 1, scale: 0.99 }}
              transition={{ type: 'spring', stiffness: 350, damping: 25 }}
              className="neo-control neo-control--cyan px-6 py-3 font-hud text-[9px] tracking-[0.22em] uppercase rounded-[14px] text-center"
            >
              TOGGLE
            </motion.button>
            <motion.button
              onClick={() => triggerAction('chaos-run')}
              whileHover={{ y: -1, scale: 1.005 }}
              whileTap={{ y: 1, scale: 0.99 }}
              transition={{ type: 'spring', stiffness: 350, damping: 25 }}
              className="neo-control neo-control--red px-6 py-3 font-hud text-[9px] tracking-[0.22em] uppercase rounded-[14px] text-center"
            >
              CHAOS RUN
            </motion.button>
          </div>
        </TacticalBox>
      </motion.div>

      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, delay: 0.05 }} className="flex-1 min-h-0 flex flex-col">
        <TacticalBox title="ACTIVE DIRECTIVES" badge={`${directiveList.length} SVC`} className="flex-1 overflow-hidden flex flex-col">
          <div className="flex-1 overflow-y-auto scrollbar-hud space-y-1.5">
            {directiveList.length === 0 ? (
              <div className="text-[9px] text-slate-600 font-data animate-pulse">NO ACTIVE DIRECTIVES...</div>
            ) : (
              directiveList.map(({ svc, target_replicas, retry_budget, queue_limit, cache_ttl_ms, reason }) => (
                <motion.div 
                  key={svc} 
                  whileHover={{ x: 1 }}
                  transition={{ type: 'spring', stiffness: 350, damping: 25 }}
                  className="neo-control p-3 rounded-[12px]"
                >
                  <div className="flex justify-between items-center mb-1.5">
                    <span className="text-[9px] font-hud text-cyan-400 tracking-wider uppercase font-bold">{svc}</span>
                  </div>
                  <div className="grid grid-cols-4 gap-3 text-[8px]">
                    <div className="flex flex-col">
                      <span className="text-[7px] font-hud text-slate-600 uppercase">Replicas</span>
                      <span className="font-data text-slate-300">{target_replicas}</span>
                    </div>
                    <div className="flex flex-col">
                      <span className="text-[7px] font-hud text-slate-600 uppercase">Retry</span>
                      <span className="font-data text-slate-300">{(retry_budget * 100).toFixed(0)}%</span>
                    </div>
                    <div className="flex flex-col">
                      <span className="text-[7px] font-hud text-slate-600 uppercase">Q Limit</span>
                      <span className="font-data text-slate-300">{queue_limit}</span>
                    </div>
                    <div className="flex flex-col">
                      <span className="text-[7px] font-hud text-slate-600 uppercase">Cache TTL</span>
                      <span className="font-data text-slate-300">{cache_ttl_ms}ms</span>
                    </div>
                  </div>
                  {reason && <div className="text-[7px] text-slate-600 mt-1.5 font-data">{reason}</div>}
                </motion.div>
              ))
            )}
          </div>
        </TacticalBox>
      </motion.div>
    </div>
  );
}