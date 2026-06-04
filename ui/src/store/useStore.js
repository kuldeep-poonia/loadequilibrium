import { incidentFromEvent, shouldResolve, advanceStage, deriveSystemMode, mkId, LIFECYCLE_LABEL } from '../lib/incidents'
import { create } from 'zustand'
import { aggregate } from '../lib/agg'

const MAX_HIST = 60

// Derive WebSocket URL from current page location so it works on any port and any host.
// - In production (Go binary serving UI on :8080): connects to same host+port
// - In Vite dev mode: vite.config.js proxy forwards /ws to backend
// - No hardcoded port - changing LE_PORT env var works automatically
const _proto  = location.protocol === 'https:' ? 'wss:' : 'ws:'
const WS_URL  = `${_proto}//${location.host}/ws`

const INIT_HIST = { rps: [], lat: [], queue: [], rho: [], risk: [], burst: [] }

const push = (arr, v) => {
  const next = [...arr, isFinite(v) ? v : 0]
  return next.length > MAX_HIST ? next.slice(-MAX_HIST) : next
}

export const useStore = create((set, get) => ({
  connected:    false,
  tick:         null,
  payload:      null,
  agg:          null,
  history:      INIT_HIST,
  prevBundles:  {},
  actuationEnabled: true,  // synced from backend control_plane.actuation_enabled on each tick
  toasts:       [],
  pendingAction:null,
  incidents:    [],   // persistent incident records
  opsLog:       [],   // immutable operations timeline
  systemMode:   'offline', // offline|normal|degraded|emergency
  selectedInc:  null, // id of incident open in detail drawer
  _incFP:       new Set(), // fingerprint dedup within window
  thresholds:   { latWarn:false, queueWarn:false, riskWarn:false, rhoWarn:false },
  prevStatus:   'waiting',

  _ws: null,
  _watchdog: null,
  _reconnTimer: null,

  connect() {
    const state = get()
    if (state._ws) { try { state._ws.close() } catch(e) {} }
    clearTimeout(state._reconnTimer)

    const ws = new WebSocket(WS_URL)

    ws.onopen = () => {
      set({ connected: true })
      get().addToast('info', 'Connected', `Live feed from ${WS_URL}`, 3000)
      get()._resetWatchdog()
    }

    ws.onmessage = (e) => {
      try {
        const p = JSON.parse(e.data)
        if (p.type === 'ping') return
        get()._ingest(p)
        get()._resetWatchdog()
      } catch(err) { console.error('ws parse', err) }
    }

    ws.onclose = () => {
      set({ connected: false })
      get()._scheduleReconnect()
    }

    ws.onerror = () => {
      set({ connected: false })
    }

    set({ _ws: ws })
  },

  _scheduleReconnect() {
    const t = setTimeout(() => get().connect(), 3000)
    set({ _reconnTimer: t })
  },

  _resetWatchdog() {
    const state = get()
    clearTimeout(state._watchdog)
    const w = setTimeout(() => {
      if (!get().connected) return
      get().addToast('warn', 'No data', 'No tick in 20s — reconnecting', 3000)
      try { get()._ws?.close() } catch(e) {}
      get().connect()
    }, 20000)
    set({ _watchdog: w })
  },

  _ingest(payload) {
    const bundles = payload.bundles || {}
    const agg     = aggregate(bundles, payload)
    const tick    = payload.control_plane?.tick ?? null
    // Sync autopilot enabled state from the backend truth (control_plane.actuation_enabled)
    // so the UI always reflects reality even after a backend restart or API call from another client.
    const actuationEnabled = payload.control_plane?.actuation_enabled ?? true

    set(s => {
      const h = s.history
      const newHist = agg ? {
        rps:   push(h.rps,   agg.totalRps),
        lat:   push(h.lat,   agg.maxLat),
        queue: push(h.queue, agg.maxQueue),
        rho:   push(h.rho,   Math.min(agg.maxRho, 1.0)),
        risk:  push(h.risk,  agg.maxRisk),
        burst: push(h.burst, agg.maxBurst),
      } : h
      return { payload, agg, tick, actuationEnabled, history: newHist, prevBundles: s.payload?.bundles || {} }
    })

    if (agg) {
      get()._checkThresholds(agg)
      get()._checkStatus(agg.status)
      get()._processIncidents(payload, agg)
    }
  },

  _processIncidents(payload, agg) {
    const events = (payload.events || []).filter(e => e?.description && e.severity >= 1)
    const dirs   = payload.directives || {}
    const now    = Date.now()
    const KEEP   = 5 * 60 * 1000 // keep resolved incidents 5 min

    set(s => {
      let incidents = [...s.incidents]
      let opsLog    = [...s.opsLog]
      let fp        = new Set(s._incFP)

      // Create new incidents from events not yet seen this cycle
      // Reset fingerprint window every 30 seconds (allow re-detection)
      if (incidents.every(i => now - i.createdAt > 30000)) fp = new Set()

      for (const e of events) {
        const fprint = `${e.description}|${e.service_id}`
        if (fp.has(fprint)) continue
        fp.add(fprint)
        const inc = incidentFromEvent(e, dirs)
        incidents.push(inc)
        opsLog.push({ id: mkId(), at: now, incidentId: inc.id, source: 'reasoning',
          stage: inc.stage, text: inc.problem, service: inc.affectedServices[0] || null,
          severity: inc.severity })
      }

      // Advance stages for open incidents
      incidents = incidents.map(inc => {
        const advanced = advanceStage(inc, agg)
        if (advanced.stage !== inc.stage) {
          opsLog.push({ id: mkId(), at: now, incidentId: inc.id, source: 'control',
            stage: advanced.stage, text: `${inc.affectedServices[0] || 'System'}: ${LIFECYCLE_LABEL[advanced.stage]}`,
            service: inc.affectedServices[0] || null, severity: inc.severity })
        }
        return advanced
      })

      // Prune resolved incidents older than KEEP, keep max 50
      incidents = incidents
        .filter(i => !['resolved','failed','overridden'].includes(i.stage) || now - i.resolvedAt < KEEP)
        .slice(-50)

      // Keep ops log last 200 entries
      opsLog = opsLog.slice(-200)

      const systemMode = deriveSystemMode(agg, incidents)
      return { incidents, opsLog, systemMode, _incFP: fp }
    })
  },

  _checkThresholds(agg) {
    const t = get().thresholds
    const next = { ...t }
    const add  = get().addToast

    if (agg.maxLat > 500 && !t.latWarn) {
      add('warn', '⚡ High Latency', `Queue wait at ${agg.maxLat.toFixed(0)}ms — requests backing up`)
      next.latWarn = true
    } else if (agg.maxLat < 300) next.latWarn = false

    if (agg.maxQueue > 100 && !t.queueWarn) {
      add('warn', '📦 Queue Building', `Depth ${agg.maxQueue.toFixed(0)} — service may be saturated`)
      next.queueWarn = true
    } else if (agg.maxQueue < 30) next.queueWarn = false

    if (agg.maxRisk > 0.7 && !t.riskWarn) {
      add('crit', '💥 Collapse Risk', `${(agg.maxRisk*100).toFixed(0)}% on ${agg.hottestSvc || 'a service'}`)
      next.riskWarn = true
    } else if (agg.maxRisk < 0.4) next.riskWarn = false

    if (agg.maxRho > 0.85 && !t.rhoWarn) {
      add('warn', '🔥 Saturation', `${(Math.min(agg.maxRho,1)*100).toFixed(0)}% utilisation — ${agg.hottestSvc}`)
      next.rhoWarn = true
    } else if (agg.maxRho < 0.6) next.rhoWarn = false

    set({ thresholds: next })
  },

  _checkStatus(status) {
    const prev = get().prevStatus
    if (status === prev) return
    set({ prevStatus: status })
    const add = get().addToast
    if (status === 'unstable' && prev !== 'waiting')
      add('crit', '🔴 System Unstable', 'Services in critical state — check incidents panel')
    else if (status === 'degraded' && prev === 'stable')
      add('warn', '🟡 System Degraded', 'One or more services under stress')
    else if (status === 'stable' && (prev === 'degraded' || prev === 'unstable'))
      add('success', '🟢 System Recovered', 'All services back to normal operation')
  },

  addToast(type, title, msg, duration) {
    duration = duration ?? (type === 'crit' ? 8000 : 5000)
    const id  = `${Date.now()}-${Math.random()}`
    set(s => ({ toasts: [...s.toasts, { id, type, title, msg, duration }] }))
    setTimeout(() => get().removeToast(id), duration + 400)
  },

  selectIncident(id) { set({ selectedInc: id }) },
  closeIncident()   { set({ selectedInc: null }) },

  overrideIncident(id, note) {
    const now = Date.now()
    set(s => {
      const incidents = s.incidents.map(i => i.id !== id ? i : {
        ...i, stage: 'overridden', humanOverride: true, updatedAt: now, resolvedAt: now,
        entries: [...i.entries, { at: now, stage: 'overridden', note: note || 'Operator override', source: 'human' }]
      })
      const opsLog = [...s.opsLog, {
        id: mkId(), at: now, incidentId: id, source: 'human',
        stage: 'overridden', text: note || 'Operator override applied', service: null, severity: 'info'
      }]
      return { incidents, opsLog }
    })
    get().addToast('info', 'Override Applied', note || 'Incident marked as overridden by operator', 4000)
  },

  logHumanAction(text, service, severity) {
    const now = Date.now()
    set(s => ({
      opsLog: [...s.opsLog.slice(-199), {
        id: mkId(), at: now, incidentId: null, source: 'human',
        stage: 'overridden', text, service: service || null, severity: severity || 'info'
      }]
    }))
  },

  removeToast(id) {
    set(s => ({ toasts: s.toasts.filter(t => t.id !== id) }))
  },

  setPendingAction(action) { set({ pendingAction: action }) },
  clearPendingAction()     { set({ pendingAction: null }) },
}))