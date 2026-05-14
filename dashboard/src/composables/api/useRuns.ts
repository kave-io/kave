import { useQuery } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { runsClient } from '@/lib/rpc/clients'

function unrefString(v?: string | Ref<string>) {
  return typeof v === 'string' ? v : v?.value
}

export function useRuns(params: {
  projectId?: string | Ref<string>
  envId?: string | Ref<string>
  agentId?: string | Ref<string>
  status?: string
  limit?: number
}) {
  return useQuery({
    queryKey: ['runs', params.projectId, params.envId, params.agentId, params.status, params.limit ?? 100],
    queryFn: () =>
      runsClient.list({
        projectId: unrefString(params.projectId),
        envId: unrefString(params.envId),
        agentId: unrefString(params.agentId),
        status: params.status,
        limit: params.limit,
      }),
    refetchInterval: 5000,
  })
}

export function useRun(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['run', id],
    queryFn: () => runsClient.get(typeof id === 'string' ? id : id.value),
  })
}
