import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { api } from './fetch'
import type { Ref } from 'vue'
import type { Agent, Policy, Run, Span, SpendReport, CreateAgentRequest, CreatePolicyRequest, PriceBook } from '@/types/api'

// ── Agents ────────────────────────────────────────────────────────────────────

export function useAgents(envId: string | Ref<string>) {
  return useQuery({
    queryKey: ['agents', envId],
    queryFn: () => {
      const id = typeof envId === 'string' ? envId : envId.value
      return api.get<Agent[]>(`/agents?env_id=${id}`)
    },
  })
}

export function useAgent(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['agent', id],
    queryFn: () => {
      const agentId = typeof id === 'string' ? id : id.value
      return api.get<Agent>(`/agents/${agentId}`)
    },
  })
}

export function useCreateAgent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateAgentRequest) => api.post<Agent>('/agents', data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useUpdateAgent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: Partial<Agent> & { id: string }) =>
      api.patch<Agent>(`/agents/${id}`, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}

// ── Policies ──────────────────────────────────────────────────────────────────

export function usePolicy(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['policy', id],
    queryFn: () => {
      const policyId = typeof id === 'string' ? id : id.value
      return api.get<Policy>(`/policies/${policyId}`)
    },
    enabled: !!(typeof id === 'string' ? id : id.value),
  })
}

export function useCreatePolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreatePolicyRequest) => api.post<Policy>('/policies', data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['policies'] }),
  })
}

// ── Runs ──────────────────────────────────────────────────────────────────────

export function useRuns(params: {
  projectId?: string | Ref<string>
  envId?: string | Ref<string>
  agentId?: string | Ref<string>
  status?: string
  limit?: number
}) {
  return useQuery({
    queryKey: ['runs', params],
    queryFn: () => {
      const q = new URLSearchParams()
      const projId = typeof params.projectId === 'string' ? params.projectId : params.projectId?.value
      const envIdVal = typeof params.envId === 'string' ? params.envId : params.envId?.value
      const agId = typeof params.agentId === 'string' ? params.agentId : params.agentId?.value
      if (projId) q.set('project_id', projId)
      if (envIdVal) q.set('env_id', envIdVal)
      if (agId) q.set('agent_id', agId)
      if (params.status) q.set('status', params.status)
      if (params.limit) q.set('limit', String(params.limit))
      return api.get<Run[]>(`/runs?${q}`)
    },
  })
}

export function useRun(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['run', id],
    queryFn: () => {
      const runId = typeof id === 'string' ? id : id.value
      return api.get<Run>(`/runs/${runId}`)
    },
  })
}

// ── Spans ─────────────────────────────────────────────────────────────────────

export function useRunSpans(runId: string | Ref<string>, limit = 50) {
  return useQuery({
    queryKey: ['spans', 'run', runId, limit],
    queryFn: () => {
      const id = typeof runId === 'string' ? runId : runId.value
      return api.get<Span[]>(`/runs/${id}/spans?limit=${limit}`)
    },
    enabled: !!(typeof runId === 'string' ? runId : runId.value),
  })
}

export function useSpans(params: { runId?: string; hasError?: boolean; limit?: number }) {
  return useQuery({
    queryKey: ['spans', params],
    queryFn: () => {
      const q = new URLSearchParams()
      if (params.runId) q.set('run_id', params.runId)
      if (params.hasError !== undefined) q.set('has_error', String(params.hasError))
      if (params.limit) q.set('limit', String(params.limit))
      return api.get<Span[]>(`/spans?${q}`)
    },
  })
}

// ── Cost ──────────────────────────────────────────────────────────────────────

export function useCostSummary(params?: { agentId?: string | Ref<string>; connector?: string; model?: string }) {
  return useQuery({
    queryKey: ['cost', 'summary', params],
    queryFn: () => {
      const q = new URLSearchParams()
      const agId = typeof params?.agentId === 'string' ? params.agentId : params?.agentId?.value
      if (agId) q.set('agent_id', agId)
      if (params?.connector) q.set('connector', params.connector)
      if (params?.model) q.set('model', params.model)
      return api.get<SpendReport>(`/cost/summary?${q}`)
    },
  })
}

export function usePricingBook() {
  return useQuery({
    queryKey: ['settings', 'pricing'],
    queryFn: () => api.get<PriceBook>('/settings/pricing'),
  })
}

export function useSavePricingBook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: PriceBook) => api.put<PriceBook, PriceBook>('/settings/pricing', data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['settings', 'pricing'] }),
  })
}
