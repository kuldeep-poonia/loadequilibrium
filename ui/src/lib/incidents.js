export const LIFECYCLE = ['detected','analyzing','predicted','action_selected','executing','stabilizing','resolved','failed','overridden']

export const LIFECYCLE_LABEL = {
  detected:        'Detected',
  analyzing:       'Analyzing',
  predicted:       'Predicted',
  action_selected: 'Action Selected',
  executing:       'Executing',
  stabilizing:     'Stabilizing',
  resolved:        'Resolved',
  failed:          'Failed',
  overridden:      'Overridden by Operator',
}

export const LIFECYCLE_COLOR = {
  detected:        'text-yellow border-yellow',
  analyzing:       'text-cyan   border-cyan',
  predicted:       'text-purple border-purple',
  action_selected: 'text-cyan   border-cyan',
  executing:       'text-yellow border-yellow',
  stabilizing:     'text-green  border-green',
  resolved:        'text-green  border-green',
  failed:          'text-red    border-red',
  overridden:      'text-muted  border-border',
}

export const SOURCE_LABEL = {
  telemetry:  'TELEMETRY',
  reasoning:  'AUTOPILOT REASONING',
  control:    'CONTROL DECISION',
  human:      'OPERATOR ACTION',
  simulation: 'SIMULATION FORECAST',
}

export const SOURCE_COLOR = {
  telemetry:  'text-text   bg-dim/40      border-borderhi',
  reasoning:  'text-cyan   bg-cyan/10     border-cyan/40',
  control:    'text-yellow bg-yellow/10   border-yellow/40',
  human:      'text-purple bg-purple/10   border-purple/40',
  simulation: 'text-muted  bg-surface2    border-border',
}

let _seq = 0
export function mkId() { return `inc-${Date.now()}-${++_seq}` }

export function incidentFromEvent(e, directives) {
  const id       = mkId()
  const now      = Date.now()
  const sevMap   = { 2: 'critical', 1: 'warning', 0: 'info' }
  const severity = sevMap[e.severity] ?? 'info'
  const svc      = e.service_id

  // Determine what lifecycle stage the event represents
  const hasAction = !!e.recommendation
  const hasPrediction = !!(e.model_chain)
  const dirForSvc = svc ? directives?.[svc] : null
  const hasControl = dirForSvc && Math.abs((dirForSvc.ScaleFactor ?? 1) - 1.0) > 0.02

  let stage = 'detected'
  if (hasPrediction) stage = 'predicted'
  if (hasAction)     stage = 'action_selected'
  if (hasControl)    stage = 'executing'

  const entries = [{ at: now, stage, note: e.description, source: 'reasoning' }]
  if (hasControl && stage === 'executing') {
    const sf = dirForSvc.ScaleFactor ?? 1
    entries.push({
      at: now + 1, stage: 'executing',
      note: `Autopilot applying scale factor ${sf.toFixed(2)} to ${svc}`,
      source: 'control',
    })
  }

  return {
    id, severity,
    affectedServices: svc ? [svc] : [],
    title: e.description,
    problem:    e.description,
    prediction: hasPrediction ? (e.model_chain ?? null) : null,
    decision:   hasAction     ? e.recommendation       : hasControl ? `Scale ${svc}` : null,
    outcome:    null,
    confidence: e.confidence ?? null,
    causalChain: e.model_chain ?? null,
    recommendation: e.recommendation ?? null,
    stage,
    entries,
    createdAt:  now,
    updatedAt:  now,
    resolvedAt: null,
    fingerprint: `${e.description}|${svc}`,
    source: 'reasoning',
    humanOverride: false,
    simulationImpact: e.simulation_impact ?? null,
  }
}

export function shouldResolve(inc, agg) {
  if (!agg) return false
  if (inc.stage === 'resolved' || inc.stage === 'failed' || inc.stage === 'overridden') return false
  if (inc.severity === 'critical' && agg.maxRisk < 0.3 && agg.maxRho < 0.6) return true
  if (inc.severity === 'warning'  && agg.maxRisk < 0.2 && agg.maxRho < 0.5) return true
  return false
}

export function advanceStage(inc, agg) {
  const order = LIFECYCLE
  const cur   = order.indexOf(inc.stage)
  if (cur < 0 || inc.stage === 'resolved' || inc.stage === 'failed' || inc.stage === 'overridden') return inc

  let next = inc.stage
  const now = Date.now()

  if (inc.stage === 'action_selected') next = 'executing'
  else if (inc.stage === 'executing')  next = 'stabilizing'
  else if (inc.stage === 'stabilizing' && shouldResolve(inc, agg)) next = 'resolved'

  if (next === inc.stage) return inc
  return {
    ...inc,
    stage: next,
    updatedAt: now,
    resolvedAt: next === 'resolved' ? now : inc.resolvedAt,
    entries: [...inc.entries, { at: now, stage: next, note: `Stage advanced to ${LIFECYCLE_LABEL[next]}`, source: 'control' }],
  }
}

export function deriveSystemMode(agg, incidents) {
  if (!agg) return 'offline'
  const openCrit = incidents.filter(i => i.severity === 'critical' && !['resolved','failed','overridden'].includes(i.stage))
  if (openCrit.length > 0 || agg.maxRisk > 0.7 || agg.maxRho > 0.9) return 'emergency'
  if (agg.status === 'degraded' || agg.satRisk > 0.35) return 'degraded'
  if (agg.status === 'stable')   return 'normal'
  return 'offline'
}