import React, { useEffect } from 'react'
import { useStore } from './store/useStore'
import Topbar from './components/Topbar'
import HeroBar from './components/HeroBar'
import MetricCharts from './components/MetricCharts'
import ServiceTable from './components/ServiceTable'
import IntelPanel from './components/IntelRow'
import EventStream from './components/EventStream'
import Toast from './components/Toast'
import ActionModal from './components/ActionModal'

export default function App() {
  const connect = useStore(s => s.connect)

  useEffect(() => {
    connect()

    return () => {
      const state = useStore.getState()
      clearTimeout(state._watchdog)
      clearTimeout(state._reconnTimer)
      try { state._ws?.close() } catch (e) {}
    }
  }, [connect])

  return (
    <div className="min-h-screen h-screen bg-bg text-text flex flex-col overflow-hidden">
      <Topbar />
      <HeroBar />

      <main className="flex-1 min-h-0 p-2 overflow-hidden">
        <div className="h-full min-h-0 grid grid-cols-[minmax(0,1fr)_390px] gap-2 max-[1180px]:grid-cols-1 max-[1180px]:overflow-y-auto">
          <section className="min-h-0 flex flex-col gap-2 overflow-hidden">
            <MetricCharts />
            <IntelPanel />
            <div className="min-h-0 flex-1 overflow-hidden">
              <ServiceTable />
            </div>
          </section>

          <aside className="min-h-0 overflow-hidden">
            <EventStream />
          </aside>
        </div>
      </main>

      <ActionModal />
      <Toast />
      <div className="scanline" />
    </div>
  )
}
