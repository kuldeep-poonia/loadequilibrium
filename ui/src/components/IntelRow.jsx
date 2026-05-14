import React, { useMemo } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useStore } from '../store/useStore'
import { ts, pct, ms, n } from '../lib/fmt'
import { safe } from '../lib/fmt'

const SEV = {
  2: { label: 'CRIT', cls: 'text-red    bg-red/10    border-red',    dot: 'bg-red animate-blink'  },
  1: { label: 'WARN', cls: 'text-yellow bg-yellow/10 border-yellow', dot: 'bg-yellow'              },
  0: { label: 'INFO', cls: 'text-cyan   bg-cyan/10   border-cyan',   dot: 'bg-cyan'                },
}

function IncidentCard({ e }) {
  const sev = SEV[e.severity] ?? SEV[0]
  return (
    <motion.div
      layout key={e.id}
      initial={{ opacity: 0, y: -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}
      transition={{ duration: 0.2 }}
      className={`border-l-2 pl-3 pr-2 py-2 rounded-r bg-surface2 mb-2 ${sev.cls.split(' ').slice(2).join(' ')}`}
    >
      <div className="flex items-center gap-2 mb-0.5">
        <span className={`font-cond text-[9px] font-bold tracking-wider px-1 rounded border ${sev.cls}`}>{sev.label}</span>
        {e.service_id && <span className="text-cyan text-[11px] font-semibold">{e.service_id}</span>}
        <span className="text-dim text-[10px] ml-auto">{ts(e.timestamp)}</span>
      </div>
      <p className="text-text text-[11px] leading-snug">{e.description}</p>
      {e.recommendation && (
        <p className="text-muted text-[10px] mt-1 italic">→ {e.recommendation}</p>
      )}
    </motion.div>
  )
}

function CausalChain({ events }) {
  const top = useMemo(() => {
    const ranked = [...events].sort((a, b) => b.operational_priority - a.operational_priority)
    return ranked[0]
  }, [events])

  if (!top?.model_chain) return (
    <div className="text-dim font-cond text-[11px] tracking-wider p-3">
      {events.length ? events[0]?.description || 'Processing…' : 'Awaiting events…'}
    </div>
  )

  const steps = top.model_chain.split('→').map(s => s.trim()).filter(Boolean)
  return (
    <div className="p-3 flex flex-col gap-1">
      {steps.map((step, i) => (
        <div key={i} className="flex items-start gap-2">
          {i > 0 && <span className="text-dim text-[10px] mt-0.5 flex-shrink-0">↓</span>}
          {i === 0 && <span className="w-2.5 flex-shrink-0" />}
          <span className={`text-[11px] leading-snug ${i === 0 ? 'text-yellow font-semibold' : i === steps.length - 1 ? 'text-red font-semibold' : 'text-text'}`}>
            {step}
          </span>
        </div>
      ))}
    </div>
  )
}

function ActionCard({ action, onApply }) {
  return (
    <div className="flex items-center gap-3 px-4 py-3 bg-surface2 border border-border rounded hover:border-borderhi transition-colors duration-200 flex-1 min-w-[220px]">
      <span className="text-[16px] flex-shrink-0">{action.icon}</span>
      <div className="flex-1 min-w-0">
        <div className="text-[11px] font-semibold text-bright truncate">{action.title}</div>
        <div className="text-[10px] text-muted mt-0.5 truncate">{action.detail}</div>
      </div>
      <button
        onClick={() => onApply(action)}
        className="flex-shrink-0 font-cond text-[11px] font-bold tracking-wider bg-cyan text-bg px-4 py-2 rounded hover:bg-[#00ceff] active:scale-95 transition-all duration-150 cursor-pointer select-none"
      >
        APPLY
      </button>
    </div>
  )
}

export default function IntelPanel() {
  const payload   = useStore(s => s.payload)
  const setPending= useStore(s => s.setPendingAction)
  const addToast  = useStore(s => s.addToast)

  const events = useMemo(() => {
    const evts = payload?.events || []
    return evts.filter(e => e?.description).sort((a, b) => b.severity - a.severity || b.operational_priority - a.operational_priority)
  }, [payload])

  const incidents = events.filter(e => e.severity >= 1).slice(0, 6)

  const actions = useMemo(() => {
    const evts   = payload?.events || []
    const dirs   = payload?.directives || {}
    const recs   = []
    const seen   = new Set()

    for (const e of evts) {
      if (e.recommendation && e.severity >= 1 && !seen.has(e.recommendation)) {
        seen.add(e.recommendation)
        recs.push({ icon: e.severity >= 2 ? '🔴' : '🟡', title: e.recommendation,
                    detail: e.service_id ? `Service: ${e.service_id}` : 'System-level', event: e })
      }
    }

    for (const [svc, dir] of Object.entries(dirs)) {
      const sf = safe(dir.ScaleFactor)
      if (sf !== null && Math.abs(sf - 1.0) > 0.05) {
        const up  = sf > 1.0
        const pct2= Math.abs((sf - 1) * 100).toFixed(0)
        const title = `Scale ${up ? 'up' : 'down'} ${svc} by ${pct2}%`
        if (!seen.has(title)) {
          seen.add(title)
          recs.push({ icon: up ? '⬆' : '⬇', title, detail: `Factor: ${sf.toFixed(2)}`, svc })
        }
      }
    }

    return recs.slice(0, 6)
  }, [payload])

  const handleApply = (action) => setPending(action)

  return (
    <div className="grid grid-cols-2 gap-2">
      <div className="bg-surface border border-border rounded overflow-hidden flex flex-col">
        <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-2 flex-shrink-0">
          <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">ACTIVE INCIDENTS</span>
          {incidents.length > 0 && (
            <span className="font-mono text-[10px] font-bold text-red bg-red/10 border border-red px-1.5 py-0.5 rounded-full">
              {incidents.length}
            </span>
          )}
        </div>
        <div className="flex-1 overflow-y-auto p-2 min-h-0" style={{ maxHeight: 200 }}>
          {incidents.length === 0 ? (
            <p className="text-dim font-cond text-[11px] tracking-wider p-2">No active incidents</p>
          ) : (
            <AnimatePresence initial={false}>
              {incidents.map(e => <IncidentCard key={e.id || e.description} e={e} />)}
            </AnimatePresence>
          )}
        </div>
      </div>

      <div className="bg-surface border border-border rounded overflow-hidden flex flex-col">
        <div className="px-3 py-2 border-b border-border bg-surface2 flex-shrink-0">
          <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">CAUSAL CHAIN</span>
        </div>
        <div className="flex-1 overflow-y-auto min-h-0" style={{ maxHeight: 200 }}>
          <CausalChain events={events} />
        </div>
      </div>

      <div className="col-span-2 bg-surface border border-border rounded overflow-hidden flex flex-col">
        <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-2 flex-shrink-0">
          <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">RECOMMENDED ACTIONS</span>
          {actions.length > 0 && (
            <span className="font-mono text-[10px] font-bold text-cyan bg-cyan/10 border border-cyan px-1.5 py-0.5 rounded-full">
              {actions.length}
            </span>
          )}
          <span className="font-cond text-[9px] text-dim ml-auto tracking-wider">Review and APPLY to act</span>
        </div>
        <div className="p-2">
          {actions.length === 0 ? (
            <p className="text-dim font-cond text-[11px] tracking-wider p-2">No recommendations at this time</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {actions.map((a, i) => <ActionCard key={i} action={a} onApply={handleApply} />)}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}