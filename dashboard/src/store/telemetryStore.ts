// Global telemetry store. Holds latest TickPayload + small per-service history
// for trend lines. Designed for 10Hz updates: shallow replace, capped buffers.
import { create } from "zustand";
import type {
  ConnectionStatus,
  ServiceBundle,
  Event,
  TickPayload,
} from "@/lib/types";

const HISTORY_LEN = 120; // ~12s at 10Hz

export interface ServiceHistoryPoint {
  seq: number;
  utilisation: number;
  collapse_risk: number;
  saturation_horizon: number;
}

interface TelemetryState {
  status: ConnectionStatus;
  lastError: string | null;
  tick: TickPayload | null;
  lastSeq: number;
  receivedAt: number;
  history: Record<string, ServiceHistoryPoint[]>;
  ackedEventIds: Set<string>;
  selectedServiceId: string | null;

  setStatus: (s: ConnectionStatus, err?: string | null) => void;
  applyTick: (t: TickPayload) => void;
  selectService: (id: string | null) => void;
  ackEvent: (id: string) => void;
}

export const useTelemetry = create<TelemetryState>((set) => ({
  status: "idle",
  lastError: null,
  tick: null,
  lastSeq: -1,
  receivedAt: 0,
  history: {},
  ackedEventIds: new Set<string>(),
  selectedServiceId: null,

  setStatus: (s, err = null) =>
    set((state) => {
      const nextErr = err ?? null;
      if (state.status === s && state.lastError === nextErr) return state;
      return { status: s, lastError: nextErr };
    }),

  applyTick: (t) =>
    set((state) => {
      // Append history per service (capped).
      const nextHistory: Record<string, ServiceHistoryPoint[]> = {
        ...state.history,
      };
      for (const [id, b] of Object.entries(t.bundles ?? {}) as [
        string,
        ServiceBundle,
      ][]) {
        const prev = nextHistory[id] ?? [];
        const point: ServiceHistoryPoint = {
          seq: t.seq,
          utilisation: b.queue.utilisation,
          collapse_risk: b.stability.collapse_risk,
          saturation_horizon: b.queue.saturation_horizon,
        };
        const next =
          prev.length >= HISTORY_LEN
            ? [...prev.slice(prev.length - HISTORY_LEN + 1), point]
            : [...prev, point];
        nextHistory[id] = next;
      }
      // Drop history for services no longer present (keep memory bounded).
      for (const id of Object.keys(nextHistory)) {
        if (!(id in (t.bundles ?? {}))) delete nextHistory[id];
      }
      return {
        tick: t,
        lastSeq: t.seq,
        receivedAt: Date.now(),
        history: nextHistory,
      };
    }),

  selectService: (id) => set({ selectedServiceId: id }),

  ackEvent: (id) =>
    set((state) => {
      const next = new Set(state.ackedEventIds);
      next.add(id);
      return { ackedEventIds: next };
    }),
}));

export const selectActiveEvents = (s: TelemetryState): Event[] => {
  const events = s.tick?.events ?? [];
  return events.filter((e) => !s.ackedEventIds.has(e.id ?? ""));
};
