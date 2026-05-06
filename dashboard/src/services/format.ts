export function pct(v: number, decimals = 1): string {
  return `${(v * 100).toFixed(decimals)}%`;
}

export function ms(v: number, decimals = 1): string {
  return `${v.toFixed(decimals)}ms`;
}

export function fixed(v: number, d = 3): string {
  return v.toFixed(d);
}

export function rho(v: number): string {
  return v.toFixed(3);
}

export function severityLabel(s: number): "INFO" | "WARN" | "CRIT" {
  if (s === 0) return "INFO";
  if (s === 1) return "WARN";
  return "CRIT";
}

export function severityVariant(s: number): "info" | "warning" | "critical" {
  if (s === 0) return "info";
  if (s === 1) return "warning";
  return "critical";
}

export function urgencyVariant(cls: string): "success" | "warning" | "danger" | "critical" | "info" | "muted" {
  switch (cls) {
    case "critical": return "critical";
    case "warning": return "warning";
    case "elevated": return "info";
    default: return "muted";
  }
}

export function stabilityZoneVariant(zone: string): "success" | "warning" | "danger" | "critical" | "info" | "muted" {
  switch (zone) {
    case "safe": return "success";
    case "warning": return "warning";
    case "collapse": return "critical";
    default: return "muted";
  }
}

export function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  return `${Math.floor(m / 60)}h ago`;
}
