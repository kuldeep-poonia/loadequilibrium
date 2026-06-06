import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useStore } from '../store/useStore'
import { setActuation, setPolicy } from '../lib/api'

const MODE = {
  normal:    { label: 'AUTONOMOUS', sub: 'Autopilot active — system self-managing', color: 'text-green',  border: 'border-green/30',  bg: 'bg-green/5'   },
  degraded:  { label: 'ADVISORY',   sub: 'Reduced confidence — monitoring elevated', color: 'text-yellow', border: 'border-yellow/30', bg: 'bg-yellow/5'  },
  emergency: { label: 'EMERGENCY',  sub: 'Human authority required — autopilot constrained', color: 'text-red', border: 'border-red/40', bg: 'bg-red/5' },
  offline:   { label: 'OFFLINE',    sub: 'No telemetry — waiting for data', color: 'text-muted', border: 'border-border', bg: 'bg-transparent' },
}

function ConfirmDialog({ action, onConfirm, onCancel }) {
  return (
    <motion.div
      initial={{ opacity: 0, scale: 0.96 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.96 }}
      className="absolute top-full right-0 mt-2 w-72 bg-surface border border-borderhi rounded-lg shadow-2xl z-50 p-4"
    >
      <p className="font-cond text-[12px] font-bold tracking-wide text-bright mb-1">{action.label}</p>
      <p className="text-muted text-[11px] leading-relaxed mb-4">{action.desc}</p>
      <div className="flex gap-2">
        <button onClick={onCancel} className="flex-1 font-cond text-[11px] font-bold tracking-wider text-muted bg-surface2 border border-border rounded py-2 hover:border-borderhi transition-colors">
          CANCEL
        </button>
        <button onClick={onConfirm} className={`flex-1 font-cond text-[11px] font-bold tracking-wider rounded py-2 transition-all ${action.confirmCls}`}>
          CONFIRM
        </button>
      </div>
    </motion.div>
  )
}

const EMERGENCY_ACTIONS = [
  {
    id: 'freeze',
    label: 'FREEZE AUTOPILOT',
    icon: '⏸',
    desc: 'Halt all autopilot scaling decisions immediately. Manual control only.',
    confirmCls: 'text-yellow bg-yellow/20 border border-yellow hover:bg-yellow/30',
    toastMsg: 'Autopilot frozen — all decisions require manual approval',
    toastType: 'warn',
  },
  {
    id: 'safemode',
    label: 'SAFE MODE',
    icon: '🛡',
    desc: 'Force all services to safe operating thresholds. Caps scaling and disables aggressive optimization.',
    confirmCls: 'text-cyan bg-cyan/20 border border-cyan hover:bg-cyan/30',
    toastMsg: 'Safe mode activated — conservative thresholds enforced',
    toastType: 'info',
  },
  {
    id: 'rollback',
    label: 'ROLLBACK ALL',
    icon: '↩',
    desc: 'Revert all recent scale decisions to last known stable state. This cannot be undone automatically.',
    confirmCls: 'text-red bg-red/20 border border-red hover:bg-red/30',
    toastMsg: 'Rollback initiated — reverting to last stable configuration',
    toastType: 'crit',
  },
]

export default function SystemModeBar() {
  const systemMode    = useStore(s => s.systemMode)
  const incidents     = useStore(s => s.incidents)
  const addToast      = useStore(s => s.addToast)
  const logHumanAction= useStore(s => s.logHumanAction)
  const [confirm, setConfirm] = useState(null)
  const [loading, setLoading] = useState(false)

  const mode = MODE[systemMode] ?? MODE.offline
  const openCrit = incidents.filter(i => i.severity === 'critical' && !['resolved','failed','overridden'].includes(i.stage))

  const handleAction = async (action) => {
    setLoading(true)
    setConfirm(null)
    try {
      if (action.id === 'freeze') {
        // FREEZE AUTOPILOT: disables actuation — engine keeps reasoning but issues no commands
        await setActuation(false)
        addToast(action.toastType, action.label, action.toastMsg, 8000)
        logHumanAction('Operator: FREEZE AUTOPILOT — actuation disabled via API', null, 'human')
      } else if (action.id === 'safemode') {
        // SAFE MODE: switch to conservative policy — caps scaling, reduces risk tolerance
        await setPolicy('conservative')
        addToast(action.toastType, action.label, action.toastMsg, 8000)
        logHumanAction('Operator: SAFE MODE — policy set to conservative via API', null, 'human')
      } else if (action.id === 'rollback') {
        // ROLLBACK ALL: disable actuation + reset to balanced policy
        // The engine will hold current capacity on next tick and stop all scaling
        await setActuation(false)
        await setPolicy('balanced')
        addToast(action.toastType, action.label, action.toastMsg, 8000)
        logHumanAction('Operator: ROLLBACK ALL — actuation disabled, policy reset to balanced via API', null, 'human')
      }
    } catch (err) {
      addToast('crit', `${action.label} FAILED`, `API error: ${err.message || 'check connection'}`, 8000)
      logHumanAction(`Operator: ${action.label} — FAILED: ${err.message}`, null, 'crit')
    } finally {
      setLoading(false)
    }
  }

  if (systemMode === 'offline' || systemMode === 'normal') return null

  return (
    <div className={`border-b ${mode.border} ${mode.bg} px-4 py-2 flex items-center gap-4 flex-shrink-0 relative`}>
      <div className="flex items-center gap-2">
        <span className={`font-cond text-[10px] font-bold tracking-[0.14em] ${mode.color}`}>
          {systemMode === 'emergency' ? '⚠' : '◑'} {mode.label}
        </span>
        <span className="text-dim text-[10px]">{mode.sub}</span>
      </div>

      {systemMode === 'emergency' && openCrit.length > 0 && (
        <div className="flex items-center gap-1.5 ml-2">
          <span className="w-1.5 h-1.5 rounded-full bg-red animate-blink flex-shrink-0" />
          <span className="text-red text-[10px] font-mono font-semibold">
            {openCrit.length} critical incident{openCrit.length > 1 ? 's' : ''} open
          </span>
        </div>
      )}

      {systemMode === 'emergency' && (
        <div className="flex items-center gap-2 ml-auto relative">
          <span className="font-cond text-[9px] text-muted tracking-wider mr-1">OPERATOR CONTROLS</span>
          {EMERGENCY_ACTIONS.map(action => (
            <div key={action.id} className="relative">
              <button
                onClick={() => setConfirm(confirm?.id === action.id ? null : action)}
                className={`font-cond text-[10px] font-bold tracking-wider px-3 py-1.5 rounded border transition-all duration-150 cursor-pointer
                  ${confirm?.id === action.id ? action.confirmCls : 'text-muted border-border hover:border-borderhi hover:text-text bg-surface2'}`}
              >
                {action.icon} {action.label}
              </button>
              <AnimatePresence>
                {confirm?.id === action.id && (
                  <ConfirmDialog
                    action={action}
                    onConfirm={() => handleAction(action)}
                    onCancel={() => setConfirm(null)}
                  />
                )}
              </AnimatePresence>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}