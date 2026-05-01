import { useQuery } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { costClient } from '@/lib/rpc/clients'

export function useCostSummary(params?: { agentId?: string | Ref<string>; connector?: string; model?: string }) {
  return useQuery({
    queryKey: ['cost', 'summary', params],
    queryFn: () =>
      costClient.summary({
        agentId: typeof params?.agentId === 'string' ? params?.agentId : params?.agentId?.value,
        connector: params?.connector,
        model: params?.model,
      }),
  })
}
