import React, { useEffect, useState } from 'react'
import { useStore } from '../store/useStore'

const STATUS = {
  stable:   { label: '● STABLE',   cls: 'text-green  border-green  bg-green/10'  },
  degraded: { label: '◐ DEGRADED', cls: 'text-yellow border-yellow bg-yellow/10' },
  unstable: { label: '■ UNSTABLE', cls: 'text-red    border-red    bg-red/10'    },
  waiting:  { label: '◌ NO DATA',  cls: 'text-muted  border-border bg-transparent' },
}

export default function Topbar() {
  const connected = useStore(s => s.connected)
  const tick      = useStore(s => s.tick)
  const agg       = useStore(s => s.agg)
  const safetyMode= useStore(s => s.payload?.safety_mode)
  const [clock, setClock] = useState('')

  useEffect(() => {
    const t = setInterval(() => setClock(new Date().toLocaleTimeString('en-GB', { hour12: false })), 1000)
    return () => clearInterval(t)
  }, [])

  const status = agg?.status ?? 'waiting'
  const st     = STATUS[status]

  return (
    <header className="h-9 bg-surface border-b border-border flex items-center px-4 gap-3 flex-shrink-0 z-10">
      <svg viewBox="0 0 32 32" width="22" height="22" className="flex-shrink-0">
        <circle cx="16" cy="16" r="15" fill="none" stroke="#00b8f5" strokeWidth="1.5" opacity="0.6"/>
        <circle cx="16" cy="16" r="11" fill="#00304d" opacity="0.9"/>
        <text x="16" y="21" textAnchor="middle" fill="#00b8f5"
          fontFamily="Barlow Condensed" fontWeight="700" fontSize="10" letterSpacing="0.5">LE</text>
      </svg>
      <span className="font-cond text-[13px] font-semibold tracking-widest text-text/60">LoadEquilibrium</span>
      <div className="w-px h-4 bg-border mx-1" />
      <span className={`font-cond text-[10px] font-bold tracking-wider px-2 py-0.5 rounded border transition-all duration-500 ${st.cls}`}>
        {st.label}
      </span>
      {safetyMode && (
        <span className="font-cond text-[9px] font-bold tracking-wider text-yellow border border-yellow px-2 py-0.5 rounded">
          ⚠ SAFETY MODE
        </span>
      )}
      <div className="flex-1" />
      <div className="flex items-center gap-4 text-muted">
        <span className="font-cond text-[10px] tracking-wider">TICK
          <span className="text-cyan font-mono ml-1">{tick?.toLocaleString() ?? '—'}</span>
        </span>
        <div className="w-px h-4 bg-border" />
        <div className="flex items-center gap-1.5">
          <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${connected ? 'bg-green animate-pulse2' : 'bg-muted'}`} />
          <span className="font-cond text-[10px] tracking-wider">{connected ? 'LIVE' : 'OFFLINE'}</span>
        </div>
        <div className="w-px h-4 bg-border" />
        <span className="font-mono text-[11px] text-bright">{clock}</span>
      </div>
    </header>
  )
}