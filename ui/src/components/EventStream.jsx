import React, { useMemo, useState } from 'react'
import { useStore } from '../store/useStore'
import { ms, n, pct, safe, ts } from '../lib/fmt'

const API_BASE = `${location.protocol}//${location.hostname}:8080/api/v1`

const SEVERITY = {
  2: { label: 'CRIT', cls: 'border-red bg-red/10 text-red', dot: 'bg-red animate-blink' },
  1: { label: 'WARN', cls: 'border-yellow bg-yellow/10 text-yellow', dot: 'bg-yellow' },
  0: { label: 'INFO', cls: 'border-cyan bg-cyan/10 text-cyan', dot: 'bg-cyan' },
}

const runtimeRows = [
  ['windows', 'avg_windows_ms'],
  ['topology', 'avg_topology_ms'],
  ['coupling', 'avg_coupling_ms'],
  ['model', 'avg_modelling_ms'],
  ['optimise', 'avg_optimise_ms'],
  ['sim', 'avg_sim_ms'],
  ['reason', 'avg_reasoning_ms'],
  ['broadcast', 'avg_broadcast_ms'],
]

function eventSort(a, b) {
  const sev = (b.severity ?? 0) - (a.severity ?? 0)
  if (sev !== 0) return sev
  const pri = (b.operational_priority ?? 0) - (a.operational_priority ?? 0)
  if (pri !== 0) return pri
  return new Date(b.timestamp || 0) - new Date(a.timestamp || 0)
}

function Stat({ label, value, tone = 'text-text' }) {
  return (
    <div className="min-w-0 bg-surface2 border border-border rounded px-2 py-2">
      <div className="font-cond text-[9px] font-bold tracking-[0.14em] text-muted uppercase truncate">{label}</div>
      <div className={`font-mono text-[12px] font-semibold truncate ${tone}`}>{value}</div>
    </div>
  )
}

function Evidence({ evidence }) {
  const rows = [
    ['rho', evidence?.utilisation, pct],
    ['risk', evidence?.collapse_risk, pct],
    ['osc', evidence?.oscillation_risk, pct],
    ['wait', evidence?.queue_wait_ms, ms],
    ['sat', evidence?.saturation_sec, v => `${n(v, 1)}s`],
    ['burst', evidence?.burst_factor, v => `${n(v, 2)}x`],
    ['cascade', evidence?.cascade_risk, pct],
    ['margin', evidence?.stability_margin, pct],
  ].filter(([, value]) => safe(value) !== null)

  if (!rows.length) return null

  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {rows.map(([label, value, format]) => (
        <span key={label} className="text-[9px] text-muted border border-border bg-bg/50 rounded px-1.5 py-0.5">
          {label}: <span className="text-text font-mono">{format(value)}</span>
        </span>
      ))}
    </div>
  )
}

function EventCard({ event, onAck, busy }) {
  const sev = SEVERITY[event.severity] ?? SEVERITY[0]
  const chain = event.model_chain ? String(event.model_chain).replace(/\u2192/g, ' -> ') : ''

  return (
    <div className={`border-l-2 ${sev.cls} rounded-r px-3 py-2 bg-surface2`}>
      <div className="flex items-center gap-2 min-w-0">
        <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${sev.dot}`} />
        <span className={`font-cond text-[9px] font-bold tracking-wider border rounded px-1 ${sev.cls}`}>
          {sev.label}
        </span>
        {event.service_id && <span className="text-cyan font-mono text-[10px] truncate">{event.service_id}</span>}
        {event.category && <span className="text-dim text-[10px] truncate">{event.category}</span>}
        <span className="ml-auto text-dim text-[10px] flex-shrink-0">{ts(event.timestamp)}</span>
      </div>

      <p className="text-text text-[11px] leading-snug mt-1.5">{event.description || 'Reasoning event'}</p>
      {event.recommendation && (
        <p className="text-muted text-[10px] leading-snug mt-1">Action: {event.recommendation}</p>
      )}
      {chain && <p className="text-dim text-[10px] leading-snug mt-1 font-mono">{chain}</p>}

      <Evidence evidence={event.evidence} />

      {event.id && (
        <div className="mt-2 flex justify-end">
          <button
            onClick={() => onAck(event)}
            disabled={busy === event.id}
            className="font-cond text-[10px] font-bold tracking-wider text-cyan border border-cyan/40 rounded px-2 py-1 hover:bg-cyan/10 disabled:opacity-50"
          >
            {busy === event.id ? 'ACKING' : 'ACK'}
          </button>
        </div>
      )}
    </div>
  )
}

function ControlButton({ children, onClick, busy, disabled }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled || busy}
      className="min-h-8 rounded border border-border bg-surface2 px-2 py-1.5 font-cond text-[10px] font-bold tracking-wider text-text hover:border-cyan hover:text-cyan disabled:opacity-45 disabled:cursor-not-allowed transition-colors"
    >
      {busy ? 'WORKING' : children}
    </button>
  )
}

export default function EventStream() {
  const connected = useStore(s => s.connected)
  const payload = useStore(s => s.payload)
  const addToast = useStore(s => s.addToast)
  const [busy, setBusy] = useState('')
  const [targetService, setTargetService] = useState('')
  const [policyPreset, setPolicyPreset] = useState('balanced')

  const controlPlane = payload?.control_plane || {}
  const runtime = payload?.runtime_metrics || {}
  const objective = payload?.objective || {}
  const network = payload?.network_equilibrium || {}
  const envelope = payload?.stability_envelope || {}

  const services = useMemo(() => Object.keys(payload?.bundles || {}).sort(), [payload])

  const events = useMemo(() => {
    return [...(payload?.events || [])]
      .filter(e => e && (e.description || e.recommendation || e.category))
      .sort(eventSort)
      .slice(0, 40)
  }, [payload])

  const serviceTarget = targetService || undefined

  const post = async (key, path, body, successTitle) => {
    setBusy(key)
    try {
      const res = await fetch(`${API_BASE}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body || {}),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(data.error || `${res.status} ${res.statusText}`)
      addToast('success', successTitle, data.status ? `Backend status: ${data.status}` : 'Backend accepted', 3500)
      return data
    } catch (err) {
      addToast('crit', 'Backend call failed', err.message || String(err), 6500)
      return null
    } finally {
      setBusy('')
    }
  }

  const ackEvent = (event) => {
    post(event.id, '/alerts/ack', { alert_id: event.id }, 'Alert acknowledged')
  }

  return (
    <div className="h-full min-h-0 bg-surface border border-border rounded overflow-hidden flex flex-col">
      <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-2 flex-shrink-0">
        <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">EVENT STREAM</span>
        <span className={`ml-auto w-1.5 h-1.5 rounded-full ${connected ? 'bg-green animate-pulse2' : 'bg-red'}`} />
        <span className="font-cond text-[10px] tracking-wider text-muted">{connected ? 'WS LIVE' : 'WS OFFLINE'}</span>
      </div>

      <div className="p-2 border-b border-border bg-bg/30 flex-shrink-0">
        <div className="grid grid-cols-3 gap-1.5">
          <Stat label="seq" value={payload?.seq ?? '-'} tone="text-cyan" />
          <Stat label="schema" value={payload?.schema_version ?? '-'} />
          <Stat label="tick" value={controlPlane.tick ?? '-'} />
          <Stat label="actuation" value={controlPlane.actuation_enabled ? 'on' : 'off'} tone={controlPlane.actuation_enabled ? 'text-green' : 'text-yellow'} />
          <Stat label="policy" value={controlPlane.policy_preset || '-'} />
          <Stat label="net risk" value={pct(network.network_saturation_risk)} tone={(network.network_saturation_risk || 0) > 0.7 ? 'text-red' : 'text-text'} />
        </div>
      </div>

      <div className="p-2 border-b border-border flex-shrink-0">
        <div className="flex items-center gap-2 mb-2">
          <select
            value={targetService}
            onChange={e => setTargetService(e.target.value)}
            className="min-w-0 flex-1 bg-bg border border-border rounded px-2 py-1.5 text-[11px] text-text outline-none focus:border-cyan"
          >
            <option value="">All services</option>
            {services.map(svc => <option key={svc} value={svc}>{svc}</option>)}
          </select>
          <select
            value={policyPreset}
            onChange={e => setPolicyPreset(e.target.value)}
            className="w-28 bg-bg border border-border rounded px-2 py-1.5 text-[11px] text-text outline-none focus:border-cyan"
          >
            <option value="balanced">balanced</option>
            <option value="latency">latency</option>
            <option value="stability">stability</option>
            <option value="cost">cost</option>
          </select>
        </div>

        <div className="grid grid-cols-2 gap-1.5">
          <ControlButton
            busy={busy === 'toggle'}
            onClick={() => post('toggle', '/control/toggle', { enabled: !controlPlane.actuation_enabled }, 'Actuation toggled')}
          >
            {controlPlane.actuation_enabled ? 'DISABLE ACTUATION' : 'ENABLE ACTUATION'}
          </ControlButton>
          <ControlButton
            busy={busy === 'step'}
            onClick={() => post('step', '/runtime/step', {}, 'Runtime stepped')}
          >
            STEP TICK
          </ControlButton>
          <ControlButton
            busy={busy === 'chaos'}
            onClick={() => post('chaos', '/control/chaos-run', { service_id: serviceTarget, duration_ticks: 30, request_factor: 2.5, latency_factor: 1.6 }, 'Chaos run scheduled')}
          >
            CHAOS RUN
          </ControlButton>
          <ControlButton
            busy={busy === 'replay'}
            onClick={() => post('replay', '/control/replay-burst', { service_id: serviceTarget, duration_ticks: 20, factor: 2.0 }, 'Replay burst scheduled')}
          >
            REPLAY BURST
          </ControlButton>
          <ControlButton
            busy={busy === 'policy'}
            onClick={() => post('policy', '/policy/update', { preset: policyPreset }, 'Policy updated')}
          >
            APPLY POLICY
          </ControlButton>
          <ControlButton
            busy={busy === 'sandbox'}
            onClick={() => post('sandbox', '/sandbox/trigger', { type: 'experiment', duration_ticks: 10 }, 'Sandbox scheduled')}
          >
            SANDBOX
          </ControlButton>
          <ControlButton
            busy={busy === 'sim'}
            onClick={() => post('sim', '/simulation/control', { action: 'run', duration_ticks: 10 }, 'Simulation forced')}
          >
            RUN SIM
          </ControlButton>
          <ControlButton
            busy={busy === 'rollout'}
            onClick={() => post('rollout', '/intelligence/rollout', { duration_ticks: 10 }, 'Rollout forced')}
          >
            RL ROLLOUT
          </ControlButton>
        </div>
      </div>

      <div className="p-2 border-b border-border flex-shrink-0">
        <div className="grid grid-cols-2 gap-1.5">
          <Stat label="objective" value={n(objective.composite_score, 3)} />
          <Stat label="p99" value={ms(objective.predicted_p99_latency_ms)} />
          <Stat label="headroom" value={pct(envelope.envelope_headroom)} tone={(envelope.envelope_headroom || 0) < 0.15 ? 'text-yellow' : 'text-text'} />
          <Stat label="critical" value={network.critical_service_id || envelope.most_vulnerable_service || '-'} tone="text-cyan" />
        </div>

        <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1">
          {runtimeRows.map(([label, key]) => (
            <div key={key} className="flex items-center justify-between gap-2 text-[10px]">
              <span className="text-muted truncate">{label}</span>
              <span className="text-text font-mono">{ms(runtime[key])}</span>
            </div>
          ))}
          <div className="flex items-center justify-between gap-2 text-[10px]">
            <span className="text-muted truncate">overruns</span>
            <span className={runtime.predicted_overrun ? 'text-red font-mono' : 'text-text font-mono'}>
              {runtime.total_overruns ?? 0}/{runtime.consec_overruns ?? 0}
            </span>
          </div>
          <div className="flex items-center justify-between gap-2 text-[10px]">
            <span className="text-muted truncate">safety</span>
            <span className={(runtime.safety_level || 0) > 0 ? 'text-yellow font-mono' : 'text-text font-mono'}>
              {runtime.safety_level ?? 0}
            </span>
          </div>
        </div>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto p-2">
        {events.length === 0 ? (
          <div className="h-full min-h-[180px] flex items-center justify-center text-center">
            <div>
              <p className="font-cond text-[12px] font-bold tracking-wider text-muted">NO REASONING EVENTS</p>
              <p className="text-dim text-[10px] mt-1">{payload ? 'Backend stream is quiet.' : 'Waiting for first WebSocket tick.'}</p>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {events.map((event, i) => (
              <EventCard key={event.id || `${event.timestamp}-${i}`} event={event} onAck={ackEvent} busy={busy} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
