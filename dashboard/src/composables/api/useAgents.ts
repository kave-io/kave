import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import type { Agent, CreateAgentRequest } from '@/types/api'
import { agentsClient } from '@/lib/rpc/clients'

export function useAgents(envId: string | Ref<string>) {
  return useQuery({
    queryKey: ['agents', envId],
    queryFn: () => agentsClient.list(typeof envId === 'string' ? envId : envId.value),
  })
}

export function useAgent(id: string | Ref<string>) {
  return useQuery({
    queryKey: ['agent', id],
    queryFn: () => agentsClient.get(typeof id === 'string' ? id : id.value),
  })
}

export function useCreateAgent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateAgentRequest) => agentsClient.create(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useUpdateAgent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: Partial<Agent> & { id: string }) => agentsClient.update(id, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}
