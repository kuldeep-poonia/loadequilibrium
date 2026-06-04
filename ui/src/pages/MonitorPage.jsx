import EventStream from '../components/EventStream'
import React, { useMemo, useState } from 'react'
import { useStore } from '../store/useStore'
import { serviceName, serviceDesc, ZONE_TEXT, METRIC_HELP, isTestService } from '../lib/names'
import { LIFECYCLE_COLOR, LIFECYCLE_LABEL, SOURCE_LABEL, SOURCE_COLOR } from '../lib/incidents'
import { n, pct, rho as fmtRho, ms, ts } from '../lib/fmt'
import { safe } from '../lib/fmt'
import Charts from '../components/MetricCharts'
import { motion, AnimatePresence } from 'framer-motion'
import IntelPanel from '../components/IntelRow'
import ServiceTable from '../components/ServiceTable'

// ── Service health cards ──────────────────────────────────────────
function ServiceCard({ svcId, bundle, prevBundle }) {
  const q   = bundle.Queue      || {}
  const st  = bundle.Stochastic || {}
  const sb  = bundle.Stability  || {}
  const prev= (prevBundle?.Queue || {})

  const util  = safe(q.Utilisation)         ?? 0
  const lat   = safe(q.MeanWaitMs)           ?? 0
  const qlen  = safe(q.MeanQueueLen)         ?? 0
  const risk  = safe(sb.CollapseRisk)        ?? 0
  const burst = safe(st.BurstAmplification)  ?? 0
  const arrRate=safe(q.ArrivalRate)          ?? 0
  const zone  = (sb.CollapseZone || '').toLowerCase()
  const prevUtil = safe(prev.Utilisation)    ?? util

  const delta = util - prevUtil
  const trend = delta > 0.005 ? '▲' : delta < -0.005 ? '▼' : '─'
  const trendColor = delta > 0.005 ? 'text-red' : delta < -0.005 ? 'text-green' : 'text-dim'

  const isTest = isTestService(svcId)
  const statusColor = zone === 'collapse' || risk > 0.7 ? 'border-red bg-red/5'
    : zone === 'warning' || util > 0.65              ? 'border-yellow bg-yellow/5'
    : 'border-border bg-surface'

  const statusDot = zone === 'collapse' || risk > 0.7 ? 'bg-red animate-blink'
    : zone === 'warning' || util > 0.65              ? 'bg-yellow'
    : arrRate > 0                                    ? 'bg-green'
    : 'bg-dim'

  const utilPct = Math.min(util * 100, 999)
  const riskPct = risk * 100

  return (
    <div className={`border rounded-lg p-4 transition-colors duration-500 ${statusColor}`}>
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="flex items-center gap-2 mb-0.5">
            <span className={`w-2 h-2 rounded-full flex-shrink-0 ${statusDot}`} />
            <h3 className="text-bright font-semibold text-[13px]">{serviceName(svcId)}</h3>
            {isTest && (
              <span className="font-cond text-[9px] px-1.5 py-0.5 rounded bg-dim text-muted border border-border">
                TEST
              </span>
            )}
          </div>
          <p className="text-dim text-[10px] ml-4">{serviceDesc(svcId)}</p>
        </div>
        <div className="text-right">
          <span className={`font-cond text-[10px] font-bold ${trendColor}`}>{trend}</span>
          <p className="text-dim text-[9px] mt-0.5">{utilPct.toFixed(0)}% loaded</p>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-3 mb-3">
        {[
          { label: 'Traffic',       val: `${n(arrRate,1)}/s`,   help: METRIC_HELP.rps   },
          { label: 'Wait Time',     val: ms(lat),                help: METRIC_HELP.lat   },
          { label: 'Queue Backlog', val: n(qlen,0),              help: METRIC_HELP.queue },
        ].map(m => (
          <div key={m.label} title={m.help} className="cursor-help">
            <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase">{m.label}</p>
            <p className="font-mono text-[13px] font-semibold text-text mt-0.5">{m.val}</p>
          </div>
        ))}
      </div>

      <div className="space-y-2">
        <div title={METRIC_HELP.rho}>
          <div className="flex justify-between items-center mb-1">
            <span className="font-cond text-[9px] text-muted uppercase tracking-wider">Capacity Used</span>
            <span className={`font-mono text-[11px] font-semibold ${util > 1 ? 'text-red' : util > 0.65 ? 'text-yellow' : 'text-green'}`}>
              {fmtRho(util)}
            </span>
          </div>
          <div className="h-1.5 bg-dim rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all duration-700 ${util > 1 ? 'bg-red' : util > 0.65 ? 'bg-yellow' : 'bg-green'}`}
              style={{ width: `${Math.min(utilPct, 100)}%` }}
            />
          </div>
        </div>

        {risk > 0.1 && (
          <div title={METRIC_HELP.risk}>
            <div className="flex justify-between items-center mb-1">
              <span className="font-cond text-[9px] text-muted uppercase tracking-wider">Failure Risk</span>
              <span className={`font-mono text-[11px] font-semibold ${risk > 0.7 ? 'text-red' : 'text-yellow'}`}>
                {pct(risk)}
              </span>
            </div>
            <div className="h-1 bg-dim rounded-full overflow-hidden">
              <div className="h-full rounded-full bg-red/70 transition-all duration-700" style={{ width: `${riskPct}%` }} />
            </div>
          </div>
        )}
      </div>

      <div className="mt-3 pt-2 border-t border-border/40">
        <p className="text-[10px] text-muted leading-snug">{ZONE_TEXT[zone] || ZONE_TEXT['']}</p>
      </div>
    </div>
  )
}

// ── Incident timeline (read-only) ─────────────────────────────────
function IncidentTimeline() {
  const incidents   = useStore(s => s.incidents)
  const [expanded, setExpanded] = React.useState(null)

  const sorted = [...incidents].sort((a, b) => b.createdAt - a.createdAt)

  if (sorted.length === 0) return (
    <div className="bg-surface border border-border rounded-lg p-6 text-center">
      <p className="text-dim font-cond text-[12px] tracking-wider">No incidents recorded</p>
      <p className="text-dim text-[10px] mt-1">System is operating normally</p>
    </div>
  )

  return (
    <div className="space-y-2">
      {sorted.map(inc => {
        const isOpen = expanded === inc.id
        const isResolved = ['resolved','failed','overridden'].includes(inc.stage)
        const sevColor = inc.severity === 'critical' ? 'border-l-red' : inc.severity === 'warning' ? 'border-l-yellow' : 'border-l-cyan'
        const age = Date.now() - inc.createdAt
        const ageStr = age < 60000 ? `${Math.round(age/1000)}s ago` : `${Math.round(age/60000)}m ago`

        return (
          <motion.div key={inc.id} layout initial={{ opacity:0, y:-4 }} animate={{ opacity:1, y:0 }}
            className={`bg-surface border border-l-2 ${sevColor} rounded-r-lg overflow-hidden transition-opacity ${isResolved ? 'opacity-60' : ''}`}
          >
            <button
              onClick={() => setExpanded(isOpen ? null : inc.id)}
              className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-surface2 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap mb-0.5">
                  {inc.affectedServices.map(s => (
                    <span key={s} className="font-semibold text-cyan text-[12px]">{serviceName(s)}</span>
                  ))}
                  <span className={`font-cond text-[9px] px-1.5 py-0.5 rounded border font-bold ${LIFECYCLE_COLOR[inc.stage]}`}>
                    {LIFECYCLE_LABEL[inc.stage]}
                  </span>
                  {inc.humanOverride && (
                    <span className="font-cond text-[9px] px-1.5 py-0.5 rounded border border-purple/40 text-purple">
                      OPERATOR ACTION
                    </span>
                  )}
                </div>
                <p className="text-text text-[11px] leading-snug truncate">{inc.problem}</p>
              </div>
              <span className="text-dim text-[10px] flex-shrink-0">{ageStr}</span>
              <span className="text-muted text-[11px]">{isOpen ? '▲' : '▼'}</span>
            </button>

            <AnimatePresence>
              {isOpen && (
                <motion.div
                  initial={{ height:0, opacity:0 }} animate={{ height:'auto', opacity:1 }}
                  exit={{ height:0, opacity:0 }} transition={{ duration:0.2 }}
                  className="border-t border-border/40 overflow-hidden"
                >
                  <div className="px-4 py-3 grid grid-cols-2 gap-4">
                    <div>
                      <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase mb-1">What happened</p>
                      <p className="text-text text-[11px] leading-snug">{inc.problem}</p>
                    </div>
                    {inc.prediction && (
                      <div>
                        <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase mb-1">What the engine predicted</p>
                        <p className="text-text text-[11px] leading-snug">
                          {inc.prediction.split('→').map((s,i,arr) => (
                            <React.Fragment key={i}>
                              <span className={i===0?'text-yellow':i===arr.length-1?'text-red':'text-text'}>{s.trim()}</span>
                              {i<arr.length-1 && <span className="text-dim mx-1">→</span>}
                            </React.Fragment>
                          ))}
                        </p>
                      </div>
                    )}
                    {inc.decision && (
                      <div>
                        <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase mb-1">Action taken</p>
                        <p className="text-cyan text-[11px]">{inc.decision}</p>
                      </div>
                    )}
                    {inc.outcome && (
                      <div>
                        <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase mb-1">Result</p>
                        <p className="text-green text-[11px]">{inc.outcome}</p>
                      </div>
                    )}
                  </div>

                  <div className="px-4 py-3 border-t border-border/40">
                    <p className="font-cond text-[9px] font-bold tracking-wider text-muted uppercase mb-2">Timeline</p>
                    <div className="space-y-1.5">
                      {inc.entries.map((entry, i) => (
                        <div key={i} className="flex items-start gap-2 text-[10px]">
                          <span className="text-dim font-mono flex-shrink-0">
                            {new Date(entry.at).toLocaleTimeString('en-GB',{hour12:false})}
                          </span>
                          <span className={`font-cond text-[8px] font-bold tracking-wider flex-shrink-0 px-1.5 py-0.5 rounded border whitespace-nowrap ${SOURCE_COLOR[entry.source]?.split(' ').slice(0,2).join(' ') || ''}`}>
                            {SOURCE_LABEL[entry.source] || entry.source}
                          </span>
                          <span className="text-muted flex-1 leading-tight">{entry.note}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </motion.div>
        )
      })}
    </div>
  )
}

// ── Autopilot reasoning feed ──────────────────────────────────────
function ReasoningFeed() {
  const payload = useStore(s => s.payload)
  const events  = useMemo(() => {
    return (payload?.events || [])
      .filter(e => e?.description)
      .sort((a,b) => b.severity - a.severity || b.operational_priority - a.operational_priority)
      .slice(0, 15)
  }, [payload])

  return (
    <div className="bg-surface border border-border rounded-lg overflow-hidden">
      <div className="px-4 py-3 border-b border-border bg-surface2">
        <h3 className="font-cond text-[11px] font-bold tracking-wider text-muted">ENGINE REASONING</h3>
        <p className="text-dim text-[10px] mt-0.5">What the autopilot is thinking right now</p>
      </div>
      <div className="divide-y divide-border/30 max-h-64 overflow-y-auto">
        {events.length === 0 ? (
          <p className="text-dim text-[11px] font-cond tracking-wider p-4 text-center">
            No active reasoning — system is stable
          </p>
        ) : events.map((e, i) => {
          const sevColor = e.severity >= 2 ? 'text-red' : e.severity === 1 ? 'text-yellow' : 'text-text'
          return (
            <div key={e.id || i} className="px-4 py-3 hover:bg-surface2 transition-colors">
              <div className="flex items-start gap-2">
                <span className="text-dim text-[10px] font-mono flex-shrink-0 pt-0.5">
                  {ts(e.timestamp)}
                </span>
                <div className="flex-1">
                  {e.service_id && (
                    <span className="text-cyan text-[10px] font-semibold mr-1">
                      {serviceName(e.service_id)}:
                    </span>
                  )}
                  <span className={`text-[11px] ${sevColor}`}>{e.description}</span>
                  {e.model_chain && (
                    <p className="text-muted text-[10px] mt-0.5 leading-snug">
                      Prediction: {e.model_chain.split('→').join(' → ')}
                    </p>
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default function MonitorPage() {
  const payload     = useStore(s => s.payload)
  const prevBundles = useStore(s => s.prevBundles)
  const agg         = useStore(s => s.agg)
  const systemMode  = useStore(s => s.systemMode)
  const incidents   = useStore(s => s.incidents)

  const bundles = payload?.bundles || {}
  const svcs    = Object.keys(bundles)

  const openCrit = incidents.filter(i => i.severity === 'critical' && !['resolved','failed','overridden'].includes(i.stage))
  const openWarn = incidents.filter(i => i.severity === 'warning'  && !['resolved','failed','overridden'].includes(i.stage))

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-7xl mx-auto px-4 py-4 space-y-6">

        {/* Situation summary */}
        {agg && (
          <div className={`rounded-lg border p-4 ${
            systemMode === 'emergency' ? 'border-red/40 bg-red/5'
            : systemMode === 'degraded' ? 'border-yellow/40 bg-yellow/5'
            : 'border-green/20 bg-green/5'
          }`}>
            <div className="flex items-center gap-3">
              <span className="text-2xl">
                {systemMode === 'emergency' ? '🔴' : systemMode === 'degraded' ? '🟡' : '🟢'}
              </span>
              <div className="flex-1">
                <h2 className="text-bright font-semibold text-[14px]">
                  {systemMode === 'emergency' ? 'System is in critical condition'
                   : systemMode === 'degraded' ? 'System is under stress'
                   : systemMode === 'offline'  ? 'Waiting for data…'
                   : 'All systems operating normally'}
                </h2>
                <p className="text-muted text-[11px] mt-0.5">
                  {openCrit.length > 0 && `${openCrit.length} critical problem${openCrit.length > 1 ? 's' : ''} active. `}
                  {openWarn.length > 0 && `${openWarn.length} warning${openWarn.length > 1 ? 's' : ''}. `}
                  {openCrit.length === 0 && openWarn.length === 0 && systemMode !== 'offline' && 'No action needed — autopilot is managing the system.'}
                  {systemMode === 'offline' && 'Connect a service to begin monitoring.'}
                </p>
              </div>
              {systemMode !== 'offline' && (
                <div className="flex items-center gap-4 text-right">
                  <div>
                    <p className="font-mono text-[18px] font-bold text-cyan">{agg.serviceCount}</p>
                    <p className="font-cond text-[9px] text-muted tracking-wider">SERVICES</p>
                  </div>
                  <div>
                    <p className={`font-mono text-[18px] font-bold ${agg.degraded > 0 ? 'text-yellow' : 'text-green'}`}>{agg.degraded}</p>
                    <p className="font-cond text-[9px] text-muted tracking-wider">DEGRADED</p>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Live charts */}
        <Charts />

        {/* Intelligence panel: active incidents + causal chain + recommended actions */}
        {/* This surfaces the reasoning engine output that was computed but never displayed */}
        <div>
          <h2 className="font-cond text-[11px] font-bold tracking-wider text-muted uppercase mb-3">
            Intelligence — Active Incidents & Recommended Actions
          </h2>
          <IntelPanel />
        </div>

        {/* Service cards */}
        {svcs.length > 0 && (
          <div>
            <h2 className="font-cond text-[11px] font-bold tracking-wider text-muted uppercase mb-3">
              Connected Services
            </h2>
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
              {svcs.map(id => (
                <ServiceCard
                  key={id}
                  svcId={id}
                  bundle={bundles[id]}
                  prevBundle={prevBundles[id]}
                />
              ))}
            </div>
          </div>
        )}

        {/* Service table: compact tabular view of all services with key metrics */}
        {svcs.length > 0 && (
          <div>
            <h2 className="font-cond text-[11px] font-bold tracking-wider text-muted uppercase mb-3">
              Service Health Table
            </h2>
            <ServiceTable />
          </div>
        )}

        {/* Engine reasoning + incident timeline side by side */}
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
          <ReasoningFeed />
          <div>
            <h2 className="font-cond text-[11px] font-bold tracking-wider text-muted uppercase mb-3">
              Incident History
            </h2>
            <IncidentTimeline />
          </div>
        </div>

        {/* Live event stream */}
        <EventStream />

      </div>
    </div>
  )
}