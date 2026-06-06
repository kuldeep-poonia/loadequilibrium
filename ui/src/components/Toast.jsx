import React, { useState, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useStore } from '../store/useStore'

const STYLES = {
  crit:    { border: 'border-red',    bg: 'bg-[rgba(15,3,3,0.96)]',  bar: 'bg-red'    },
  warn:    { border: 'border-yellow', bg: 'bg-[rgba(15,12,0,0.96)]', bar: 'bg-yellow' },
  info:    { border: 'border-cyan',   bg: 'bg-[rgba(0,8,18,0.96)]',  bar: 'bg-cyan'   },
  success: { border: 'border-green',  bg: 'bg-[rgba(0,10,6,0.96)]',  bar: 'bg-green'  },
}

function ToastItem({ toast }) {
  const remove = useStore(s => s.removeToast)
  const [progress, setProgress] = useState(100)
  const st = STYLES[toast.type] ?? STYLES.info

  useEffect(() => {
    const interval = 50
    const step = (100 / toast.duration) * interval
    const t = setInterval(() => {
      setProgress(p => {
        if (p <= 0) { clearInterval(t); return 0 }
        return p - step
      })
    }, interval)
    return () => clearInterval(t)
  }, [toast.duration])

  return (
    <motion.div
      layout
      initial={{ opacity: 0, x: 24, scale: 0.96 }}
      animate={{ opacity: 1, x: 0, scale: 1 }}
      exit={{ opacity: 0, x: 24, scale: 0.94 }}
      transition={{ duration: 0.22 }}
      className={`relative overflow-hidden flex items-start gap-3 px-4 py-3 rounded border-l-[3px] shadow-2xl max-w-sm w-80 ${st.border} ${st.bg}`}
    >
      <div className="flex-1 min-w-0">
        <p className={`font-cond font-bold text-[12px] tracking-wide ${
          toast.type === 'crit' ? 'text-red' : toast.type === 'warn' ? 'text-yellow' :
          toast.type === 'success' ? 'text-green' : 'text-cyan'
        }`}>{toast.title}</p>
        <p className="text-muted text-[10px] mt-0.5 leading-snug">{toast.msg}</p>
      </div>
      <button
        onClick={() => remove(toast.id)}
        className="text-dim hover:text-text transition-colors flex-shrink-0 text-[12px] mt-0.5"
      >✕</button>
      <div
        className={`absolute bottom-0 left-0 h-[2px] ${st.bar} transition-all`}
        style={{ width: `${progress}%` }}
      />
    </motion.div>
  )
}

export default function Toast() {
  const toasts = useStore(s => s.toasts)
  return (
    <div className="fixed bottom-5 right-5 z-[19999] flex flex-col gap-2 items-end pointer-events-none">
      <AnimatePresence mode="popLayout">
        {toasts.map(t => (
          <div key={t.id} className="pointer-events-auto">
            <ToastItem toast={t} />
          </div>
        ))}
      </AnimatePresence>
    </div>
  )
}