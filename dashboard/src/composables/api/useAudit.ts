import { useQuery } from '@tanstack/vue-query'
import { auditClient } from '@/lib/rpc/clients'

export function useAuditEntries(params: { orgId?: string; projectId?: string; envId?: string; limit?: number } = {}) {
  return useQuery({
    queryKey: ['audit', params],
    queryFn: () => auditClient.list(params),
  })
}
