import { create } from '@bufbuild/protobuf'
import { createClient } from '@connectrpc/connect'
import { ControlPlaneService } from '@/gen/kave/control/v1/control_pb.js'
import {
  CreateAgentRequestSchema,
  CreatePolicyRequestSchema,
  GetAgentRequestSchema,
  GetPolicyRequestSchema,
  ListAgentsRequestSchema,
  UpdateAgentRequestSchema,
} from '@/gen/kave/control/v1/control_pb.js'
import { AgentStatus } from '@/gen/kave/control/v1/agent_pb.js'
import { RuntimeService } from '@/gen/kave/runtime/v1/runtime_pb.js'
import {
  GetPriceBookRequestSchema,
  GetRunRequestSchema,
  GetSpendReportRequestSchema,
  ListRunsRequestSchema,
  QuerySpansRequestSchema,
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
} from '@/types/api'
import { transport } from './transport'

const control = createClient(ControlPlaneService, transport)
const runtime = createClient(RuntimeService, transport)

function nanosToString(amount: unknown): string | undefined {
  if (!amount || typeof amount !== 'object') return undefined
  const value = (amount as { amount?: unknown }).amount
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
    return {
      total: nanosToString(resp.total) ?? '0',
      by_agent: Object.fromEntries(
        Object.entries(resp.byAgent).map(([k, v]) => [k, nanosToString(v) ?? '0']),
      ),
      by_connector: Object.fromEntries(
        Object.entries(resp.byConnector).map(([k, v]) => [k, nanosToString(v) ?? '0']),
      ),
      by_model: Object.fromEntries(
        Object.entries(resp.byModel).map(([k, v]) => [k, nanosToString(v) ?? '0']),
      ),
      period_start: Number(resp.periodStartMs),
      period_end: Number(resp.periodEndMs),
    }
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
    const res = await fetch('/api/v1/settings/pricing', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      throw new Error(`Failed to save pricing (${res.status})`)
    }
    return (await res.json()) as PriceBook
  },
}
