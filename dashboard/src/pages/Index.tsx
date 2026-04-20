import { useTelemetry } from "@/store/telemetryStore";
import { useWebSocket } from "@/hooks/useWebSocket";
import { MissionHeader } from "@/components/layout/MissionHeader";
import { SystemOverview } from "@/components/panels/SystemOverview";
import { InsightPanel } from "@/components/panels/InsightPanel";
import { AlertPanel } from "@/components/panels/AlertPanel";
import { ServiceGrid } from "@/components/panels/ServiceGrid";
import { TopologyGraph } from "@/components/panels/TopologyGraph";
import { ServiceDetail } from "@/components/panels/ServiceDetail";
import { ControlPanel } from "@/components/panels/ControlPanel";

const DisconnectedNotice = () => {
  const status = useTelemetry((s) => s.status);
  const lastError = useTelemetry((s) => s.lastError);
  if (status === "open") return null;
  return (
    <div className="panel border-warn/40 bg-warn/5 px-4 py-2 flex items-center gap-3">
      <span className="led led-warn animate-pulse-dot" />
      <div className="flex-1 min-w-0">
        <div className="text-[11px] font-mono uppercase tracking-widest text-warn glow-warn">
          ◉ Awaiting telemetry uplink
        </div>
        <div className="text-[10px] text-muted-foreground mt-0.5 font-mono truncate">
          {lastError ?? "Set VITE_WS_URL and VITE_API_URL to establish the stream."}
        </div>
      </div>
      <div className="text-[10px] font-mono uppercase tracking-widest text-muted-foreground">
        Status · {status.toUpperCase()}
      </div>
    </div>
  );
};

const Index = () => {
  useWebSocket();

  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col">
      <MissionHeader />

      <main className="flex-1 p-2 grid gap-2 lg:grid-cols-12 grid-rows-[auto_auto_minmax(0,1fr)] min-h-0">
        <div className="lg:col-span-12">
          <SystemOverview />
        </div>

        <div className="lg:col-span-12">
          <DisconnectedNotice />
        </div>

        {/* Main 3-column working area */}
        <div className="lg:col-span-3 flex flex-col gap-2 min-h-0 lg:row-start-3">
          <div className="flex-1 min-h-[260px]">
            <InsightPanel />
          </div>
          <div className="flex-1 min-h-[220px]">
            <AlertPanel />
          </div>
        </div>

        <div className="lg:col-span-6 flex flex-col gap-2 min-h-0 lg:row-start-3">
          <div className="flex-[1.2] min-h-[280px]">
            <TopologyGraph />
          </div>
          <div className="flex-1 min-h-[260px]">
            <ServiceGrid />
          </div>
        </div>

        <div className="lg:col-span-3 flex flex-col gap-2 min-h-0 lg:row-start-3">
          <div className="flex-1 min-h-[320px]">
            <ServiceDetail />
          </div>
          <div className="min-h-[260px]">
            <ControlPanel />
          </div>
        </div>
      </main>
    </div>
  );
};

export default Index;
