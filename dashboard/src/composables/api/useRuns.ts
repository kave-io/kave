import { useQuery } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { runsClient } from '@/lib/rpc/clients'

export function useRuns(params: {
  projectId?: string | Ref<string>
  envId?: string | Ref<string>
  agentId?: string | Ref<string>
  status?: string
  limit?: number
}) {
  return useQuery({
    queryKey: ['runs', params],
    queryFn: () =>
      runsClient.list({
        projectId: typeof params.projectId === 'string' ? params.projectId : params.projectId?.value,
        envId: typeof params.envId === 'string' ? params.envId : params.envId?.value,
        agentId: typeof params.agentId === 'string' ? params.agentId : params.agentId?.value,
        status: params.status,
        limit: params.limit,
      }),
  })
}

export function useRun(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['run', id],
    queryFn: () => runsClient.get(typeof id === 'string' ? id : id.value),
  })
}
