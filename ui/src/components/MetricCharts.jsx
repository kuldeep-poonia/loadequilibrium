import React from 'react'
import { AreaChart, Area, ResponsiveContainer, ReferenceLine, Tooltip } from 'recharts'
import { useStore } from '../store/useStore'
import { rps, ms, n, pct, rho } from '../lib/fmt'

const CHARTS = [
  { key: 'rps',   label: 'Requests / sec',    color: '#00b8f5', fmt: rps,               threshold: null   },
  { key: 'lat',   label: 'Queue Wait',         color: '#f5c400', fmt: v => ms(v),        threshold: 500    },
  { key: 'queue', label: 'Queue Depth',        color: '#f09800', fmt: v => n(v, 0),      threshold: 100    },
  { key: 'rho',   label: 'Utilisation ρ',      color: '#9d72ff', fmt: v => rho(v),       threshold: 0.85   },
  { key: 'risk',  label: 'Collapse Risk',      color: '#ff3c3c', fmt: pct,               threshold: 0.7    },
  { key: 'burst', label: 'Burst Amplification',color: '#00d488', fmt: v => n(v, 2) + '×', threshold: 2.0  },
]

const CustomTooltip = ({ active, payload, fmt }) => {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-surface2 border border-border px-2 py-1 rounded text-[10px] font-mono text-bright">
      {fmt(payload[0].value)}
    </div>
  )
}

function Sparkline({ cfg, data }) {
  const pts   = data.map((v, i) => ({ v }))
  const last  = data.length ? data[data.length - 1] : null
  const max   = Math.max(...data, 0.001)

  return (
    <div className="bg-surface rounded border border-border p-3 flex flex-col gap-1.5">
      <div className="flex items-center justify-between">
        <span className="font-cond text-[9px] font-bold tracking-[0.14em] text-muted uppercase">
          {cfg.label}
        </span>
        <span className="font-mono text-[13px] font-semibold" style={{ color: cfg.color }}>
          {last !== null ? cfg.fmt(last) : '—'}
        </span>
      </div>
      <div className="h-14">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={pts} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id={`g-${cfg.key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%"  stopColor={cfg.color} stopOpacity={0.25} />
                <stop offset="95%" stopColor={cfg.color} stopOpacity={0}    />
              </linearGradient>
            </defs>
            {cfg.threshold !== null && cfg.threshold <= max && (
              <ReferenceLine y={cfg.threshold} stroke="rgba(255,60,60,0.4)" strokeDasharray="3 3" />
            )}
            <Tooltip
              content={<CustomTooltip fmt={cfg.fmt} />}
              cursor={{ stroke: cfg.color, strokeWidth: 1, strokeOpacity: 0.4 }}
            />
            <Area
              type="monotone" dataKey="v" stroke={cfg.color} strokeWidth={1.5}
              fill={`url(#g-${cfg.key})`} dot={false} isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}

export default function Charts() {
  const history = useStore(s => s.history)

  return (
    <div className="bg-surface border border-border rounded overflow-hidden">
      <div className="px-3 py-2 border-b border-border bg-surface2 flex items-center gap-2">
        <span className="font-cond text-[10px] font-bold tracking-[0.14em] text-muted">LIVE METRICS</span>
      </div>
      <div className="grid grid-cols-3 gap-px bg-border p-px">
        {CHARTS.map(cfg => (
          <Sparkline key={cfg.key} cfg={cfg} data={history[cfg.key]} />
        ))}
      </div>
    </div>
  )
}