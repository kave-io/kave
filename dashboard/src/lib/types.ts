export type Int64 = string | number | bigint

export interface Scope {
  tenant: string
  actor?: string
  billTo: string
  session?: string
  feature?: string
}

export interface NamespaceSpec {
  account: string
  application: string
  environment: string
}

export interface RouteSpec {
  name: string
  provider: string
  allowedModels: string[]
  defaultModel: string
  pricingRevision: Int64
}

export interface AgentSpec {
  name: string
  kind: 'AGENT_KIND_LLM' | 'AGENT_KIND_EMBEDDING' | 'AGENT_KIND_UNSPECIFIED'
  route: string
  enabled: boolean
}

export interface LimitSelector {
  tenant?: string
  actor?: string
  billTo?: string
  agent?: string
  model?: string
  feature?: string
}

export interface LimitSpec {
  key: string
  metric: string
  selector?: LimitSelector
  window:
    | 'LIMIT_WINDOW_ALL_TIME'
    | 'LIMIT_WINDOW_DAY'
    | 'LIMIT_WINDOW_MONTH'
    | 'LIMIT_WINDOW_UNSPECIFIED'
  hardCap: Int64
  softCap?: Int64
  enabled: boolean
}

export interface Manifest {
  namespace?: NamespaceSpec
  routes: RouteSpec[]
  agents: AgentSpec[]
  limits: LimitSpec[]
}

export interface NamespaceState {
  namespaceId: string
  revision: Int64
  manifest?: Manifest
}

export interface UsageEntry {
  id: string
  invocationId: string
  metric: string
  units: Int64
  costNanoUsd: Int64
  provider: string
  model: string
  attempt: number
  eventKind: string
  createdAtMs: Int64
  requestCount: Int64
  inputTokens: Int64
  outputTokens: Int64
  cacheReadTokens: Int64
  cacheWriteTokens: Int64
  reasoningTokens: Int64
  estimated: boolean
}

export interface Invocation {
  id: string
  agent: string
  model: string
  scope?: Scope
  decision: 'DECISION_STATUS_ADMITTED' | 'DECISION_STATUS_REJECTED' | 'DECISION_STATUS_UNSPECIFIED'
  status: string
  idempotencyKey?: string
  createdAtMs: Int64
  settledAtMs?: Int64
}

export interface AuditEvent {
  id: string
  eventKind: string
  actorKind: string
  actorId: string
  resourceKind: string
  resourceId: string
  outcome: string
  metadata?: Record<string, string>
  createdAtMs: Int64
}

export interface PageResult<T> {
  items: T[]
  nextPageToken: string
}

// Tenant identifiers are opaque application assertions. The console never
// treats them as people, emails, or other identity records.
export interface TenantSummary {
  tenant: string
  billTo?: string
  status: string
  lastSeenAtMs?: Int64
  invocationCount: Int64
  requestCount?: Int64
  costNanoUsd?: Int64
  activeLimits?: number
}

export interface TenantPage {
  tenants: TenantSummary[]
  nextPageToken: string
}

export interface TimeRange {
  fromMs: number
  toMs: number
}
