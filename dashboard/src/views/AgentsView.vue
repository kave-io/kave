<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import RunStatusBadge from '../components/RunStatusBadge.vue'
import DetailRow from '../components/DetailRow.vue'
import type { Agent } from '@/types/api'
import { useAgents, useRuns } from '@/lib/queries'
import { workspaceId } from '@/stores/workspace'

const { t } = useI18n()

const { data: agents, isLoading, error } = useAgents(workspaceId)
const { data: allRuns } = useRuns({ workspaceId, limit: 50 })
const search = ref('')
const selectedAgent = ref<Agent | null>(null)

const filteredAgents = computed(() =>
  (agents.value ?? [])
    .filter(a => !search.value || a.name.toLowerCase().includes(search.value.toLowerCase()))
    .map(a => ({
      id: a.id,
      name: a.name,
      description: a.description || '—',
      budget: a.monthly_budget != null ? `$${a.monthly_budget.toFixed(2)}` : 'Unlimited',
      created: new Date(a.created_at).toLocaleDateString(),
      _agent: a,
    }))
)

const recentActivity = computed(() => {
  if (!selectedAgent.value || !allRuns.value) return []
  return (allRuns.value ?? [])
    .filter(r => r.agent_id === selectedAgent.value?.id)
    .slice(0, 5)
    .map(r => ({
      status: r.status,
      time: new Date(r.started_at).toLocaleString(),
      cost: r.spent_usd != null ? `$${r.spent_usd.toFixed(4)}` : '—',
    }))
})

</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <PageHeader :title="t('pages.agents.title')" :subtitle="t('pages.agents.subtitle')" icon="i-lucide-bot" />

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

      <template v-else>
        <div class="divide-y divide-border/50">
          <div
            v-for="agent in filteredAgents"
            :key="agent.id"
            class="flex items-center justify-between px-4 py-2.5 hover:bg-muted/40 cursor-pointer transition"
            @click="selectedAgent = agent._agent"
          >
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium">{{ agent.name }}</p>
              <p class="text-xs text-muted truncate">{{ agent.description }}</p>
            </div>
            <div class="flex items-center gap-6 ml-4 shrink-0 text-xs text-muted">
              <span class="tabular-nums">{{ agent.budget }}</span>
              <span class="whitespace-nowrap">{{ agent.created }}</span>
            </div>
          </div>
        </div>
      </template>
    </UCard>

    <!-- Agent detail drawer -->
    <USlideover v-model="selectedAgent" title="">
      <template v-if="selectedAgent" #header>
        <div class="space-y-1">
          <h2 class="text-base font-semibold">{{ selectedAgent.name }}</h2>
          <p class="text-xs text-muted font-mono">{{ selectedAgent.id }}</p>
        </div>
      </template>
      <div v-if="selectedAgent" class="space-y-6 pr-0">
        <!-- Policy & Budget -->
        <section class="space-y-3">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Policy & Budget</h3>
          <div class="space-y-3">
            <DetailRow
              label="Policy ID"
              :value="selectedAgent.policy_id || '—'"
              mono
            />
            <DetailRow
              v-if="selectedAgent.monthly_budget"
              label="Monthly Budget"
              :value="`$${selectedAgent.monthly_budget.toFixed(2)}`"
              large
            />
          </div>
        </section>

        <!-- Recent Activity -->
        <section class="space-y-3 border-t border-default pt-4">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Recent Runs</h3>
          <div v-if="recentActivity.length > 0" class="space-y-1.5">
            <div
              v-for="(run, idx) in recentActivity"
              :key="idx"
              class="flex items-center justify-between text-xs py-2 px-2.5 rounded hover:bg-muted/30 transition"
            >
              <div class="flex items-center gap-2 min-w-0">
                <RunStatusBadge :status="run.status" />
                <span class="text-muted truncate">{{ run.time }}</span>
              </div>
              <span class="font-mono font-semibold tabular-nums shrink-0">{{ run.cost }}</span>
            </div>
          </div>
          <div v-else class="text-xs text-muted py-2">
            No runs yet.
          </div>
        </section>

        <!-- Metadata -->
        <section class="space-y-3 border-t border-default pt-4">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Created</h3>
          <p class="text-sm">{{ new Date(selectedAgent.created_at).toLocaleString() }}</p>
        </section>
      </div>
    </USlideover>
  </div>
</template>
