import type {
  AlertAckRequest,
  AlertAckResponse,
  ChaosRunRequest,
  ChaosRunResponse,
  ControlToggleRequest,
  ControlToggleResponse,
  HealthResponse,
  IntelligenceRolloutRequest,
  IntelligenceRolloutResponse,
  PolicyUpdateRequest,
  PolicyUpdateResponse,
  ReplayBurstRequest,
  ReplayBurstResponse,
  RuntimeStepResponse,
  SandboxTriggerRequest,
  SandboxTriggerResponse,
  SimulationControlRequest,
  SimulationControlResponse,
} from "@/types/api";
import type { TickPayload } from "@/types/tick";

const BASE_URL =
  process.env.NEXT_PUBLIC_BACKEND_URL?.replace(/\/$/, "") ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { "Content-Type": "application/json", ...init?.headers },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error((body as { error?: string }).error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export const api = {
  health: (): Promise<HealthResponse> => request<HealthResponse>("/health"),

  snapshot: (): Promise<TickPayload> => request<TickPayload>("/api/v1/snapshot"),

  toggleActuation: (body?: ControlToggleRequest): Promise<ControlToggleResponse> =>
    post<ControlToggleResponse>("/api/v1/control/toggle", body),

  chaosRun: (body?: ChaosRunRequest): Promise<ChaosRunResponse> =>
    post<ChaosRunResponse>("/api/v1/control/chaos-run", body),

  replayBurst: (body?: ReplayBurstRequest): Promise<ReplayBurstResponse> =>
    post<ReplayBurstResponse>("/api/v1/control/replay-burst", body),

  updatePolicy: (body: PolicyUpdateRequest): Promise<PolicyUpdateResponse> =>
    post<PolicyUpdateResponse>("/api/v1/policy/update", body),

  runtimeStep: (): Promise<RuntimeStepResponse> =>
    post<RuntimeStepResponse>("/api/v1/runtime/step"),

  sandboxTrigger: (body?: SandboxTriggerRequest): Promise<SandboxTriggerResponse> =>
    post<SandboxTriggerResponse>("/api/v1/sandbox/trigger", body),

  simulationControl: (body: SimulationControlRequest): Promise<SimulationControlResponse> =>
    post<SimulationControlResponse>("/api/v1/simulation/control", body),

  intelligenceRollout: (body?: IntelligenceRolloutRequest): Promise<IntelligenceRolloutResponse> =>
    post<IntelligenceRolloutResponse>("/api/v1/intelligence/rollout", body),

  alertAck: (body: AlertAckRequest): Promise<AlertAckResponse> =>
    post<AlertAckResponse>("/api/v1/alerts/ack", body),
};

export function getWsUrl(): string {
  const backendUrl = process.env.NEXT_PUBLIC_BACKEND_URL?.replace(/\/$/, "") ?? "http://localhost:8080";
  return backendUrl.replace(/^http/, "ws") + "/ws";
}
