"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useTickStream } from "@/hooks/useTickStream";
import { useTelemetryHistory } from "@/hooks/useTelemetryHistory";
import { ConnectionBar } from "@/components/layout/ConnectionBar";
import { Sidebar, type NavSection } from "@/components/layout/Sidebar";
import { OverviewPanel } from "@/components/panels/OverviewPanel";
import { TelemetryPanel } from "@/components/panels/TelemetryPanel";
import { AutopilotPanel } from "@/components/panels/AutopilotPanel";
import { AnomalyPanel } from "@/components/panels/AnomalyPanel";
import { SimulationPanel } from "@/components/panels/SimulationPanel";
import { PolicyPanel } from "@/components/panels/PolicyPanel";
import { ControlPanel } from "@/components/panels/ControlPanel";
import { IntelligencePanel } from "@/components/panels/IntelligencePanel";
import { LogsPanel } from "@/components/panels/LogsPanel";
import type { TickPayload } from "@/types/tick";

const SECTION_TITLES: Record<NavSection, string> = {
  overview: "System Overview",
  telemetry: "Real-time Telemetry",
  autopilot: "Autopilot / Decision Engine",
  anomalies: "Anomaly Detection",
  simulation: "Simulation & Chaos",
  policy: "Policy Engine",
  control: "Control System",
  intelligence: "Intelligence Layer",
  logs: "Logs & Audit",
};

function NoDataScreen({ connectionState }: { connectionState: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-4 text-text-tertiary">
      <div className="w-8 h-8 border-2 border-surface-4 border-t-brand rounded-full animate-spin" />
      <div className="flex flex-col items-center gap-1">
        <span className="text-sm text-text-secondary">
          {connectionState === "error" ? "Connection failed" : "Awaiting first tick…"}
        </span>
        <span className="text-xs font-mono">
          {process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080"}
        </span>
      </div>
    </div>
  );
}

export default function DashboardPage() {
  const [activeSection, setActiveSection] = useState<NavSection>("overview");
  const { tick, connectionState, lastSeq, reconnectCount } = useTickStream();
  const history = useTelemetryHistory(tick);

  // Keep a ring buffer of raw ticks for the logs panel
  const tickHistoryRef = useRef<TickPayload[]>([]);
  if (tick) {
    const buf = tickHistoryRef.current;
    if (buf.length === 0 || buf[buf.length - 1].seq !== tick.seq) {
      if (buf.length >= 60) buf.shift();
      buf.push(tick);
    }
  }

  const alertCount = useMemo(
    () => (tick?.events ?? []).filter((e) => e.severity >= 1).length,
    [tick?.events]
  );

  const handleSelect = useCallback((s: NavSection) => setActiveSection(s), []);

  return (
    <div className="flex flex-col h-screen overflow-hidden bg-surface-0">
      <ConnectionBar
        state={connectionState}
        seq={lastSeq}
        reconnectCount={reconnectCount}
      />

      <div className="flex flex-1 min-h-0">
        <Sidebar
          active={activeSection}
          onSelect={handleSelect}
          alertCount={alertCount}
        />

        <main className="flex-1 flex flex-col min-w-0 min-h-0">
          <header className="flex items-center justify-between px-4 py-2.5 border-b border-surface-3 bg-surface-1 shrink-0">
            <h1 className="text-xs font-semibold uppercase tracking-widest text-text-secondary">
              {SECTION_TITLES[activeSection]}
            </h1>
            <div className="flex items-center gap-4 text-[10px] font-mono text-text-tertiary">
              {tick && (
                <>
                  <span>
                    tick <span className="text-text-secondary">{tick.control_plane.tick.toLocaleString()}</span>
                  </span>
                  <span>
                    schema v<span className="text-text-secondary">{tick.schema_version}</span>
                  </span>
                  {tick.safety_mode && (
                    <span className="text-danger font-bold animate-pulse2">⚠ SAFETY MODE</span>
                  )}
                  {(tick.runtime_metrics?.consec_overruns ?? 0) > 0 && (
                    <span className="text-warning">
                      overruns: {tick.runtime_metrics.consec_overruns}
                    </span>
                  )}
                </>
              )}
            </div>
          </header>

          <div className="flex-1 min-h-0 overflow-y-auto">
            {!tick ? (
              <NoDataScreen connectionState={connectionState} />
            ) : (
              <>
                {activeSection === "overview" && <OverviewPanel tick={tick} />}
                {activeSection === "telemetry" && (
                  <TelemetryPanel tick={tick} history={history} />
                )}
                {activeSection === "autopilot" && <AutopilotPanel tick={tick} />}
                {activeSection === "anomalies" && <AnomalyPanel tick={tick} />}
                {activeSection === "simulation" && <SimulationPanel tick={tick} />}
                {activeSection === "policy" && <PolicyPanel tick={tick} />}
                {activeSection === "control" && <ControlPanel tick={tick} />}
                {activeSection === "intelligence" && <IntelligencePanel tick={tick} />}
                {activeSection === "logs" && (
                  <LogsPanel tick={tick} history={tickHistoryRef.current} />
                )}
              </>
            )}
          </div>
        </main>
      </div>
    </div>
  );
}
