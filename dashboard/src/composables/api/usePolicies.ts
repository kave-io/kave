import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import type { CreatePolicyRequest } from '@/types/api'
import { policiesClient } from '@/lib/rpc/clients'

export function usePolicy(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['policy', id],
    queryFn: () => policiesClient.get(typeof id === 'string' ? id : id.value),
    enabled: !!(typeof id === 'string' ? id : id.value),
  })
}

export function useCreatePolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreatePolicyRequest) => policiesClient.create(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['policies'] }),
  })
}
