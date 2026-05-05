import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import type { CreatePolicyRequest } from '@/types/api'
import { policiesClient } from '@/lib/rpc/clients'

function unrefString(v: string | Ref<string>) {
  return typeof v === 'string' ? v : v.value
}

export function usePolicies(envId: string | Ref<string>) {
  return useQuery({
    queryKey: ['policies', envId],
    queryFn: () => policiesClient.list(unrefString(envId)),
  })
}

export function usePolicy(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['policy', id],
    queryFn: () => policiesClient.get(unrefString(id)),
    enabled: () => !!unrefString(id),
  })
}

export function useCreatePolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreatePolicyRequest) => policiesClient.create(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['policies'] }),
  })
}
