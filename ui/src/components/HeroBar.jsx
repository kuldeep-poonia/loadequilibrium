import React from 'react'
import { useStore } from '../store/useStore'
import { rps, ms, n, pct, rho } from '../lib/fmt'

function Metric({ label, value, level = 'normal' }) {
  const color = level === 'crit' ? 'text-red' : level === 'warn' ? 'text-yellow' : level === 'ok' ? 'text-green' : 'text-cyan'
  return (
    <div className="flex flex-col justify-center px-4 border-l border-border h-full flex-1 min-w-0 overflow-hidden">
      <span className="font-cond text-[9px] font-bold tracking-[0.14em] text-muted uppercase truncate">{label}</span>
      <span className={`font-mono text-[15px] font-semibold leading-tight truncate transition-colors duration-300 ${color}`}>
        {value}
      </span>
    </div>
  )
}

function lvRho(v)   { return v > 1.0 ? 'crit' : v > 0.65 ? 'warn' : 'normal' }
function lvLat(v)   { return v > 500 ? 'crit' : v > 200 ? 'warn' : 'normal' }
function lvQueue(v) { return v > 100 ? 'crit' : v > 30 ? 'warn' : 'normal' }
function lvRisk(v)  { return v > 0.7 ? 'crit' : v > 0.35 ? 'warn' : 'normal' }

export default function HeroBar() {
  const agg = useStore(s => s.agg)

  if (!agg) return (
    <div className="h-11 bg-surface border-b border-border flex items-center px-4 flex-shrink-0">
      <span className="font-cond text-[11px] tracking-wider text-muted animate-pulse2">
        Waiting for telemetry…
      </span>
    </div>
  )

  return (
    <div className="h-11 bg-surface border-b border-border flex items-stretch flex-shrink-0 overflow-hidden">
      <Metric label="Req / sec"     value={rps(agg.totalRps)}      level="normal" />
      <Metric label="Queue Wait"    value={ms(agg.maxLat)}          level={lvLat(agg.maxLat)} />
      <Metric label="Queue Depth"   value={n(agg.maxQueue, 0)}      level={lvQueue(agg.maxQueue)} />
      <Metric label="Collapse Risk" value={pct(agg.maxRisk)}        level={lvRisk(agg.maxRisk)} />
      <Metric label="Utilisation ρ" value={rho(agg.maxRho)}         level={lvRho(agg.maxRho)} />
      <Metric label="Services"      value={agg.serviceCount}         level="normal" />
      <Metric label="Degraded"      value={agg.degraded}             level={agg.degraded > 2 ? 'crit' : agg.degraded > 0 ? 'warn' : 'ok'} />
      <Metric label="Hottest"       value={agg.hottestSvc || '—'}   level="normal" />
    </div>
  )
}