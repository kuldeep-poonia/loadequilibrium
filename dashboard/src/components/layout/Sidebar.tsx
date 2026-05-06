"use client";

import { clsx } from "clsx";

export type NavSection =
  | "overview"
  | "telemetry"
  | "autopilot"
  | "anomalies"
  | "simulation"
  | "policy"
  | "control"
  | "intelligence"
  | "logs";

const NAV_ITEMS: { id: NavSection; label: string; icon: string }[] = [
  { id: "overview", label: "Overview", icon: "◈" },
  { id: "telemetry", label: "Telemetry", icon: "⌗" },
  { id: "autopilot", label: "Autopilot", icon: "⊙" },
  { id: "anomalies", label: "Anomalies", icon: "⚡" },
  { id: "simulation", label: "Simulation", icon: "⧖" },
  { id: "policy", label: "Policy", icon: "⊞" },
  { id: "control", label: "Control", icon: "⊛" },
  { id: "intelligence", label: "Intelligence", icon: "◎" },
  { id: "logs", label: "Logs / Audit", icon: "≡" },
];

interface SidebarProps {
  active: NavSection;
  onSelect: (s: NavSection) => void;
  alertCount: number;
}

export function Sidebar({ active, onSelect, alertCount }: SidebarProps) {
  return (
    <nav className="flex flex-col w-[190px] shrink-0 bg-surface-1 border-r border-surface-3 py-4 gap-0.5">
      <div className="px-4 pb-4 mb-1 border-b border-surface-3">
        <div className="flex items-center gap-2">
          <span className="text-brand text-lg">▣</span>
          <span className="text-xs font-bold tracking-widest text-text-primary uppercase">
            Control Plane
          </span>
        </div>
      </div>

      {NAV_ITEMS.map((item) => (
        <button
          key={item.id}
          onClick={() => onSelect(item.id)}
          className={clsx(
            "relative flex items-center gap-3 mx-2 px-3 py-2 rounded text-xs font-medium transition-colors duration-100 text-left",
            active === item.id
              ? "bg-brand/15 text-brand"
              : "text-text-secondary hover:text-text-primary hover:bg-surface-2"
          )}
        >
          <span className="w-4 text-center shrink-0">{item.icon}</span>
          {item.label}
          {item.id === "anomalies" && alertCount > 0 && (
            <span className="ml-auto text-[10px] font-mono font-bold bg-danger/20 text-danger border border-danger/30 rounded px-1">
              {alertCount}
            </span>
          )}
        </button>
      ))}
    </nav>
  );
}
