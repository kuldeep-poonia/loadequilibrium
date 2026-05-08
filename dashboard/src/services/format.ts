export function pct(v?: number | null, decimals = 1): string {
  if (v === undefined || v === null || Number.isNaN(v)) {
    return `${(0).toFixed(decimals)}%`;
  }

  return `${(v * 100).toFixed(decimals)}%`;
}

export function ms(v?: number | null, decimals = 1): string {
  if (v === undefined || v === null || Number.isNaN(v)) {
    return `${(0).toFixed(decimals)}ms`;
  }

  return `${v.toFixed(decimals)}ms`;
}

export function fixed(v?: number | null, d = 3): string {
  if (v === undefined || v === null || Number.isNaN(v)) {
    return (0).toFixed(d);
  }

  return v.toFixed(d);
}

export function rho(v?: number | null): string {
  if (v === undefined || v === null || Number.isNaN(v)) {
    return "0.000";
  }

  return v.toFixed(3);
}

export function severityLabel(
  s?: number | null,
): "INFO" | "WARN" | "CRIT" {
  if (s === undefined || s === null || Number.isNaN(s)) {
    return "INFO";
  }

  if (s === 0) return "INFO";
  if (s === 1) return "WARN";
  return "CRIT";
}

export function severityVariant(
  s?: number | null,
): "info" | "warning" | "critical" {
  if (s === undefined || s === null || Number.isNaN(s)) {
    return "info";
  }

  if (s === 0) return "info";
  if (s === 1) return "warning";
  return "critical";
}

export function urgencyVariant(
  cls: string,
): "success" | "warning" | "danger" | "critical" | "info" | "muted" {
  switch (cls) {
    case "critical":
      return "critical";

    case "warning":
      return "warning";

    case "elevated":
      return "info";

    default:
      return "muted";
  }
}

export function stabilityZoneVariant(
  zone: string,
): "success" | "warning" | "danger" | "critical" | "info" | "muted" {
  switch (zone) {
    case "safe":
      return "success";

    case "warning":
      return "warning";

    case "collapse":
      return "critical";

    default:
      return "muted";
  }
}

export function relativeTime(iso?: string | null): string {
  if (!iso) {
    return "unknown";
  }

  const parsed = new Date(iso).getTime();

  if (Number.isNaN(parsed)) {
    return "unknown";
  }

  const diff = Date.now() - parsed;

  const s = Math.floor(diff / 1000);

  if (s < 60) {
    return `${s}s ago`;
  }

  const m = Math.floor(s / 60);

  if (m < 60) {
    return `${m}m ago`;
  }

  return `${Math.floor(m / 60)}h ago`;
}