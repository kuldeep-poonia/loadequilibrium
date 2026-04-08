'use client';

import React from 'react';
import dynamic from 'next/dynamic';
import { TacticalBox, StatusTube } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { motion } from 'framer-motion';

const ResponsiveContainer = dynamic(() => import('recharts').then(mod => mod.ResponsiveContainer), { ssr: false });
const LineChart = dynamic(() => import('recharts').then(mod => mod.LineChart), { ssr: false });
const AreaChart = dynamic(() => import('recharts').then(mod => mod.AreaChart), { ssr: false });
const Area = dynamic(() => import('recharts').then(mod => mod.Area), { ssr: false });
const Line = dynamic(() => import('recharts').then(mod => mod.Line), { ssr: false });
const XAxis = dynamic(() => import('recharts').then(mod => mod.XAxis), { ssr: false });
const YAxis = dynamic(() => import('recharts').then(mod => mod.YAxis), { ssr: false });
const CartesianGrid = dynamic(() => import('recharts').then(mod => mod.CartesianGrid), { ssr: false });
const Tooltip = dynamic(() => import('recharts').then(mod => mod.Tooltip), { ssr: false });

export function TelemetryModule() {
  const { tick, history } = useTelemetryStore();
  const bundles = tick?.bundles || {};
  const serviceList = Object.entries(bundles).map(([id, b]) => ({
    id,
    utilisation: b.queue?.utilisation || 0,
    trend: b.queue?.utilisation_trend || 0,
    p99: b.queue?.last_p99_latency_ms || 0,
    risk: b.stability?.collapse_risk || 0,
    zone: b.stability?.collapse_zone || 'safe',
    pressure: tick?.pressure_heatmap?.[id] || 0,
    arrivalRate: b.queue?.arrival_rate || 0,
    serviceRate: b.queue?.service_rate || 0,
    queueLen: b.queue?.mean_queue_len || 0,
    burstFactor: b.queue?.burst_factor || 0,
  })).sort((a, b) => b.risk - a.risk);

  const degradedCount = tick?.degraded_services?.length || 0;

  return (
    <div className="flex flex-col gap-3 h-full">
      {/* Live trace chart */}
      <motion.div initial={{ y: -10, opacity: 0 }} animate={{ y: 0, opacity: 1 }} transition={{ duration: 0.3 }} className="h-48 shrink-0">
        <TacticalBox title="NETWORK LOAD TRACE" badge={`SEQ ${tick?.seq || 0}`} className="h-full">
          <div className="h-full w-full">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={history}>
                <defs>
                  <linearGradient id="objGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#00f2ff" stopOpacity={0.3} />
                    <stop offset="100%" stopColor="#00f2ff" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="cascGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#ff3366" stopOpacity={0.3} />
                    <stop offset="100%" stopColor="#ff3366" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" vertical={false} />
                <XAxis dataKey="seq" hide />
                <YAxis domain={[0, 1]} hide />
                <Tooltip
                  contentStyle={{ backgroundColor: '#020617', border: '1px solid #0ea5e926', fontSize: '9px', fontFamily: 'var(--font-data)' }}
                  itemStyle={{ fontSize: '9px' }}
                />
                <Area type="monotone" dataKey="obj" stroke="#00f2ff" strokeWidth={1.5} fill="url(#objGrad)" dot={false} isAnimationActive={false} name="Objective" />
                <Area type="monotone" dataKey="casc" stroke="#ff3366" strokeWidth={1.5} fill="url(#cascGrad)" dot={false} isAnimationActive={false} name="Cascade" />
                <Line type="monotone" dataKey="rhoMean" stroke="#f59e0b" strokeWidth={1} strokeDasharray="4 2" dot={false} isAnimationActive={false} name="ρ Mean" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </TacticalBox>
      </motion.div>

      {/* Service Registry */}
      <motion.div initial={{ y: 10, opacity: 0 }} animate={{ y: 0, opacity: 1 }} transition={{ duration: 0.3, delay: 0.05 }} className="flex-1 flex flex-col min-h-0">
        <TacticalBox
          title="SERVICE REGISTRY"
          badge={`${serviceList.length} SVC`}
          status={degradedCount > 0 ? 'warning' : 'nominal'}
          className="flex-1 overflow-hidden flex flex-col"
        >
          <div className="flex-1 overflow-y-auto pr-1 space-y-2 scrollbar-hud">
            {serviceList.length === 0 && (
              <div className="text-[9px] text-slate-600 font-data p-2 animate-pulse">AWAITING SERVICE DISCOVERY...</div>
            )}
            {serviceList.map((s) => (
              <div key={s.id} className="p-2.5 bg-white/[0.02] border border-white/5 hover:border-cyan-500/20 hover:bg-cyan-500/[0.03] transition-all group">
                <div className="flex justify-between items-center mb-1.5">
                  <span className="text-[9px] font-bold text-cyan-400 tracking-wider uppercase font-hud">{s.id}</span>
                  <span className={`text-[8px] font-hud px-1.5 py-0.5 rounded-sm ${
                    s.zone === 'collapse' ? 'text-red-400 bg-red-500/10' :
                    s.zone === 'warning' ? 'text-amber-400 bg-amber-500/10' :
                    'text-cyan-400/60 bg-cyan-500/5'
                  }`}>
                    {s.zone.toUpperCase()}
                  </span>
                </div>
                <div className="grid grid-cols-4 gap-2">
                  <div className="flex flex-col">
                    <span className="text-[7px] font-hud text-slate-600 uppercase">ρ</span>
                    <span className="text-[10px] font-data font-semibold text-slate-300">{(s.utilisation * 100).toFixed(0)}%</span>
                  </div>
                  <div className="flex flex-col">
                    <span className="text-[7px] font-hud text-slate-600 uppercase">P99</span>
                    <span className="text-[10px] font-data font-semibold text-slate-300">{Math.round(s.p99)}ms</span>
                  </div>
                  <div className="flex flex-col">
                    <span className="text-[7px] font-hud text-slate-600 uppercase">Risk</span>
                    <span className={`text-[10px] font-data font-semibold ${s.risk > 0.6 ? 'text-red-400' : s.risk > 0.3 ? 'text-amber-400' : 'text-slate-300'}`}>
                      {(s.risk * 100).toFixed(0)}%
                    </span>
                  </div>
                  <div className="flex flex-col">
                    <span className="text-[7px] font-hud text-slate-600 uppercase">Q̄</span>
                    <span className="text-[10px] font-data font-semibold text-slate-300">{s.queueLen.toFixed(1)}</span>
                  </div>
                </div>
                {/* Micro utilisation bar */}
                <div className="mt-1.5 h-1 w-full bg-slate-900 rounded-full overflow-hidden">
                  <div
                    className={`h-full transition-all duration-700 ${s.utilisation > 0.85 ? 'bg-red-500' : s.utilisation > 0.6 ? 'bg-amber-500' : 'bg-cyan-500'}`}
                    style={{ width: `${Math.min(100, s.utilisation * 100)}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </TacticalBox>
      </motion.div>
    </div>
  );
}
