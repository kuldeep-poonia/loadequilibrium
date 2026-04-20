// REST control plane client. Uses VITE_API_URL.
const BASE = (import.meta.env.VITE_API_URL as string | undefined)?.replace(
  /\/$/,
  "",
) ?? "";

const headers = (): HeadersInit => {
  const h: Record<string, string> = { "Content-Type": "application/json" };
  const token = import.meta.env.VITE_INGEST_TOKEN as string | undefined;
  if (token) h["X-Ingest-Token"] = token;
  return h;
};

const post = async <T>(path: string, body?: unknown): Promise<T> => {
  if (!BASE) throw new Error("VITE_API_URL not configured");
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: headers(),
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    throw new Error(`${path} failed: ${res.status}`);
  }
  return (await res.json()) as T;
};

export const api = {
  toggleActuation: () =>
    post<{ actuation_enabled: boolean }>("/api/v1/control/toggle"),
  chaosRun: (scenario: string, duration_ticks: number) =>
    post<{ scheduled_until_tick: number }>("/api/v1/control/chaos-run", {
      scenario,
      duration_ticks,
    }),
  updatePolicy: (preset: "balanced" | "conservative" | "aggressive") =>
    post<{ policy: string }>("/api/v1/policy/update", { preset }),
  ackAlert: (alert_id: string) =>
    post<{ acknowledged_count: number }>("/api/v1/alerts/ack", { alert_id }),
};

export const apiConfigured = (): boolean => BASE.length > 0;
export const apiBase = (): string => BASE;
