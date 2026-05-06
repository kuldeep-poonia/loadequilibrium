"use client";

import { clsx } from "clsx";

interface ConnectionBarProps {
  state: "connecting" | "connected" | "disconnected" | "error";
  seq: number;
  reconnectCount: number;
}

export function ConnectionBar({ state, seq, reconnectCount }: ConnectionBarProps) {
  const label = {
    connecting: "Connecting…",
    connected: "Live",
    disconnected: "Reconnecting…",
    error: "Connection error",
  }[state];

  const dotColor = {
    connecting: "bg-warning animate-pulse",
    connected: "bg-success",
    disconnected: "bg-warning animate-pulse",
    error: "bg-danger",
  }[state];

  return (
    <div className="flex items-center gap-3 px-4 py-1.5 bg-surface-1 border-b border-surface-3 text-xs">
      <span className={clsx("w-2 h-2 rounded-full shrink-0", dotColor)} />
      <span className="text-text-secondary font-mono">{label}</span>
      {seq > 0 && (
        <span className="text-text-tertiary font-mono ml-1">
          seq <span className="text-text-secondary">{seq.toLocaleString()}</span>
        </span>
      )}
      {reconnectCount > 0 && (
        <span className="text-text-tertiary font-mono ml-auto">
          reconnects: {reconnectCount}
        </span>
      )}
    </div>
  );
}
