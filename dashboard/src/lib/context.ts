import { computed, reactive, ref } from 'vue'
import type { Scope, TimeRange } from './types'

export const RANGE_OPTIONS = [
  { value: '1h', label: 'Last hour', milliseconds: 60 * 60 * 1000 },
  { value: '24h', label: 'Last 24 hours', milliseconds: 24 * 60 * 60 * 1000 },
  { value: '7d', label: 'Last 7 days', milliseconds: 7 * 24 * 60 * 60 * 1000 },
  { value: '30d', label: 'Last 30 days', milliseconds: 30 * 24 * 60 * 60 * 1000 },
] as const

export type RangeKey = (typeof RANGE_OPTIONS)[number]['value']
const MAX_SCOPE_VALUE_LENGTH = 256
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/

export const consoleContext = reactive({
  tenant: '',
  billTo: '',
  actor: '',
  feature: '',
  range: '24h' as RangeKey,
})

export const contextRevision = ref(0)
export const hasReportingScope = computed(
  () => consoleContext.tenant.length > 0 && consoleContext.billTo.length > 0,
)

export function setReportingScope(values: Partial<Scope>): void {
  const next = {
    tenant: normalizeScopeValue(values.tenant, 'Tenant reference'),
    billTo: normalizeScopeValue(values.billTo, 'Bill-to reference'),
    actor: normalizeScopeValue(values.actor, 'Actor filter'),
    feature: normalizeScopeValue(values.feature, 'Feature filter'),
  }
  consoleContext.tenant = next.tenant
  consoleContext.billTo = next.billTo
  consoleContext.actor = next.actor
  consoleContext.feature = next.feature
  contextRevision.value += 1
}

export function selectTenant(tenant: string, billTo?: string): void {
  // A tenant-only limit can produce a directory row without a billing
  // assertion. Never invent that missing dimension: exact reporting must wait
  // until an operator supplies the real bill-to reference.
  setReportingScope({ tenant, billTo: billTo ?? '' })
}

export function setRange(value: RangeKey): void {
  if (!RANGE_OPTIONS.some((option) => option.value === value)) {
    throw new Error('Unsupported reporting range.')
  }
  consoleContext.range = value
  contextRevision.value += 1
}

export function currentScope(): Scope {
  return {
    tenant: consoleContext.tenant,
    billTo: consoleContext.billTo,
    actor: consoleContext.actor || undefined,
    feature: consoleContext.feature || undefined,
  }
}

export function currentRange(now = Date.now()): TimeRange {
  if (!Number.isSafeInteger(now) || now < 0) throw new Error('Invalid reporting time.')
  const option =
    RANGE_OPTIONS.find((item) => item.value === consoleContext.range) ?? RANGE_OPTIONS[1]
  return { fromMs: now - option.milliseconds, toMs: now }
}

function normalizeScopeValue(value: string | undefined, label: string): string {
  const normalized = value?.trim() ?? ''
  if (CONTROL_CHARACTERS.test(normalized)) throw new Error(`${label} contains control characters.`)
  if (normalized.length > MAX_SCOPE_VALUE_LENGTH) {
    throw new Error(`${label} must be ${MAX_SCOPE_VALUE_LENGTH} characters or fewer.`)
  }
  return normalized
}
