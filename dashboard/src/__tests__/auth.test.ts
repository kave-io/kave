import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  authSessionStorageKey,
  authState,
  connectCredential,
  currentCredential,
  disconnectCredential,
  setNamespaceId,
} from '@/lib/auth'

const canonicalKey = `kv2_${'A'.repeat(24)}.${'B'.repeat(42)}A`

describe('credential session', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    sessionStorage.clear()
    disconnectCredential()
  })

  it('keeps a service key in memory by default', () => {
    connectCredential(canonicalKey, ' namespace/prod ', false)

    expect(currentCredential()).toBe(canonicalKey)
    expect(authState.namespaceId.value).toBe('namespace/prod')
    expect(sessionStorage.getItem(authSessionStorageKey)).toBeNull()
  })

  it('uses tab-scoped storage only with explicit opt-in and clears it', () => {
    connectCredential(canonicalKey, 'nsp_prod', true)

    expect(JSON.parse(sessionStorage.getItem(authSessionStorageKey) ?? '{}')).toEqual({
      serviceKey: canonicalKey,
      namespaceId: 'nsp_prod',
    })
    setNamespaceId('nsp_next')
    expect(sessionStorage.getItem(authSessionStorageKey)).toContain('nsp_next')

    disconnectCredential()
    expect(sessionStorage.getItem(authSessionStorageKey)).toBeNull()
    expect(() => currentCredential()).toThrow('required')
  })

  it('rejects malformed, non-canonical, and control-bearing input', () => {
    expect(() => connectCredential('kv2_short.secret', '', false)).toThrow('canonical')
    expect(() => connectCredential(`kv2_${'A'.repeat(24)}.${'B'.repeat(43)}`, '', false)).toThrow(
      'canonical',
    )
    expect(() => connectCredential(canonicalKey, 'bad\nnamespace', false)).toThrow(
      'control characters',
    )
    expect(authState.connected.value).toBe(false)
  })

  it('falls back to memory when tab storage is unavailable', () => {
    const unavailableStorage = {
      length: 0,
      clear: vi.fn(),
      getItem: vi.fn(() => null),
      key: vi.fn(() => null),
      removeItem: vi.fn(),
      setItem: vi.fn(() => {
        throw new DOMException('storage disabled', 'SecurityError')
      }),
    } satisfies Storage
    vi.stubGlobal('sessionStorage', unavailableStorage)

    connectCredential(canonicalKey, 'nsp_prod', true)

    expect(currentCredential()).toBe(canonicalKey)
    expect(authState.rememberedForTab.value).toBe(false)
    expect(unavailableStorage.removeItem).toHaveBeenCalledWith(authSessionStorageKey)
  })
})
