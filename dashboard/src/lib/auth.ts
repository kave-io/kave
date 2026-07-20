import { computed, readonly, ref, shallowRef } from 'vue'

const SESSION_KEY = 'kave.console.v2.credential'
// A 32-byte unpadded base64url secret has 43 characters. Its final character
// can only carry four significant bits, so restricting it prevents accepting
// non-canonical encodings that the kernel will reject.
const KEY_PATTERN = /^kv2_[A-Za-z0-9_-]{24}\.[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$/

interface StoredCredential {
  serviceKey: string
  namespaceId: string
}

const serviceKey = shallowRef<string | null>(null)
const namespaceId = ref('')
const rememberedForTab = ref(false)

function readSession(): StoredCredential | null {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<StoredCredential>
    if (typeof parsed.serviceKey !== 'string' || !KEY_PATTERN.test(parsed.serviceKey)) {
      sessionStorage.removeItem(SESSION_KEY)
      return null
    }
    return {
      serviceKey: parsed.serviceKey,
      namespaceId:
        typeof parsed.namespaceId === 'string' ? normalizeNamespaceId(parsed.namespaceId) : '',
    }
  } catch {
    clearStoredCredential()
    return null
  }
}

function clearStoredCredential(): void {
  try {
    sessionStorage.removeItem(SESSION_KEY)
  } catch {
    // Storage can be disabled by browser policy. Runtime state remains usable.
  }
}

const restored = readSession()
if (restored) {
  serviceKey.value = restored.serviceKey
  namespaceId.value = restored.namespaceId
  rememberedForTab.value = true
}

export const authState = {
  serviceKey: readonly(serviceKey),
  namespaceId: readonly(namespaceId),
  rememberedForTab: readonly(rememberedForTab),
  connected: computed(() => serviceKey.value !== null),
}

export function connectCredential(
  rawServiceKey: string,
  rawNamespaceId: string,
  remember: boolean,
): void {
  const candidate = rawServiceKey.trim()
  if (!KEY_PATTERN.test(candidate)) {
    throw new Error('Enter a canonical Kave V2 service key.')
  }
  const nextNamespaceId = normalizeNamespaceId(rawNamespaceId)
  serviceKey.value = candidate
  namespaceId.value = nextNamespaceId
  rememberedForTab.value = remember

  try {
    if (remember) {
      const stored: StoredCredential = { serviceKey: candidate, namespaceId: nextNamespaceId }
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(stored))
    } else {
      clearStoredCredential()
    }
  } catch {
    rememberedForTab.value = false
    clearStoredCredential()
  }
}

export function disconnectCredential(): void {
  serviceKey.value = null
  namespaceId.value = ''
  rememberedForTab.value = false
  clearStoredCredential()
}

export function currentCredential(): string {
  if (!serviceKey.value) throw new Error('A Kave V2 service key is required.')
  return serviceKey.value
}

export function setNamespaceId(value: string): void {
  namespaceId.value = normalizeNamespaceId(value)
  if (!rememberedForTab.value || !serviceKey.value) return
  try {
    sessionStorage.setItem(
      SESSION_KEY,
      JSON.stringify({
        serviceKey: serviceKey.value,
        namespaceId: namespaceId.value,
      } satisfies StoredCredential),
    )
  } catch {
    // Keep the runtime state even when tab storage is unavailable.
  }
}

function normalizeNamespaceId(value: string): string {
  const normalized = value.trim()
  if (/[\u0000-\u001f\u007f]/.test(normalized)) {
    throw new Error('Namespace ID contains control characters.')
  }
  if (normalized.length > 256) throw new Error('Namespace ID must be 256 characters or fewer.')
  return normalized
}

export const authSessionStorageKey = SESSION_KEY
