export const safe = (v) => {
  if (v === null || v === undefined) return null
  const n = Number(v)
  return isFinite(n) ? n : null
}

export const n = (v, d = 2) => {
  const s = safe(v)
  return s === null ? '—' : s.toFixed(d)
}

export const pct = (v) => {
  const s = safe(v)
  if (s === null) return '—'
  if (s > 99.99) return '>9999%'
  return (s * 100).toFixed(1) + '%'
}

export const rho = (v) => {
  const s = safe(v)
  if (s === null) return '—'
  if (s > 9.99) return '>999%'
  return (s * 100).toFixed(1) + '%'
}

export const ms = (v) => {
  const s = safe(v)
  if (s === null) return '—'
  if (s >= 1000) return (s / 1000).toFixed(1) + 's'
  return s.toFixed(0) + 'ms'
}

export const rps = (v) => {
  const s = safe(v)
  if (s === null) return '—'
  if (s >= 1000000) return (s / 1000000).toFixed(1) + 'M/s'
  if (s >= 1000) return (s / 1000).toFixed(1) + 'k/s'
  return s.toFixed(1) + '/s'
}

export const ts = (v) => {
  if (!v) return new Date().toLocaleTimeString('en-GB', { hour12: false })
  const d = new Date(v)
  return isNaN(d) ? '—' : d.toLocaleTimeString('en-GB', { hour12: false })
}

export const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v))