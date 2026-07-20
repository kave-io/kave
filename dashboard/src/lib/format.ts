import type { Int64 } from './types'

export function toBigInt(value: Int64 | null | undefined): bigint {
  if (typeof value === 'bigint') return value
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value)) return 0n
    return BigInt(value)
  }
  if (!value || !/^-?\d+$/.test(value)) return 0n
  return BigInt(value)
}

export function toMillis(value: Int64 | null | undefined): number {
  const parsed = Number(toBigInt(value))
  return Number.isSafeInteger(parsed) ? parsed : 0
}

export function formatInteger(value: Int64 | null | undefined): string {
  return toBigInt(value).toLocaleString('en-US')
}

export function formatCompactInteger(value: Int64 | null | undefined): string {
  const amount = toBigInt(value)
  const absolute = amount < 0n ? -amount : amount
  const scales = [
    { threshold: 1_000_000_000_000n, suffix: 'T' },
    { threshold: 1_000_000_000n, suffix: 'B' },
    { threshold: 1_000_000n, suffix: 'M' },
    { threshold: 1_000n, suffix: 'K' },
  ]
  const scale = scales.find((item) => absolute >= item.threshold)
  if (!scale) return amount.toLocaleString('en-US')
  const tenths = (amount * 10n) / scale.threshold
  const whole = tenths / 10n
  const fraction = tenths < 0n ? -(tenths % 10n) : tenths % 10n
  return `${whole.toString()}${fraction === 0n ? '' : `.${fraction.toString()}`}${scale.suffix}`
}

export function formatNanoUsd(value: Int64 | null | undefined): string {
  const nanos = toBigInt(value)
  const sign = nanos < 0n ? '-' : ''
  const absolute = nanos < 0n ? -nanos : nanos
  const dollars = absolute / 1_000_000_000n
  const remainder = absolute % 1_000_000_000n
  if (absolute === 0n) return '$0.00'
  if (absolute < 1_000n) return sign ? '>-$0.000001' : '<$0.000001'
  if (dollars === 0n && remainder < 1_000_000n) return `${sign}$${formatDecimal(remainder, 9, 6)}`
  return `${sign}$${formatDecimal(absolute, 9, dollars < 1n ? 4 : 2)}`
}

function formatDecimal(value: bigint, scaleDigits: number, visibleDigits: number): string {
  const scale = 10n ** BigInt(scaleDigits)
  const whole = value / scale
  const fraction = (value % scale).toString().padStart(scaleDigits, '0').slice(0, visibleDigits)
  return `${whole.toLocaleString('en-US')}.${fraction}`
}

export function formatTime(value: Int64 | null | undefined): string {
  const millis = toMillis(value)
  if (!isValidDateMillis(millis)) return '—'
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(millis))
}

export function formatRelativeTime(value: Int64 | null | undefined, now = Date.now()): string {
  const millis = toMillis(value)
  if (!isValidDateMillis(millis) || !Number.isSafeInteger(now)) return 'never'
  const seconds = Math.round((millis - now) / 1000)
  const absolute = Math.abs(seconds)
  const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
  if (absolute < 60) return formatter.format(seconds, 'second')
  if (absolute < 3600) return formatter.format(Math.round(seconds / 60), 'minute')
  if (absolute < 86_400) return formatter.format(Math.round(seconds / 3600), 'hour')
  return formatter.format(Math.round(seconds / 86_400), 'day')
}

export function dateTimeAttribute(value: Int64 | null | undefined): string | undefined {
  const millis = toMillis(value)
  return isValidDateMillis(millis) ? new Date(millis).toISOString() : undefined
}

function isValidDateMillis(value: number): boolean {
  return value !== 0 && Number.isFinite(value) && Math.abs(value) <= 8_640_000_000_000_000
}

export function shortId(value: string, visible = 10): string {
  if (!value) return '—'
  return value.length <= visible + 4 ? value : `${value.slice(0, visible)}…${value.slice(-3)}`
}

export function humanize(value: string): string {
  if (!value) return 'unknown'
  return value
    .replace(/^DECISION_STATUS_/, '')
    .replace(/^AGENT_KIND_/, '')
    .replace(/^LIMIT_WINDOW_/, '')
    .replace(/_/g, ' ')
    .replace(/\./g, ' · ')
    .toLowerCase()
}
