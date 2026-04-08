'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { motion, AnimatePresence } from 'framer-motion';
import {
  BarChart3,
  Activity,
  Network,
  Cpu,
  Drill,
  Brain,
  ShieldCheck,
  Rocket,
  Settings,
  AlertTriangle,
  Power,
  Layers,
  Orbit,
  Radio,
} from 'lucide-react';
import { clsx } from 'clsx';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { useWebSocket } from '@/hooks/useWebSocket';

const SIDEBAR_GROUPS = [
  {
    name: 'MONITORING',
    items: [
      { label: 'COMMAND', short: 'CMD', href: '/', icon: <Radio className="h-4 w-4" /> },
      { label: 'TELEMETRY', short: 'TEL', href: '/telemetry', icon: <BarChart3 className="h-4 w-4" /> },
      { label: 'TOPOLOGY', short: 'TOP', href: '/topology', icon: <Network className="h-4 w-4" /> },
      { label: 'CASCADE', short: 'CAS', href: '/cascade', icon: <Activity className="h-4 w-4" /> },
    ],
  },
  {
    name: 'INTELLIGENCE',
    items: [
      { label: 'AUTONOMY', short: 'AI', href: '/intelligence', icon: <Brain className="h-4 w-4" /> },
      { label: 'ALERTS', short: 'ALT', href: '/alerts', icon: <AlertTriangle className="h-4 w-4" /> },
    ],
  },
  {
    name: 'SIMULATION',
    items: [
      { label: 'RUNTIME', short: 'RUN', href: '/runtime', icon: <Orbit className="h-4 w-4" /> },
      { label: 'SANDBOX', short: 'BOX', href: '/sandbox', icon: <Layers className="h-4 w-4" /> },
      { label: 'SIMULATION', short: 'SIM', href: '/simulation', icon: <Cpu className="h-4 w-4" /> },
      { label: 'REPLAY', short: 'RPL', href: '/replay', icon: <Power className="h-4 w-4" /> },
    ],
  },
  {
    name: 'CONTROL',
    items: [
      { label: 'ACTUATOR', short: 'ACT', href: '/actuator', icon: <Drill className="h-4 w-4" /> },
      { label: 'POLICY', short: 'POL', href: '/policy', icon: <ShieldCheck className="h-4 w-4" /> },
    ],
  },
];

export function Sidebar() {
  const pathname = usePathname();
  const { connected, tick } = useTelemetryStore();
  const activeAlerts = tick?.priority_risk_queue?.length || 0;

  return (
    <aside className="row-span-4 z-50 flex w-[72px] flex-col overflow-y-auto border-r border-white/5 bg-[#010206]/92 px-1.5 pt-3 pb-2 shadow-2xl backdrop-blur-3xl scrollbar-none">
      <div className="group mb-4 flex cursor-pointer flex-col items-center gap-1.5 px-1">
        <div className="relative flex h-8 w-8 items-center justify-center overflow-hidden rounded-md border border-cyan-500/30 bg-cyan-500/10">
          <div className="absolute inset-0 bg-cyan-400/20 blur-xl transition-all duration-700 group-hover:bg-cyan-400/40" />
          <Rocket className="z-10 h-3.5 w-3.5 text-cyan-400" />
        </div>
        <div className="flex flex-col items-center">
          <span className="font-hud text-[7px] font-bold uppercase tracking-[0.18em] text-slate-300">NOC</span>
          <div className="mt-1 flex items-center gap-1">
            <span
              className={clsx(
                'h-1.5 w-1.5 rounded-full shadow-lg',
                connected ? 'bg-cyan-400 shadow-cyan-400/50' : 'animate-pulse bg-red-500 shadow-red-500/50'
              )}
            />
            <span
              className={clsx(
                'font-hud text-[5px] uppercase tracking-[0.14em]',
                connected ? 'text-cyan-400/80' : 'text-red-500/80'
              )}
            >
              {connected ? 'UP' : 'OFF'}
            </span>
          </div>
        </div>
      </div>

      <div className="flex flex-1 flex-col gap-3 px-0.5">
        {SIDEBAR_GROUPS.map((group) => (
          <div key={group.name} className="flex flex-col gap-1">
            {group.items.map((item) => {
              const isActive = pathname === item.href;
              const isAlert = item.label === 'ALERTS' && activeAlerts > 0;

              return (
                <Link
                  key={item.href}
                  href={item.href}
                  title={item.label}
                  className={clsx(
                    'group relative flex flex-col items-center gap-1 overflow-hidden rounded-md px-1.5 py-2 transition-all duration-300',
                    isActive ? 'bg-white/5' : 'hover:bg-white/[0.02]'
                  )}
                >
                  {isActive && (
                    <motion.div
                      layoutId="sidebarActiveIndicator"
                      className="absolute top-0 bottom-0 left-0 w-0.5 bg-cyan-400 shadow-[0_0_10px_#00f2ff]"
                      initial={false}
                      transition={{ type: 'spring', stiffness: 300, damping: 30 }}
                    />
                  )}

                  <div
                    className={clsx(
                      'flex-shrink-0 transition-colors duration-300',
                      isActive ? 'text-cyan-400' : isAlert ? 'animate-pulse text-amber-500' : 'text-slate-500 group-hover:text-cyan-400/70'
                    )}
                  >
                    {item.icon}
                  </div>

                  <span
                    className={clsx(
                      'text-[6px] font-hud tracking-[0.12em] transition-colors duration-300',
                      isActive ? 'font-bold text-cyan-50' : 'text-slate-500 group-hover:text-slate-300'
                    )}
                  >
                    {item.short}
                  </span>

                  {isAlert && (
                    <span className="absolute top-1.5 right-1.5 rounded border border-amber-500/30 bg-amber-500/10 px-1 py-0.5 text-[7px] font-hud text-amber-500">
                      {activeAlerts}
                    </span>
                  )}
                </Link>
              );
            })}
          </div>
        ))}
      </div>

      <div className="mt-3 px-0.5">
        <Link
          href="/settings"
          title="SETTINGS"
          className="group flex flex-col items-center gap-1 rounded-md px-1.5 py-2 text-slate-500 transition-all duration-300 hover:bg-white/[0.02] hover:text-cyan-400/70"
        >
          <Settings className="h-4 w-4 flex-shrink-0" />
          <span className="text-[6px] font-hud tracking-[0.12em]">CFG</span>
        </Link>
      </div>
    </aside>
  );
}

export function Header() {
  const { connected } = useTelemetryStore();
  const [clock, setClock] = useState('00:00:00:00');

  useEffect(() => {
    const start = Date.now();
    const timer = setInterval(() => {
      const elapsed = Math.floor((Date.now() - start) / 1000);
      const cs = Math.floor((Date.now() % 1000) / 10);
      const h = String(Math.floor(elapsed / 3600)).padStart(2, '0');
      const m = String(Math.floor((elapsed % 3600) / 60)).padStart(2, '0');
      const s = String(elapsed % 60).padStart(2, '0');
      const f = String(cs).padStart(2, '0');
      setClock(`${h}:${m}:${s}:${f}`);
    }, 50);

    return () => clearInterval(timer);
  }, []);

  return (
    <header className="z-40 flex h-[34px] items-center justify-between border-b border-white/5 bg-[#010206]/90 px-3 backdrop-blur-2xl">
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          <div className="h-1.5 w-1.5 rounded-sm bg-cyan-400 shadow-[0_0_6px_#00f2ff] animate-pulse" />
          <span className="font-hud text-[10px] font-black uppercase tracking-[0.16em] text-cyan-200 drop-shadow-[0_0_10px_rgba(34,211,238,0.6)]">LOADEQUILIBRIUM //</span>
        </div>
      </div>

      <div className="flex items-center gap-3">
        <div className="flex min-w-[66px] flex-col items-end">
          <span className="font-hud text-[5px] uppercase tracking-[0.2em] text-cyan-400/70 drop-shadow-[0_0_6px_rgba(34,211,238,0.4)]">MISSION</span>
          <span className="font-data text-[9px] font-bold tabular-nums tracking-[0.08em] text-cyan-400">{clock}</span>
        </div>
        <div className="flex flex-col items-end">
          <span className="font-hud text-[5px] uppercase tracking-[0.2em] text-cyan-400/70 drop-shadow-[0_0_6px_rgba(34,211,238,0.4)]">TRANSPORT</span>
          <span
            className={clsx(
              'rounded-sm border border-white/5 px-1.5 py-0.5 font-hud text-[6px] font-bold uppercase tracking-[0.12em] shadow-inner',
              connected ? 'bg-cyan-500/10 text-cyan-400' : 'animate-pulse bg-red-500/10 text-red-400'
            )}
          >
            {connected ? 'SYNC_ACTIVE' : 'OFFLINE'}
          </span>
        </div>
      </div>
    </header>
  );
}

export function KpiBar() {
  const { tick } = useTelemetryStore();

  const kpis = [
    { label: 'Services', value: Object.keys(tick?.bundles || {}).length || '--', sub: 'Discovery active' },
    { label: 'Objective', value: tick?.objective?.composite_score ? `${(tick.objective.composite_score * 100).toFixed(1)}%` : '--', sub: 'Optimisation score' },
    { label: 'Cascade Risk', value: tick?.objective?.cascade_failure_probability ? `${(tick.objective.cascade_failure_probability * 100).toFixed(1)}%` : '--', sub: 'System stability' },
    { label: 'P99 Latency', value: tick?.objective?.predicted_p99_latency_ms ? `${Math.round(tick.objective.predicted_p99_latency_ms)}ms` : '--', sub: 'Network health' },
    { label: 'Equilibrium', value: tick?.network_equilibrium?.system_rho_mean ? `${(tick.network_equilibrium.system_rho_mean * 100).toFixed(1)}%` : '--', sub: 'State convergence' },
    { label: 'Collapse P', value: tick?.fixed_point_equilibrium?.systemic_collapse_prob != null ? `${(tick.fixed_point_equilibrium.systemic_collapse_prob * 100).toFixed(1)}%` : '--', sub: 'Systemic risk' },
  ];

  return (
    <div className="z-40 grid grid-cols-6 divide-x divide-white/5 border-b border-white/5 bg-[#010206]/95 backdrop-blur-xl">
      {kpis.map((kpi) => (
        <div key={kpi.label} className="group flex min-w-0 items-center justify-between gap-2 px-3 py-1.5 transition-colors hover:bg-white/[0.02]">
          <div className="flex min-w-0 flex-col justify-center">
            <span className="mb-0.5 truncate font-hud text-[6px] uppercase tracking-[0.16em] text-cyan-300/60 drop-shadow-[0_0_4px_rgba(34,211,238,0.3)]">{kpi.label}</span>
            <span className="truncate font-hud text-[7px] uppercase tracking-[0.1em] text-cyan-200/50 drop-shadow-[0_0_4px_rgba(34,211,238,0.25)]">{kpi.sub}</span>
          </div>
          <span className="shrink-0 font-data text-sm font-bold tabular-nums text-cyan-50 drop-shadow-sm transition-colors group-hover:text-cyan-400">
            {kpi.value}
          </span>
        </div>
      ))}
    </div>
  );
}

export default function DashboardShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  useWebSocket();

  return (
    <div className="relative z-10 flex h-screen w-screen overflow-hidden selection:bg-cyan-500/30 selection:text-white">
      <Sidebar />
      <div className="flex h-full flex-1 flex-col bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-cyan-900/10 via-[#010206] to-[#010206]">
        <Header />
        <div className="relative flex h-full flex-1 flex-col overflow-hidden">
          <KpiBar />
          <div className="relative h-full flex-1 overflow-hidden">
            <AnimatePresence mode="wait">
              <motion.main
                key={pathname}
                initial={{ opacity: 0, y: 15, filter: 'blur(4px)' }}
                animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
                exit={{ opacity: 0, y: -15, filter: 'blur(4px)' }}
                transition={{ duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
                className="scrollbar-hud absolute inset-0 z-10 overflow-y-auto overflow-x-hidden p-2.5 lg:p-3"
              >
                {children}
              </motion.main>
            </AnimatePresence>
          </div>
        </div>
      </div>
    </div>
  );
}
