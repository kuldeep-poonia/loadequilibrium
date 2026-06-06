import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useStore } from '../store/useStore'
import { LIFECYCLE_LABEL, LIFECYCLE_COLOR, SOURCE_LABEL, SOURCE_COLOR } from '../lib/incidents'
import { ts } from '../lib/fmt'

const SEV_COLOR = {
  critical: 'text-red    border-red',
  warning:  'text-yellow border-yellow',
  info:     'text-cyan   border-cyan',
}

const STAGE_TERMINAL = ['resolved', 'failed', 'overridden']

function LifecycleProgress({ stage }) {
  const steps = ['detected','analyzing','predicted','action_selected','executing','stabilizing','resolved']
  const cur   = steps.indexOf(stage)
  const term  = STAGE_TERMINAL.includes(stage)

  if (stage === 'failed')     return <span className="font-cond text-[9px] text-red font-bold tracking-wider">FAILED</span>
  if (stage === 'overridden') return <span className="font-cond text-[9px] text-muted font-bold tracking-wider">OVERRIDDEN</span>

  return (
    <div className="flex items-center gap-0.5 mt-1.5">
      {steps.map((s, i) => (
        <React.Fragment key={s}>
          <div className={`h-1 rounded-full transition-all duration-500 ${
            i <= cur ? (stage === 'resolved' ? 'bg-green' : 'bg-cyan') : 'bg-dim'
          }`} style={{ width: i <= cur ? 18 : 10 }} />
          {i < steps.length - 1 && <div className="w-1 h-px bg-dim" />}
        </React.Fragment>
      ))}
      <span className={`font-cond text-[9px] font-bold tracking-wider ml-2 ${LIFECYCLE_COLOR[stage]?.split(' ')[0] || 'text-muted'}`}>
        {LIFECYCLE_LABEL[stage]}
      </span>
    </div>
  )
}

function IncidentRecord({ inc, onSelect, selected }) {
  const sevColor  = SEV_COLOR[inc.severity] || SEV_COLOR.info
  const isResolved= STAGE_TERMINAL.includes(inc.stage)
  const override  = useStore(s => s.overrideIncident)

  const age = Date.now() - inc.createdAt
  const ageStr = age < 60000 ? `${Math.round(age/1000)}s ago` : `${Math.round(age/60000)}m ago`

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, height: 0 }}
      transition={{ duration: 0.2 }}
      className={`border rounded mb-2 overflow-hidden transition-all duration-300 cursor-pointer
        ${selected ? 'border-cyan/50 bg-cyan/5' : isResolved ? 'border-border/40 bg-surface opacity-70' : `border-l-2 bg-surface2 ${sevColor.split(' ')[1]}`}
      `}
      onClick={() => onSelect(inc.id)}
    >
      <div className="px-3 py-2">
        <div className="flex items-start gap-2">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-0.5 flex-wrap">
              {inc.affectedServices.map(s => (
                <span key={s} className="font-mono text-[10px] font-semibold text-cyan bg-cyan/10 px-1.5 rounded">{s}</span>
              ))}
              <span className={`font-cond text-[9px] font-bold tracking-wider px-1 rounded border ${sevColor}`}>
                {inc.severity.toUpperCase()}
              </span>
              {inc.humanOverride && (
                <span className="font-cond text-[9px] font-bold tracking-wider text-purple border border-purple/40 px-1 rounded">
                  HUMAN OVERRIDE
                </span>
              )}
              <span className="text-dim text-[10px] ml-auto">{ageStr}</span>
            </div>
            <p className="text-text text-[11px] leading-snug line-clamp-2">{inc.problem}</p>
            <LifecycleProgress stage={inc.stage} />
          </div>
        </div>

        <AnimatePresence>
          {selected && (
            <motion.div
              initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }}
              transition={{ duration: 0.2 }}
              className="mt-3 pt-3 border-t border-border/50"
            >
              <div className="grid grid-cols-2 gap-3 mb-3">
                {inc.prediction && (
                  <div>
                    <span className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase block mb-1">Prediction</span>
                    <p className="text-[10px] text-text leading-snug">
                      {inc.prediction.split('→').map((s,i,arr) => (
                        <React.Fragment key={i}>
                          <span className={i === 0 ? 'text-yellow' : i === arr.length-1 ? 'text-red' : 'text-text'}>
                            {s.trim()}
                          </span>
                          {i < arr.length-1 && <span className="text-dim mx-1">→</span>}
                        </React.Fragment>
                      ))}
                    </p>
                  </div>
                )}
                {inc.decision && (
                  <div>
                    <span className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase block mb-1">Decision</span>
                    <p className="text-[10px] text-cyan leading-snug">{inc.decision}</p>
                  </div>
                )}
                {inc.confidence !== null && inc.confidence !== undefined && (
                  <div>
                    <span className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase block mb-1">Confidence</span>
                    <div className="flex items-center gap-2">
                      <div className="flex-1 h-1 bg-dim rounded-full overflow-hidden">
                        <div className="h-full bg-cyan rounded-full" style={{ width: `${Math.min(100, inc.confidence * 100)}%` }} />
                      </div>
                      <span className="text-[10px] text-cyan font-mono">{(inc.confidence * 100).toFixed(0)}%</span>
                    </div>
                  </div>
                )}
                {inc.outcome && (
                  <div>
                    <span className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase block mb-1">Outcome</span>
                    <p className="text-[10px] text-green">{inc.outcome}</p>
                  </div>
                )}
              </div>

              <div className="border-t border-border/40 pt-2 mt-2">
                <span className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase block mb-1.5">Timeline</span>
                <div className="space-y-1 max-h-24 overflow-y-auto">
                  {inc.entries.map((entry, i) => (
                    <div key={i} className="flex items-start gap-2 text-[10px]">
                      <span className="text-dim flex-shrink-0 font-mono">{new Date(entry.at).toLocaleTimeString('en-GB',{hour12:false})}</span>
                      <span className={`font-cond text-[8px] font-bold tracking-wider flex-shrink-0 px-1 rounded border ${SOURCE_COLOR[entry.source]?.split(' ').slice(0,2).join(' ') || ''}`}>
                        {SOURCE_LABEL[entry.source] || entry.source}
                      </span>
                      <span className="text-muted flex-1 leading-tight">{entry.note}</span>
                    </div>
                  ))}
                </div>
              </div>

              {!STAGE_TERMINAL.includes(inc.stage) && (
                <div className="mt-3 flex justify-end">
                  <button
                    onClick={(e) => { e.stopPropagation(); override(inc.id, 'Operator closed incident') }}
                    className="font-cond text-[10px] font-bold tracking-wider text-muted border border-border px-3 py-1.5 rounded hover:border-borderhi hover:text-text transition-colors"
                  >
                    MARK OVERRIDDEN
                  </button>
                </div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  )
}

function OpsLogEntry({ entry }) {
  const srcColor = SOURCE_COLOR[entry.source] || SOURCE_COLOR.reasoning
  return (
    <div className="flex items-start gap-2 py-1.5 border-b border-border/30 last:border-0">
      <span className="text-dim text-[10px] font-mono flex-shrink-0 pt-px">
        {new Date(entry.at).toLocaleTimeString('en-GB',{hour12:false})}
      </span>
      <span className={`font-cond text-[8px] font-bold tracking-wider flex-shrink-0 px-1.5 py-0.5 rounded border mt-px ${srcColor}`}>
        {SOURCE_LABEL[entry.source] || entry.source}
      </span>
      {entry.service && (
        <span className="text-cyan text-[10px] font-mono flex-shrink-0">[{entry.service}]</span>
      )}
      <span className="text-text text-[10px] leading-snug flex-1">{entry.text}</span>
    </div>
  )
}

export default function OperationsLog() {
  const incidents    = useStore(s => s.incidents)
  const opsLog       = useStore(s => s.opsLog)
  const selectedInc  = useStore(s => s.selectedInc)
  const selectInc    = useStore(s => s.selectIncident)
  const closeInc     = useStore(s => s.closeIncident)
  const systemMode   = useStore(s => s.systemMode)
  const [tab, setTab] = useState('incidents')

  const open     = incidents.filter(i => !STAGE_TERMINAL.includes(i.stage))
  const resolved = incidents.filter(i => STAGE_TERMINAL.includes(i.stage))
  const sorted   = [...open, ...resolved]

  const handleSelect = (id) => {
    if (selectedInc === id) closeInc()
    else selectInc(id)
  }

  const modeColor = { normal: 'text-green', degraded: 'text-yellow', emergency: 'text-red', offline: 'text-muted' }

  return (
    <div className="bg-surface border border-border rounded overflow-hidden flex flex-col">
      <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-3 flex-shrink-0">
        <div className="flex gap-1">
          {[['incidents', 'INCIDENTS'], ['opslog', 'OPS LOG']].map(([key, label]) => (
            <button
              key={key}
              onClick={() => setTab(key)}
              className={`font-cond text-[10px] font-bold tracking-[0.12em] px-3 py-1 rounded transition-colors ${
                tab === key ? 'text-bright bg-surface border border-border' : 'text-muted hover:text-text'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        {tab === 'incidents' && (
          <div className="flex items-center gap-3 ml-auto">
            {open.length > 0 && (
              <span className="font-mono text-[10px] text-red bg-red/10 border border-red/40 px-2 py-0.5 rounded-full font-bold">
                {open.length} open
              </span>
            )}
            {resolved.length > 0 && (
              <span className="font-mono text-[10px] text-muted border border-border px-2 py-0.5 rounded-full">
                {resolved.length} resolved
              </span>
            )}
          </div>
        )}
        {tab === 'opslog' && (
          <span className={`font-cond text-[9px] tracking-wider ml-auto ${modeColor[systemMode]}`}>
            {systemMode.toUpperCase()} MODE
          </span>
        )}
      </div>

      <div className="overflow-y-auto" style={{ maxHeight: 320 }}>
        {tab === 'incidents' ? (
          <div className="p-2">
            {sorted.length === 0 ? (
              <div className="py-8 text-center">
                <p className="text-dim font-cond text-[11px] tracking-wider">No operational incidents</p>
                <p className="text-dim text-[10px] mt-1">System operating normally</p>
              </div>
            ) : (
              <AnimatePresence initial={false}>
                {sorted.map(inc => (
                  <IncidentRecord
                    key={inc.id}
                    inc={inc}
                    selected={selectedInc === inc.id}
                    onSelect={handleSelect}
                  />
                ))}
              </AnimatePresence>
            )}
          </div>
        ) : (
          <div className="px-3 py-1">
            {opsLog.length === 0 ? (
              <p className="text-dim font-cond text-[11px] tracking-wider py-6 text-center">No operations logged</p>
            ) : (
              [...opsLog].reverse().map(entry => <OpsLogEntry key={entry.id} entry={entry} />)
            )}
          </div>
        )}
      </div>
    </div>
  )
}