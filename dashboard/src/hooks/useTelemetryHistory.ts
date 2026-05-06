"use client";

import { useEffect, useRef, useState } from "react";
import type { TickPayload } from "@/types/tick";

export interface TelemetryPoint {
  seq: number;
  ts: number;
  tickHealthMs: number;
  degradedFraction: number;
  jitterMs: number;
  compositeScore: number;
  cascadeFailureProb: number;
  p99LatencyMs: number;
  maxCollapseRisk: number;
  oscillationRisk: number;
  safetyMode: boolean;
}

const MAX_HISTORY = 120;

function tickToPoint(t: TickPayload): TelemetryPoint {
  return {
    seq: t.seq,
    ts: Date.now(),
    tickHealthMs: t.tick_health_ms,
    degradedFraction: t.degraded_fraction,
    jitterMs: t.jitter_ms,
    compositeScore: t.objective?.composite_score ?? 0,
    cascadeFailureProb: t.objective?.cascade_failure_probability ?? 0,
    p99LatencyMs: t.objective?.predicted_p99_latency_ms ?? 0,
    maxCollapseRisk: t.objective?.max_collapse_risk ?? 0,
    oscillationRisk: t.objective?.oscillation_risk ?? 0,
    safetyMode: t.safety_mode,
  };
}

export function useTelemetryHistory(tick: TickPayload | null): TelemetryPoint[] {
  const bufferRef = useRef<TelemetryPoint[]>([]);
  const lastSeqRef = useRef<number>(-1);
  const [history, setHistory] = useState<TelemetryPoint[]>([]);

  useEffect(() => {
    if (!tick || tick.seq === lastSeqRef.current) return;
    lastSeqRef.current = tick.seq;
    const point = tickToPoint(tick);
    const buf = bufferRef.current;
    if (buf.length >= MAX_HISTORY) buf.shift();
    buf.push(point);
    setHistory([...buf]);
  }, [tick]);

  return history;
}
