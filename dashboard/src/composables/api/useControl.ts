import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { credentialsClient, daemonClient, tokensClient, workspaceClient } from '@/lib/rpc/clients'

function unrefString(v: string | Ref<string>) {
  return typeof v === 'string' ? v : v.value
}

export function useDaemonStatus() {
  return useQuery({
    queryKey: ['daemon', 'status'],
    queryFn: () => daemonClient.status(),
    refetchInterval: 5000,
    retry: 1,
  })
}

export function useDoctor() {
  return useQuery({ queryKey: ['daemon', 'doctor'], queryFn: () => daemonClient.doctor() })
}

export function useConfigPaths() {
  return useQuery({ queryKey: ['daemon', 'config-paths'], queryFn: () => daemonClient.configPaths() })
}

export function useWorkspaceProjects() {
  return useQuery({ queryKey: ['workspace', 'projects'], queryFn: () => workspaceClient.listProjects() })
}

export function useWorkspaceEnvironments(projectId: string | Ref<string>) {
  return useQuery({
    queryKey: ['workspace', 'environments', projectId],
    queryFn: () => workspaceClient.listEnvironments(unrefString(projectId)),
  })
}

export function useCredentials(envId: string | Ref<string>) {
  return useQuery({
    queryKey: ['credentials', envId],
    queryFn: () => credentialsClient.list(unrefString(envId)),
  })
}

export function useAgentTokens(agentId: string | Ref<string>) {
  return useQuery({
    queryKey: ['tokens', agentId],
    queryFn: () => tokensClient.list(unrefString(agentId)),
    enabled: () => !!unrefString(agentId),
  })
}

export function useCreateAgentToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, name }: { agentId: string; name: string }) => tokensClient.create(agentId, name),
    onSuccess: (_data, vars) => queryClient.invalidateQueries({ queryKey: ['tokens', vars.agentId] }),
  })
}
