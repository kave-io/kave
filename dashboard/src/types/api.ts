// API response types — mirror server/api/types.go (snake_case)

export interface Agent {
  id: string
  project_id: string
  env_id: string
  name: string
  description: string
  policy_id?: string
  monthly_budget?: string
  monthly_budget_display?: DisplayMoney
  status: string
  metadata: Record<string, unknown>
  created_at: number // UnixMilli
  updated_at: number // UnixMilli
}

export interface DisplayMoney {
  amount: string
  currency: string
  fx_rate?: string
  fx_source?: string
  fx_as_of_date?: string
  fx_fetched_at?: number
  rounded?: boolean
}

export interface Policy {
  id: string
  project_id: string
  env_id: string
  name: string
  description: string
  allowed_types: string[]
  allowed_connectors: string[]
  allowed_methods: string[]
  budget_cap?: string
  budget_cap_display?: DisplayMoney
  budget_period: string
  budget_behavior: string
  trace_input: boolean
  trace_output: boolean
  retention_days: number
  mode: string
  status: string
  config: Record<string, unknown>
  created_at: number
  updated_at: number
}

export interface Run {
  id: string
  project_id: string
  env_id: string
  agent_id: string
  policy_id?: string
  name: string
  status: string
  budget_cap?: string
  spent?: string
  budget_cap_display?: DisplayMoney
  spent_display?: DisplayMoney
  metadata: Record<string, unknown>
  error_message?: string
  started_at: number // UnixMilli
  ended_at?: number  // UnixMilli; absent if still running
  created_at: number
  updated_at: number
}

export interface Span {
  id: string
  run_id: string
  action_id: string
  parent_id?: string
  name: string
  started_at: number  // UnixMilli
  ended_at?: number   // UnixMilli
  duration_ms: number
  error?: string
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  model?: string
  cost?: string
  cost_display?: DisplayMoney
  created_at: number
}

export interface SpendReport {
  total: string
  total_display?: DisplayMoney
  by_agent: Record<string, string>
  by_connector: Record<string, string>
  by_model: Record<string, string>
  period_start: number // UnixMilli
  period_end: number   // UnixMilli
}

export interface PriceModel {
  provider: string
  match: string
  source: string
  currency: string
  input_per_million: string
  output_per_million: string
  cache_read_per_million: string
  cache_write_per_million: string
}

export interface PriceBook {
  version: string
  entries: PriceModel[]
}

export interface CreateAgentRequest {
  project_id: string
  env_id: string
  name: string
  description?: string
  policy_id?: string
  monthly_budget?: string
  metadata?: Record<string, unknown>
}

export interface CreatePolicyRequest {
  project_id: string
  env_id: string
  name: string
  description?: string
  allowed_types?: string[]
  allowed_connectors?: string[]
  allowed_methods?: string[]
  config?: Record<string, unknown>
}

export interface AgentToken {
  id: string
  agent_id: string
  name: string
  token_prefix: string
  status: string
  created_at: number
  last_used_at?: number
  expires_at?: number
}

export interface ConnectorCredential {
  id: string
  env_id: string
  connector_type: string
  label: string
  status: string
  created_at: number
  last_used_at?: number
}

export interface DaemonStatus {
  pid: string
  version: string
  uptime: string
  started_at: number
  status: Record<string, string>
}

export interface AuditEntry {
  id: string
  ts: number
  actor: string
  action: string
  resource: string
  resource_type: string
  result: 'ok' | 'blocked'
}

export type EventTone = 'success' | 'info' | 'danger' | 'warning' | 'neutral'

export interface LiveEvent {
  id: string
  ts: number
  kind: string
  tone: EventTone
  agent: string
  agentId: string
  runId: string
  traceId: string
  provider: string
  model: string
  method: string
  duration: number | null
  cost: number | null
  inputTokens: number
  outputTokens: number
}

export interface DashboardAgentSpend {
  agent_id: string
  agent_name: string
  spend: string
  run_count: number
}

export interface DashboardOverview {
  total_runs: number
  active_runs: number
  blocked_runs: number
  failed_runs: number
  avg_latency_ms: number
  total_input_tokens: number
  total_output_tokens: number
  spend: SpendReport
  recent_runs: Run[]
  recent_attention_runs: Run[]
  top_agents: DashboardAgentSpend[]
}

export interface TraceGraphNode {
  span_id: string
  parent_span_id: string
  name: string
  connector: string
  model: string
  has_error: boolean
  depth: number
  offset_ms: number
  duration_ms: number
  cost: string
  input_tokens: number
  output_tokens: number
}

export interface TraceGraph {
  run?: Run
  spans: Span[]
  nodes: TraceGraphNode[]
  total_duration_ms: number
  total_cost: string
  total_input_tokens: number
  total_output_tokens: number
}
