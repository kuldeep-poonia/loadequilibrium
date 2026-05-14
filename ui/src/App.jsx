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
    <div className="min-h-screen bg-bg text-text flex flex-col">
      <Topbar />
      <HeroBar />

      <main className="flex-1 p-2">
        <div className="grid grid-cols-[minmax(0,1fr)_390px] gap-2 items-start max-[1180px]:grid-cols-1">
          <section className="min-w-0 flex flex-col gap-2">
            <MetricCharts />
            <IntelPanel />
            <ServiceTable />
          </section>

          <aside className="min-w-0 h-[calc(100vh-76px)] sticky top-2 overflow-hidden max-[1180px]:static max-[1180px]:h-[680px]">
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
