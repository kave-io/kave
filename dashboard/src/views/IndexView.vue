<script setup lang="ts">
import { computed } from 'vue'
import StatCard from '../components/dashboard/StatCard.vue'
import TraceTable from '../components/dashboard/TraceTable.vue'
import AlertsPanel from '../components/dashboard/AlertsPanel.vue'
import ConnectorList from '../components/dashboard/ConnectorList.vue'
import { useAgents, useRuns, useCostSummary } from '@/lib/queries'
import { workspaceId } from '@/stores/workspace'

const { data: agents } = useAgents(workspaceId)
const { data: runs } = useRuns({ workspaceId, limit: 20 })
const { data: cost } = useCostSummary()

const stats = computed(() => {
  const activeAgents = agents.value?.length ?? 0
  const todayRuns = runs.value?.length ?? 0
  const successRuns = runs.value?.filter(r => r.status === 'completed').length ?? 0
  const successRate = todayRuns > 0 ? ((successRuns / todayRuns) * 100).toFixed(1) : '—'
  const totalSpend = cost.value?.total_usd ?? 0

  const errorRuns = runs.value?.filter(r => r.status === 'failed').length ?? 0

  return [
    { label: 'Active Agents', value: String(activeAgents), hint: 'registered in workspace', icon: 'i-lucide-bot' },
    { label: 'Runs Today', value: String(todayRuns), hint: `${successRate}% success`, icon: 'i-lucide-activity' },
    { label: 'Spend This Month', value: `$${totalSpend.toFixed(2)}`, hint: 'from budget ledger', icon: 'i-lucide-wallet' },
    { label: 'Failed Runs', value: String(errorRuns), hint: errorRuns > 0 ? 'check traces' : 'all clear', icon: 'i-lucide-shield-alert' },
  ]
})

const recentTraces = computed(() =>
  (runs.value ?? []).slice(0, 8).map(r => ({
    id: r.id.slice(0, 8),
    agent: r.agent_id.slice(0, 8),
    model: '—',
    status: r.status,
    latency: r.ended_at ? `${r.ended_at - r.started_at}ms` : '…',
    cost: `$${r.spent_usd.toFixed(4)}`,
    startedAt: new Date(r.started_at).toLocaleTimeString(),
  }))
)

// Connector health is static for now — real health checks come post-v1
const connectors = [
  { name: 'OpenAI Proxy', status: 'Healthy', latency: '—' },
  { name: 'Anthropic Proxy', status: 'Healthy', latency: '—' },
  { name: 'Ollama', status: 'Healthy', latency: '—' },
]

// Alerts derived from data
const alerts = computed(() => {
  const items = []
  const failedRuns = runs.value?.filter(r => r.status === 'failed') ?? []
  if (failedRuns.length > 0) {
    items.push({ title: `${failedRuns.length} failed runs`, description: 'Check traces for error details.', tone: 'error' as const })
  }
  const totalSpend = cost.value?.total_usd ?? 0
  if (totalSpend > 10) {
    items.push({ title: 'Spend accumulating', description: `$${totalSpend.toFixed(2)} recorded this period.`, tone: 'warning' as const })
  }
  if (items.length === 0) {
    items.push({ title: 'All systems nominal', description: 'No active alerts.', tone: 'success' as const })
  }
  return items
})
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold tracking-tight">Overview</h1>
        <p class="text-sm text-muted mt-0.5">Production workspace</p>
      </div>
    </div>

    <section class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        v-for="item in stats"
        :key="item.label"
        :label="item.label"
        :value="item.value"
        :hint="item.hint"
        :icon="item.icon"
      />
    </section>

    <section class="grid gap-6 xl:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.9fr)]">
      <TraceTable :rows="recentTraces" />
      <AlertsPanel :items="alerts" />
    </section>

    <section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
      <UCard class="rounded-xl">
        <template #header>
          <div>
            <h3 class="text-base font-semibold">Spend by model</h3>
            <p class="text-sm text-muted">From the budget ledger.</p>
          </div>
        </template>
        <div v-if="cost?.by_model && Object.keys(cost.by_model).length" class="space-y-2">
          <div v-for="(usd, model) in cost.by_model" :key="model" class="flex items-center justify-between text-sm">
            <span class="font-mono text-muted">{{ model }}</span>
            <span class="font-semibold">${{ usd.toFixed(4) }}</span>
          </div>
        </div>
        <div v-else class="grid h-32 place-items-center text-sm text-muted">
          No spend data yet.
        </div>
      </UCard>

      <ConnectorList :items="connectors" />
    </section>
  </div>
</template>
