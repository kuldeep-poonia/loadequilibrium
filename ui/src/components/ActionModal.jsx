import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useStore } from '../store/useStore'
import { pct, ms, n } from '../lib/fmt'
import { setActuation, setPolicy, triggerRollout, runStressTest } from '../lib/api'

export default function ActionModal() {
  const action       = useStore(s => s.pendingAction)
  const clearPending = useStore(s => s.clearPendingAction)
  const addToast     = useStore(s => s.addToast)
  const [loading, setLoading]   = useState(false)
  const [applied, setApplied]   = useState(false)

  if (!action && !applied) return null

  const svc = action?.svc || action?.event?.service_id || 'system'
  const ev  = action?.event?.evidence || {}

  const evidence = [
    ['Utilisation',    ev.utilisation    != null ? pct(ev.utilisation)      : null],
    ['Collapse Risk',  ev.collapse_risk  != null ? pct(ev.collapse_risk)    : null],
    ['Queue Wait',     ev.queue_wait_ms  != null ? ms(ev.queue_wait_ms)     : null],
    ['Burst Factor',   ev.burst_factor   != null ? n(ev.burst_factor, 2) + '×' : null],
    ['Cascade Risk',   ev.cascade_risk   != null ? pct(ev.cascade_risk)     : null],
    ['Stability Margin', ev.stability_margin != null ? pct(ev.stability_margin) : null],
  ].filter(([, v]) => v !== null)

  const handleApply = async () => {
    setLoading(true)
    try {
      const title = (action.title || '').toLowerCase()
      const svc   = action.svc || ''

      if (title.includes('scale up') || title.includes('scale down')) {
        // Scale directive: trigger a rollout so the engine re-evaluates with urgency
        await triggerRollout(10)
      } else if (title.includes('freeze') || title.includes('disable')) {
        await setActuation(false)
      } else if (title.includes('enable') || title.includes('resume')) {
        await setActuation(true)
      } else if (title.includes('safe mode') || title.includes('conservative')) {
        await setPolicy('conservative')
      } else if (title.includes('balanced') || title.includes('reset')) {
        await setPolicy('balanced')
      } else {
        // Default: trigger an intelligence rollout so the engine recalculates urgently
        await triggerRollout(10)
      }

      setLoading(false)
      setApplied(true)
      addToast('info', '⚙ Action Dispatched', `${action.title} — engine applies on next tick`)
      setTimeout(() => {
        addToast('info', '🔄 Stabilising', 'Engine recalculating control surface — watch metrics')
      }, 3000)
      setTimeout(() => {
        clearPending()
        setApplied(false)
      }, 1400)
    } catch (err) {
      setLoading(false)
      addToast('crit', 'Dispatch Failed', `API error: ${err.message || 'check connection'}`, 8000)
    }
  }

  const handleDeny = () => {
    addToast('info', 'Action Denied', 'Recommendation dismissed — no changes made', 3000)
    clearPending()
    setApplied(false)
  }

  return (
    <AnimatePresence>
      <motion.div
        key="overlay"
        initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
        className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-[9999]"
        onClick={(e) => { if (e.target === e.currentTarget) handleDeny() }}
      >
        <motion.div
          initial={{ opacity: 0, scale: 0.96, y: 10 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.96 }}
          transition={{ duration: 0.2 }}
          className="bg-surface border border-cyan/40 rounded-lg w-[480px] max-w-[94vw] shadow-[0_0_40px_rgba(0,0,0,0.8),0_0_0_1px_rgba(0,184,245,0.15)]"
        >
          {applied ? (
            <div className="p-8 text-center">
              <motion.div
                initial={{ scale: 0 }} animate={{ scale: 1 }}
                transition={{ type: 'spring', stiffness: 300 }}
                className="text-4xl mb-3"
              >✓</motion.div>
              <p className="font-cond text-[16px] font-bold tracking-wider text-cyan">ACTION DISPATCHED</p>
              <p className="text-muted text-[11px] mt-1">Engine will apply on next tick</p>
            </div>
          ) : (
            <>
              <div className="flex items-center px-5 py-4 border-b border-border">
                <div>
                  <p className="font-cond text-[13px] font-bold tracking-wider text-cyan">APPLY RECOMMENDATION?</p>
                  <p className="text-muted text-[11px] mt-0.5">This action will be dispatched to the control engine</p>
                </div>
                <button onClick={handleDeny} className="ml-auto text-muted hover:text-red transition-colors text-[14px] p-1">✕</button>
              </div>

              <div className="px-5 py-4">
                <div className="mb-3">
                  <span className="font-cond text-[10px] font-bold tracking-wider text-muted uppercase">Service</span>
                  <p className="text-cyan font-mono font-semibold text-[13px] mt-0.5">{svc}</p>
                </div>
                <div className="mb-4">
                  <span className="font-cond text-[10px] font-bold tracking-wider text-muted uppercase">Action</span>
                  <p className="text-bright text-[12px] mt-0.5 leading-relaxed">{action.title}</p>
                </div>

                {evidence.length > 0 && (
                  <div className="bg-surface2 border border-border rounded p-3 grid grid-cols-2 gap-2">
                    {evidence.map(([k, v]) => (
                      <div key={k} className="flex justify-between items-center">
                        <span className="text-dim text-[10px]">{k}</span>
                        <span className="text-text text-[10px] font-mono font-semibold">{v}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="flex gap-3 px-5 py-4 border-t border-border">
                <button
                  onClick={handleDeny}
                  className="flex-1 font-cond text-[12px] font-bold tracking-wider text-muted bg-surface2 border border-border rounded py-2.5 hover:border-borderhi hover:text-text transition-all duration-150 cursor-pointer"
                >
                  DENY
                </button>
                <button
                  onClick={handleApply}
                  disabled={loading}
                  className="flex-1 font-cond text-[12px] font-bold tracking-wider text-bg bg-cyan rounded py-2.5 hover:bg-[#00ceff] active:scale-[0.98] transition-all duration-150 cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                >
                  {loading ? (
                    <>
                      <motion.span
                        animate={{ rotate: 360 }} transition={{ duration: 0.8, repeat: Infinity, ease: 'linear' }}
                        className="inline-block"
                      >↻</motion.span>
                      DISPATCHING…
                    </>
                  ) : 'APPLY ACTION'}
                </button>
              </div>
            </>
          )}
        </motion.div>
      </motion.div>
    </AnimatePresence>
  )
}