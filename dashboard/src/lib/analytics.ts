import { toBigInt, toMillis } from './format'
import type { Invocation, TimeRange, UsageEntry } from './types'

export interface UsageTotals {
  requests: bigint
  inputTokens: bigint
  outputTokens: bigint
  cachedTokens: bigint
  reasoningTokens: bigint
  costNanoUsd: bigint
  estimatedRows: number
}

export function summarizeUsage(entries: UsageEntry[]): UsageTotals {
  return entries.reduce<UsageTotals>(
    (total, entry) => ({
      requests: total.requests + toBigInt(entry.requestCount),
      inputTokens: total.inputTokens + toBigInt(entry.inputTokens),
      outputTokens: total.outputTokens + toBigInt(entry.outputTokens),
      cachedTokens:
        total.cachedTokens + toBigInt(entry.cacheReadTokens) + toBigInt(entry.cacheWriteTokens),
      reasoningTokens: total.reasoningTokens + toBigInt(entry.reasoningTokens),
      costNanoUsd: total.costNanoUsd + toBigInt(entry.costNanoUsd),
      estimatedRows: total.estimatedRows + (entry.estimated ? 1 : 0),
    }),
    {
      requests: 0n,
      inputTokens: 0n,
      outputTokens: 0n,
      cachedTokens: 0n,
      reasoningTokens: 0n,
      costNanoUsd: 0n,
      estimatedRows: 0,
    },
  )
}

export function isRejected(invocation: Invocation): boolean {
  return invocation.decision === 'DECISION_STATUS_REJECTED' || invocation.status === 'rejected'
}

export function rejectionRate(invocations: Invocation[]): number {
  if (invocations.length === 0) return 0
  return (invocations.filter(isRejected).length / invocations.length) * 100
}

export interface UsageBucket {
  at: number
  costNanoUsd: bigint
  requests: bigint
}

export function bucketUsage(entries: UsageEntry[], range: TimeRange, count = 24): UsageBucket[] {
  const bucketCount = Number.isSafeInteger(count) ? Math.min(512, Math.max(1, count)) : 24
  const width = Math.max(1, range.toMs - range.fromMs)
  const bucketWidth = width / bucketCount
  const buckets = Array.from({ length: bucketCount }, (_, index) => ({
    at: range.fromMs + index * bucketWidth,
    costNanoUsd: 0n,
    requests: 0n,
  }))
  for (const entry of entries) {
    const createdAt = toMillis(entry.createdAtMs)
    if (createdAt < range.fromMs || createdAt >= range.toMs) continue
    const index = Math.min(bucketCount - 1, Math.floor((createdAt - range.fromMs) / bucketWidth))
    const bucket = buckets[index]
    if (!bucket) continue
    bucket.costNanoUsd += toBigInt(entry.costNanoUsd)
    bucket.requests += toBigInt(entry.requestCount)
  }
  return buckets
}

const SENSITIVE_METADATA_KEY =
  /secret|token|credential|plaintext|ciphertext|raw[_-]?key|password|authorization|api[_-]?key/i
const CREDENTIAL_VALUE =
  /(?:\bBearer\s+\S+|\bkv2_[A-Za-z0-9_-]{24}\.[A-Za-z0-9_-]{43}\b|\bsk-[A-Za-z0-9_-]{12,}\b)/i

export function safeAuditMetadata(
  metadata: Record<string, string> | undefined,
  maximum = 4,
): Array<[string, string]> {
  const limit = Number.isSafeInteger(maximum) ? Math.min(16, Math.max(0, maximum)) : 4
  const result: Array<[string, string]> = []
  for (const [rawKey, rawValue] of Object.entries(metadata ?? {})) {
    if (result.length >= limit) break
    if (SENSITIVE_METADATA_KEY.test(rawKey)) continue
    const key = printable(rawKey, 64)
    if (!key) continue
    const value = CREDENTIAL_VALUE.test(rawValue) ? '[redacted]' : printable(rawValue, 160)
    result.push([key, value])
  }
  return result
}

function printable(value: string, maximum: number): string {
  const clean = value.replace(/[\u0000-\u001f\u007f]/g, '�')
  return clean.length <= maximum ? clean : `${clean.slice(0, maximum - 1)}…`
}

export interface ModelTotal {
  model: string
  provider: string
  requests: bigint
  tokens: bigint
  costNanoUsd: bigint
}

export function usageByModel(entries: UsageEntry[]): ModelTotal[] {
  const totals = new Map<string, ModelTotal>()
  for (const entry of entries) {
    const model = entry.model || 'unattributed'
    const provider = entry.provider || 'custom'
    const key = `${provider}\u0000${model}`
    const total = totals.get(key) ?? { model, provider, requests: 0n, tokens: 0n, costNanoUsd: 0n }
    total.requests += toBigInt(entry.requestCount)
    total.tokens += toBigInt(entry.inputTokens) + toBigInt(entry.outputTokens)
    total.costNanoUsd += toBigInt(entry.costNanoUsd)
    totals.set(key, total)
  }
  return [...totals.values()].sort((left, right) =>
    left.costNanoUsd === right.costNanoUsd ? 0 : left.costNanoUsd > right.costNanoUsd ? -1 : 1,
  )
}
