import { describe, expect, it, vi } from 'vitest'
import { KernelClient, KernelError, safeBaseUrl } from '@/lib/kernel'

const credential = `kv2_${'A'.repeat(24)}.${'B'.repeat(42)}A`
const origin = 'https://console.example'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('generated kernel client', () => {
  it('allows only same-origin, credential-free base URLs', () => {
    expect(safeBaseUrl('/kernel/', origin)).toBe('https://console.example/kernel')
    expect(() => safeBaseUrl('https://api.example', origin)).toThrow('own origin')
    expect(() => safeBaseUrl('https://console.example?redirect=evil', origin)).toThrow('query')
    expect(() => safeBaseUrl('https://user:pass@console.example', origin)).toThrow('credentials')
  })

  it('uses the generated request shape and hardened fetch options', async () => {
    const fetcher = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        entries: [
          {
            id: 'usage-1',
            invocationId: 'inv-1',
            metric: 'requests',
            units: '1',
            costNanoUsd: '250',
            provider: 'openai',
            model: 'gpt-test',
            attempt: 1,
            eventKind: 'provider.settled',
            createdAtMs: '1500',
            requestCount: '1',
            inputTokens: '10',
            outputTokens: '5',
          },
        ],
        nextPageToken: 'next-page',
      }),
    )
    const client = new KernelClient({
      baseUrl: '/kernel',
      origin,
      fetch: fetcher,
      credential: () => credential,
    })

    const result = await client.queryUsage(
      { tenant: 'tenant-a', billTo: 'billing-a', actor: 'actor-a' },
      { fromMs: 1_000, toMs: 2_000 },
      { metric: 'requests', pageSize: 25 },
    )

    expect(result.items[0]).toMatchObject({ id: 'usage-1', costNanoUsd: 250n, inputTokens: 10n })
    expect(result.nextPageToken).toBe('next-page')
    const [requestUrl, requestInit] = fetcher.mock.calls[0] ?? []
    expect(String(requestUrl)).toBe(
      'https://console.example/kernel/kave.kernel.v2.KernelService/QueryUsage',
    )
    const headers = new Headers(requestInit?.headers)
    expect(headers.get('authorization')).toBe(`Bearer ${credential}`)
    expect(headers.get('cache-control')).toBe('no-store')
    expect(requestInit).toMatchObject({
      cache: 'no-store',
      credentials: 'omit',
      mode: 'same-origin',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
    })
    const rawBody = requestInit?.body
    const requestBody =
      typeof rawBody === 'string'
        ? rawBody
        : ArrayBuffer.isView(rawBody)
          ? new TextDecoder().decode(rawBody)
          : rawBody instanceof ArrayBuffer
            ? new TextDecoder().decode(rawBody)
            : requestUrl instanceof Request
              ? await requestUrl.clone().text()
              : ''
    expect(JSON.parse(requestBody)).toMatchObject({
      scope: { tenant: 'tenant-a', billTo: 'billing-a', actor: 'actor-a' },
      metric: 'requests',
      fromMs: '1000',
      toMs: '2000',
      pageSize: 25,
    })
  })

  it('exposes no control-plane or consumption mutation methods', () => {
    const client = new KernelClient({ origin, fetch: vi.fn(), credential: () => credential })
    const publicSurface = client as unknown as Record<string, unknown>

    for (const method of [
      'apply',
      'issueServiceKey',
      'revokeServiceKey',
      'putSecret',
      'revokeSecret',
      'activateProviderRoute',
      'syncLimits',
      'consume',
    ]) {
      expect(publicSurface[method]).toBeUndefined()
    }
    expect(publicSurface.client).toBeUndefined()
    expect(publicSurface.credential).toBeUndefined()
  })

  it('projects only safe manifest metadata for the namespace view', async () => {
    const fetcher = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        namespaceId: 'nsp_prod',
        revision: '7',
        manifest: {
          namespace: { account: 'acme', application: 'chat', environment: 'prod' },
          routes: [
            {
              name: 'primary',
              provider: 'openai',
              baseUrl: 'https://provider.invalid',
              secret: 'provider-key-reference',
              allowedModels: ['gpt-test'],
              defaultModel: 'gpt-test',
              pricingRevision: '3',
            },
          ],
          agents: [{ name: 'support', kind: 'AGENT_KIND_LLM', route: 'primary', enabled: true }],
          limits: [
            {
              key: 'tenant-requests',
              metric: 'requests',
              selector: { tenant: 'tenant-a' },
              window: 'LIMIT_WINDOW_DAY',
              hardCap: '100',
              enabled: true,
            },
          ],
        },
      }),
    )
    const client = new KernelClient({ origin, fetch: fetcher, credential: () => credential })

    const state = await client.getState('nsp_prod')

    expect(state.revision).toBe(7n)
    expect(state.manifest?.agents[0]?.kind).toBe('AGENT_KIND_LLM')
    expect(state.manifest?.limits[0]?.window).toBe('LIMIT_WINDOW_DAY')
    expect(state.manifest?.routes[0]).toEqual({
      name: 'primary',
      provider: 'openai',
      allowedModels: ['gpt-test'],
      defaultModel: 'gpt-test',
      pricingRevision: 3n,
    })
  })

  it('maps tenant aggregates from the generated ListTenants contract', async () => {
    const fetcher = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        tenants: [
          {
            tenant: 'tenant-a',
            billTo: 'billing-a',
            status: 'active',
            lastSeenAtMs: '1500',
            invocationCount: '9',
            requestCount: '12',
            costNanoUsd: '340',
            activeLimits: 2,
          },
        ],
      }),
    )
    const client = new KernelClient({ origin, fetch: fetcher, credential: () => credential })

    await expect(client.listTenants({ fromMs: 1_000, toMs: 2_000 })).resolves.toEqual({
      tenants: [
        {
          tenant: 'tenant-a',
          billTo: 'billing-a',
          status: 'active',
          lastSeenAtMs: 1500n,
          invocationCount: 9n,
          requestCount: 12n,
          costNanoUsd: 340n,
          activeLimits: 2,
        },
      ],
      nextPageToken: '',
    })
  })

  it('returns safe categorized errors without reflecting a credential', async () => {
    const fetcher = vi.fn<typeof fetch>(async () =>
      jsonResponse({ code: 'unauthenticated', message: `bad credential ${credential}` }, 401),
    )
    const client = new KernelClient({ origin, fetch: fetcher, credential: () => credential })

    const error = await client.listTenants({ fromMs: 1_000, toMs: 2_000 }).catch((caught) => caught)

    expect(error).toBeInstanceOf(KernelError)
    expect(error).toMatchObject({ code: 'unauthenticated' })
    expect((error as Error).message).toBe('The service key was not accepted.')
    expect((error as Error).message).not.toContain(credential)
  })
})
