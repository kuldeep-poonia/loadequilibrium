"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getWsUrl } from "@/services/api";
import type { TickPayload } from "@/types/tick";

type ConnectionState = "connecting" | "connected" | "disconnected" | "error";

interface WsState {
  tick: TickPayload | null;
  connectionState: ConnectionState;
  lastSeq: number;
  reconnectCount: number;
}

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_MAX_MS = 30_000;
const RECONNECT_JITTER_MS = 500;

function backoff(attempt: number): number {
  const delay = Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_MAX_MS);
  return delay + Math.random() * RECONNECT_JITTER_MS;
}

export function useTickStream(): WsState {
  const [state, setState] = useState<WsState>({
    tick: null,
    connectionState: "connecting",
    lastSeq: 0,
    reconnectCount: 0,
  });

  const wsRef = useRef<WebSocket | null>(null);
  const attemptRef = useRef(0);
  const unmountedRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connect = useCallback(() => {
    if (unmountedRef.current) return;

    const url = getWsUrl();
    const ws = new WebSocket(url);
    wsRef.current = ws;

    setState((prev) => ({ ...prev, connectionState: "connecting" }));

    ws.onopen = () => {
      if (unmountedRef.current) { ws.close(); return; }
      attemptRef.current = 0;
      setState((prev) => ({ ...prev, connectionState: "connected", reconnectCount: prev.reconnectCount }));
    };

    ws.onmessage = (event: MessageEvent) => {
      if (unmountedRef.current) return;
      try {
        const payload = JSON.parse(event.data as string) as TickPayload;
        if (payload.type === "ping") return;
        setState((prev) => ({
          ...prev,
          tick: payload,
          lastSeq: payload.seq,
        }));
      } catch {
        // Malformed frame — skip
      }
    };

    ws.onerror = () => {
      if (unmountedRef.current) return;
      setState((prev) => ({ ...prev, connectionState: "error" }));
    };

    ws.onclose = () => {
      if (unmountedRef.current) return;
      setState((prev) => ({ ...prev, connectionState: "disconnected" }));
      const delay = backoff(attemptRef.current);
      attemptRef.current += 1;
      timerRef.current = setTimeout(() => {
        setState((prev) => ({ ...prev, reconnectCount: prev.reconnectCount + 1 }));
        connect();
      }, delay);
    };
  }, []);

  useEffect(() => {
    unmountedRef.current = false;
    connect();
    return () => {
      unmountedRef.current = true;
      if (timerRef.current) clearTimeout(timerRef.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return state;
}
