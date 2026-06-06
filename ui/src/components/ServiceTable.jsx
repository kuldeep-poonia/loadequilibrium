import React, { useMemo } from 'react'
import { useStore } from '../store/useStore'
import { safe, n, ms, pct, rho } from '../lib/fmt'

function Bar({ val, max = 1, level }) {
  const pct2 = Math.min(100, ((safe(val) ?? 0) / max) * 100)
  const bg   = level === 'crit' ? 'bg-red' : level === 'warn' ? 'bg-yellow' : 'bg-green'
  return (
    <div className="flex items-center gap-1.5">
      <div className="w-12 h-1 bg-dim rounded-full overflow-hidden flex-shrink-0">
        <div className={`h-full rounded-full transition-all duration-500 ${bg}`} style={{ width: `${pct2}%` }} />
      </div>
    </div>
  )
}

const ZONE_STYLE = {
  collapse: 'text-red    border-red    bg-red/10',
  warning:  'text-yellow border-yellow bg-yellow/10',
  safe:     'text-green  border-green  bg-green/10',
  '':       'text-muted  border-border bg-transparent',
}

export default function ServiceTable() {
  const payload     = useStore(s => s.payload)
  const prevBundles = useStore(s => s.prevBundles)

  const rows = useMemo(() => {
    if (!payload) return []
    const bundles = payload.bundles || {}
    const prq     = payload.priority_risk_queue || []
    const ordered = [
      ...prq.map(r => r.service_id).filter(id => bundles[id]),
      ...Object.keys(bundles).filter(id => !prq.find(r => r.service_id === id)),
    ]
    return ordered.map(svc => {
      const b  = bundles[svc] || {}
      const q  = b.Queue      || {}
      const st = b.Stochastic || {}
      const sb = b.Stability  || {}
      const prev = (prevBundles?.[svc] || {}).Queue || {}

      const util  = safe(q.Utilisation)         ?? 0
      const lat   = safe(q.MeanWaitMs)           ?? safe(q.MeanSojournMs) ?? 0
      const qlen  = safe(q.MeanQueueLen)         ?? 0
      const risk  = safe(sb.CollapseRisk)        ?? 0
      const burst = safe(st.BurstAmplification)  ?? 0
      const zone  = (sb.CollapseZone || '').toLowerCase()
      const arrRate = safe(q.ArrivalRate)        ?? 0
      const prevRho = safe(prev.Utilisation)     ?? util
      const delta   = util - prevRho

      let statusLabel, statusDot
      if (zone === 'collapse' || risk > 0.7)     { statusLabel = 'CRITICAL'; statusDot = 'bg-red animate-blink' }
      else if (zone === 'warning' || util > 0.65) { statusLabel = 'DEGRADED'; statusDot = 'bg-yellow' }
      else if (arrRate > 0 || util > 0)           { statusLabel = 'HEALTHY';  statusDot = 'bg-green' }
      else                                        { statusLabel = 'WAITING';  statusDot = 'bg-dim' }

      const rhoLv  = util > 1.0 ? 'crit' : util > 0.65 ? 'warn' : 'ok'
      const riskLv = risk > 0.7 ? 'crit' : risk > 0.35 ? 'warn' : 'ok'
      const zoneStyle = ZONE_STYLE[zone] || ZONE_STYLE['']
      const trend = delta > 0.005 ? { icon: '▲', cls: 'text-red' }
                  : delta < -0.005 ? { icon: '▼', cls: 'text-green' }
                  : { icon: '—', cls: 'text-muted' }

      const prqItem = prq.find(r => r.service_id === svc)
      const urgency = prqItem?.urgency_class || ''

      return { svc, statusLabel, statusDot, arrRate, lat, qlen, util, risk, burst, zone, zoneStyle, rhoLv, riskLv, trend, urgency }
    })
  }, [payload, prevBundles])

  return (
    <div className="bg-surface border border-border rounded overflow-hidden flex flex-col">
      <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-2 flex-shrink-0">
        <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">SERVICE HEALTH</span>
        <span className="font-cond text-[10px] text-dim ml-auto">{rows.length} service{rows.length !== 1 ? 's' : ''} · live</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[11px]">
          <thead>
            <tr className="bg-surface2 border-b border-border">
              {['Service','Status','Req/s','Wait','Queue','Utilisation','Risk','Burst','Zone','Trend'].map(h => (
                <th key={h} className="text-left px-3 py-2 font-cond text-[9px] font-bold tracking-[0.12em] text-muted uppercase whitespace-nowrap">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr><td colSpan={10} className="text-center py-8 font-cond text-[11px] text-dim tracking-wider">
                Waiting for telemetry…
              </td></tr>
            ) : rows.map(r => (
              <tr key={r.svc} className="border-b border-border/50 hover:bg-surface2 transition-colors duration-150">
                <td className="px-3 py-2 font-mono font-semibold text-cyan whitespace-nowrap">{r.svc}</td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-1.5">
                    <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${r.statusDot}`} />
                    <span className={r.statusLabel === 'CRITICAL' ? 'text-red font-semibold' : r.statusLabel === 'DEGRADED' ? 'text-yellow' : r.statusLabel === 'HEALTHY' ? 'text-green' : 'text-muted'}>
                      {r.statusLabel}
                    </span>
                  </div>
                </td>
                <td className="px-3 py-2 text-text">{n(r.arrRate, 1)}/s</td>
                <td className="px-3 py-2 text-text">{ms(r.lat)}</td>
                <td className="px-3 py-2 text-text">{n(r.qlen, 0)}</td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    <Bar val={r.util} max={1} level={r.rhoLv} />
                    <span className={r.rhoLv === 'crit' ? 'text-red' : r.rhoLv === 'warn' ? 'text-yellow' : 'text-text'}>{rho(r.util)}</span>
                  </div>
                </td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    <Bar val={r.risk} max={1} level={r.riskLv} />
                    <span className={r.riskLv === 'crit' ? 'text-red' : r.riskLv === 'warn' ? 'text-yellow' : 'text-text'}>{pct(r.risk)}</span>
                  </div>
                </td>
                <td className="px-3 py-2" style={{ color: r.burst > 2 ? '#ff3c3c' : r.burst > 1.3 ? '#f5c400' : '#00d488' }}>
                  {n(r.burst, 2)}×
                </td>
                <td className="px-3 py-2">
                  <span className={`font-cond text-[9px] font-bold tracking-wider px-1.5 py-0.5 rounded border ${r.zoneStyle}`}>
                    {(r.zone || 'unknown').toUpperCase()}
                  </span>
                </td>
                <td className={`px-3 py-2 ${r.trend.cls}`}>{r.trend.icon}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}