import { create } from 'zustand'
import { aggregate } from '../lib/agg'
import { WS_URL } from '../lib/backend'

const MAX_HIST = 60

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
  toasts:       [],
  pendingAction:null,
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
      return { payload, agg, tick, history: newHist, prevBundles: s.payload?.bundles || {} }
    })

    if (agg) {
      get()._checkThresholds(agg)
      get()._checkStatus(agg.status)
    }
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

  removeToast(id) {
    set(s => ({ toasts: s.toasts.filter(t => t.id !== id) }))
  },

  setPendingAction(action) { set({ pendingAction: action }) },
  clearPendingAction()     { set({ pendingAction: null }) },
}))
