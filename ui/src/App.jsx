import React, { useEffect, useState } from 'react'
import { useStore } from './store/useStore'
import Topbar from './components/Topbar'
import SystemModeBar from './components/SystemModeBar'
import HeroBar from './components/HeroBar'
import MonitorPage from './pages/MonitorPage'
import ControlPage from './pages/ControlPage'
import ActionModal from './components/ActionModal'
import Toast from './components/Toast'

const NAV = [
  { id: 'monitor', label: 'Monitor', icon: '◉', desc: 'Observe — read-only view of system health and autopilot reasoning' },
  { id: 'control', label: 'Control', icon: '⚙', desc: 'Act — send commands to the engine and configure autopilot behaviour' },
]

export default function App() {
  const connect    = useStore(s => s.connect)
  const systemMode = useStore(s => s.systemMode)
  const incidents  = useStore(s => s.incidents)
  const [page, setPage] = useState('monitor')

  useEffect(() => { connect() }, [])

  const openCrit = incidents.filter(i =>
    i.severity === 'critical' && !['resolved','failed','overridden'].includes(i.stage)
  ).length

  const modeColor = {
    normal:    'text-green',
    degraded:  'text-yellow',
    emergency: 'text-red animate-pulse2',
    offline:   'text-muted',
  }[systemMode] || 'text-muted'

  return (
    <div className="flex flex-col h-screen overflow-hidden bg-bg">
      <div className="scanline" />
      <Topbar />
      <SystemModeBar />
      <HeroBar />

      {/* Page navigation tabs */}
      <div className="flex-shrink-0 bg-surface border-b border-border px-4 flex items-center gap-1">
        {NAV.map(nav => {
          const active = page === nav.id
          const showBadge = nav.id === 'monitor' && openCrit > 0
          return (
            <button
              key={nav.id}
              onClick={() => setPage(nav.id)}
              title={nav.desc}
              className={`relative flex items-center gap-2 px-4 py-2.5 font-cond text-[12px] font-bold tracking-wider border-b-2 transition-all duration-150 cursor-pointer
                ${active
                  ? 'text-bright border-cyan'
                  : 'text-muted border-transparent hover:text-text hover:border-border'}`}
            >
              <span>{nav.icon}</span>
              {nav.label}
              {showBadge && (
                <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-red animate-blink" />
              )}
            </button>
          )
        })}
        <div className="ml-auto flex items-center gap-2 py-2">
          <span className={`font-cond text-[10px] font-bold tracking-wider ${modeColor}`}>
            {page === 'monitor'
              ? 'Observation mode — no actions available on this page'
              : 'Control mode — actions here affect the live system'}
          </span>
        </div>
      </div>

      {page === 'monitor' ? <MonitorPage /> : <ControlPage />}

      <ActionModal />
      <Toast />
    </div>
  )
}