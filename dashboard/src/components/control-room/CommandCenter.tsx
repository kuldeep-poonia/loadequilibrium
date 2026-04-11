'use client';

import { motion } from 'framer-motion';
import {
  Activity,
  AlertTriangle,
  Bot,
  Gauge,
  Orbit,
  Play,
  Radar,
  RotateCcw,
  ShieldCheck,
  Sparkles,
  Zap,
  type LucideIcon,
} from 'lucide-react';
import { TacticalBox } from '@/components/ui/HUD';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { TopologyCanvas } from './TopologyModule';

type Tone = 'cyan' | 'amber' | 'red';

const interactionSpring = {
  type: 'spring',
  stiffness: 260,
  damping: 22,
  mass: 0.78,
} as const;

const scrubEase = [0.22, 1, 0.36, 1] as const;

function formatPercent(value?: number | null, digits = 1) {
  if (value == null || Number.isNaN(value)) return '--';
  return `${(value * 100).toFixed(digits)}%`;
}

function formatMs(value?: number | null, digits = 1) {
  if (value == null || Number.isNaN(value)) return '--';
  return `${value.toFixed(digits)}ms`;
}

function formatSigned(value?: number | null, digits = 3) {
  if (value == null || Number.isNaN(value)) return '--';
  return `${value >= 0 ? '+' : ''}${value.toFixed(digits)}`;
}

function formatClock(timestamp?: string | null) {
  if (!timestamp) return '--:--:--';
  const parsed = new Date(timestamp);
  if (Number.isNaN(parsed.getTime())) return timestamp;
  return parsed.toLocaleTimeString('en-US', { hour12: false });
}

function polylinePoints(values: number[], width: number, height: number, maxValue?: number) {
  if (!values.length) return '';
  const resolvedMax = Math.max(maxValue ?? 0, ...values, 1);

  return values
    .map((value, index) => {
      const x = values.length === 1 ? width / 2 : (index / (values.length - 1)) * width;
      const y = height - (Math.max(0, value) / resolvedMax) * height;
      return `${x},${Number.isFinite(y) ? y : height}`;
    })
    .join(' ');
}

function ActionButton({
  icon: Icon,
  label,
  detail,
  tone,
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  detail: string;
  tone: Tone;
  onClick: () => void;
}) {
  const toneClasses =
    tone === 'red'
      ? 'text-red-200 hover:text-red-100'
      : tone === 'amber'
        ? 'text-amber-200 hover:text-amber-100'
        : 'text-cyan-200 hover:text-cyan-100';

  return (
    <motion.button
      type="button"
      onClick={onClick}
      title={detail}
      aria-label={label}
      whileHover={{ y: -2, scale: 1.008 }}
      whileTap={{ y: 1.5, scale: 0.986 }}
      transition={interactionSpring}
      className={`neo-control control-rail-button control-rail-feedback neo-control--${tone} w-full min-w-0 rounded-[18px] p-2.5 text-left ${toneClasses}`}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="control-rail-glyph flex h-9 w-9 items-center justify-center rounded-[12px] border border-white/8 bg-[#10151d] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
            <Icon className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <div className="font-hud text-[8px] font-bold uppercase tracking-[0.18em] leading-4 break-words">{label}</div>
          </div>
        </div>
        <div className="control-rail-indicator h-2 w-2 rounded-full bg-current" />
      </div>
    </motion.button>
  );
}

function TrendChart({ objective, cascade, tickMs }: { objective: number[]; cascade: number[]; tickMs: number[] }) {
  const width = 360;
  const height = 92;
  const objectiveLine = polylinePoints(objective, width, height, 1);
  const cascadeLine = polylinePoints(cascade, width, height, 1);
  const tickLine = polylinePoints(tickMs, width, height, Math.max(100, ...tickMs, 1));

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="h-full w-full">
      {Array.from({ length: 4 }).map((_, index) => (
        <line
          key={`h-${index}`}
          x1="0"
          y1={(height / 3) * index}
          x2={width}
          y2={(height / 3) * index}
          stroke="rgba(148,163,184,0.12)"
          strokeWidth="1"
        />
      ))}
      <polyline points={tickLine} fill="none" stroke="rgba(245,158,11,0.95)" strokeWidth="2" strokeDasharray="5 4" />
      <polyline points={objectiveLine} fill="none" stroke="rgba(34,211,238,0.95)" strokeWidth="3" />
      <polyline points={cascadeLine} fill="none" stroke="rgba(248,113,113,0.95)" strokeWidth="2.5" />
    </svg>
  );
}

function RiskBars({ values }: { values: number[] }) {
  return (
    <div className="flex h-10 items-end gap-1">
      {values.map((value, index) => (
        <div key={index} className="flex-1">
          <div
            className={`w-full rounded-t-sm ${value > 0.7 ? 'bg-red-500' : value > 0.35 ? 'bg-amber-500' : 'bg-cyan-500/70'}`}
            style={{ height: `${Math.max(10, value * 100)}%` }}
          />
        </div>
      ))}
    </div>
  );
}

function FrameStrip({
  frames,
}: {
  frames: Array<{ seq: number; obj: number; casc: number; tickMs: number }>;
}) {
  if (!frames.length) {
    return (
      <div className="flex h-full items-center justify-center rounded-[18px] border border-dashed border-white/10 font-data text-[11px] text-slate-500">
        awaiting replay frames
      </div>
    );
  }

  const maxTick = Math.max(100, ...frames.map((frame) => frame.tickMs), 1);

  return (
    <div className="grid h-full grid-cols-1 gap-2 md:grid-cols-2 2xl:grid-cols-4">
      {frames.map((frame, index) => (
        <motion.div
          key={frame.seq}
          initial={{ opacity: 0, y: -12, height: 0 }}
          animate={{ opacity: 1, y: 0, height: 'auto' }}
          exit={{ opacity: 0, height: 0 }}
          transition={{ duration: 0.32, ease: [0.22, 1, 0.36, 1] }}
          whileHover={{ y: -2 }}
          className="industrial-inset dock-transition timeline-frame rounded-[18px] px-3 py-3 min-h-[120px]"
        >
          <div className="flex items-center justify-between gap-2">
            <span className="font-hud text-[8px] uppercase tracking-[0.22em] text-slate-500">frame {frame.seq}</span>
            <span className="font-data text-[10px] text-slate-500">{frame.tickMs.toFixed(1)}ms</span>
          </div>
          <div className="mt-3 space-y-2">
            <div>
              <div className="mb-1 flex items-center justify-between font-data text-[9px] text-slate-500">
                <span>objective</span>
                <span>{formatPercent(frame.obj)}</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-[#06090e] shadow-[inset_0_1px_2px_rgba(0,0,0,0.6)]">
                <motion.div
                  initial={false}
                  animate={{ width: `${Math.max(frame.obj * 100, 8)}%` }}
                  transition={{ duration: 0.42, ease: scrubEase, delay: index * 0.03 }}
                  className="timeline-bar h-full rounded-full bg-cyan-500"
                />
              </div>
            </div>
            <div>
              <div className="mb-1 flex items-center justify-between font-data text-[9px] text-slate-500">
                <span>cascade</span>
                <span>{formatPercent(frame.casc)}</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-[#06090e] shadow-[inset_0_1px_2px_rgba(0,0,0,0.6)]">
                <motion.div
                  initial={false}
                  animate={{ width: `${Math.max(frame.casc * 100, 8)}%` }}
                  transition={{ duration: 0.42, ease: scrubEase, delay: index * 0.03 + 0.04 }}
                  className="timeline-bar h-full rounded-full bg-red-500"
                />
              </div>
            </div>
            <div>
              <div className="mb-1 flex items-center justify-between font-data text-[9px] text-slate-500">
                <span>dt</span>
                <span>{frame.tickMs.toFixed(1)}ms</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-[#06090e] shadow-[inset_0_1px_2px_rgba(0,0,0,0.6)]">
                <motion.div
                  initial={false}
                  animate={{ width: `${Math.max((frame.tickMs / maxTick) * 100, 8)}%` }}
                  transition={{ duration: 0.42, ease: scrubEase, delay: index * 0.03 + 0.08 }}
                  className="timeline-bar h-full rounded-full bg-amber-500"
                />
              </div>
            </div>
          </div>
        </motion.div>
      ))}
    </div>
  );
}

export default function CommandCenter() {
  const { tick, history, connected, triggerAction, triggerDomain } = useTelemetryStore();
  const bundles = tick?.bundles || {};
  const runtime = tick?.runtime_metrics;
  const stageMetrics = runtime
    ? [
        { label: 'PRUNE', value: runtime.avg_prune_ms },
        { label: 'WINDOWS', value: runtime.avg_windows_ms },
        { label: 'TOPOLOGY', value: runtime.avg_topology_ms },
        { label: 'COUPLING', value: runtime.avg_coupling_ms },
        { label: 'MODELS', value: runtime.avg_modelling_ms },
        { label: 'OPTIMISE', value: runtime.avg_optimise_ms },
        { label: 'SIM', value: runtime.avg_sim_ms },
        { label: 'REASON', value: runtime.avg_reasoning_ms },
        { label: 'BCAST', value: runtime.avg_broadcast_ms },
      ]
    : [];
  const totalRuntimeMs = stageMetrics.reduce((total, stage) => total + (stage.value || 0), 0);
  const objective = tick?.objective;
  const equilibrium = tick?.network_equilibrium;
  const criticalPath = tick?.topology?.critical_path;
  const fixedPoint = tick?.fixed_point_equilibrium;
  const stabilityEnvelope = tick?.stability_envelope;
  const historyWindow = history.slice(-24);
  const objectiveTrend = historyWindow.map((sample) => sample.obj);
  const cascadeTrend = historyWindow.map((sample) => sample.casc);
  const tickTrend = historyWindow.map((sample) => sample.tickMs);
  const replayFrames = historyWindow.slice(-8);
  const directives = Object.entries(tick?.directives || {}).map(([service, directive]) => ({ service, ...directive })).slice(0, 5);
  const watchlist = (tick?.priority_risk_queue || []).slice(0, 5);
  const sensitivity = tick?.topology_sensitivity;
  const runwayList = Object.entries(tick?.risk_timeline || {})
    .map(([service, points]) => ({
      service,
      latestRisk: points[points.length - 1]?.risk || 0,
      values: points.slice(-12).map((point) => point.risk || 0),
    }))
    .sort((left, right) => right.latestRisk - left.latestRisk)
    .slice(0, 4);
  const events = [...(tick?.events || [])].slice(-10).reverse();
  const signalTelemetry = Object.entries(bundles)
    .map(([service, bundle]) => {
      const signal = bundle.signal;
      const queue = bundle.queue;
      const stability = bundle.stability;
      const confidence = bundle.stochastic?.confidence ?? queue?.confidence ?? 0;
      const cusumMagnitude = Math.max(signal?.cusum_pos ?? 0, Math.abs(signal?.cusum_neg ?? 0));
      const fluxResidual = queue?.service_rate
        ? Math.abs((queue.arrival_rate || 0) - queue.service_rate) / Math.max(queue.service_rate, 1)
        : 0;

      return {
        service,
        confidence,
        cusumMagnitude,
        variance: signal?.ewma_variance ?? 0,
        spike: signal?.spike_detected ?? false,
        drift: stability?.stability_derivative ?? 0,
        margin: stability?.stability_margin ?? 0,
        fluxResidual,
        utilisation: queue?.utilisation ?? 0,
      };
    })
    .sort((left, right) => (left.confidence - right.confidence) || (right.cusumMagnitude - left.cusumMagnitude))
    .slice(0, 6);
  const conservationRows = Object.entries(bundles)
    .map(([service, bundle]) => {
      const queue = bundle.queue;
      const arrival = queue?.arrival_rate ?? 0;
      const serviceRate = queue?.service_rate ?? 0;
      const residual = serviceRate > 0 ? Math.abs(arrival - serviceRate) / Math.max(serviceRate, 1) : 0;

      return {
        service,
        arrival,
        serviceRate,
        residual,
      };
    })
    .sort((left, right) => right.residual - left.residual)
    .slice(0, 4);
  const meanSignalConfidence = signalTelemetry.length
    ? signalTelemetry.reduce((total, sample) => total + sample.confidence, 0) / signalTelemetry.length
    : 0;
  const meanFluxResidual = conservationRows.length
    ? conservationRows.reduce((total, sample) => total + sample.residual, 0) / conservationRows.length
    : 0;
  const convergenceResidual = Math.abs(equilibrium?.equilibrium_delta ?? 0);
  const driftProxy = Math.abs(stabilityEnvelope?.worst_perturbation_delta ?? 0);
  const predictionBands = Object.entries(tick?.prediction_timeline || {})
    .map(([service, points]) => {
      const latest = points[points.length - 1];
      return {
        service,
        rho: latest?.rho ?? 0,
        spread: Math.max((latest?.hi ?? 0) - (latest?.lo ?? 0), 0),
        horizon: latest?.t ?? 0,
      };
    })
    .sort((left, right) => right.spread - left.spread)
    .slice(0, 4);
  const headerStatus = !connected ? 'critical' : tick?.safety_mode ? 'critical' : runtime?.predicted_overrun ? 'warning' : 'nominal';
  const simulationActive = connected && !tick?.safety_mode;

  const runDomain = async (domain: string, payload?: unknown, label?: string) => {
    const result = await triggerDomain(domain, payload);
    if (!result.ok) {
      console.error(`[command-center] ${(label || domain).toLowerCase()} failed:`, result.error);
    }
  };

  return (
    <div className="cockpit-surface flex min-h-full flex-col gap-2.5 overflow-x-hidden text-slate-200">
      <motion.section initial={{ opacity: 0, y: -10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.26 }} className="shrink-0">
        <TacticalBox title="NUMERICAL OBSERVATORY HEADER" badge={connected ? 'STREAMING' : 'OFFLINE'} status={headerStatus} scan={false}>
          <div className="flex min-h-0 min-w-0 flex-col gap-3">
            <div className="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.55fr)]">
              <div className="min-w-0 space-y-2">
                <div className="min-w-0">
                  <div className="font-hud text-[7px] uppercase tracking-[0.22em] text-slate-500">
                    simulation laboratory software
                  </div>
                   <h1 className="mt-0.5 break-words font-hud text-[0.82rem] font-semibold uppercase tracking-[0.08em] text-cyan-200 xl:text-[0.92rem] drop-shadow-[0_0_12px_rgba(34,211,238,0.55)]">
                     LOADEQUILIBRIUM
                   </h1>
                  <p className="mt-1 max-w-2xl text-pretty break-words font-data text-[9px] leading-4 text-slate-400">
                    Convergence, conservation, signal integrity, and topology stability are surfaced as live numerical readouts over the existing control-room bindings.
                  </p>
                </div>

                <div className="flex flex-wrap gap-1.5">
                  <span className={`industrial-chip rounded-full px-2.5 py-1 font-hud text-[7px] uppercase tracking-[0.18em] ${connected ? 'industrial-chip--cyan text-cyan-200' : 'industrial-chip--red text-red-200'}`}>
                    {connected ? 'telemetry stream locked' : 'stream disconnected'}
                  </span>
                  <span className={`industrial-chip rounded-full px-2.5 py-1 font-hud text-[7px] uppercase tracking-[0.18em] ${tick?.safety_mode ? 'industrial-chip--red text-red-200' : 'industrial-chip--cyan text-cyan-200'} ${simulationActive ? 'active-sim-glow' : ''}`}>
                    {tick?.safety_mode ? 'conservation guard active' : 'solver free-running'}
                  </span>
                  <span className={`industrial-chip rounded-full px-2.5 py-1 font-hud text-[7px] uppercase tracking-[0.18em] ${runtime?.predicted_overrun ? 'industrial-chip--amber text-amber-200' : 'industrial-chip--cyan text-cyan-200'}`}>
                    {runtime?.predicted_overrun ? 'timestep overrun watch' : 'integrator cadence stable'}
                  </span>
                </div>
              </div>

              <div className="grid min-w-0 grid-cols-2 gap-2 lg:grid-cols-4">
                <div className="neo-telemetry-card neo-telemetry-card--cyan rounded-[18px] px-3.5 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.22em] text-slate-500">Integration timestep</div>
                  <div className="mt-2 font-data text-base text-cyan-200">{formatMs(tick?.tick_health_ms)}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">frame age {tick?.sim_overlay?.sim_tick_age ?? 0}</div>
                </div>
                <div className="neo-telemetry-card rounded-[18px] px-3.5 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.22em] text-slate-500">Convergence residual</div>
                  <div className="mt-2 font-data text-base text-slate-100">{formatSigned(convergenceResidual)}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">spectral rate {fixedPoint?.convergence_rate?.toFixed(3) ?? '--'}</div>
                </div>
                <div className="neo-telemetry-card neo-telemetry-card--amber rounded-[18px] px-3.5 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.22em] text-slate-500">Conservation status</div>
                  <div className="mt-2 font-data text-base text-amber-200">{formatPercent(1 - Math.min(meanFluxResidual, 1))}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">flux residual {formatSigned(meanFluxResidual)}</div>
                </div>
                <div className="neo-telemetry-card neo-telemetry-card--red rounded-[18px] px-3.5 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.22em] text-slate-500">Signal integrity</div>
                  <div className="mt-2 font-data text-base text-red-200">{formatPercent(meanSignalConfidence)}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">jitter {formatMs(tick?.jitter_ms)}</div>
                </div>
              </div>
            </div>

            <div className="grid min-w-0 gap-1.5 grid-cols-2 md:grid-cols-3 lg:grid-cols-5 2xl:grid-cols-9">
              {stageMetrics.length === 0 ? (
                <div className="col-span-full rounded-2xl border border-dashed border-white/10 px-4 py-4 text-center font-data text-[11px] text-slate-500">
                  awaiting runtime stage telemetry
                </div>
              ) : (
                stageMetrics.map((stage, index) => (
                  <motion.div layout key={stage.label} className="industrial-inset rounded-[16px] px-2.5 py-2">
                    <div className="flex items-center justify-between gap-2">
                      <span className="break-words font-hud text-[7px] uppercase tracking-[0.16em] text-slate-500">{stage.label}</span>
                      <span className="font-data text-[10px] text-slate-300">{formatMs(stage.value)}</span>
                    </div>
                    <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-[#06090e] shadow-[inset_0_1px_2px_rgba(0,0,0,0.6)]">
                      <motion.div
                        initial={false}
                        animate={{ width: `${Math.max(totalRuntimeMs > 0 ? (stage.value / totalRuntimeMs) * 100 : 0, 8)}%` }}
                        transition={{ duration: 0.42, ease: scrubEase, delay: index * 0.02 }}
                        className={`h-full rounded-full ${stage.value > 20 ? 'bg-amber-500' : 'bg-cyan-500'}`}
                      />
                    </div>
                  </motion.div>
                ))
              )}
            </div>
          </div>
        </TacticalBox>
      </motion.section>

      <div className="grid min-h-0 flex-1 gap-2.5 xl:grid-cols-[200px_minmax(0,1fr)_300px] 2xl:grid-cols-[216px_minmax(0,1fr)_320px]">
        <motion.aside initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.28, delay: 0.04 }} className="min-h-0 min-w-0">
          <div className="flex h-full min-h-0 flex-col gap-3">
            <TacticalBox title="SIM CONTROL" badge={tick?.safety_mode ? 'HOLD' : 'HOT'} status={tick?.safety_mode ? 'critical' : 'nominal'} scan={false}>
              <div className="flex min-h-0 min-w-0 flex-col gap-3">
                <div className="grid min-w-0 gap-1.5">
                  <ActionButton icon={Play} label="Start" detail="simulation/control:start" tone="cyan" onClick={() => { void runDomain('simulation/control', { action: 'start' }, 'simulation start'); }} />
                  <ActionButton icon={RotateCcw} label="Reset" detail="simulation/control:reset" tone="amber" onClick={() => { void runDomain('simulation/control', { action: 'reset' }, 'simulation reset'); }} />
                  <ActionButton icon={Activity} label="Replay" detail="control/replay-burst" tone="amber" onClick={() => { void triggerAction('replay-burst'); }} />
                  <ActionButton icon={AlertTriangle} label="Chaos" detail="control/chaos-run" tone="red" onClick={() => { void triggerAction('chaos-run'); }} />
                  <ActionButton icon={Bot} label="Rollout" detail="intelligence/rollout" tone="amber" onClick={() => { void runDomain('intelligence/rollout', undefined, 'rl rollout'); }} />
                  <ActionButton icon={Zap} label="Toggle" detail="control/toggle" tone="cyan" onClick={() => { void triggerAction('toggle'); }} />
                </div>

                <div className="industrial-inset rounded-[16px] p-2.5">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">policy presets</div>
                      <div className="mt-1 break-words font-data text-[10px] leading-4 text-slate-500">existing policy/update backend route</div>
                    </div>
                    <ShieldCheck className="h-4 w-4 shrink-0 text-cyan-300/80" />
                  </div>
                  <div className="mt-2.5 grid grid-cols-1 gap-1.5">
                    {[
                      { value: 'aggressive', label: 'AGGR' },
                      { value: 'balanced', label: 'BAL' },
                      { value: 'conservative', label: 'CONS' },
                      { value: 'defensive', label: 'DEF' },
                    ].map((preset) => (
                      <motion.button
                        key={preset.value}
                        type="button"
                        onClick={() => {
                          void runDomain('policy/update', { preset: preset.value }, `policy ${preset.value}`);
                        }}
                        title={preset.value}
                        whileHover={{ y: -1.5, scale: 1.01 }}
                        whileTap={{ y: 1, scale: 0.985 }}
                        transition={interactionSpring}
                        className="neo-control control-rail-button control-rail-feedback neo-control--cyan min-w-0 rounded-[12px] px-2 py-2 font-hud text-[7px] uppercase tracking-[0.16em] text-slate-300 hover:text-cyan-200"
                      >
                        <span className="block break-words">{preset.label}</span>
                      </motion.button>
                    ))}
                  </div>
                </div>
              </div>
            </TacticalBox>

            <TacticalBox title="SIGNAL / CONSERVATION" badge={`${signalTelemetry.length} CH`} className="min-h-0 flex-1">
              <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
                <div className="grid min-w-0 grid-cols-1 gap-2">
                  <div className="neo-telemetry-card neo-telemetry-card--cyan rounded-[18px] px-3 py-3">
                    <div className="break-words font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">Signal integrity</div>
                    <div className="mt-2 font-data text-base text-slate-100">{formatPercent(meanSignalConfidence)}</div>
                    <div className="mt-1 font-data text-[10px] text-slate-500">weakest channels below</div>
                  </div>
                  <div className="neo-telemetry-card neo-telemetry-card--amber rounded-[18px] px-3 py-3">
                    <div className="break-words font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">Flux balance</div>
                    <div className="mt-2 font-data text-base text-slate-100">{formatSigned(meanFluxResidual)}</div>
                    <div className="mt-1 font-data text-[10px] text-slate-500">arrival vs service residual</div>
                  </div>
                </div>

                <div className="grid min-w-0 gap-2">
                  {signalTelemetry.slice(0, 2).map((sample) => (
                    <div key={sample.service} className={`neo-telemetry-card min-w-0 rounded-[18px] p-3 ${sample.spike || sample.cusumMagnitude > 1 ? 'neo-telemetry-card--red' : 'neo-telemetry-card--cyan'}`}>
                      <div className="flex items-start justify-between gap-3">
                        <span className="min-w-0 break-all font-hud text-[8px] uppercase tracking-[0.2em] text-slate-100">{sample.service}</span>
                        <span className="shrink-0 font-data text-[10px] text-slate-500">{formatPercent(sample.confidence)}</span>
                      </div>
                      <div className="mt-2 grid grid-cols-3 gap-2 font-data text-[10px] text-slate-500">
                        <span>var {sample.variance.toFixed(3)}</span>
                        <span>cusum {sample.cusumMagnitude.toFixed(2)}</span>
                        <span>{sample.spike ? 'spike' : 'smooth'}</span>
                      </div>
                    </div>
                  ))}
                </div>

                <div className="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1 scrollbar-hud">
                  {conservationRows.length === 0 ? (
                    <div className="rounded-2xl border border-dashed border-white/10 px-4 py-4 text-center font-data text-[11px] text-slate-500">
                      no conservation diagnostics available
                    </div>
                  ) : (
                    conservationRows.map((sample) => (
                      <div key={sample.service} className={`neo-telemetry-card min-w-0 rounded-[18px] p-3 ${sample.residual > 0.2 ? 'neo-telemetry-card--red' : sample.residual > 0.08 ? 'neo-telemetry-card--amber' : 'neo-telemetry-card--cyan'}`}>
                        <div className="flex items-start justify-between gap-3">
                          <span className="min-w-0 break-all font-hud text-[8px] uppercase tracking-[0.2em] text-slate-100">{sample.service}</span>
                          <span className={`shrink-0 font-data text-[10px] ${sample.residual > 0.2 ? 'text-red-200' : sample.residual > 0.08 ? 'text-amber-200' : 'text-cyan-200'}`}>{formatSigned(sample.residual)}</span>
                        </div>
                        <div className="mt-2 grid grid-cols-2 gap-2 font-data text-[10px] text-slate-500">
                          <span>in {sample.arrival.toFixed(1)}</span>
                          <span>out {sample.serviceRate.toFixed(1)}</span>
                        </div>
                        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-[#06090e] shadow-[inset_0_1px_2px_rgba(0,0,0,0.6)]">
                          <div className={`h-full rounded-full ${sample.residual > 0.2 ? 'bg-red-500' : sample.residual > 0.08 ? 'bg-amber-500' : 'bg-cyan-500'}`} style={{ width: `${Math.min(Math.max(sample.residual * 100, 8), 100)}%` }} />
                        </div>
                      </div>
                    ))
                  )}
                </div>

                <div className="industrial-inset rounded-[18px] px-3 py-3">
                  <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">actuation field</div>
                  <div className="mt-2 space-y-2">
                    {directives.slice(0, 3).map((directive) => (
                      <div key={directive.service} className="flex items-start justify-between gap-3 font-data text-[10px] text-slate-400">
                        <span className="min-w-0 break-all font-hud text-[8px] uppercase tracking-[0.18em] text-cyan-200">{directive.service}</span>
                        <span className="shrink-0">R {directive.target_replicas} | Q {directive.queue_limit}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </TacticalBox>
          </div>
        </motion.aside>

        <div className="grid min-h-0 min-w-0 gap-3 xl:grid-rows-[minmax(0,1fr)_680px]">
          <motion.section initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.3, delay: 0.08 }} className="min-h-[420px] min-w-0 xl:min-h-0">
            <TacticalBox
              title="TOPOLOGY FIELD"
              badge={tick?.topology?.nodes?.length ? `${tick.topology.nodes.length}N/${tick.topology.edges.length}E` : 'WARMING'}
              status={(criticalPath?.cascade_risk || 0) > 0.45 ? 'critical' : 'nominal'}
              className="h-full"
              scan={false}
            >
              <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
                <div className="grid min-w-0 gap-3 lg:grid-cols-[minmax(0,1fr)_220px]">
                  <div className="flex min-w-0 flex-wrap gap-2">
                    <span className="industrial-chip industrial-chip--cyan rounded-full px-3 py-1 font-hud text-[8px] uppercase tracking-[0.2em] text-cyan-200">
                      dt {formatMs(tick?.tick_health_ms)}
                    </span>
                    <span className="industrial-chip industrial-chip--amber rounded-full px-3 py-1 font-hud text-[8px] uppercase tracking-[0.2em] text-amber-200">
                      residual {formatSigned(convergenceResidual)}
                    </span>
                    <span className="industrial-chip industrial-chip--red rounded-full px-3 py-1 font-hud text-[8px] uppercase tracking-[0.2em] text-red-200">
                      drift {formatSigned(driftProxy)}
                    </span>
                    <span className="industrial-chip industrial-chip--cyan rounded-full px-3 py-1 font-hud text-[8px] uppercase tracking-[0.2em] text-cyan-200">
                      objective {formatPercent(objective?.composite_score)}
                    </span>
                  </div>

                  <div className={`industrial-inset rounded-[18px] px-3 py-3 text-right ${simulationActive ? 'active-sim-glow' : ''}`}>
                    <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">solver frame</div>
                    <div className="mt-1 font-data text-sm text-slate-100">SEQ {tick?.seq ?? 0}</div>
                    <div className="mt-1 font-data text-[10px] text-slate-500">frame age {tick?.sim_overlay?.sim_tick_age ?? 0}</div>
                  </div>
                </div>

                <div className={`topology-zone cockpit-focus-well relative flex min-h-[260px] min-w-0 flex-1 overflow-hidden rounded-[24px] p-3 ${simulationActive ? 'topology-zone-active' : ''}`}>
                  <div className="cockpit-grid-panel absolute inset-0 opacity-60" />
                  <div className="absolute inset-0 bg-[linear-gradient(180deg,_rgba(255,255,255,0.025),_transparent_18%,_transparent_84%,_rgba(0,0,0,0.22))]" />
                  <div className="relative z-10 flex min-h-0 min-w-0 flex-1 overflow-hidden rounded-[18px] border border-white/6 bg-[#04070d]">
                    <TopologyCanvas />
                  </div>
                </div>

                <div className="grid min-w-0 gap-3 lg:grid-cols-[minmax(0,1.2fr)_220px] 2xl:grid-cols-[minmax(0,1.25fr)_236px]">
                  <div className="industrial-inset timeline-scrub cockpit-trace-grid min-w-0 rounded-[18px] p-3">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">numerical progression timeline</div>
                        <div className="mt-1 break-words font-data text-[10px] leading-4 text-slate-500">objective, instability, and timestep evolution across recent solver frames</div>
                      </div>
                      <div className="font-data text-[10px] text-slate-500">{equilibrium?.is_converging ? 'residuals decreasing' : 'residuals drifting'}</div>
                    </div>
                    <div className="mt-3 h-24 min-w-0">
                      <TrendChart objective={objectiveTrend} cascade={cascadeTrend} tickMs={tickTrend} />
                    </div>
                  </div>

                  <div className="industrial-inset min-w-0 rounded-[18px] p-3">
                    <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">topology stability metrics</div>
                    <div className="mt-3 grid grid-cols-2 gap-3">
                      <div className="min-w-0">
                        <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-600">fragility</div>
                        <div className="mt-1 font-data text-sm text-slate-100">{formatPercent(sensitivity?.system_fragility)}</div>
                      </div>
                      <div className="min-w-0">
                        <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-600">rho variance</div>
                        <div className="mt-1 font-data text-sm text-slate-100">{formatSigned(equilibrium?.system_rho_variance ?? 0)}</div>
                      </div>
                      <div className="min-w-0">
                        <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-600">critical risk</div>
                        <div className="mt-1 font-data text-sm text-slate-100">{formatPercent(criticalPath?.cascade_risk)}</div>
                      </div>
                      <div className="min-w-0">
                        <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-600">equilibrium rho</div>
                        <div className="mt-1 font-data text-sm text-slate-100">{formatPercent(equilibrium?.system_rho_mean, 0)}</div>
                      </div>
                    </div>
                    {criticalPath?.nodes?.length ? (
                      <div className="mt-3 flex flex-wrap gap-2">
                        {criticalPath.nodes.map((node) => (
                          <span key={node} className="industrial-chip industrial-chip--red break-all rounded-full px-3 py-1 font-hud text-[8px] uppercase tracking-[0.18em] text-red-200">
                            {node}
                          </span>
                        ))}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            </TacticalBox>
          </motion.section>

          <motion.section initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.34, delay: 0.12 }} className="min-h-0 min-w-0">
            <TacticalBox title="REPLAY DOCK" badge={`${events.length} EVENTS`} className="h-full">
              <div className="grid h-full min-h-0 min-w-0 gap-3 xl:grid-rows-[auto_minmax(0,1fr)]">
                <div className="industrial-inset min-w-0 rounded-[18px] p-3">
                  <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">replay controls</div>
                  <div className="mt-3 grid grid-cols-2 gap-2 lg:grid-cols-4">
                    <div className="neo-telemetry-card rounded-[14px] px-3 py-2.5">
                      <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-500">Frame</div>
                      <div className="mt-1.5 font-data text-sm text-slate-100">{tick?.seq ?? 0}</div>
                    </div>
                    <div className="neo-telemetry-card neo-telemetry-card--cyan rounded-[14px] px-3 py-2.5">
                      <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-500">Timestep</div>
                      <div className="mt-1.5 font-data text-sm text-slate-100">{formatMs(tick?.tick_health_ms, 1)}</div>
                    </div>
                    <div className="neo-telemetry-card neo-telemetry-card--amber rounded-[14px] px-3 py-2.5">
                      <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-500">Residual</div>
                      <div className="mt-1.5 font-data text-sm text-slate-100">{formatSigned(convergenceResidual, 2)}</div>
                    </div>
                    <div className="neo-telemetry-card neo-telemetry-card--red rounded-[14px] px-3 py-2.5">
                      <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-500">Frame age</div>
                      <div className="mt-1.5 font-data text-sm text-slate-100">{tick?.sim_overlay?.sim_tick_age ?? 0}</div>
                    </div>
                  </div>

                  <motion.button
                    type="button"
                    onClick={() => {
                      void triggerAction('replay-burst');
                    }}
                    whileHover={{ y: -2, scale: 1.008 }}
                    whileTap={{ y: 1.5, scale: 0.986 }}
                    transition={interactionSpring}
                    className={`neo-control control-rail-button control-rail-feedback neo-control--cyan mt-3 flex w-full min-w-0 items-center justify-center gap-3 rounded-[16px] px-3 py-3 font-hud text-[8px] uppercase tracking-[0.2em] text-cyan-200 ${simulationActive ? 'active-sim-glow' : ''}`}
                  >
                    <Sparkles className="h-4 w-4 shrink-0" />
                    <span className="break-words">replay burst</span>
                  </motion.button>
                </div>

                <div className="grid h-full min-h-0 min-w-0 gap-3 lg:grid-cols-[minmax(0,1fr)_260px] xl:grid-cols-[minmax(0,1fr)_280px]">
                <div className="industrial-inset timeline-scrub cockpit-trace-grid flex min-h-0 min-w-0 flex-col rounded-[18px] p-3">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">frame evolution</div>
                      <div className="mt-1 break-words font-data text-[10px] leading-4 text-slate-500">per-frame numerical progression across the latest solver history</div>
                    </div>
                    <div className="shrink-0 font-data text-[10px] text-slate-500">{formatClock(tick?.ts)}</div>
                  </div>
                  <div className="mt-3 min-h-0 min-w-0 flex-1 overflow-y-auto scrollbar-hud">
                    <FrameStrip frames={replayFrames} />
                  </div>
                </div>

                <div className="industrial-inset flex min-h-0 min-w-0 flex-col rounded-[18px] p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">numerical event timeline</div>
                      <div className="mt-1 break-words font-data text-[10px] leading-4 text-slate-500">latest runtime and simulation markers</div>
                    </div>
                    <Orbit className="h-4 w-4 shrink-0 text-cyan-300/80" />
                  </div>

                  <div className="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto pr-1 scrollbar-hud">
                    {events.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 px-4 py-4 text-center font-data text-[11px] text-slate-500">
                        no event timeline entries yet
                      </div>
                    ) : (
                      events.map((event, index) => (
                        <motion.div
                          layout
                          key={`${event.timestamp || 'evt'}-${index}`}
                          whileHover={{ x: 2 }}
                          transition={interactionSpring}
                          className="timeline-event min-w-0 rounded-[16px] border border-white/8 bg-[#141a22] p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="break-words font-hud text-[8px] uppercase tracking-[0.18em] text-slate-100">{event.category}</div>
                              <div className="mt-1.5 break-words font-data text-[10px] leading-4 text-slate-400">{event.description}</div>
                            </div>
                            <div className="shrink-0 text-right">
                              <div className={`font-hud text-[7px] uppercase tracking-[0.18em] ${event.severity === 'critical' ? 'text-red-200' : event.severity === 'warning' ? 'text-amber-200' : 'text-cyan-200'}`}>
                                {event.severity}
                              </div>
                              <div className="mt-1 font-data text-[10px] text-slate-500">{formatClock(event.timestamp)}</div>
                            </div>
                          </div>
                          {event.service_id ? (
                            <div className="mt-2 border-t border-white/5 pt-2 font-data text-[10px] text-slate-500">
                              service <span className="break-all">{event.service_id}</span>
                            </div>
                          ) : null}
                        </motion.div>
                      ))
                    )}
                  </div>
                </div>
                </div>
              </div>
            </TacticalBox>
          </motion.section>
        </div>

        <motion.aside initial={{ opacity: 0, x: 10 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.3, delay: 0.1 }} className="min-h-0 min-w-0">
          <div className="flex h-full min-h-0 flex-col gap-3">
            <TacticalBox
              title="CONVERGENCE"
              badge={tick?.fixed_point_equilibrium?.converged ? 'CONVERGED' : 'ITERATING'}
              status={tick?.fixed_point_equilibrium?.converged ? 'nominal' : 'warning'}
              scan={false}
            >
              <div className="grid min-w-0 gap-2 sm:grid-cols-2">
                <div className="neo-telemetry-card neo-telemetry-card--cyan rounded-[18px] px-3 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">Converged state</div>
                  <div className="mt-2 font-data text-base text-cyan-200">{tick?.fixed_point_equilibrium?.converged ? 'YES' : 'NO'}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">rate {tick?.fixed_point_equilibrium?.convergence_rate?.toFixed(3) ?? '--'}</div>
                </div>
                <div className="neo-telemetry-card neo-telemetry-card--red rounded-[18px] px-3 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">Residual error</div>
                  <div className="mt-2 font-data text-base text-red-200">{formatSigned(convergenceResidual)}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">{tick?.fixed_point_equilibrium?.converged_iterations ?? 0} iterations</div>
                </div>
                <div className="neo-telemetry-card neo-telemetry-card--amber rounded-[18px] px-3 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">Drift readability</div>
                  <div className="mt-2 font-data text-base text-amber-200">{formatSigned(driftProxy)}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">worst perturbation delta</div>
                </div>
                <div className="neo-telemetry-card rounded-[18px] px-3 py-3">
                  <div className="break-words font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">Conservation envelope</div>
                  <div className="mt-2 font-data text-base text-slate-100">{formatPercent(stabilityEnvelope?.envelope_headroom)}</div>
                  <div className="mt-1 font-data text-[10px] text-slate-500">safe rho {formatPercent(stabilityEnvelope?.safe_system_rho_max, 0)}</div>
                </div>
              </div>
            </TacticalBox>

            <TacticalBox title="DRIFT FIELD" badge={`${runwayList.length} TRACKS`} className="min-h-0 flex-1">
              <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
                <div className="industrial-inset rounded-[18px] px-3 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">topology stability field</div>
                      <div className="mt-1 break-words font-data text-[10px] leading-4 text-slate-500">
                        critical {equilibrium?.critical_service_id || stabilityEnvelope?.most_vulnerable_service || 'none'}
                      </div>
                    </div>
                    <Gauge className="h-4 w-4 shrink-0 text-cyan-300/80" />
                  </div>
                </div>

                <div className="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1 scrollbar-hud">
                  {signalTelemetry.length === 0 ? (
                    <div className="rounded-2xl border border-dashed border-white/10 px-4 py-4 text-center font-data text-[11px] text-slate-500">
                      no drift diagnostics available yet
                    </div>
                  ) : (
                    signalTelemetry.slice(0, 4).map((sample) => (
                      <div key={sample.service} className={`neo-telemetry-card min-w-0 rounded-[18px] p-3 ${Math.abs(sample.drift) > 0.15 ? 'neo-telemetry-card--red' : Math.abs(sample.drift) > 0.05 ? 'neo-telemetry-card--amber' : 'neo-telemetry-card--cyan'}`}>
                        <div className="flex items-start justify-between gap-3">
                          <span className="min-w-0 break-all font-hud text-[8px] uppercase tracking-[0.18em] text-slate-100">{sample.service}</span>
                          <span className={`shrink-0 font-data text-[10px] ${Math.abs(sample.drift) > 0.15 ? 'text-red-200' : Math.abs(sample.drift) > 0.05 ? 'text-amber-200' : 'text-cyan-200'}`}>
                            {formatSigned(sample.drift)}
                          </span>
                        </div>
                        <div className="mt-2 grid grid-cols-3 gap-2 font-data text-[10px] text-slate-500">
                          <span>margin {sample.margin.toFixed(2)}</span>
                          <span>rho {formatPercent(sample.utilisation, 0)}</span>
                          <span>var {sample.variance.toFixed(2)}</span>
                        </div>
                      </div>
                    ))
                  )}

                  <div className="industrial-inset min-w-0 rounded-[18px] px-3 py-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">prediction band spread</div>
                        <div className="mt-1 break-words font-data text-[10px] leading-4 text-slate-500">horizon uncertainty from existing prediction timeline</div>
                      </div>
                      <Radar className="h-4 w-4 shrink-0 text-cyan-300/80" />
                    </div>
                    <div className="mt-3 space-y-2">
                      {predictionBands.map((band) => (
                        <div key={band.service} className="flex items-start justify-between gap-3 font-data text-[10px] text-slate-400">
                          <span className="min-w-0 break-all font-hud text-[8px] uppercase tracking-[0.18em] text-cyan-200">{band.service}</span>
                          <span className="shrink-0">rho {formatPercent(band.rho)} | spread {formatSigned(band.spread)}</span>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="industrial-inset min-w-0 rounded-[18px] px-3 py-3">
                    <div className="font-hud text-[8px] uppercase tracking-[0.2em] text-slate-500">instability runway</div>
                    <div className="mt-3 space-y-3">
                      {runwayList.map((track) => (
                        <div key={track.service} className="min-w-0">
                          <div className="mb-2 flex items-start justify-between gap-3 font-data text-[10px] text-slate-400">
                            <span className="min-w-0 break-all font-hud text-[8px] uppercase tracking-[0.18em] text-slate-100">{track.service}</span>
                            <span className="shrink-0">{formatPercent(track.latestRisk)}</span>
                          </div>
                          <RiskBars values={track.values} />
                        </div>
                      ))}
                    </div>
                    <div className="mt-4 border-t border-white/6 pt-3">
                      <div className="font-hud text-[7px] uppercase tracking-[0.18em] text-slate-500">priority stability queue</div>
                      <div className="mt-2 flex flex-wrap gap-2">
                        {watchlist.slice(0, 5).map((item) => (
                          <span
                            key={item.service_id}
                            className={`industrial-chip break-all rounded-full px-3 py-1 font-hud text-[8px] uppercase tracking-[0.18em] ${
                              item.collapse_risk > 0.5
                                ? 'industrial-chip--red text-red-200'
                                : item.collapse_risk > 0.25
                                  ? 'industrial-chip--amber text-amber-200'
                                  : 'industrial-chip--cyan text-cyan-200'
                            }`}
                          >
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
