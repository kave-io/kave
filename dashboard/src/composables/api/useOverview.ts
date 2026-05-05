import { useQuery } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { overviewClient } from '@/lib/rpc/clients'

function unrefString(v?: string | Ref<string>) {
  return typeof v === 'string' ? v : v?.value
}

export function useDashboardOverview(params: { projectId?: string | Ref<string>; envId?: string | Ref<string>; recentLimit?: number }) {
  return useQuery({
    queryKey: ['dashboard-overview', params],
    queryFn: () => overviewClient.get({
      projectId: unrefString(params.projectId),
      envId: unrefString(params.envId),
      recentLimit: params.recentLimit,
    }),
    refetchInterval: 5000,
  })
}
