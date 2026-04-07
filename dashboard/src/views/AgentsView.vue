<script setup lang="ts">
import { ref } from 'vue'
import { useAgents } from '@/lib/queries'
import { workspaceId } from '@/stores/workspace'

const { data: agents, isLoading, error } = useAgents(workspaceId)
const search = ref('')

const columns = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'description', header: 'Description' },
  { accessorKey: 'budget', header: 'Monthly Budget' },
  { accessorKey: 'created', header: 'Created' },
]

function toRows(agents: typeof agentsData.value) {
  return (agents ?? [])
    .filter(a => !search.value || a.name.toLowerCase().includes(search.value.toLowerCase()))
    .map(a => ({
      id: a.id,
      name: a.name,
      description: a.description || '—',
      budget: a.monthly_budget != null ? `$${a.monthly_budget.toFixed(2)}` : 'Unlimited',
      created: new Date(a.created_at).toLocaleDateString(),
    }))
}

const agentsData = agents
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold tracking-tight">Agents</h1>
        <p class="text-sm text-muted mt-0.5">Registered agent identities and their policies.</p>
      </div>
    </div>

    <UCard class="rounded-xl">
      <template #header>
        <div class="flex items-center justify-between gap-4">
          <UInput v-model="search" icon="i-lucide-search" placeholder="Filter agents…" class="w-64" />
        </div>
      </template>

      <div v-if="isLoading" class="grid h-32 place-items-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <div v-else-if="error" class="grid h-32 place-items-center text-sm text-red-500">
        Failed to load agents: {{ error.message }}
      </div>

      <div v-else-if="!agents?.length" class="grid h-32 place-items-center text-sm text-muted">
        No agents registered yet.
      </div>

      <UTable v-else :data="toRows(agents)" :columns="columns" />
    </UCard>
  </div>
</template>
