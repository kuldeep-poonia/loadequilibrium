import { safe } from './fmt'

export function aggregate(bundles, payload) {
  const svcs = Object.keys(bundles || {})
  if (!svcs.length) return null

  let totalRps = 0, maxLat = 0, maxQueue = 0
  let maxRho = 0, maxRisk = 0, maxBurst = 0
  let hottestSvc = '', hottestRho = 0, degraded = 0

  for (const [svc, b] of Object.entries(bundles)) {
    const q  = b.Queue      || {}
    const st = b.Stochastic || {}
    const sb = b.Stability  || {}

    const arrRate = safe(q.ArrivalRate)         ?? 0
    const wait    = safe(q.MeanWaitMs)          ?? safe(q.MeanSojournMs) ?? 0
    const qLen    = safe(q.MeanQueueLen)        ?? 0
    const util    = safe(q.Utilisation)         ?? 0
    const cRisk   = safe(sb.CollapseRisk)       ?? 0
    const burst   = safe(st.BurstAmplification) ?? 0
    const zone    = (sb.CollapseZone || '').toLowerCase()

    totalRps += arrRate
    if (wait  > maxLat)   maxLat   = wait
    if (qLen  > maxQueue) maxQueue = qLen
    if (util  > maxRho)   maxRho   = util
    if (cRisk > maxRisk)  maxRisk  = cRisk
    if (burst > maxBurst) maxBurst = burst
    if (util  > hottestRho) { hottestRho = util; hottestSvc = svc }
    if (zone === 'collapse' || zone === 'warning' || cRisk > 0.5) degraded++
  }

  const neq     = payload.network_equilibrium || {}
  const satRisk = safe(neq.network_saturation_risk) ?? maxRisk

  const rhoForStatus = Math.min(maxRho, 1.0)
  let status = 'stable'
  if (satRisk > 0.7 || rhoForStatus > 0.9 || degraded >= svcs.length * 0.5 || maxRisk > 0.8)
    status = 'unstable'
  else if (satRisk > 0.35 || rhoForStatus > 0.65 || degraded > 0 || maxRisk > 0.4)
    status = 'degraded'

  return { totalRps, maxLat, maxQueue, maxRho, maxRisk, maxBurst,
           hottestSvc, degraded, satRisk, status,
           serviceCount: svcs.length }
}