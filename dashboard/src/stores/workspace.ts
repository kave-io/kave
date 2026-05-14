import { ref } from 'vue'
import { runtimeEnv } from '@/lib/env'
import { workspaceClient } from '@/lib/rpc/clients'

// Active project and environment — set by the selector in the layout.
// Defaults to 'default' for local daemon mode.
export const projectId = ref<string>(runtimeEnv.VITE_PROJECT_ID || 'default')
export const envId = ref<string>(runtimeEnv.VITE_ENV_ID || 'default')

let initialized = false

function pickCurrentOrFirst<T extends { id: string; slug?: string }>(items: T[], currentId: string, preferredSlug?: string) {
  if (items.some((item) => item.id === currentId)) return currentId
  if (preferredSlug) {
    const preferred = items.find((item) => item.slug === preferredSlug)
    if (preferred) return preferred.id
  }
  return items[0]?.id ?? ''
}

export async function initializeWorkspaceContext() {
  if (initialized) return
  initialized = true
  try {
    const projects = await workspaceClient.listProjects()
    if (projects.length === 0) return
    projectId.value = pickCurrentOrFirst(projects, projectId.value, 'kave-local')

    const environments = await workspaceClient.listEnvironments(projectId.value)
    if (environments.length === 0) return
    envId.value = pickCurrentOrFirst(environments, envId.value, 'dev')
  } catch {
    // Keep env-provided defaults when workspace lookup is unavailable.
  }
}
