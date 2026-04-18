'use client';

import { useEffect, useRef, useCallback } from 'react';
import { useTelemetryStore } from '@/store/useTelemetryStore';
import { normalizeTickPayload } from '@/lib/telemetry';
import { WS_URL } from '@/lib/config';

const BACKOFF_BASE = 1000;
const BACKOFF_MAX = 30000;
const STALE_TIMEOUT = 10000;

export function useWebSocket() {
  const { setTick, setConnected } = useTelemetryStore();
  const socketRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<NodeJS.Timeout | null>(null);
  const staleTimerRef = useRef<NodeJS.Timeout | null>(null);
  const attemptRef = useRef(0);
  const lastSeqRef = useRef(0);
  const mountedRef = useRef(true);

  const resetStaleTimer = useCallback(() => {
    if (staleTimerRef.current) clearTimeout(staleTimerRef.current);
    staleTimerRef.current = setTimeout(() => {
      if (mountedRef.current) {
        console.warn('[ws] stale stream — no tick in 10s');
        setConnected(false);
      }
    }, STALE_TIMEOUT);
  }, [setConnected]);

  // connect declared first, scheduleReconnect references via ref
  const connectRef = useRef<(() => void) | null>(null);

  // Declare scheduleReconnect FIRST to fix hoisting / temporal dead zone
  const scheduleReconnect = useCallback(() => {
    if (reconnectTimerRef.current) return;
    if (!mountedRef.current) return;
    
    // Exponential backoff with jitter
    const base = Math.min(BACKOFF_BASE * Math.pow(2, attemptRef.current), BACKOFF_MAX);
    const jitter = base * 0.3 * Math.random();
    const delay = base + jitter;
    attemptRef.current++;
    
    console.log(`[ws] reconnect in ${Math.round(delay)}ms (attempt ${attemptRef.current})`);
    reconnectTimerRef.current = setTimeout(() => {
      reconnectTimerRef.current = null;
      if (mountedRef.current && connectRef.current) {
        connectRef.current();
      }
    }, delay);
  }, []);

  const connect = useCallback(() => {
    if (typeof window === 'undefined') return;
    if (socketRef.current?.readyState === WebSocket.OPEN) return;
    if (socketRef.current?.readyState === WebSocket.CONNECTING) return;

    try {
      const socket = new WebSocket(WS_URL);
      socketRef.current = socket;

      socket.onopen = () => {
        if (!mountedRef.current) { socket.close(); return; }
        console.log('[ws] connected');
        setConnected(true);
        attemptRef.current = 0;
        resetStaleTimer();
      };

      socket.onmessage = (event) => {
        if (!mountedRef.current) return;
        try {
          const raw = JSON.parse(event.data);
          if (raw.type === 'tick') {
            const seq = raw.seq ?? raw.sequence_no ?? 0;
            // Sequence gap detection
            if (lastSeqRef.current > 0 && seq > lastSeqRef.current + 1) {
              console.warn(`[ws] seq gap: expected ${lastSeqRef.current + 1}, got ${seq} (${seq - lastSeqRef.current - 1} dropped)`);
            }
            lastSeqRef.current = seq;

            const tick = normalizeTickPayload(raw);
            if (tick) {
              setTick(tick);
              resetStaleTimer();
            }
          }
        } catch (err) {
          console.error('[ws] parse error', err);
        }
      };

      socket.onerror = () => {
        if (!mountedRef.current) return;
        setConnected(false);
      };

      socket.onclose = () => {
        if (!mountedRef.current) return;
        setConnected(false);
        socketRef.current = null;
        scheduleReconnect();
      };
    } catch {
      scheduleReconnect();
    }
  }, [setTick, setConnected, resetStaleTimer, scheduleReconnect]);

  useEffect(() => {
    mountedRef.current = true;
    connect();
    return () => {
      mountedRef.current = false;
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (staleTimerRef.current) {
        clearTimeout(staleTimerRef.current);
        staleTimerRef.current = null;
      }
    };
  }, [connect]);
}
