import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useStore } from '../store/useStore'
import { serviceName, serviceDesc, isTestService } from '../lib/names'
import { safe, n, pct, rho as fmtRho, ms } from '../lib/fmt'
import {
  setActuation, setPolicy, stepOnce,
  runStressTest, replayBurst, triggerSandbox,
  controlSimulation, triggerRollout
} from '../lib/api'

// ── Generic action button with loading + confirm ──────────────────
function ActionButton({ label, description, icon, danger, onClick, className }) {
  const [loading, setLoading] = useState(false)
  const [done, setDone]       = useState(false)

  const handleClick = async () => {
    if (loading || done) return
    setLoading(true)
    try {
      await onClick()
      setDone(true)
      setTimeout(() => setDone(false), 2500)
    } catch(e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  return (
    <button
      onClick={handleClick}
      disabled={loading}
      title={description}
      className={`flex items-center gap-2 px-4 py-2.5 rounded border font-cond text-[11px] font-bold tracking-wider transition-all duration-150 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed
        ${done    ? 'text-green border-green bg-green/10' :
          danger  ? 'text-red border-red/40 bg-red/5 hover:bg-red/15 hover:border-red' :
                    'text-text border-border bg-surface2 hover:border-borderhi hover:text-bright'}
        ${className || ''}`}
    >
      {loading ? (
        <motion.span animate={{ rotate:360 }} transition={{ duration:0.7, repeat:Infinity, ease:'linear' }}
          className="text-[12px]">↻</motion.span>
      ) : done ? '✓' : icon}
      {loading ? 'Working…' : done ? 'Done' : label}
    </button>
  )
}

// ── Confirm-before-act wrapper ────────────────────────────────────
function ConfirmAction({ label, description, icon, danger, confirmText, onConfirm }) {
  const [phase, setPhase] = useState('idle') // idle | confirm | loading | done
  const addToast = useStore(s => s.addToast)
  const logHuman = useStore(s => s.logHumanAction)

  const handleConfirm = async () => {
    setPhase('loading')
    try {
      const result = await onConfirm()
      setPhase('done')
      addToast('success', label, description)
      logHuman(`Operator: ${label}`, null, 'human')
      setTimeout(() => setPhase('idle'), 3000)
    } catch(err) {
      setPhase('idle')
      addToast('crit', 'Action Failed', err.message || 'Backend returned an error')
    }
  }

  return (
    <div className="relative">
      {phase === 'idle' && (
        <button
          onClick={() => setPhase('confirm')}
          className={`w-full flex items-center gap-2 px-4 py-2.5 rounded border font-cond text-[11px] font-bold tracking-wider transition-all duration-150 cursor-pointer
            ${danger ? 'text-red border-red/40 bg-red/5 hover:bg-red/15 hover:border-red'
                     : 'text-text border-border bg-surface2 hover:border-borderhi hover:text-bright'}`}
        >
          <span>{icon}</span>{label}
        </button>
      )}
      {phase === 'confirm' && (
        <div className="border border-borderhi rounded bg-surface2 p-3">
          <p className="text-bright text-[11px] font-semibold mb-1">{confirmText || `Confirm: ${label}`}</p>
          <p className="text-muted text-[10px] mb-3">{description}</p>
          <div className="flex gap-2">
            <button onClick={() => setPhase('idle')}
              className="flex-1 font-cond text-[10px] font-bold tracking-wider text-muted border border-border py-1.5 rounded hover:border-borderhi transition-colors">
              CANCEL
            </button>
            <button onClick={handleConfirm}
              className={`flex-1 font-cond text-[10px] font-bold tracking-wider py-1.5 rounded transition-colors
                ${danger ? 'text-red border border-red bg-red/10 hover:bg-red/20' : 'text-bg bg-cyan hover:bg-[#00ceff]'}`}>
              CONFIRM
            </button>
          </div>
        </div>
      )}
      {phase === 'loading' && (
        <div className="flex items-center gap-2 px-4 py-2.5 border border-border rounded bg-surface2">
          <motion.span animate={{rotate:360}} transition={{duration:0.7,repeat:Infinity,ease:'linear'}} className="text-cyan">↻</motion.span>
          <span className="font-cond text-[11px] text-muted">Sending to engine…</span>
        </div>
      )}
      {phase === 'done' && (
        <div className="flex items-center gap-2 px-4 py-2.5 border border-green rounded bg-green/10">
          <span>✓</span>
          <span className="font-cond text-[11px] text-green font-bold">Applied</span>
        </div>
      )}
    </div>
  )
}

// ── Slider input ──────────────────────────────────────────────────
function Slider({ label, min, max, step, value, onChange, fmt }) {
  return (
    <div>
      <div className="flex justify-between items-center mb-1">
        <span className="font-cond text-[10px] text-muted uppercase tracking-wider">{label}</span>
        <span className="font-mono text-[11px] text-bright">{fmt ? fmt(value) : value}</span>
      </div>
      <input type="range" min={min} max={max} step={step} value={value}
        onChange={e => onChange(Number(e.target.value))}
        className="w-full h-1 bg-dim rounded-full appearance-none cursor-pointer accent-cyan"
      />
    </div>
  )
}

// ── Section wrapper ───────────────────────────────────────────────
function Section({ title, subtitle, children }) {
  return (
    <div className="bg-surface border border-border rounded-lg overflow-hidden">
      <div className="px-5 py-4 border-b border-border bg-surface2">
        <h3 className="font-cond text-[13px] font-bold tracking-wide text-bright">{title}</h3>
        {subtitle && <p className="text-dim text-[10px] mt-0.5">{subtitle}</p>}
      </div>
      <div className="px-5 py-4">{children}</div>
    </div>
  )
}

// ── Service selector ──────────────────────────────────────────────
function ServiceSelect({ value, onChange, includeAll }) {
  const payload = useStore(s => s.payload)
  const svcs    = Object.keys(payload?.bundles || {})

  return (
    <div>
      <p className="font-cond text-[10px] text-muted uppercase tracking-wider mb-1.5">Target Service</p>
      <div className="flex flex-wrap gap-2">
        {includeAll && (
          <button
            onClick={() => onChange('')}
            className={`font-cond text-[10px] font-bold tracking-wider px-3 py-1.5 rounded border transition-colors cursor-pointer
              ${value === '' ? 'text-bright border-borderhi bg-surface' : 'text-muted border-border hover:text-text'}`}
          >
            All Services
          </button>
        )}
        {svcs.map(id => (
          <button
            key={id}
            onClick={() => onChange(id)}
            className={`font-cond text-[10px] font-bold tracking-wider px-3 py-1.5 rounded border transition-colors cursor-pointer
              ${value === id ? 'text-cyan border-cyan bg-cyan/10' : 'text-muted border-border hover:text-text'}
              ${isTestService(id) ? 'opacity-60' : ''}`}
          >
            {serviceName(id)}
            {isTestService(id) && <span className="text-[8px] ml-1 opacity-60">(test)</span>}
          </button>
        ))}
        {svcs.length === 0 && <p className="text-dim text-[11px]">No services connected yet</p>}
      </div>
    </div>
  )
}

// ── Current autopilot status ──────────────────────────────────────
function AutopilotStatus() {
  const payload    = useStore(s => s.payload)
  const systemMode = useStore(s => s.systemMode)
  const [autopilotOn, setAutopilotOn] = useState(true)
  const addToast   = useStore(s => s.addToast)
  const logHuman   = useStore(s => s.logHumanAction)

  const safetyMode = payload?.safety_mode
  const dirs       = payload?.directives || {}
  const activeDirs = Object.entries(dirs).filter(([,d]) => Math.abs((d.ScaleFactor??1)-1.0) > 0.02)

  const toggleAutopilot = async () => {
    const next = !autopilotOn
    try {
      await setActuation(next)
      setAutopilotOn(next)
      const msg = next ? 'Autopilot enabled — resuming autonomous control' : 'Autopilot frozen — no scaling decisions will execute'
      addToast(next ? 'success' : 'warn', next ? '▶ Autopilot Enabled' : '⏸ Autopilot Frozen', msg)
      logHuman(next ? 'Operator enabled autopilot' : 'Operator froze autopilot', null, 'human')
    } catch(e) {
      addToast('crit', 'Failed', e.message)
    }
  }

  return (
    <Section
      title="Autopilot Control"
      subtitle="The autopilot continuously monitors all services and applies scaling decisions automatically"
    >
      <div className="flex items-center justify-between mb-4">
        <div>
          <p className="text-bright font-semibold text-[13px]">
            {autopilotOn ? '▶ Running autonomously' : '⏸ Frozen — standing by'}
          </p>
          <p className="text-muted text-[11px] mt-0.5">
            {autopilotOn
              ? 'Autopilot is watching all services and scaling them as needed. You do not need to intervene.'
              : 'Autopilot is paused. No scaling actions will execute until you re-enable it.'}
          </p>
          {safetyMode && (
            <p className="text-yellow text-[10px] mt-1 font-semibold">
              ⚠ Safety mode active — engine is running conservatively
            </p>
          )}
        </div>
        <button
          onClick={toggleAutopilot}
          className={`flex-shrink-0 font-cond text-[12px] font-bold tracking-wider px-5 py-3 rounded border transition-all duration-200 cursor-pointer
            ${autopilotOn
              ? 'text-yellow border-yellow bg-yellow/10 hover:bg-yellow/20'
              : 'text-green border-green bg-green/10 hover:bg-green/20'}`}
        >
          {autopilotOn ? '⏸ FREEZE' : '▶ ENABLE'}
        </button>
      </div>

      {activeDirs.length > 0 && (
        <div className="border border-border rounded p-3 bg-surface2">
          <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase mb-2">
            Currently executing
          </p>
          <div className="space-y-1">
            {activeDirs.map(([svc, dir]) => {
              const sf  = dir.ScaleFactor ?? 1
              const up  = sf > 1
              return (
                <div key={svc} className="flex items-center gap-2 text-[11px]">
                  <span className={up ? 'text-yellow' : 'text-cyan'}>{up ? '⬆' : '⬇'}</span>
                  <span className="text-text">{serviceName(svc)}</span>
                  <span className="text-muted ml-auto">scale factor {sf.toFixed(2)}</span>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </Section>
  )
}

// ── Policy preset controls ────────────────────────────────────────
function PolicyControl() {
  const addToast = useStore(s => s.addToast)
  const logHuman = useStore(s => s.logHumanAction)
  const [current, setCurrent] = useState('balanced')

  const presets = [
    {
      id:    'conservative',
      label: 'Safe Mode',
      desc:  'Slow, careful scaling. Prioritises stability over performance. Use during incidents.',
      color: 'text-cyan border-cyan',
      bg:    'bg-cyan/10',
    },
    {
      id:    'balanced',
      label: 'Normal',
      desc:  'Default behaviour. Balances speed of response with stability.',
      color: 'text-green border-green',
      bg:    'bg-green/10',
    },
    {
      id:    'aggressive',
      label: 'Performance',
      desc:  'Fast, aggressive scaling. Responds quickly but accepts more risk. Use only during known load events.',
      color: 'text-yellow border-yellow',
      bg:    'bg-yellow/10',
    },
  ]

  const apply = async (preset) => {
    try {
      await setPolicy(preset.id)
      setCurrent(preset.id)
      addToast('info', `Policy: ${preset.label}`, preset.desc)
      logHuman(`Operator set policy to ${preset.label}`, null, 'human')
    } catch(e) {
      addToast('crit', 'Policy Failed', e.message)
    }
  }

  return (
    <Section
      title="Operating Policy"
      subtitle="Controls how aggressively the autopilot responds to load changes"
    >
      <div className="grid grid-cols-3 gap-3">
        {presets.map(p => (
          <button
            key={p.id}
            onClick={() => apply(p)}
            className={`p-3 rounded border text-left transition-all duration-200 cursor-pointer
              ${current === p.id ? `${p.color} ${p.bg}` : 'border-border bg-surface2 hover:border-borderhi'}`}
          >
            <p className={`font-cond text-[11px] font-bold tracking-wider mb-1 ${current === p.id ? p.color.split(' ')[0] : 'text-bright'}`}>
              {current === p.id && '✓ '}{p.label}
            </p>
            <p className="text-muted text-[10px] leading-snug">{p.desc}</p>
          </button>
        ))}
      </div>
    </Section>
  )
}

// ── Traffic simulation controls ───────────────────────────────────
function TrafficSimulation() {
  const [svc, setSvc]       = useState('')
  const [reqFactor, setReq] = useState(2.5)
  const [latFactor, setLat] = useState(1.6)
  const [dur, setDur]       = useState(20)
  const addToast  = useStore(s => s.addToast)
  const logHuman  = useStore(s => s.logHumanAction)

  const runStress = async () => {
    await runStressTest(svc, dur, reqFactor, latFactor)
    const svcLabel = svc ? serviceName(svc) : 'all services'
    addToast('warn', '🔬 Stress Test Started',
      `Sending ${reqFactor}× traffic with ${latFactor}× latency to ${svcLabel} for ${dur} ticks (~${dur*2}s)`)
    logHuman(`Stress test on ${svcLabel}`, svc || null, 'human')
  }

  const runBurst = async () => {
    await replayBurst(svc, dur, reqFactor)
    const svcLabel = svc ? serviceName(svc) : 'all services'
    addToast('warn', '💥 Traffic Burst Started',
      `Sending a ${reqFactor}× spike to ${svcLabel} for ${dur} ticks (~${dur*2}s)`)
    logHuman(`Traffic burst on ${svcLabel}`, svc || null, 'human')
  }

  return (
    <Section
      title="Traffic Simulation"
      subtitle="Send synthetic load to test how the system responds. Use this to verify the autopilot is working before a real event."
    >
      <div className="space-y-4">
        <ServiceSelect value={svc} onChange={setSvc} includeAll />

        <div className="grid grid-cols-3 gap-4">
          <Slider label="Traffic Multiplier" min={1} max={10} step={0.5}
            value={reqFactor} onChange={setReq}
            fmt={v => `${v}× normal`} />
          <Slider label="Latency Multiplier" min={1} max={5} step={0.1}
            value={latFactor} onChange={setLat}
            fmt={v => `${v}× normal`} />
          <Slider label="Duration" min={5} max={120} step={5}
            value={dur} onChange={setDur}
            fmt={v => `${v} ticks (~${v*2}s)`} />
        </div>

        <div className="flex gap-3 pt-2">
          <ConfirmAction
            label="Run Stress Test"
            icon="🔬"
            description={`Degrades ${svc ? serviceName(svc) : 'all services'} with ${reqFactor}× traffic + ${latFactor}× latency for ~${dur*2} seconds. Watch how the autopilot responds.`}
            confirmText="This will inject artificial load — confirm?"
            onConfirm={runStress}
          />
          <ConfirmAction
            label="Replay Traffic Burst"
            icon="💥"
            description={`Sends a clean ${reqFactor}× traffic spike to ${svc ? serviceName(svc) : 'all services'} for ~${dur*2} seconds.`}
            confirmText="This will inject a traffic spike — confirm?"
            onConfirm={runBurst}
          />
        </div>
      </div>
    </Section>
  )
}

// ── Engine utilities ──────────────────────────────────────────────
function EngineTools() {
  const addToast = useStore(s => s.addToast)
  const logHuman = useStore(s => s.logHumanAction)

  const tools = [
    {
      label:  'Step Engine Once',
      icon:   '⏭',
      desc:   'Force the control engine to run one processing cycle immediately, instead of waiting for the 2-second tick.',
      action: async () => {
        const r = await stepOnce()
        addToast('info', 'Engine Stepped', `Tick ${r?.tick || '?'} executed manually`)
        logHuman('Manual engine step', null, 'human')
      },
    },
    {
      label:  'Run Simulation Now',
      icon:   '🎲',
      desc:   'Force the Monte Carlo simulation stage to run immediately and project future states.',
      action: async () => {
        await controlSimulation('run', 10)
        addToast('info', 'Simulation Running', 'Monte Carlo projection started for 10 ticks')
        logHuman('Manual simulation trigger', null, 'human')
      },
    },
    {
      label:  'Retrain Intelligence',
      icon:   '🧠',
      desc:   'Ask the autopilot to re-evaluate its policy model based on recent observations.',
      action: async () => {
        await triggerRollout(10)
        addToast('info', 'Intelligence Rollout', 'Autopilot re-evaluating policy for 10 ticks')
        logHuman('Manual intelligence rollout', null, 'human')
      },
    },
    {
      label:  'Run Sandbox Experiment',
      icon:   '🧪',
      desc:   'Trigger an internal sandbox experiment — the engine will generate and test a synthetic scenario.',
      action: async () => {
        await triggerSandbox('experiment', 10)
        addToast('info', 'Sandbox Started', 'Internal experiment scheduled for 10 ticks')
        logHuman('Sandbox experiment triggered', null, 'human')
      },
    },
  ]

  return (
    <Section
      title="Engine Utilities"
      subtitle="Advanced controls for inspecting and debugging the control engine"
    >
      <div className="grid grid-cols-2 gap-3">
        {tools.map(t => (
          <button
            key={t.label}
            onClick={async () => {
              try { await t.action() }
              catch(e) { addToast('crit', 'Failed', e.message) }
            }}
            title={t.desc}
            className="flex items-start gap-3 p-3 rounded border border-border bg-surface2 hover:border-borderhi hover:bg-surface text-left transition-all duration-150 cursor-pointer"
          >
            <span className="text-[18px] flex-shrink-0">{t.icon}</span>
            <div>
              <p className="font-cond text-[11px] font-bold tracking-wide text-bright">{t.label}</p>
              <p className="text-muted text-[10px] mt-0.5 leading-snug">{t.desc}</p>
            </div>
          </button>
        ))}
      </div>
    </Section>
  )
}

// ── Operations log sidebar ────────────────────────────────────────
function OpsLogSidebar() {
  const opsLog = useStore(s => s.opsLog)
  const human  = [...opsLog].reverse().filter(e => e.source === 'human').slice(0, 20)

  return (
    <div className="bg-surface border border-border rounded-lg overflow-hidden sticky top-4">
      <div className="px-4 py-3 border-b border-border bg-surface2">
        <h3 className="font-cond text-[11px] font-bold tracking-wider text-muted">YOUR ACTIONS</h3>
        <p className="text-dim text-[10px] mt-0.5">Everything you've done this session</p>
      </div>
      <div className="divide-y divide-border/30 max-h-[calc(100vh-200px)] overflow-y-auto">
        {human.length === 0 ? (
          <p className="text-dim text-[11px] font-cond tracking-wider p-4 text-center">
            No operator actions yet
          </p>
        ) : human.map(e => (
          <div key={e.id} className="px-3 py-2.5 hover:bg-surface2 transition-colors">
            <p className="text-[10px] text-text leading-snug">{e.text}</p>
            <p className="text-dim text-[9px] font-mono mt-0.5">
              {new Date(e.at).toLocaleTimeString('en-GB',{hour12:false})}
            </p>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function ControlPage() {
  const systemMode = useStore(s => s.systemMode)
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-7xl mx-auto px-4 py-4">
        <div className="grid grid-cols-[1fr_260px] gap-6">
          <div className="space-y-4">
            {systemMode === 'offline' && (
              <div className="border border-border rounded-lg p-4 bg-surface2 text-center">
                <p className="text-muted font-cond text-[12px] tracking-wider">No services connected</p>
                <p className="text-dim text-[10px] mt-1">Connect a service to enable control actions</p>
              </div>
            )}
            <AutopilotStatus />
            <PolicyControl />
            <TrafficSimulation />
            <EngineTools />
          </div>
          <div>
            <OpsLogSidebar />
          </div>
        </div>
      </div>
    </div>
  )
}