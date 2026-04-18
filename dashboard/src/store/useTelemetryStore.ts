import { create } from 'zustand';
import { TickPayload } from '@/types/backend';
import { API_BASE_URL } from '@/lib/config';

type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };
type DomainPayload = Record<string, JsonValue>;
type ApiResult = { ok: true; data?: unknown } | { ok: false; error: string };

interface HistoryPoint {
  seq: number;
  obj: number;
  casc: number;
  p99: number;
  rhoMean: number;
  tickMs: number;
  throughput: number;
  queueDepth: number;
  workers: number;
}

interface TelemetryState {
  tick: TickPayload | null;
  history: HistoryPoint[];
  connected: boolean;
  lastTickMs: number;
  tickAge: number; // ms since last tick

  setTick: (tick: TickPayload) => void;
  setConnected: (connected: boolean) => void;
  reset: () => void;
  triggerAction: (action: string) => Promise<void>;
  triggerDomain: (domain: string, payload?: DomainPayload) => Promise<ApiResult>;
}

const MAX_HISTORY = 60;

function responseError(data: unknown) {
  if (data && typeof data === 'object' && 'error' in data) {
    const error = (data as { error?: unknown }).error;
    if (typeof error === 'string') return error;
  }
  return undefined;
}

export const useTelemetryStore = create<TelemetryState>((set) => ({
  tick: null,
  history: [],
  connected: false,
  lastTickMs: 0,
  tickAge: 0,

  setTick: (tick) => set((state) => {
    if (state.tick?.seq === tick.seq) return state;

    const now = Date.now();
    let throughput = 0;
    let queueDepth = 0;
    let workers = 0;

    if (tick.bundles) {
      for (const serviceId in tick.bundles) {
        const queue = tick.bundles[serviceId]?.queue;
        if (queue) {
          throughput += queue.arrival_rate || 0;
          queueDepth += queue.mean_queue_len || 0;
          workers += queue.concurrency || 0;
        }
      }
    }

    const newHistory = [...state.history, {
      seq: tick.seq,
      obj: tick.objective?.composite_score ?? 0,
      casc: tick.objective?.cascade_failure_probability ?? 0,
      p99: tick.objective?.predicted_p99_latency_ms ?? 0,
      rhoMean: tick.network_equilibrium?.system_rho_mean ?? 0,
      tickMs: tick.tick_health_ms ?? 0,
      throughput,
      queueDepth,
      workers,
    }].slice(-MAX_HISTORY);

    return {
      tick,
      history: newHistory,
      lastTickMs: now,
      tickAge: 0,
    };
  }),

  setConnected: (connected) => set({ connected }),

  reset: () => set({ tick: null, history: [], connected: false, lastTickMs: 0, tickAge: 0 }),

  triggerAction: async (action: string) => {
    try {
      const res = await fetch(`${API_BASE_URL}/api/v1/control/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });
      if (!res.ok) console.error(`[store] action ${action} failed: ${res.status}`);
    } catch (e) {
      console.error('[store] trigger failed:', e);
    }
  },

  triggerDomain: async (domain: string, payload?: DomainPayload) => {
    try {
      const res = await fetch(`${API_BASE_URL}/api/v1/${domain}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: payload ? JSON.stringify(payload) : undefined,
      });
      const data = await res.json().catch(() => null);
      if (!res.ok) {
        return { ok: false, error: responseError(data) || `HTTP ${res.status}` };
      }
      return { ok: true, data };
    } catch (e: unknown) {
      return { ok: false, error: e instanceof Error ? e.message : 'Network error' };
    }
  },
}));
