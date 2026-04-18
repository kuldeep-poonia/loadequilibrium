'use client';

import { useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import {
  AreaChart, Area, ResponsiveContainer,
} from 'recharts';
import {
  Activity, AlertTriangle, Bot, Gauge, Orbit, Power, Radar, ShieldCheck, Zap,
} from 'lucide-react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { TopologyCanvas } from './TopologyModule';

type Tone = 'cyan' | 'amber' | 'red';

const ease = [0.22, 1, 0.36, 1] as const;

function fmt(value?: number | null, digits = 1) {
  if (value == null || Number.isNaN(value)) return '--';
  return `${(value * 100).toFixed(digits)}%`;
}
function fmtMs(value?: number | null, digits = 1) {
  if (value == null || Number.isNaN(value)) return '--';
  return `${value.toFixed(digits)}ms`;
}
function fmtSigned(value?: number | null, digits = 3) {
  if (value == null || Number.isNaN(value)) return '--';
  return `${value >= 0 ? '+' : ''}${value.toFixed(digits)}`;
}
function fmtClock(ts?: string | null) {
  if (!ts) return '--:--:--';
  const d = new Date(ts);
  return isNaN(d.getTime()) ? ts : d.toLocaleTimeString('en-US', { hour12: false });
}
function polyPts(values: number[], w: number, h: number, maxV?: number) {
  if (!values.length) return '';
  const m = Math.max(maxV ?? 0, ...values, 1);
  return values.map((v, i) => {
    const x = values.length === 1 ? w / 2 : (i / (values.length - 1)) * w;
    const y = h - (Math.max(0, v) / m) * h;
    return `${x},${Number.isFinite(y) ? y : h}`;
  }).join(' ');
}

/* ── Stat Card ────────────────────────────────────────────────────────────── */
function StatCard({
  label, value, sub, tone = 'cyan',
}: { label: string; value: string; sub?: string; tone?: Tone }) {
  const color = tone === 'red' ? '#FF453A' : tone === 'amber' ? '#FF9F0A' : '#00D4FF';
  return (
    <div className={`neo-card neo-card--${tone} rounded-2xl px-3.5 py-3`}>
      <span className="label-xs block">{label}</span>
      <div
        className="data-value mt-2 text-[15px] font-semibold"
        style={{ color, letterSpacing: '-0.01em' }}
      >
        {value}
      </div>
      {sub && <div className="label-xs mt-1 opacity-60">{sub}</div>}
    </div>
  );
}

/* ── Stage Pill ───────────────────────────────────────────────────────────── */
function StagePill({
  label, value, pct, index,
}: { label: string; value: string; pct: number; index: number }) {
  const warn = (parseFloat(value) || 0) > 20;
  return (
    <motion.div layout className="neo-inset rounded-xl px-2.5 py-2">
      <div className="flex items-center justify-between gap-1">
        <span className="label-xs truncate" style={{ fontSize: '8px', color: '#4A4D55' }}>{label}</span>
        <span className="data-value text-[10px]" style={{ color: warn ? '#FF9F0A' : '#A8ABB4' }}>{value}</span>
      </div>
      <div className="progress-track mt-1.5">
        <motion.div
          className="progress-bar"
          initial={false}
          animate={{ width: `${Math.max(pct, 6)}%` }}
          transition={{ duration: 0.4, ease, delay: index * 0.02 }}
          style={{ background: warn ? '#FF9F0A' : '#00D4FF' }}
        />
      </div>
    </motion.div>
  );
}

/* ── Mini Trend Chart ─────────────────────────────────────────────────────── */
function ControlButton({
  label, value, tone = 'cyan', icon: Icon, busy, onClick,
}: { label: string; value: string; tone?: Tone; icon: typeof Zap; busy: boolean; onClick: () => void }) {
  const color = tone === 'red' ? '#FF453A' : tone === 'amber' ? '#FF9F0A' : '#00D4FF';
  return (
    <button
      type="button"
      disabled={busy}
      onClick={onClick}
      className="neo-card rounded-lg px-2.5 py-2 text-left transition-opacity disabled:opacity-45"
      style={{ borderColor: `${color}33` }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="label-xs" style={{ color }}>{label}</span>
        <Icon className="h-3.5 w-3.5 flex-shrink-0" style={{ color }} />
      </div>
      <div className="data-value mt-1 text-[9px]" style={{ color: '#6E7380' }}>
        {busy ? 'sending' : value}
      </div>
    </button>
  );
}

function TrendChart({ objective, cascade, tickMs }: { objective: number[]; cascade: number[]; tickMs: number[] }) {
  const W = 360, H = 90;
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="h-full w-full">
      {[0, 1, 2, 3].map(i => (
        <line key={i} x1="0" y1={(H / 3) * i} x2={W} y2={(H / 3) * i} stroke="rgba(255,255,255,0.04)" strokeWidth="1" />
      ))}
      <polyline points={polyPts(tickMs, W, H, Math.max(100, ...tickMs, 1))} fill="none" stroke="rgba(255,159,10,0.7)" strokeWidth="1.5" strokeDasharray="4 3" />
      <polyline points={polyPts(objective, W, H, 1)} fill="none" stroke="rgba(0,212,255,0.8)" strokeWidth="2" />
      <polyline points={polyPts(cascade, W, H, 1)} fill="none" stroke="rgba(255,69,58,0.7)" strokeWidth="1.5" />
    </svg>
  );
}

/* ── Risk Bars ────────────────────────────────────────────────────────────── */
function RiskBars({ values }: { values: number[] }) {
  return (
    <div className="flex h-8 items-end gap-0.5">
      {values.map((v, i) => (
        <div key={i} className="flex-1">
          <div
            className="w-full rounded-t-sm"
            style={{
              height: `${Math.max(12, v * 100)}%`,
              background: v > 0.7 ? '#FF453A' : v > 0.35 ? '#FF9F0A' : '#00D4FF',
              opacity: 0.7,
            }}
          />
        </div>
      ))}
    </div>
  );
}

/* ── Frame Strip ──────────────────────────────────────────────────────────── */
function FrameStrip({ frames }: { frames: Array<{ seq: number; obj: number; casc: number; tickMs: number }> }) {
  if (!frames.length) {
    return (
      <div className="neo-inset flex h-full items-center justify-center rounded-2xl">
        <span className="label-xs opacity-40">Awaiting replay frames</span>
      </div>
    );
  }
  const maxTick = Math.max(100, ...frames.map(f => f.tickMs), 1);
  return (
    <div className="grid h-full grid-cols-1 gap-2 md:grid-cols-2 2xl:grid-cols-4">
      {frames.map((frame, index) => (
        <motion.div
          key={frame.seq}
          initial={{ opacity: 0, y: -8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.25, ease }}
          className="neo-inset rounded-2xl px-3 py-3"
        >
          <div className="flex items-center justify-between">
            <span className="label-xs" style={{ fontSize: '8px' }}>FRAME {frame.seq}</span>
            <span className="data-value text-[10px]" style={{ color: '#4A4D55' }}>{frame.tickMs.toFixed(1)}ms</span>
          </div>
          <div className="mt-3 space-y-2">
            {[
              { key: 'objective', val: frame.obj, color: '#00D4FF', max: 1 },
              { key: 'cascade',   val: frame.casc, color: '#FF453A', max: 1 },
              { key: 'dt',        val: frame.tickMs, color: '#FF9F0A', max: maxTick },
            ].map(row => (
              <div key={row.key}>
                <div className="mb-1 flex items-center justify-between">
                  <span className="label-xs" style={{ fontSize: '8px', color: '#4A4D55' }}>{row.key}</span>
                  <span className="data-value text-[9px]" style={{ color: '#6E7380' }}>
                    {row.key === 'dt' ? `${row.val.toFixed(1)}ms` : fmt(row.val)}
                  </span>
                </div>
                <div className="progress-track">
                  <motion.div
                    className="progress-bar"
                    initial={false}
                    animate={{ width: `${Math.max((row.val / row.max) * 100, 6)}%` }}
                    transition={{ duration: 0.4, ease, delay: index * 0.03 }}
                    style={{ background: row.color }}
                  />
                </div>
              </div>
            ))}
          </div>
        </motion.div>
      ))}
    </div>
  );
}

/* ── Main Component ───────────────────────────────────────────────────────── */
export default function CommandCenter() {
  const { tick, history, connected, triggerDomain } = useTelemetryStore();
  const [chartsReady, setChartsReady] = useState(false);
  const [controlBusy, setControlBusy] = useState<string | null>(null);
  const bundles = tick?.bundles || {};
  const runtime = tick?.runtime_metrics;
  const controlPlane = tick?.control_plane;
  const stageMetrics = runtime ? [
    { label: 'PRUNE',    value: runtime.avg_prune_ms },
    { label: 'WINDOWS',  value: runtime.avg_windows_ms },
    { label: 'TOPOLOGY', value: runtime.avg_topology_ms },
    { label: 'COUPLING', value: runtime.avg_coupling_ms },
    { label: 'MODELS',   value: runtime.avg_modelling_ms },
    { label: 'OPTIMISE', value: runtime.avg_optimise_ms },
    { label: 'SIM',      value: runtime.avg_sim_ms },
    { label: 'REASON',   value: runtime.avg_reasoning_ms },
    { label: 'BCAST',    value: runtime.avg_broadcast_ms },
  ] : [];
  const totalRuntimeMs = stageMetrics.reduce((s, m) => s + (m.value || 0), 0);
  const objective      = tick?.objective;
  const equilibrium    = tick?.network_equilibrium;
  const criticalPath   = tick?.topology?.critical_path;
  const fixedPoint     = tick?.fixed_point_equilibrium;
  const stabilityEnvelope = tick?.stability_envelope;
  const historyWindow  = history.slice(-24);
  const objectiveTrend = historyWindow.map(s => s.obj);
  const cascadeTrend   = historyWindow.map(s => s.casc);
  const tickTrend      = historyWindow.map(s => s.tickMs);
  const replayFrames   = historyWindow.slice(-8);
  const directives     = Object.entries(tick?.directives || {}).map(([service, d]) => ({ service, ...d })).slice(0, 5);
  const watchlist      = (tick?.priority_risk_queue || []).slice(0, 5);
  const sensitivity    = tick?.topology_sensitivity;
  const runwayList     = Object.entries(tick?.risk_timeline || {})
    .map(([service, pts]) => ({ service, latestRisk: pts[pts.length - 1]?.risk || 0, values: pts.slice(-12).map(p => p.risk || 0) }))
    .sort((a, b) => b.latestRisk - a.latestRisk).slice(0, 4);
  const events = [...(tick?.events || [])].slice(-10).reverse();
  const signalTelemetry = Object.entries(bundles).map(([service, bundle]) => {
    const signal = bundle.signal, queue = bundle.queue, stability = bundle.stability;
    const confidence = bundle.stochastic?.confidence ?? queue?.confidence ?? 0;
    const cusumMagnitude = Math.max(signal?.cusum_pos ?? 0, Math.abs(signal?.cusum_neg ?? 0));
    const fluxResidual = queue?.service_rate ? Math.abs((queue.arrival_rate || 0) - queue.service_rate) / Math.max(queue.service_rate, 1) : 0;
    return { service, confidence, cusumMagnitude, variance: signal?.ewma_variance ?? 0, spike: signal?.spike_detected ?? false, drift: stability?.stability_derivative ?? 0, margin: stability?.stability_margin ?? 0, fluxResidual, utilisation: queue?.utilisation ?? 0 };
  }).sort((a, b) => (a.confidence - b.confidence) || (b.cusumMagnitude - a.cusumMagnitude)).slice(0, 6);
  const conservationRows = Object.entries(bundles).map(([service, bundle]) => {
    const queue = bundle.queue;
    const arrival = queue?.arrival_rate ?? 0, serviceRate = queue?.service_rate ?? 0;
    const residual = serviceRate > 0 ? Math.abs(arrival - serviceRate) / Math.max(serviceRate, 1) : 0;
    return { service, arrival, serviceRate, residual };
  }).sort((a, b) => b.residual - a.residual).slice(0, 4);
  const meanSignalConfidence = signalTelemetry.length ? signalTelemetry.reduce((s, x) => s + x.confidence, 0) / signalTelemetry.length : 0;
  const meanFluxResidual = conservationRows.length ? conservationRows.reduce((s, x) => s + x.residual, 0) / conservationRows.length : 0;
  const convergenceResidual = Math.abs(equilibrium?.equilibrium_delta ?? 0);
  const driftProxy = Math.abs(stabilityEnvelope?.worst_perturbation_delta ?? 0);
  const predictionBands = Object.entries(tick?.prediction_timeline || {}).map(([service, pts]) => {
    const latest = pts[pts.length - 1];
    return { service, rho: latest?.rho ?? 0, spread: Math.max((latest?.hi ?? 0) - (latest?.lo ?? 0), 0), horizon: latest?.t ?? 0 };
  }).sort((a, b) => b.spread - a.spread).slice(0, 4);
  const headerStatus = !connected ? 'critical' : tick?.safety_mode ? 'critical' : runtime?.predicted_overrun ? 'warning' : 'nominal';

  const runControl = async (key: string, domain: string, payload?: Record<string, string | number | boolean>) => {
    setControlBusy(key);
    const result = await triggerDomain(domain, payload);
    if (!result.ok) {
      console.error(`[control] ${domain} failed: ${result.error}`);
    }
    setControlBusy(null);
  };

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => setChartsReady(true));
    return () => window.cancelAnimationFrame(frame);
  }, []);

  /* Row/section fade-in */
  const section = (delay: number) => ({
    initial: { opacity: 0, y: 10 },
    animate: { opacity: 1, y: 0 },
    transition: { duration: 0.24, ease: "easeOut" as const, delay },
  });

  return (
    <div className="flex min-h-full flex-col gap-2.5">

      {/* ── Observatory Header ──────────────────────────────────────────── */}
      <motion.section {...section(0)} className="flex-shrink-0">
        <TacticalBox title="NUMERICAL OBSERVATORY" badge={connected ? 'STREAMING' : 'OFFLINE'} status={headerStatus}>
          <div className="flex min-w-0 flex-col gap-3">

            {/* Top row: info + stat cards */}
            <div className="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
              {/* Description */}
              <div className="flex flex-col gap-2">
                <div>
                  <p className="label-xs" style={{ color: '#4A4D55' }}>Simulation Laboratory Software</p>
                  <h1 className="data-value mt-1 text-[15px] font-semibold tracking-tight" style={{ color: '#E8EAF0' }}>
                    LOADEQUILIBRIUM
                  </h1>
                  <p className="mt-1.5 text-[10px] leading-[1.55]" style={{ color: '#4A4D55', fontFamily: 'var(--font-data)' }}>
                    Convergence, conservation, signal integrity, and topology stability surfaced as live numerical readouts.
                  </p>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  <span className={`chip ${connected ? 'chip--cyan' : 'chip--red'}`}>
                    {connected ? 'Stream Locked' : 'Disconnected'}
                  </span>
                  <span className={`chip ${tick?.safety_mode ? 'chip--red' : 'chip--cyan'}`}>
                    {tick?.safety_mode ? 'Guard Active' : 'Free-Running'}
                  </span>
                  <span className={`chip ${runtime?.predicted_overrun ? 'chip--amber' : 'chip--cyan'}`}>
                    {runtime?.predicted_overrun ? 'Overrun Watch' : 'Cadence Stable'}
                  </span>
                </div>
              </div>

              {/* Stat cards */}
              <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
                <StatCard label="Integration dt" value={fmtMs(tick?.tick_health_ms)} sub={`Age ${tick?.sim_overlay?.sim_tick_age ?? 0}`} tone="cyan" />
                <StatCard label="Convergence" value={fmtSigned(convergenceResidual)} sub={`Rate ${fixedPoint?.convergence_rate?.toFixed(3) ?? '--'}`} />
                <StatCard label="Conservation" value={fmt(1 - Math.min(meanFluxResidual, 1))} sub={`Residual ${fmtSigned(meanFluxResidual)}`} tone="amber" />
                <StatCard label="Signal" value={fmt(meanSignalConfidence)} sub={`Jitter ${fmtMs(tick?.jitter_ms)}`} tone="red" />
              </div>
            </div>

            {/* Stage pipeline */}
            <div className="grid min-w-0 gap-1.5 grid-cols-3 md:grid-cols-5 lg:grid-cols-9">
              {stageMetrics.length === 0 ? (
                <div className="col-span-full neo-inset rounded-2xl px-4 py-3 text-center">
                  <span className="label-xs opacity-40">Awaiting runtime stage telemetry</span>
                </div>
              ) : (
                stageMetrics.map((s, i) => (
                  <StagePill
                    key={s.label} label={s.label}
                    value={fmtMs(s.value)}
                    pct={totalRuntimeMs > 0 ? (s.value / totalRuntimeMs) * 100 : 0}
                    index={i}
                  />
                ))
              )}
            </div>
          </div>
        </TacticalBox>
      </motion.section>

      {/* ── Main 3-column grid ─────────────────────────────────────────── */}
      <div className="grid min-h-0 flex-1 gap-2.5 xl:grid-cols-[200px_minmax(0,1fr)_296px] 2xl:grid-cols-[216px_minmax(0,1fr)_316px]">

        {/* ── Left column ─────────────────────────────────────────────── */}
        <motion.aside {...section(0.04)} className="min-h-0 min-w-0">
          <div className="flex h-full flex-col gap-2.5">

            {/* System Telemetry charts */}
            <TacticalBox title="SYSTEM TELEMETRY" badge="LIVE" status="nominal">
              <div className="flex h-full flex-col gap-2">
                {[
                  { key: 'throughput', label: 'Throughput · req/s', color: '#00D4FF', id: 'tp' },
                  { key: 'queueDepth', label: 'Queue Depth',        color: '#A855F7', id: 'qd' },
                  { key: 'workers',    label: 'Active Workers',      color: '#FF9F0A', id: 'wk' },
                ].map(chart => (
                  <div key={chart.key} className="neo-inset flex flex-1 min-h-[110px] flex-col rounded-xl p-2.5">
                    <span className="label-xs mb-1" style={{ color: chart.color, opacity: 0.7 }}>{chart.label}</span>
                    <div className="flex-1">
                      {chartsReady && (
                        <ResponsiveContainer width="100%" height="100%" minWidth={0} minHeight={0}>
                          <AreaChart data={history} margin={{ top: 4, right: 0, left: 0, bottom: 0 }}>
                            <defs>
                              <linearGradient id={`g-${chart.id}`} x1="0" y1="0" x2="0" y2="1">
                                <stop offset="5%"  stopColor={chart.color} stopOpacity={0.2} />
                                <stop offset="95%" stopColor={chart.color} stopOpacity={0} />
                              </linearGradient>
                            </defs>
                            <Area type="monotone" dataKey={chart.key} stroke={chart.color} strokeWidth={1.5} fill={`url(#g-${chart.id})`} isAnimationActive={false} />
                          </AreaChart>
                        </ResponsiveContainer>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </TacticalBox>

            {/* Signal / Conservation */}
            <TacticalBox title="SIGNAL / CONSERVATION" badge={`${signalTelemetry.length} CH`} className="min-h-0 flex-1">
              <div className="flex h-full flex-col gap-2">
                <div className="grid grid-cols-2 gap-2">
                  <div className="neo-card neo-card--cyan rounded-xl px-3 py-2.5">
                    <span className="label-xs block">Signal</span>
                    <div className="data-value mt-1.5 text-sm font-semibold" style={{ color: '#00D4FF' }}>{fmt(meanSignalConfidence)}</div>
                    <div className="label-xs mt-0.5 opacity-50">integrity</div>
                  </div>
                  <div className="neo-card neo-card--amber rounded-xl px-3 py-2.5">
                    <span className="label-xs block">Flux</span>
                    <div className="data-value mt-1.5 text-sm font-semibold" style={{ color: '#FF9F0A' }}>{fmtSigned(meanFluxResidual)}</div>
                    <div className="label-xs mt-0.5 opacity-50">balance</div>
                  </div>
                </div>

                <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto scrollbar-none">
                  {conservationRows.map(row => (
                    <div key={row.service} className={`neo-card neo-card--${row.residual > 0.2 ? 'red' : row.residual > 0.08 ? 'amber' : 'cyan'} rounded-xl p-2.5`}>
                      <div className="flex items-start justify-between gap-2">
                        <span className="data-value break-all text-[9px]" style={{ color: '#A8ABB4' }}>{row.service}</span>
                        <span className="data-value text-[10px] flex-shrink-0" style={{ color: row.residual > 0.2 ? '#FF453A' : row.residual > 0.08 ? '#FF9F0A' : '#00D4FF' }}>{fmtSigned(row.residual)}</span>
                      </div>
                      <div className="mt-1.5 progress-track">
                        <div className="progress-bar" style={{ width: `${Math.min(row.residual * 100, 100)}%`, background: row.residual > 0.2 ? '#FF453A' : row.residual > 0.08 ? '#FF9F0A' : '#00D4FF' }} />
                      </div>
                    </div>
                  ))}
                  {conservationRows.length === 0 && (
                    <div className="neo-inset rounded-xl p-3 text-center">
                      <span className="label-xs opacity-30">No conservation data</span>
                    </div>
                  )}

                  {/* Directives */}
                  <div className="neo-inset rounded-xl px-3 py-2.5">
                    <span className="label-xs block mb-1.5">Actuation Field</span>
                    <div className="space-y-1.5">
                      {directives.slice(0, 3).map(d => (
                        <div key={d.service} className="flex items-center justify-between gap-2">
                          <span className="data-value break-all text-[9px]" style={{ color: '#00D4FF' }}>{d.service}</span>
                          <span className="data-value text-[9px]" style={{ color: '#4A4D55' }}>
                            x{(d.scale_factor ?? 1).toFixed(2)} u{fmt(d.target_utilisation ?? 0, 0)}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </TacticalBox>
          </div>
        </motion.aside>

        {/* ── Center column ───────────────────────────────────────────── */}
        <div className="grid min-h-0 min-w-0 gap-2.5 xl:grid-rows-[minmax(0,1fr)_660px]">

          {/* Topology Field */}
          <motion.section {...section(0.08)} className="min-h-[400px] min-w-0 xl:min-h-0">
            <TacticalBox
              title="TOPOLOGY FIELD"
              badge={tick?.topology?.nodes?.length ? `${tick.topology.nodes.length}N / ${tick.topology.edges.length}E` : 'WARMING'}
              status={(criticalPath?.cascade_risk || 0) > 0.45 ? 'critical' : 'nominal'}
              className="h-full"
            >
              <div className="flex h-full flex-col gap-2.5">
                {/* Chips row */}
                <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap gap-1.5">
                    <span className="chip chip--cyan">dt {fmtMs(tick?.tick_health_ms)}</span>
                    <span className="chip chip--amber">Δ {fmtSigned(convergenceResidual)}</span>
                    <span className="chip chip--red">drift {fmtSigned(driftProxy)}</span>
                    <span className="chip chip--cyan">obj {fmt(objective?.composite_score)}</span>
                  </div>
                  <div className="neo-inset rounded-xl px-3 py-1.5 text-right">
                    <span className="label-xs" style={{ fontSize: '8px', color: '#4A4D55' }}>Solver Frame</span>
                    <div className="data-value mt-0.5 text-sm font-semibold" style={{ color: '#E8EAF0' }}>SEQ {tick?.seq ?? 0}</div>
                  </div>
                </div>

                {/* Canvas */}
                <div
                  className="relative flex min-h-[240px] flex-1 overflow-hidden rounded-2xl"
                  style={{ background: '#05080C', border: '1px solid rgba(255,255,255,0.04)' }}
                >
                  <TopologyCanvas />
                </div>

                {/* Timeline + metrics */}
                <div className="grid min-w-0 gap-2.5 lg:grid-cols-[minmax(0,1fr)_216px] 2xl:grid-cols-[minmax(0,1fr)_232px]">
                  <div className="neo-inset rounded-xl p-3">
                    <div className="flex items-center justify-between gap-2 mb-2">
                      <span className="label-xs">Numerical Progression</span>
                      <span className="label-xs opacity-50">{equilibrium?.is_converging ? 'converging' : 'drifting'}</span>
                    </div>
                    <div className="h-[72px]">
                      <TrendChart objective={objectiveTrend} cascade={cascadeTrend} tickMs={tickTrend} />
                    </div>
                  </div>
                  <div className="neo-inset rounded-xl p-3">
                    <span className="label-xs block mb-2">Stability Metrics</span>
                    <div className="grid grid-cols-2 gap-2">
                      {[
                        { label: 'Fragility',   value: fmt(sensitivity?.system_fragility) },
                        { label: 'ρ Variance',  value: fmtSigned(equilibrium?.system_rho_variance ?? 0) },
                        { label: 'Crit Risk',   value: fmt(criticalPath?.cascade_risk) },
                        { label: 'Equil ρ',     value: fmt(equilibrium?.system_rho_mean, 0) },
                      ].map(m => (
                        <div key={m.label}>
                          <span className="label-xs" style={{ fontSize: '8px', color: '#3A3D44' }}>{m.label}</span>
                          <div className="data-value mt-0.5 text-sm font-semibold" style={{ color: '#E8EAF0' }}>{m.value}</div>
                        </div>
                      ))}
                    </div>
                    {criticalPath?.nodes?.length ? (
                      <div className="mt-2 flex flex-wrap gap-1">
                        {criticalPath.nodes.map(n => (
                          <span key={n} className="chip chip--red" style={{ fontSize: '8px' }}>{n}</span>
                        ))}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            </TacticalBox>
          </motion.section>

          {/* Replay Dock */}
          <motion.section {...section(0.12)} className="min-h-0 min-w-0">
            <TacticalBox title="REPLAY DOCK" badge={`${events.length} EVENTS`} className="h-full">
              <div className="grid h-full min-h-0 gap-2.5 xl:grid-rows-[auto_minmax(0,1fr)]">

                {/* Replay stats */}
                <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
                  {[
                    { label: 'Frame',     value: String(tick?.seq ?? 0) },
                    { label: 'Timestep',  value: fmtMs(tick?.tick_health_ms, 1), tone: 'cyan' },
                    { label: 'Residual',  value: fmtSigned(convergenceResidual, 2), tone: 'amber' },
                    { label: 'Frame Age', value: String(tick?.sim_overlay?.sim_tick_age ?? 0), tone: 'red' },
                  ].map(s => (
                    <StatCard key={s.label} label={s.label} value={s.value} tone={(s.tone as Tone) || 'cyan'} />
                  ))}
                </div>

                {/* Frame strip + events */}
                <div className="grid h-full min-h-0 gap-2.5 lg:grid-cols-[minmax(0,1fr)_256px]">
                  <div className="neo-inset flex flex-col rounded-2xl p-3">
                    <div className="mb-2 flex items-center justify-between">
                      <span className="label-xs">Frame Evolution</span>
                      <span className="label-xs opacity-40">{fmtClock(tick?.ts)}</span>
                    </div>
                    <div className="min-h-0 flex-1 overflow-y-auto scrollbar-none">
                      <FrameStrip frames={replayFrames} />
                    </div>
                  </div>

                  {/* Event log */}
                  <div className="neo-inset flex flex-col rounded-2xl p-3">
                    <div className="mb-2 flex items-center justify-between">
                      <span className="label-xs">Event Timeline</span>
                      <Orbit className="h-3.5 w-3.5" style={{ color: '#3A3D44' }} />
                    </div>
                    <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto scrollbar-none">
                      {events.length === 0 ? (
                        <div className="flex h-full items-center justify-center">
                          <span className="label-xs opacity-30">No events yet</span>
                        </div>
                      ) : events.map((e, i) => (
                        <div
                          key={`${e.timestamp || 'evt'}-${i}`}
                          className="rounded-xl p-2.5 transition-colors hover:bg-white/[0.02]"
                          style={{ border: '1px solid rgba(255,255,255,0.04)', background: '#0A0C10' }}
                        >
                          <div className="flex items-start justify-between gap-2">
                            <div className="min-w-0">
                              <div className="data-value text-[9px] font-semibold" style={{ color: '#A8ABB4' }}>{e.category}</div>
                              <div className="mt-0.5 text-[9px] leading-snug" style={{ color: '#4A4D55', fontFamily: 'var(--font-data)' }}>{e.description}</div>
                            </div>
                            <div className="flex-shrink-0 text-right">
                              <div className="label-xs" style={{ fontSize: '8px', color: e.severity === 'critical' ? '#FF453A' : e.severity === 'warning' ? '#FF9F0A' : '#00D4FF' }}>
                                {e.severity}
                              </div>
                              <div className="label-xs mt-0.5 opacity-40">{fmtClock(e.timestamp)}</div>
                              {e.id && (
                                <button
                                  type="button"
                                  className="label-xs mt-1 rounded-md px-1.5 py-0.5"
                                  style={{ border: '1px solid rgba(255,255,255,0.08)', color: '#6E7380' }}
                                  disabled={controlBusy === `ack-${e.id}`}
                                  onClick={() => runControl(`ack-${e.id}`, 'alerts/ack', { alert_id: e.id || '' })}
                                >
                                  ack
                                </button>
                              )}
                            </div>
                          </div>
                          {e.service_id && (
                            <div className="mt-1.5 border-t pt-1.5 text-[9px]" style={{ borderColor: 'rgba(255,255,255,0.04)', color: '#3A3D44', fontFamily: 'var(--font-data)' }}>
                              {e.service_id}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </TacticalBox>
          </motion.section>
        </div>

        {/* ── Right column ────────────────────────────────────────────── */}
        <motion.aside {...section(0.10)} className="min-h-0 min-w-0">
          <div className="flex h-full flex-col gap-2.5">

            {/* Convergence */}
            <TacticalBox
              title="CONVERGENCE"
              badge={tick?.fixed_point_equilibrium?.converged ? 'CONVERGED' : 'ITERATING'}
              status={tick?.fixed_point_equilibrium?.converged ? 'nominal' : 'warning'}
            >
              <div className="grid min-w-0 grid-cols-2 gap-2">
                <StatCard label="Converged"   value={tick?.fixed_point_equilibrium?.converged ? 'YES' : 'NO'} sub={`Rate ${tick?.fixed_point_equilibrium?.convergence_rate?.toFixed(3) ?? '--'}`} tone="cyan" />
                <StatCard label="Residual"    value={fmtSigned(convergenceResidual)} sub={`${tick?.fixed_point_equilibrium?.converged_iterations ?? 0} iters`} tone="red" />
                <StatCard label="Drift"       value={fmtSigned(driftProxy)} sub="perturbation Δ" tone="amber" />
                <StatCard label="Envelope"    value={fmt(stabilityEnvelope?.envelope_headroom)} sub={`ρ max ${fmt(stabilityEnvelope?.safe_system_rho_max, 0)}`} />
              </div>
            </TacticalBox>

            <TacticalBox
              title="CONTROL PLANE"
              badge={controlPlane?.actuation_enabled ? 'ACTUATING' : 'HELD'}
              status={controlPlane?.actuation_enabled ? 'nominal' : 'warning'}
            >
              <div className="grid grid-cols-2 gap-2">
                <ControlButton
                  label="Actuation"
                  value={controlPlane?.actuation_enabled ? 'enabled' : 'held'}
                  icon={Power}
                  tone={controlPlane?.actuation_enabled ? 'cyan' : 'amber'}
                  busy={controlBusy === 'toggle'}
                  onClick={() => runControl('toggle', 'control/toggle')}
                />
                <ControlButton
                  label="Step"
                  value={`tick ${controlPlane?.tick ?? tick?.seq ?? 0}`}
                  icon={Activity}
                  busy={controlBusy === 'step'}
                  onClick={() => runControl('step', 'runtime/step')}
                />
                <ControlButton
                  label="Policy"
                  value={controlPlane?.policy_preset || 'balanced'}
                  icon={ShieldCheck}
                  tone="amber"
                  busy={controlBusy === 'policy'}
                  onClick={() => runControl('policy', 'policy/update', { preset: 'stability' })}
                />
                <ControlButton
                  label="Sandbox"
                  value={`until ${controlPlane?.forced_sandbox_until ?? 0}`}
                  icon={AlertTriangle}
                  tone="red"
                  busy={controlBusy === 'sandbox'}
                  onClick={() => runControl('sandbox', 'sandbox/trigger', { type: 'operator', duration_ticks: 8 })}
                />
                <ControlButton
                  label="Simulation"
                  value={`until ${controlPlane?.forced_simulation_until ?? 0}`}
                  icon={Zap}
                  tone="cyan"
                  busy={controlBusy === 'simulation'}
                  onClick={() => runControl('simulation', 'simulation/control', { action: 'run', duration_ticks: 8 })}
                />
                <ControlButton
                  label="Rollout"
                  value={`until ${controlPlane?.forced_intelligence_until ?? 0}`}
                  icon={Bot}
                  tone="amber"
                  busy={controlBusy === 'rollout'}
                  onClick={() => runControl('rollout', 'intelligence/rollout', { duration_ticks: 8 })}
                />
              </div>
            </TacticalBox>

            {/* Drift Field */}
            <TacticalBox title="DRIFT FIELD" badge={`${runwayList.length} TRACKS`} className="min-h-0 flex-1">
              <div className="flex h-full flex-col gap-2.5">

                <div className="neo-inset rounded-xl px-3 py-2.5">
                  <div className="flex items-center justify-between">
                    <span className="label-xs">Topology Stability</span>
                    <Gauge className="h-3.5 w-3.5" style={{ color: '#3A3D44' }} />
                  </div>
                  <div className="mt-0.5 label-xs opacity-40">
                    critical: {equilibrium?.critical_service_id || stabilityEnvelope?.most_vulnerable_service || 'none'}
                  </div>
                </div>

                <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto scrollbar-none">
                  {signalTelemetry.slice(0, 4).map(s => (
                    <div key={s.service} className={`neo-card neo-card--${Math.abs(s.drift) > 0.15 ? 'red' : Math.abs(s.drift) > 0.05 ? 'amber' : 'cyan'} rounded-xl p-2.5`}>
                      <div className="flex items-start justify-between gap-2">
                        <span className="data-value break-all text-[9px]" style={{ color: '#A8ABB4' }}>{s.service}</span>
                        <span className="data-value text-[9px] flex-shrink-0" style={{ color: Math.abs(s.drift) > 0.15 ? '#FF453A' : Math.abs(s.drift) > 0.05 ? '#FF9F0A' : '#00D4FF' }}>
                          {fmtSigned(s.drift)}
                        </span>
                      </div>
                      <div className="mt-1.5 flex gap-3 text-[9px]" style={{ color: '#4A4D55', fontFamily: 'var(--font-data)' }}>
                        <span>↔ {s.margin.toFixed(2)}</span>
                        <span>ρ {fmt(s.utilisation, 0)}</span>
                        <span>σ {s.variance.toFixed(2)}</span>
                      </div>
                    </div>
                  ))}

                  {/* Prediction bands */}
                  <div className="neo-inset rounded-xl px-3 py-2.5">
                    <div className="flex items-center justify-between mb-2">
                      <span className="label-xs">Prediction Bands</span>
                      <Radar className="h-3.5 w-3.5" style={{ color: '#3A3D44' }} />
                    </div>
                    {predictionBands.map(b => (
                      <div key={b.service} className="flex items-start justify-between gap-2 py-1">
                        <span className="data-value break-all text-[9px]" style={{ color: '#00D4FF' }}>{b.service}</span>
                        <span className="data-value text-[9px] flex-shrink-0" style={{ color: '#4A4D55' }}>ρ {fmt(b.rho)} · Δ {fmtSigned(b.spread)}</span>
                      </div>
                    ))}
                    {predictionBands.length === 0 && <span className="label-xs opacity-30">No prediction data</span>}
                  </div>

                  {/* Runway */}
                  <div className="neo-inset rounded-xl px-3 py-2.5">
                    <span className="label-xs block mb-2">Instability Runway</span>
                    <div className="space-y-3">
                      {runwayList.map(track => (
                        <div key={track.service}>
                          <div className="flex items-start justify-between gap-2 mb-1">
                            <span className="data-value break-all text-[9px]" style={{ color: '#A8ABB4' }}>{track.service}</span>
                            <span className="data-value text-[9px] flex-shrink-0" style={{ color: '#6E7380' }}>{fmt(track.latestRisk)}</span>
                          </div>
                          <RiskBars values={track.values} />
                        </div>
                      ))}
                    </div>
                    {/* Priority watchlist */}
                    <div className="mt-3 border-t pt-3" style={{ borderColor: 'rgba(255,255,255,0.05)' }}>
                      <span className="label-xs block mb-1.5" style={{ fontSize: '8px', color: '#3A3D44' }}>Priority Queue</span>
                      <div className="flex flex-wrap gap-1">
                        {watchlist.map(item => (
                          <span key={item.service_id} className={`chip chip--${item.collapse_risk > 0.5 ? 'red' : item.collapse_risk > 0.25 ? 'amber' : 'cyan'}`} style={{ fontSize: '8px' }}>
                            {item.service_id}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </TacticalBox>
          </div>
        </motion.aside>
      </div>
    </div>
  );
}
