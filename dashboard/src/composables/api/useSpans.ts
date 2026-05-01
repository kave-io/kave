import { useQuery } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { spansClient } from '@/lib/rpc/clients'

export function useRunSpans(runId: string | Ref<string>, limit = 50) {
  return useQuery({
    queryKey: ['spans', 'run', runId, limit],
    queryFn: () => spansClient.listByRun(typeof runId === 'string' ? runId : runId.value, limit),
    enabled: !!(typeof runId === 'string' ? runId : runId.value),
  })
}

export function useSpans(params: { runId?: string; hasError?: boolean; limit?: number }) {
  return useQuery({
    queryKey: ['spans', params],
    queryFn: () => spansClient.list(params),
  })
}
