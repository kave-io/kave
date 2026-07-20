import { describe, expect, it } from 'vitest'
import {
  bucketUsage,
  rejectionRate,
  safeAuditMetadata,
  summarizeUsage,
  usageByModel,
} from '@/lib/analytics'
import type { Invocation, UsageEntry } from '@/lib/types'

function usage(overrides: Partial<UsageEntry> = {}): UsageEntry {
  return {
    id: 'usage-1',
    invocationId: 'inv-1',
    metric: 'requests',
    units: 1n,
    costNanoUsd: 15n,
    provider: 'openai',
    model: 'gpt-test',
    attempt: 1,
    eventKind: 'provider.settled',
    createdAtMs: 1_500n,
    requestCount: 1n,
    inputTokens: 10n,
    outputTokens: 5n,
    cacheReadTokens: 3n,
    cacheWriteTokens: 2n,
    reasoningTokens: 4n,
    estimated: false,
    ...overrides,
  }
}

describe('ledger analytics', () => {
  it('uses exact bigint arithmetic for totals and model groups', () => {
    const entries = [
      usage({ costNanoUsd: 9_007_199_254_740_993n }),
      usage({ id: 'usage-2', requestCount: 2n, inputTokens: 7n, estimated: true }),
    ]

    expect(summarizeUsage(entries)).toMatchObject({
      requests: 3n,
      inputTokens: 17n,
      outputTokens: 10n,
      cachedTokens: 10n,
      reasoningTokens: 8n,
      costNanoUsd: 9_007_199_254_741_008n,
      estimatedRows: 1,
    })
    expect(usageByModel(entries)[0]).toMatchObject({ requests: 3n, tokens: 27n })
  })

  it('buckets only events inside the half-open reporting interval', () => {
    const buckets = bucketUsage(
      [
        usage({ createdAtMs: 999n }),
        usage({ createdAtMs: 1_000n, requestCount: 2n }),
        usage({ createdAtMs: 1_999n, requestCount: 3n }),
        usage({ createdAtMs: 2_000n, requestCount: 9n }),
      ],
      { fromMs: 1_000, toMs: 2_000 },
      2,
    )

    expect(buckets.map((bucket) => bucket.requests)).toEqual([2n, 3n])
  })

  it('redacts credential-like metadata and drops sensitive fields', () => {
    const key = `kv2_${'A'.repeat(24)}.${'B'.repeat(42)}A`
    const rows = safeAuditMetadata({
      operation: 'provider.activate',
      authorization: `Bearer ${key}`,
      note: `attempted with ${key}`,
      password_hint: 'never show',
      multiline: 'safe\nbut controlled',
    })

    expect(rows).toEqual([
      ['operation', 'provider.activate'],
      ['note', '[redacted]'],
      ['multiline', 'safe�but controlled'],
    ])
  })

  it('computes rejection rate from normalized decisions', () => {
    const base: Invocation = {
      id: 'inv-1',
      agent: 'agent',
      model: 'model',
      decision: 'DECISION_STATUS_ADMITTED',
      status: 'settled',
      createdAtMs: 1n,
    }
    expect(
      rejectionRate([
        base,
        { ...base, id: 'inv-2', decision: 'DECISION_STATUS_REJECTED', status: 'rejected' },
      ]),
    ).toBe(50)
  })
})
