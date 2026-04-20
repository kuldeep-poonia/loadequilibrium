// Single centralized WebSocket connection. Reconnects with backoff.
// Pushes every TickPayload into the telemetry store.
import { useEffect, useRef } from "react";
import { useTelemetry } from "@/store/telemetryStore";
import type { TickPayload } from "@/lib/types";

const wsUrl = (): string | null => {
  const raw = import.meta.env.VITE_WS_URL as string | undefined;
  if (!raw || raw.length === 0) return null;
  
  // Convert http(s) URL to ws(s) and append /ws endpoint
  const trimmed = raw.replace(/\/$/, "");
  const wsProto = trimmed.startsWith("https") ? "wss" : "ws";
  const base = trimmed.replace(/^https?/, wsProto);
  return `${base}/ws`;
};

export const useWebSocket = (): void => {
  const setStatus = useTelemetry((s) => s.setStatus);
  const applyTick = useTelemetry((s) => s.applyTick);
  const sockRef = useRef<WebSocket | null>(null);
  const retryRef = useRef(0);
  const stoppedRef = useRef(false);

  useEffect(() => {
    const url = wsUrl();
    if (!url) {
      setStatus("idle", "VITE_WS_URL not set");
      return;
    }

    const connect = (): void => {
      if (stoppedRef.current) return;
      setStatus("connecting");
      let sock: WebSocket;
      try {
        sock = new WebSocket(url);
      } catch (err) {
        setStatus("error", err instanceof Error ? err.message : String(err));
        scheduleRetry();
        return;
      }
      sockRef.current = sock;

      sock.onopen = () => {
        retryRef.current = 0;
        setStatus("open");
      };
      sock.onmessage = (ev: MessageEvent<string>) => {
        try {
          const data = JSON.parse(ev.data) as TickPayload;
          if (data && data.type === "tick") applyTick(data);
        } catch {
          // ignore malformed frames
        }
      };
      sock.onerror = () => {
        setStatus("error", "websocket error");
      };
      sock.onclose = () => {
        setStatus("closed");
        scheduleRetry();
      };
    };

    const scheduleRetry = (): void => {
      if (stoppedRef.current) return;
      retryRef.current = Math.min(retryRef.current + 1, 6);
      const delay = Math.min(1000 * 2 ** retryRef.current, 15000);
      window.setTimeout(connect, delay);
    };

    connect();

    return () => {
      stoppedRef.current = true;
      if (sockRef.current) {
        try {
          sockRef.current.close();
        } catch {
          /* noop */
        }
      }
    };
    // setStatus/applyTick are stable from zustand; run once on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
};
