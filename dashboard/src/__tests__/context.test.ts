import { beforeEach, describe, expect, it } from 'vitest'
import {
  consoleContext,
  currentRange,
  currentScope,
  hasReportingScope,
  selectTenant,
  setRange,
  setReportingScope,
} from '@/lib/context'

describe('reporting context', () => {
  beforeEach(() => {
    setReportingScope({ tenant: '', billTo: '', actor: '', feature: '' })
    setRange('24h')
  })

  it('requires an exact tenant and billing boundary', () => {
    setReportingScope({ tenant: ' tenant-a ', billTo: ' invoice-a ', actor: 'agent-a' })

    expect(hasReportingScope.value).toBe(true)
    expect(currentScope()).toEqual({
      tenant: 'tenant-a',
      billTo: 'invoice-a',
      actor: 'agent-a',
      feature: undefined,
    })
  })

  it('selects opaque tenants and computes bounded ranges', () => {
    selectTenant('tenant-a')
    setRange('1h')

    expect(currentScope()).toMatchObject({ tenant: 'tenant-a', billTo: '' })
    expect(hasReportingScope.value).toBe(false)
    selectTenant('tenant-a', 'billing-a')
    expect(currentScope()).toMatchObject({ tenant: 'tenant-a', billTo: 'billing-a' })
    expect(hasReportingScope.value).toBe(true)
    expect(currentRange(10_000_000)).toEqual({ fromMs: 6_400_000, toMs: 10_000_000 })
  })

  it('rejects invalid range and scope values without partially applying them', () => {
    setReportingScope({ tenant: 'stable', billTo: 'billing' })
    expect(() => setReportingScope({ tenant: 'changed', billTo: 'bad\u0000value' })).toThrow(
      'control characters',
    )
    expect(consoleContext.tenant).toBe('stable')
    expect(() => setRange('year' as never)).toThrow('Unsupported')
  })
})
