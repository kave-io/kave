export function fmtMoney(value: number | string | null | undefined, currency = 'USD'): string {
  if (value == null || value === '') return '-'
  const n = typeof value === 'string' ? Number(value) : value
  if (!Number.isFinite(n)) return String(value)
  if (currency === 'USD') return '$' + n.toFixed(Math.abs(n) < 1 ? 4 : 2)
  if (currency === 'IRT') return Math.round(n * 60000).toLocaleString() + ' IRR'
  return n.toFixed(2) + ' ' + currency
}

export function fmtMs(ms: number | null | undefined): string {
  if (ms == null || !Number.isFinite(ms)) return '-'
  if (ms < 1000) return Math.round(ms) + 'ms'
  return (ms / 1000).toFixed(2) + 's'
}

export function fmtRel(ts: number | null | undefined): string {
  if (!ts) return '-'
  const delta = Math.max(0, (Date.now() - ts) / 1000)
  if (delta < 5) return 'just now'
  if (delta < 60) return Math.floor(delta) + 's ago'
  if (delta < 3600) return Math.floor(delta / 60) + 'm ago'
  if (delta < 86400) return Math.floor(delta / 3600) + 'h ago'
  return Math.floor(delta / 86400) + 'd ago'
}

export function fmtTime(ts: number | null | undefined): string {
  if (!ts) return '--:--:--'
  return new Date(ts).toTimeString().slice(0, 8)
}

export function asNumber(value: string | number | null | undefined): number {
  if (value == null || value === '') return 0
  const n = typeof value === 'string' ? Number(value) : value
  return Number.isFinite(n) ? n : 0
}
