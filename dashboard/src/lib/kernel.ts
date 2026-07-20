import {
  Code,
  ConnectError,
  createClient,
  type Client,
  type Interceptor,
} from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import {
  AgentKind,
  DecisionStatus,
  KernelService,
  LimitWindow,
} from '@/gen/kave/kernel/v2/kernel_pb'
import { currentCredential } from './auth'
import type {
  AuditEvent,
  Invocation,
  NamespaceState,
  PageResult,
  Scope,
  TenantPage,
  TimeRange,
  UsageEntry,
} from './types'

const DEFAULT_TIMEOUT_MS = 15_000

export class KernelError extends Error {
  readonly code: string

  constructor(message: string, code = 'unknown') {
    super(message)
    this.name = 'KernelError'
    this.code = code
  }

  get unauthenticated(): boolean {
    return this.code === 'unauthenticated'
  }

  get permissionDenied(): boolean {
    return this.code === 'permission_denied'
  }

  get unimplemented(): boolean {
    return this.code === 'unimplemented'
  }
}

export interface KernelClientOptions {
  baseUrl?: string
  fetch?: typeof globalThis.fetch
  credential?: () => string
  timeoutMs?: number
  origin?: string
}

export class KernelClient {
  readonly #client: Client<typeof KernelService>
  readonly #credential: () => string

  constructor(options: KernelClientOptions = {}) {
    const origin = options.origin ?? window.location.origin
    const rawBaseUrl = options.baseUrl ?? (options.origin ? origin : runtimeBaseUrl())
    const baseUrl = safeBaseUrl(rawBaseUrl, origin)
    const fetcher = options.fetch ?? globalThis.fetch.bind(globalThis)
    this.#credential = options.credential ?? currentCredential

    const authorize: Interceptor = (next) => async (request) => {
      const credential = this.#credential()
      request.header.set('Authorization', `Bearer ${credential}`)
      request.header.set('Cache-Control', 'no-store')
      return await next(request)
    }
    const transport = createConnectTransport({
      baseUrl,
      defaultTimeoutMs: options.timeoutMs ?? DEFAULT_TIMEOUT_MS,
      fetch: (input, init) =>
        fetcher(input, {
          ...init,
          cache: 'no-store',
          credentials: 'omit',
          mode: 'same-origin',
          redirect: 'error',
          referrerPolicy: 'no-referrer',
        }),
      interceptors: [authorize],
      useBinaryFormat: false,
      useHttpGet: false,
    })
    this.#client = createClient(KernelService, transport)
  }

  async getState(namespaceId: string): Promise<NamespaceState> {
    const state = await this.invoke(() => this.#client.getState({ namespaceId }))
    return {
      namespaceId: state.namespaceId,
      revision: state.revision,
      manifest: state.manifest
        ? {
            namespace: state.manifest.namespace
              ? {
                  account: state.manifest.namespace.account,
                  application: state.manifest.namespace.application,
                  environment: state.manifest.namespace.environment,
                }
              : undefined,
            routes: state.manifest.routes.map((route) => ({
              name: route.name,
              provider: route.provider,
              allowedModels: [...route.allowedModels],
              defaultModel: route.defaultModel,
              pricingRevision: route.pricingRevision,
            })),
            agents: state.manifest.agents.map((agent) => ({
              name: agent.name,
              kind: agentKindName(agent.kind),
              route: agent.route,
              enabled: agent.enabled,
            })),
            limits: state.manifest.limits.map((limit) => ({
              key: limit.key,
              metric: limit.metric,
              selector: limit.selector
                ? {
                    tenant: limit.selector.tenant || undefined,
                    actor: limit.selector.actor || undefined,
                    billTo: limit.selector.billTo || undefined,
                    agent: limit.selector.agent || undefined,
                    model: limit.selector.model || undefined,
                    feature: limit.selector.feature || undefined,
                  }
                : undefined,
              window: limitWindowName(limit.window),
              hardCap: limit.hardCap,
              softCap: limit.softCap,
              enabled: limit.enabled,
            })),
          }
        : undefined,
    }
  }

  async queryUsage(
    scope: Scope,
    range: TimeRange,
    filters: { agent?: string; metric?: string; pageSize?: number; pageToken?: string } = {},
  ): Promise<PageResult<UsageEntry>> {
    const response = await this.invoke(() =>
      this.#client.queryUsage({
        scope: toWireScope(scope),
        agent: filters.agent ?? '',
        metric: filters.metric ?? '',
        fromMs: BigInt(range.fromMs),
        toMs: BigInt(range.toMs),
        pageSize: filters.pageSize ?? 200,
        pageToken: filters.pageToken ?? '',
      }),
    )
    return {
      items: response.entries.map((entry) => ({
        id: entry.id,
        invocationId: entry.invocationId,
        metric: entry.metric,
        units: entry.units,
        costNanoUsd: entry.costNanoUsd,
        provider: entry.provider,
        model: entry.model,
        attempt: entry.attempt,
        eventKind: entry.eventKind,
        createdAtMs: entry.createdAtMs,
        requestCount: entry.requestCount,
        inputTokens: entry.inputTokens,
        outputTokens: entry.outputTokens,
        cacheReadTokens: entry.cacheReadTokens,
        cacheWriteTokens: entry.cacheWriteTokens,
        reasoningTokens: entry.reasoningTokens,
        estimated: entry.estimated,
      })),
      nextPageToken: response.nextPageToken,
    }
  }

  async queryInvocations(
    scope: Scope,
    range: TimeRange,
    filters: { agent?: string; decision?: string; pageSize?: number; pageToken?: string } = {},
  ): Promise<PageResult<Invocation>> {
    const response = await this.invoke(() =>
      this.#client.queryInvocations({
        scope: toWireScope(scope),
        agent: filters.agent ?? '',
        status: toDecisionStatus(filters.decision),
        fromMs: BigInt(range.fromMs),
        toMs: BigInt(range.toMs),
        pageSize: filters.pageSize ?? 200,
        pageToken: filters.pageToken ?? '',
      }),
    )
    return {
      items: response.invocations.map((invocation) => ({
        id: invocation.id,
        agent: invocation.agent,
        model: invocation.model,
        scope: invocation.scope
          ? {
              tenant: invocation.scope.tenant,
              actor: invocation.scope.actor || undefined,
              billTo: invocation.scope.billTo,
              session: invocation.scope.session || undefined,
              feature: invocation.scope.feature || undefined,
            }
          : undefined,
        decision: decisionStatusName(invocation.decision),
        status: invocation.status,
        idempotencyKey: invocation.idempotencyKey || undefined,
        createdAtMs: invocation.createdAtMs,
        settledAtMs: invocation.settledAtMs || undefined,
      })),
      nextPageToken: response.nextPageToken,
    }
  }

  async queryAuditEvents(
    range: TimeRange,
    filters: { eventKind?: string; pageSize?: number; pageToken?: string } = {},
  ): Promise<PageResult<AuditEvent>> {
    const response = await this.invoke(() =>
      this.#client.queryAuditEvents({
        eventKind: filters.eventKind ?? '',
        fromMs: BigInt(range.fromMs),
        toMs: BigInt(range.toMs),
        pageSize: filters.pageSize ?? 200,
        pageToken: filters.pageToken ?? '',
      }),
    )
    return {
      items: response.events.map((event) => ({
        id: event.id,
        eventKind: event.eventKind,
        actorKind: event.actorKind,
        actorId: event.actorId,
        resourceKind: event.resourceKind,
        resourceId: event.resourceId,
        outcome: event.outcome,
        metadata: { ...event.metadata },
        createdAtMs: event.createdAtMs,
      })),
      nextPageToken: response.nextPageToken,
    }
  }

  async listTenants(
    range: TimeRange,
    filters: { pageSize?: number; pageToken?: string } = {},
  ): Promise<TenantPage> {
    const response = await this.invoke(() =>
      this.#client.listTenants({
        fromMs: BigInt(range.fromMs),
        toMs: BigInt(range.toMs),
        pageSize: filters.pageSize ?? 200,
        pageToken: filters.pageToken ?? '',
      }),
    )
    return {
      tenants: response.tenants.map((tenant) => ({
        tenant: tenant.tenant,
        billTo: tenant.billTo || undefined,
        status: tenant.status,
        lastSeenAtMs: tenant.lastSeenAtMs || undefined,
        invocationCount: tenant.invocationCount,
        requestCount: tenant.requestCount,
        costNanoUsd: tenant.costNanoUsd,
        activeLimits: tenant.activeLimits,
      })),
      nextPageToken: response.nextPageToken,
    }
  }

  private async invoke<T>(call: () => Promise<T>): Promise<T> {
    try {
      return await call()
    } catch (error) {
      throw normalizeError(error, credentialForRedaction(this.#credential))
    }
  }
}

function runtimeBaseUrl(): string {
  const value = (import.meta.env.VITE_KERNEL_BASE_URL as string | undefined)?.trim()
  return value || window.location.origin
}

export function safeBaseUrl(raw: string, origin = window.location.origin): string {
  const expected = new URL(origin)
  const resolved = new URL(raw, expected.origin)
  if (resolved.origin !== expected.origin || !['http:', 'https:'].includes(resolved.protocol)) {
    throw new Error('Kave Console only sends service keys to its own origin.')
  }
  if (resolved.username || resolved.password || resolved.search || resolved.hash) {
    throw new Error('Kave Console base URL cannot contain credentials, a query, or a fragment.')
  }
  return resolved.href.replace(/\/$/, '')
}

function toWireScope(scope: Scope) {
  return {
    tenant: scope.tenant,
    actor: scope.actor ?? '',
    billTo: scope.billTo,
    session: scope.session ?? '',
    feature: scope.feature ?? '',
  }
}

function toDecisionStatus(value: string | undefined): DecisionStatus {
  switch (value?.toLowerCase()) {
    case 'admitted':
    case 'decision_status_admitted':
      return DecisionStatus.ADMITTED
    case 'rejected':
    case 'decision_status_rejected':
      return DecisionStatus.REJECTED
    default:
      return DecisionStatus.UNSPECIFIED
  }
}

function decisionStatusName(value: DecisionStatus): Invocation['decision'] {
  switch (value) {
    case DecisionStatus.ADMITTED:
      return 'DECISION_STATUS_ADMITTED'
    case DecisionStatus.REJECTED:
      return 'DECISION_STATUS_REJECTED'
    default:
      return 'DECISION_STATUS_UNSPECIFIED'
  }
}

function agentKindName(
  value: AgentKind,
): 'AGENT_KIND_LLM' | 'AGENT_KIND_EMBEDDING' | 'AGENT_KIND_UNSPECIFIED' {
  switch (value) {
    case AgentKind.LLM:
      return 'AGENT_KIND_LLM'
    case AgentKind.EMBEDDING:
      return 'AGENT_KIND_EMBEDDING'
    default:
      return 'AGENT_KIND_UNSPECIFIED'
  }
}

function limitWindowName(
  value: LimitWindow,
):
  | 'LIMIT_WINDOW_ALL_TIME'
  | 'LIMIT_WINDOW_DAY'
  | 'LIMIT_WINDOW_MONTH'
  | 'LIMIT_WINDOW_UNSPECIFIED' {
  switch (value) {
    case LimitWindow.ALL_TIME:
      return 'LIMIT_WINDOW_ALL_TIME'
    case LimitWindow.DAY:
      return 'LIMIT_WINDOW_DAY'
    case LimitWindow.MONTH:
      return 'LIMIT_WINDOW_MONTH'
    default:
      return 'LIMIT_WINDOW_UNSPECIFIED'
  }
}

function credentialForRedaction(read: () => string): string {
  try {
    return read()
  } catch {
    return ''
  }
}

function normalizeError(error: unknown, credential: string): KernelError {
  const connected = ConnectError.from(error)
  const code = codeName(connected.code)
  const publicMessage = safeErrorMessage(connected, code)
  return new KernelError(redactCredential(publicMessage, credential), code)
}

function safeErrorMessage(error: ConnectError, code: string): string {
  switch (code) {
    case 'unauthenticated':
      return 'The service key was not accepted.'
    case 'permission_denied':
      return 'This service key does not permit that operation.'
    case 'deadline_exceeded':
    case 'canceled':
      return 'The Kave request timed out or was canceled.'
    case 'unavailable':
    case 'unknown':
    case 'internal':
    case 'data_loss':
      return 'The Kave kernel could not complete the request.'
    default:
      return error.rawMessage || 'The Kave request failed.'
  }
}

function codeName(code: Code): string {
  return (Code[code] || 'Unknown').replace(/([a-z0-9])([A-Z])/g, '$1_$2').toLowerCase()
}

function redactCredential(message: string, credential: string): string {
  return credential && message.includes(credential)
    ? message.split(credential).join('[redacted]')
    : message
}

export const kernel = new KernelClient()
