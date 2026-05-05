import { create } from '@bufbuild/protobuf'
import { EmptySchema } from '@bufbuild/protobuf/wkt'
import { createClient } from '@connectrpc/connect'
import { ControlPlaneService } from '@/gen/kave/control/v1/control_pb.js'
import {
  CreateAgentRequestSchema,
  CreatePolicyRequestSchema,
  CreateTokenRequestSchema,
  ListCredentialsRequestSchema,
  ListEnvironmentsRequestSchema,
  ListPoliciesRequestSchema,
  ListProjectsRequestSchema,
  ListTokensRequestSchema,
  GetAgentRequestSchema,
  GetPolicyRequestSchema,
  ListAgentsRequestSchema,
  UpdateAgentRequestSchema,
} from '@/gen/kave/control/v1/control_pb.js'
import { AgentStatus } from '@/gen/kave/control/v1/agent_pb.js'
import { DaemonService } from '@/gen/kave/control/v1/daemon_pb.js'
import { AuditService } from '@/gen/kave/audit/v1/audit_pb.js'
import { QueryAuditsRequestSchema } from '@/gen/kave/audit/v1/audit_pb.js'
import { RuntimeService } from '@/gen/kave/runtime/v1/runtime_pb.js'
import {
  GetPriceBookRequestSchema,
  GetDashboardOverviewRequestSchema,
  GetRunRequestSchema,
  GetSpendReportRequestSchema,
  GetTraceGraphRequestSchema,
  ListRunsRequestSchema,
  QuerySpansRequestSchema,
  StreamSpansRequestSchema,
  WatchEventsRequestSchema,
  WatchRunsRequestSchema,
} from '@/gen/kave/runtime/v1/runtime_pb.js'
import { RunStatus } from '@/gen/kave/runtime/v1/run_pb.js'
import type {
  Agent,
  Policy,
  Run,
  Span,
  SpendReport,
  CreateAgentRequest,
  CreatePolicyRequest,
  PriceBook,
  AgentToken,
  ConnectorCredential,
  DaemonStatus,
  AuditEntry,
  LiveEvent,
  DashboardOverview,
  TraceGraph,
} from '@/types/api'
import { transport } from './transport'

const control = createClient(ControlPlaneService, transport)
const runtime = createClient(RuntimeService, transport)
const daemon = createClient(DaemonService, transport)
const audit = createClient(AuditService, transport)

function nanosToString(amount: unknown): string | undefined {
  if (!amount || typeof amount !== 'object') return undefined
  const value = (amount as { decimal?: unknown; amount?: unknown }).decimal ?? (amount as { amount?: unknown }).amount
  return typeof value === 'string' ? value : undefined
}

function toMillis(v?: bigint): number | undefined {
  if (v == null) return undefined
  return Number(v)
}

function mapAgentStatus(status: AgentStatus): string {
  switch (status) {
    case AgentStatus.ACTIVE:
      return 'active'
    case AgentStatus.DISABLED:
      return 'disabled'
    default:
      return 'unknown'
  }
}

function mapRunStatus(status: RunStatus): string {
  switch (status) {
    case RunStatus.ACTIVE:
      return 'active'
    case RunStatus.COMPLETED:
      return 'completed'
    case RunStatus.FAILED:
      return 'failed'
    case RunStatus.CANCELLED:
      return 'cancelled'
    case RunStatus.TIMED_OUT:
      return 'timed_out'
    case RunStatus.BLOCKED:
      return 'blocked'
    default:
      return 'pending'
  }
}

function mapAgent(a: any): Agent {
  return {
    id: a.id,
    project_id: a.projectId,
    env_id: a.envId,
    name: a.name,
    description: a.description,
    policy_id: a.policyId,
    monthly_budget: nanosToString(a.monthlyBudget),
    status: mapAgentStatus(a.status),
    metadata: (a.metadata ?? {}) as Record<string, unknown>,
    created_at: Number(a.createdAtMs),
    updated_at: Number(a.updatedAtMs),
  }
}

function mapPolicy(p: any): Policy {
  return {
    id: p.id,
    project_id: p.projectId,
    env_id: p.envId,
    name: p.name,
    description: p.description,
    allowed_types: p.allowedTypes ?? [],
    allowed_connectors: p.allowedConnectors ?? [],
    allowed_methods: p.allowedMethods ?? [],
    budget_cap: nanosToString(p.budgetCap),
    budget_period: String(p.budgetPeriod ?? ''),
    budget_behavior: String(p.budgetBehavior ?? ''),
    trace_input: !!p.traceInput,
    trace_output: !!p.traceOutput,
    retention_days: p.retentionDays ?? 0,
    mode: String(p.mode ?? ''),
    status: String(p.status ?? ''),
    config: (p.config ?? {}) as Record<string, unknown>,
    created_at: Number(p.createdAtMs),
    updated_at: Number(p.updatedAtMs),
  }
}

function mapRun(r: any): Run {
  return {
    id: r.id,
    project_id: r.projectId,
    env_id: r.envId,
    agent_id: r.agentId,
    policy_id: r.policyId,
    name: r.name,
    status: mapRunStatus(r.status),
    budget_cap: nanosToString(r.budgetCap),
    spent: nanosToString(r.spent),
    metadata: (r.metadata ?? {}) as Record<string, unknown>,
    error_message: r.errorMessage,
    started_at: Number(r.startedAtMs),
    ended_at: toMillis(r.endedAtMs),
    created_at: Number(r.createdAtMs),
    updated_at: Number(r.updatedAtMs),
  }
}

function mapSpan(s: any): Span {
  return {
    id: s.id,
    run_id: s.runId,
    action_id: s.actionId,
    parent_id: s.parentId,
    name: s.name,
    started_at: Number(s.startedAtMs),
    ended_at: toMillis(s.endedAtMs),
    duration_ms: Number(s.durationMs ?? 0),
    error: s.error,
    input_tokens: s.inputTokens,
    output_tokens: s.outputTokens,
    cache_read_tokens: s.cacheReadTokens,
    cache_write_tokens: s.cacheWriteTokens,
    model: s.model,
    cost: nanosToString(s.cost),
    created_at: Number(s.createdAtMs),
  }
}

function mapToken(t: any): AgentToken {
  return {
    id: t.id,
    agent_id: t.agentId,
    name: t.name,
    token_prefix: t.tokenPrefix,
    status: t.revokedAtMs ? 'revoked' : 'active',
    created_at: Number(t.createdAtMs),
    last_used_at: toMillis(t.lastUsedAtMs),
    expires_at: toMillis(t.expiresAtMs),
  }
}

function mapCredential(c: any): ConnectorCredential {
  return {
    id: c.id,
    env_id: c.envId,
    connector_type: c.connectorType,
    label: c.label,
    status: String(c.status ?? ''),
    created_at: Number(c.createdAtMs),
    last_used_at: toMillis(c.lastUsedAtMs),
  }
}

function mapAuditEntry(e: any): AuditEntry {
  return {
    id: e.id,
    ts: Number(e.createdAtMs),
    actor: e.actorId || 'system',
    action: e.event,
    resource: e.resourceId,
    resource_type: e.resourceType,
    result: e.event.includes('blocked') || e.event.includes('denied') ? 'blocked' : 'ok',
  }
}

const textDecoder = new TextDecoder()

function runtimeEventToLiveEvent(ev: any): LiveEvent {
  let payload: any = {}
  try {
    payload = ev.data?.length ? JSON.parse(textDecoder.decode(ev.data)) : {}
  } catch {
    payload = {}
  }
  const run = payload
  return {
    id: `${ev.kind}:${ev.at}:${run.id ?? run.Id ?? ''}`,
    ts: Number(ev.at),
    kind: ev.kind,
    tone: ev.kind.includes('failed') || ev.kind.includes('denied') || ev.kind.includes('blocked') ? 'danger' : ev.kind.includes('completed') ? 'success' : 'info',
    agent: run.agentId || run.agent_id || 'unknown-agent',
    agentId: run.agentId || run.agent_id || '',
    runId: run.id || run.Id || run.runId || run.run_id || '',
    traceId: run.id || run.Id || run.runId || run.run_id || '',
    provider: run.connector || '',
    model: run.model || '',
    method: run.name || ev.kind,
    duration: run.endedAtMs && run.startedAtMs ? Number(run.endedAtMs) - Number(run.startedAtMs) : null,
    cost: Number(nanosToString(run.spent) ?? 0),
    inputTokens: run.inputTokens ?? 0,
    outputTokens: run.outputTokens ?? 0,
  }
}

export const agentsClient = {
  async list(envId: string): Promise<Agent[]> {
    const resp = await control.listAgents(create(ListAgentsRequestSchema, { envId, limit: 1000 }))
    return resp.agents.map(mapAgent)
  },
  async get(id: string): Promise<Agent> {
    const resp = await control.getAgent(create(GetAgentRequestSchema, { id }))
    return mapAgent(resp)
  },
  async create(body: CreateAgentRequest): Promise<Agent> {
    const resp = await control.createAgent(
      create(CreateAgentRequestSchema, {
        envId: body.env_id,
        name: body.name,
        description: body.description ?? '',
        policyId: body.policy_id,
      }),
    )
    return mapAgent(resp)
  },
  async update(id: string, body: Partial<Agent>): Promise<Agent> {
    const resp = await control.updateAgent(
      create(UpdateAgentRequestSchema, {
        id,
        update: {
          description: body.description,
          policyId: body.policy_id,
          status: undefined,
        },
      }),
    )
    return mapAgent(resp)
  },
}

export const policiesClient = {
  async list(envId: string): Promise<Policy[]> {
    const resp = await control.listPolicies(create(ListPoliciesRequestSchema, { envId, limit: 1000 }))
    return resp.policies.map(mapPolicy)
  },
  async get(id: string): Promise<Policy> {
    const resp = await control.getPolicy(create(GetPolicyRequestSchema, { id }))
    return mapPolicy(resp)
  },
  async create(body: CreatePolicyRequest): Promise<Policy> {
    const resp = await control.createPolicy(
      create(CreatePolicyRequestSchema, {
        envId: body.env_id,
        name: body.name,
        description: body.description ?? '',
      }),
    )
    return mapPolicy(resp)
  },
}

export const tokensClient = {
  async list(agentId: string): Promise<AgentToken[]> {
    const resp = await control.listTokens(create(ListTokensRequestSchema, { agentId, limit: 1000 }))
    return resp.tokens.map(mapToken)
  },
  async create(agentId: string, name: string): Promise<{ token: AgentToken; raw_token: string }> {
    const resp = await control.createToken(create(CreateTokenRequestSchema, { agentId, name }))
    return { token: mapToken(resp.token), raw_token: resp.rawToken }
  },
}

export const credentialsClient = {
  async list(envId: string): Promise<ConnectorCredential[]> {
    const resp = await control.listCredentials(
      create(ListCredentialsRequestSchema, { filter: { envId }, limit: 1000 }),
    )
    return resp.credentials.map(mapCredential)
  },
}

export const workspaceClient = {
  async listProjects() {
    const resp = await control.listProjects(create(ListProjectsRequestSchema, { orgId: 'default', limit: 100 }))
    return resp.projects
  },
  async listEnvironments(projectId: string) {
    const resp = await control.listEnvironments(create(ListEnvironmentsRequestSchema, { projectId, limit: 100 }))
    return resp.environments
  },
}

export const runsClient = {
  async list(params: {
    projectId?: string
    envId?: string
    agentId?: string
    status?: string
    limit?: number
  }): Promise<Run[]> {
    const resp = await runtime.listRuns(
      create(ListRunsRequestSchema, {
        filter: {
          projectId: params.projectId ?? '',
          envId: params.envId ?? '',
          agentId: params.agentId ?? '',
        },
        limit: params.limit ?? 100,
      }),
    )
    return resp.runs.map(mapRun)
  },
  async get(id: string): Promise<Run> {
    const resp = await runtime.getRun(create(GetRunRequestSchema, { id }))
    return mapRun(resp)
  },
  async watch(params: { envId: string; agentId?: string }, signal?: AbortSignal): Promise<AsyncIterable<Run>> {
    const stream = runtime.watchRuns(create(WatchRunsRequestSchema, { envId: params.envId, agentId: params.agentId }), { signal })
    async function* mapped() {
      for await (const run of stream) {
        yield mapRun(run)
      }
    }
    return mapped()
  },
}

function mapSpendReport(resp: any): SpendReport {
  return {
    total: nanosToString(resp.total) ?? '0',
    by_agent: Object.fromEntries(
      Object.entries(resp.byAgent ?? {}).map(([k, v]) => [k, nanosToString(v) ?? '0']),
    ),
    by_connector: Object.fromEntries(
      Object.entries(resp.byConnector ?? {}).map(([k, v]) => [k, nanosToString(v) ?? '0']),
    ),
    by_model: Object.fromEntries(
      Object.entries(resp.byModel ?? {}).map(([k, v]) => [k, nanosToString(v) ?? '0']),
    ),
    period_start: Number(resp.periodStartMs ?? 0),
    period_end: Number(resp.periodEndMs ?? 0),
  }
}

export const overviewClient = {
  async get(params: { projectId?: string; envId?: string; fromMs?: number; toMs?: number; recentLimit?: number }): Promise<DashboardOverview> {
    const resp = await runtime.getDashboardOverview(
      create(GetDashboardOverviewRequestSchema, {
        projectId: params.projectId ?? '',
        envId: params.envId ?? '',
        fromMs: params.fromMs == null ? undefined : BigInt(params.fromMs),
        toMs: params.toMs == null ? undefined : BigInt(params.toMs),
        recentLimit: params.recentLimit ?? 12,
      }),
    )
    return {
      total_runs: resp.totalRuns,
      active_runs: resp.activeRuns,
      blocked_runs: resp.blockedRuns,
      failed_runs: resp.failedRuns,
      avg_latency_ms: Number(resp.avgLatencyMs),
      total_input_tokens: Number(resp.totalInputTokens),
      total_output_tokens: Number(resp.totalOutputTokens),
      spend: mapSpendReport(resp.spend ?? {}),
      recent_runs: resp.recentRuns.map(mapRun),
      recent_attention_runs: resp.recentAttentionRuns.map(mapRun),
      top_agents: resp.topAgents.map(a => ({
        agent_id: a.agentId,
        agent_name: a.agentName,
        spend: nanosToString(a.spend) ?? '0',
        run_count: a.runCount,
      })),
    }
  },
}

export const spansClient = {
  async listByRun(runId: string, limit = 50): Promise<Span[]> {
    const resp = await runtime.querySpans(
      create(QuerySpansRequestSchema, { filter: { runId }, limit }),
    )
    return resp.spans.map(mapSpan)
  },
  async list(params: { runId?: string; hasError?: boolean; limit?: number }): Promise<Span[]> {
    const resp = await runtime.querySpans(
      create(QuerySpansRequestSchema, {
        filter: {
          runId: params.runId ?? '',
          hasError: params.hasError,
        },
        limit: params.limit ?? 100,
      }),
    )
    return resp.spans.map(mapSpan)
  },
  async stream(params: { projectId?: string; envId?: string; runId?: string }, signal?: AbortSignal): Promise<AsyncIterable<Span>> {
    const stream = runtime.streamSpans(
      create(StreamSpansRequestSchema, {
        projectId: params.projectId ?? '',
        envId: params.envId ?? '',
        runId: params.runId ?? '',
      }),
      { signal },
    )
    async function* mapped() {
      for await (const event of stream) {
        if (event.span) yield mapSpan(event.span)
      }
    }
    return mapped()
  },
}

export const traceClient = {
  async graph(params: { runId?: string; traceId?: string; limit?: number }): Promise<TraceGraph> {
    const resp = await runtime.getTraceGraph(
      create(GetTraceGraphRequestSchema, {
        runId: params.runId ?? '',
        traceId: params.traceId ?? '',
        limit: params.limit ?? 1000,
      }),
    )
    return {
      run: resp.run ? mapRun(resp.run) : undefined,
      spans: resp.spans.map(mapSpan),
      nodes: resp.nodes.map(n => ({
        span_id: n.spanId,
        parent_span_id: n.parentSpanId,
        name: n.name,
        connector: n.connector,
        model: n.model,
        has_error: n.hasError,
        depth: n.depth,
        offset_ms: Number(n.offsetMs),
        duration_ms: Number(n.durationMs),
        cost: nanosToString(n.cost) ?? '0',
        input_tokens: Number(n.inputTokens),
        output_tokens: Number(n.outputTokens),
      })),
      total_duration_ms: Number(resp.totalDurationMs),
      total_cost: nanosToString(resp.totalCost) ?? '0',
      total_input_tokens: Number(resp.totalInputTokens),
      total_output_tokens: Number(resp.totalOutputTokens),
    }
  },
}

export const eventsClient = {
  async watch(params: { projectId?: string; envId?: string; kind?: string }, signal?: AbortSignal): Promise<AsyncIterable<LiveEvent>> {
    const stream = runtime.watchEvents(
      create(WatchEventsRequestSchema, {
        projectId: params.projectId ?? '',
        envId: params.envId ?? '',
        kind: params.kind ?? '',
      }),
      { signal },
    )
    async function* mapped() {
      for await (const event of stream) {
        yield runtimeEventToLiveEvent(event)
      }
    }
    return mapped()
  },
}

export const costClient = {
  async summary(params?: {
    agentId?: string
    connector?: string
    model?: string
  }): Promise<SpendReport> {
    const resp = await runtime.getSpendReport(
      create(GetSpendReportRequestSchema, {
        filter: {
          agentId: params?.agentId ?? '',
          connector: params?.connector ?? '',
          model: params?.model ?? '',
        },
      }),
    )
    return mapSpendReport(resp)
  },
}

export const settingsClient = {
  async getPricing(): Promise<PriceBook> {
    const resp = await runtime.getPriceBook(create(GetPriceBookRequestSchema, {}))
    return {
      version: resp.version,
      entries: resp.entries.map((e: any) => ({
        provider: e.provider,
        match: e.match,
        source: e.source,
        currency: e.currency,
        input_per_million: nanosToString(e.inputPerMillion) ?? '0',
        output_per_million: nanosToString(e.outputPerMillion) ?? '0',
        cache_read_per_million: nanosToString(e.cacheReadPerMillion) ?? '0',
        cache_write_per_million: nanosToString(e.cacheWritePerMillion) ?? '0',
      })),
    }
  },
  async savePricing(body: PriceBook): Promise<PriceBook> {
    void body
    throw new Error('Editing the price book is not exposed by the v1 RPC contract yet.')
  },
}

export const daemonClient = {
  async status(): Promise<DaemonStatus> {
    const resp = await daemon.status(create(EmptySchema, {}))
    return {
      pid: resp.pid,
      version: resp.version,
      uptime: resp.uptime,
      started_at: Number(resp.startedAt),
      status: resp.status,
    }
  },
  async doctor() {
    const resp = await daemon.doctor(create(EmptySchema, {}))
    return resp.checks
  },
  async configPaths() {
    return daemon.configPaths(create(EmptySchema, {}))
  },
}

export const auditClient = {
  async list(params: { orgId?: string; projectId?: string; envId?: string; limit?: number } = {}): Promise<AuditEntry[]> {
    const resp = await audit.queryAudits(
      create(QueryAuditsRequestSchema, {
        filter: {
          orgId: params.orgId ?? 'default',
          projectId: params.projectId ?? '',
          envId: params.envId ?? '',
        },
        limit: params.limit ?? 100,
      }),
    )
    return resp.entries.map(mapAuditEntry)
  },
}
