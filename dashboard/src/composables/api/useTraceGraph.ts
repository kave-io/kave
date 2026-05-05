import { useQuery } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { traceClient } from '@/lib/rpc/clients'

function unrefString(v: string | Ref<string>) {
  return typeof v === 'string' ? v : v.value
}

export function useTraceGraph(runId: string | Ref<string>, limit = 1000) {
  return useQuery({
    queryKey: ['trace-graph', runId, limit],
    queryFn: () => traceClient.graph({ runId: unrefString(runId), limit }),
    enabled: () => !!unrefString(runId),
  })
}
