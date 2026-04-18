'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { AnimatePresence, motion } from 'framer-motion';
import { Radio } from 'lucide-react';
import { clsx } from 'clsx';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { useWebSocket } from '@/hooks/useWebSocket';

/* ── Sidebar ──────────────────────────────────────────────────────────────── */
export function Sidebar() {
  const pathname = usePathname();
  const { connected } = useTelemetryStore();

  return (
    <aside
      className="z-50 flex w-[60px] flex-col overflow-y-auto border-r scrollbar-none"
      style={{
        background: '#000000',
        borderColor: 'rgba(255,255,255,0.05)',
        paddingTop: '16px',
        paddingBottom: '12px',
        paddingLeft: '6px',
        paddingRight: '6px',
      }}
    >
      {/* Logo mark */}
      <div className="mb-5 flex flex-col items-center gap-1.5">
        <div
          className="neo-inset flex h-8 w-8 items-center justify-center rounded-xl"
          style={{ borderColor: 'rgba(0,212,255,0.12)' }}
        >
          <span
            className="data-value text-[10px] font-bold"
            style={{ color: '#00D4FF', letterSpacing: '-0.02em' }}
          >
            LE
          </span>
        </div>
        <div className="flex items-center gap-1">
          <div className={clsx('status-dot', connected ? 'status-dot--live' : 'status-dot--crit')} />
        </div>
      </div>

      <div className="flex flex-1 flex-col gap-1">
        <Link
          href="/"
          className={clsx(
            'group relative flex flex-col items-center gap-1 rounded-xl px-1 py-2.5 transition-all duration-200',
            pathname === '/'
              ? 'neo-inset'
              : 'hover:bg-white/[0.02]'
          )}
          title="Command Center"
        >
          {pathname === '/' && (
            <div
              className="absolute left-0 top-1/2 h-5 w-[2px] -translate-y-1/2 rounded-r-full"
              style={{ background: '#00D4FF' }}
            />
          )}
          <Radio
            className="h-4 w-4 transition-colors duration-200"
            style={{ color: pathname === '/' ? '#00D4FF' : '#3A3D44' }}
          />
          <span
            className="label-xs"
            style={{
              fontSize: '7px',
              color: pathname === '/' ? '#A8ABB4' : '#3A3D44',
              letterSpacing: '0.1em',
            }}
          >
            CMD
          </span>
        </Link>
      </div>
    </aside>
  );
}

/* ── Header ───────────────────────────────────────────────────────────────── */
export function Header() {
  const { connected } = useTelemetryStore();
  const [clock, setClock] = useState('00:00:00');

  useEffect(() => {
    const timer = setInterval(() => {
      setClock(new Date().toLocaleTimeString('en-US', { hour12: false }));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  return (
    <header
      className="z-40 flex h-[42px] flex-shrink-0 items-center justify-between border-b px-4"
      style={{ background: '#000000', borderColor: 'rgba(255,255,255,0.05)' }}
    >
      {/* Left — brand */}
      <div className="flex items-center gap-3">
        <span
          className="data-value text-[11px] font-semibold tracking-[0.18em]"
          style={{ color: '#E8EAF0', textTransform: 'uppercase' }}
        >
          LOADEQUILIBRIUM
        </span>
        <div className="h-3 w-[1px]" style={{ background: 'rgba(255,255,255,0.1)' }} />
        <span className="label-xs opacity-60">CONTROL ROOM</span>
      </div>

      {/* Right — status + clock */}
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <div className={clsx('status-dot', connected ? 'status-dot--live' : 'status-dot--crit')} />
          <span
            className="data-value text-[10px]"
            style={{ color: connected ? '#30D158' : '#FF453A' }}
          >
            {connected ? 'LIVE' : 'OFFLINE'}
          </span>
        </div>
        <div
          className="h-3 w-[1px]"
          style={{ background: 'rgba(255,255,255,0.08)' }}
        />
        <span
          className="data-value text-[11px] font-medium"
          style={{ color: '#6E7380', letterSpacing: '0.06em' }}
        >
          {clock}
        </span>
      </div>
    </header>
  );
}

/* ── KPI Bar ──────────────────────────────────────────────────────────────── */
export function KpiBar() {
  const { tick } = useTelemetryStore();

  const kpis = [
    {
      label: 'Services',
      value: Object.keys(tick?.bundles || {}).length || '--',
      sub: 'Discovered',
    },
    {
      label: 'Objective',
      value: tick?.objective?.composite_score
        ? `${(tick.objective.composite_score * 100).toFixed(1)}%`
        : '--',
      sub: 'Score',
    },
    {
      label: 'Cascade Risk',
      value: tick?.objective?.cascade_failure_probability
        ? `${(tick.objective.cascade_failure_probability * 100).toFixed(1)}%`
        : '--',
      sub: 'Stability',
      warn: (tick?.objective?.cascade_failure_probability ?? 0) > 0.4,
    },
    {
      label: 'P99 Latency',
      value: tick?.objective?.predicted_p99_latency_ms
        ? `${Math.round(tick.objective.predicted_p99_latency_ms)}ms`
        : '--',
      sub: 'Network',
    },
    {
      label: 'Equilibrium',
      value: tick?.network_equilibrium?.system_rho_mean
        ? `${(tick.network_equilibrium.system_rho_mean * 100).toFixed(1)}%`
        : '--',
      sub: 'Convergence',
    },
    {
      label: 'Collapse P',
      value: tick?.fixed_point_equilibrium?.systemic_collapse_prob != null
        ? `${(tick.fixed_point_equilibrium.systemic_collapse_prob * 100).toFixed(1)}%`
        : '--',
      sub: 'Systemic',
      warn: (tick?.fixed_point_equilibrium?.systemic_collapse_prob ?? 0) > 0.3,
    },
  ];

  return (
    <div
      className="z-30 grid grid-cols-6 divide-x border-b flex-shrink-0"
      style={{
        background: '#000000',
        borderColor: 'rgba(255,255,255,0.05)',
      }}
    >
      {kpis.map((kpi) => (
        <div
          key={kpi.label}
          className="flex items-center justify-between px-4 py-2 transition-colors hover:bg-white/[0.015]"
          style={{ borderColor: 'rgba(255,255,255,0.04)' }}
        >
          <div className="flex flex-col">
            <span className="label-xs" style={{ fontSize: '8px', color: '#4A4D55' }}>
              {kpi.label}
            </span>
            <span className="label-xs" style={{ fontSize: '8px', color: '#3A3D44' }}>
              {kpi.sub}
            </span>
          </div>
          <span
            className="data-value text-sm font-semibold"
            style={{
              color: kpi.warn ? '#FF9F0A' : '#E8EAF0',
              letterSpacing: '-0.01em',
            }}
          >
            {kpi.value}
          </span>
        </div>
      ))}
    </div>
  );
}

/* ── Shell ────────────────────────────────────────────────────────────────── */
export default function DashboardShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  useWebSocket();

  return (
    <div
      className="relative flex h-screen w-screen overflow-hidden"
      style={{ background: '#000000' }}
    >
      <Sidebar />
      <div className="flex h-full flex-1 flex-col overflow-hidden">
        <Header />
        <KpiBar />
        <div className="relative flex-1 overflow-hidden">
          <AnimatePresence mode="wait">
            <motion.main
              key={pathname}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: 0.22, ease: [0.22, 1, 0.36, 1] }}
              className="scrollbar-none absolute inset-0 z-10 overflow-y-auto overflow-x-hidden p-3"
            >
              {children}
            </motion.main>
          </AnimatePresence>
        </div>
      </div>
    </div>
  );
}
