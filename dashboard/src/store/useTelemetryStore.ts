import { create } from 'zustand';
import { TickPayload } from '@/types/backend';

interface HistoryPoint {
  seq: number;
  obj: number;
  casc: number;
  p99: number;
  rhoMean: number;
  tickMs: number;
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
  triggerDomain: (domain: string, payload?: any) => Promise<{ ok: boolean; data?: any; error?: string }>;
}

const MAX_HISTORY = 60;
const API_BASE = 'http://localhost:8081';

export const useTelemetryStore = create<TelemetryState>((set, get) => ({
  tick: null,
  history: [],
  connected: false,
  lastTickMs: 0,
  tickAge: 0,

  setTick: (tick) => set((state) => {
    if (state.tick?.seq === tick.seq) return state;

    const now = Date.now();
    const newHistory = [...state.history, {
      seq: tick.seq,
      obj: tick.objective?.composite_score ?? 0,
      casc: tick.objective?.cascade_failure_probability ?? 0,
      p99: tick.objective?.predicted_p99_latency_ms ?? 0,
      rhoMean: tick.network_equilibrium?.system_rho_mean ?? 0,
      tickMs: tick.tick_health_ms ?? 0,
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
      const res = await fetch(`${API_BASE}/api/v1/control/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });
      if (!res.ok) console.error(`[store] action ${action} failed: ${res.status}`);
    } catch (e) {
      console.error('[store] trigger failed:', e);
    }
  },

  triggerDomain: async (domain: string, payload?: any) => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/${domain}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: payload ? JSON.stringify(payload) : null,
      });
      const data = await res.json().catch(() => null);
      if (!res.ok) {
        return { ok: false, error: data?.error || `HTTP ${res.status}` };
      }
      return { ok: true, data };
    } catch (e: any) {
      return { ok: false, error: e?.message || 'Network error' };
    }
  },
}));
