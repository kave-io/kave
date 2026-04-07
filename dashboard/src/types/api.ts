// API response types — mirror server/api/types.go (snake_case)

export interface Agent {
  id: string
  workspace_id: string
  name: string
  description: string
  policy_id?: string
  monthly_budget?: number
  metadata: Record<string, unknown>
  created_at: number // UnixMilli
  updated_at: number // UnixMilli
}

export interface Policy {
  id: string
  workspace_id: string
  name: string
  description: string
  allowed_connectors: string[]
  allowed_methods: string[]
  budget_cap_usd: number
  config: Record<string, unknown>
  created_at: number
  updated_at: number
}

export interface Run {
  id: string
  workspace_id: string
  agent_id: string
  policy_id?: string
  name: string
  status: 'active' | 'completed' | 'failed'
  budget_cap_usd: number
  spent_usd: number
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
  cost_usd?: number
  created_at: number
}

export interface SpendReport {
  total_usd: number
  by_agent: Record<string, number>
  by_connector: Record<string, number>
  by_model: Record<string, number>
  period_start: number // UnixMilli
  period_end: number   // UnixMilli
}

export interface CreateAgentRequest {
  workspace_id: string
  name: string
  description?: string
  policy_id?: string
  monthly_budget?: number
  metadata?: Record<string, unknown>
}

export interface CreatePolicyRequest {
  workspace_id: string
  name: string
  description?: string
  allowed_connectors?: string[]
  allowed_methods?: string[]
  budget_cap_usd?: number
  config?: Record<string, unknown>
}
