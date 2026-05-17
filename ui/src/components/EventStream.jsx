import React, { useRef, useEffect, useState } from 'react'
import { useStore } from '../store/useStore'
import { ts } from '../lib/fmt'
import { SOURCE_COLOR, SOURCE_LABEL } from '../lib/incidents'

const MAX = 200

export default function EventStream() {
  const opsLog  = useStore(s => s.opsLog)
  const payload = useStore(s => s.payload)
  const [paused, setPaused] = useState(false)
  const [filter, setFilter] = useState('all')
  const containerRef = useRef(null)

  const rawEvents = (payload?.events || [])
    .filter(e => e?.description)
    .map(e => ({
      id: `evt-${e.description}-${e.service_id}`,
      at: new Date(e.timestamp || Date.now()).getTime(),
      source: 'telemetry',
      service: e.service_id || null,
      text: e.description,
      severity: e.severity >= 2 ? 'critical' : e.severity === 1 ? 'warning' : 'info',
    }))

  const combined = [...opsLog, ...rawEvents]
    .sort((a, b) => b.at - a.at)
    .slice(0, MAX)

  const filtered = filter === 'all' ? combined
    : combined.filter(e => e.source === filter)

  const SEV_COLOR = { critical: 'text-red', warning: 'text-yellow', info: 'text-muted' }

  return (
    <div className="bg-surface border border-border rounded overflow-hidden flex flex-col">
      <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-3 flex-shrink-0">
        <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">EVENT STREAM</span>
        <div className="flex gap-1 ml-2">
          {[['all','ALL'],['telemetry','TELEMETRY'],['reasoning','AUTOPILOT'],['control','CONTROL'],['human','OPERATOR']].map(([k,l]) => (
            <button
              key={k}
              onClick={() => setFilter(k)}
              className={`font-cond text-[9px] font-bold tracking-wider px-2 py-0.5 rounded border transition-colors ${
                filter === k ? 'text-bright border-borderhi bg-surface' : 'text-dim border-transparent hover:text-muted'
              }`}
            >
              {l}
            </button>
          ))}
        </div>
        <label className="flex items-center gap-1.5 ml-auto cursor-pointer">
          <input type="checkbox" className="hidden" checked={paused} onChange={e => setPaused(e.target.checked)} />
          <span className={`font-cond text-[9px] font-bold tracking-wider px-2 py-0.5 rounded border transition-colors cursor-pointer ${
            paused ? 'text-yellow border-yellow bg-yellow/10' : 'text-dim border-border hover:text-muted'
          }`}>
            {paused ? '⏸ PAUSED' : 'PAUSE'}
          </span>
        </label>
      </div>
      <div ref={containerRef} className="overflow-y-auto font-mono" style={{ maxHeight: 180 }}>
        {filtered.length === 0 ? (
          <p className="text-dim font-cond text-[11px] tracking-wider py-6 text-center">No events</p>
        ) : filtered.map((entry, i) => (
          <div key={entry.id || i} className="flex items-start gap-2 px-3 py-1.5 border-b border-border/20 last:border-0 hover:bg-surface2 transition-colors">
            <span className="text-dim text-[10px] flex-shrink-0 pt-px">
              {new Date(entry.at).toLocaleTimeString('en-GB', { hour12: false })}
            </span>
            <span className={`font-cond text-[8px] font-bold tracking-wider flex-shrink-0 px-1.5 py-0.5 rounded border mt-px whitespace-nowrap ${SOURCE_COLOR[entry.source] || ''}`}>
              {SOURCE_LABEL[entry.source] || entry.source}
            </span>
            {entry.service && (
              <span className="text-cyan text-[10px] flex-shrink-0">[{entry.service}]</span>
            )}
            <span className={`text-[11px] flex-1 leading-tight ${SEV_COLOR[entry.severity] || 'text-text'}`}>
              {entry.text}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}