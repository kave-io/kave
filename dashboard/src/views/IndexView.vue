<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import LiveStatusBadge from '../components/LiveStatusBadge.vue'
import StatCard from '../components/dashboard/StatCard.vue'
import AlertsPanel from '../components/dashboard/AlertsPanel.vue'
import ConnectorList from '../components/dashboard/ConnectorList.vue'
import { useAgents, useRuns, useCostSummary } from '@/lib/queries'
import { useSpanStream } from '@/composables/useSpanStream'
import { workspaceId } from '@/stores/workspace'

const { t } = useI18n()
const { data: agents } = useAgents(workspaceId)
const { data: runs } = useRuns({ workspaceId, limit: 20 })
const { data: cost } = useCostSummary()
const { spans: liveSpans, isLive } = useSpanStream()

const stats = computed(() => {
  const activeAgents = agents.value?.length ?? 0
  const todayRuns = runs.value?.length ?? 0
  const successRuns = runs.value?.filter(r => r.status === 'completed').length ?? 0
  const successRate = todayRuns > 0 ? ((successRuns / todayRuns) * 100).toFixed(1) : '—'
  const totalSpend = cost.value?.total_usd ?? 0
  const errorRuns = runs.value?.filter(r => r.status === 'failed').length ?? 0

  return [
    { label: t('pages.overview.stat_active_agents'), value: String(activeAgents), hint: t('pages.overview.stat_active_agents_hint'), icon: 'i-lucide-bot' },
    { label: t('pages.overview.stat_runs_today'), value: String(todayRuns), hint: `${successRate}% success`, icon: 'i-lucide-activity' },
    { label: t('pages.overview.stat_spend_month'), value: `$${totalSpend.toFixed(2)}`, hint: t('pages.overview.stat_spend_hint'), icon: 'i-lucide-wallet' },
    { label: t('pages.overview.stat_failed_runs'), value: String(errorRuns), hint: errorRuns > 0 ? t('pages.overview.stat_failed_runs_hint_error') : t('pages.overview.stat_failed_runs_hint_ok'), icon: 'i-lucide-shield-alert' },
  ]
})

// Connector health is static for now — real health checks come post-v1
const connectors = [
  { name: 'OpenAI Proxy', status: 'Healthy', latency: '—' },
  { name: 'Anthropic Proxy', status: 'Healthy', latency: '—' },
  { name: 'Ollama', status: 'Healthy', latency: '—' },
]

// Alerts derived from data — only show real alerts
const alerts = computed(() => {
  const items = []
  const failedRuns = runs.value?.filter(r => r.status === 'failed') ?? []
  if (failedRuns.length > 0) {
    items.push({ title: `${failedRuns.length} ${failedRuns.length === 1 ? t('pages.overview.failed_runs_alert') : t('pages.overview.failed_runs_plural')}`, description: t('pages.overview.check_traces'), tone: 'error' as const })
  }
  const totalSpend = cost.value?.total_usd ?? 0
  if (totalSpend > 10) {
    items.push({ title: t('pages.overview.spend_alert'), description: `$${totalSpend.toFixed(2)} ${t('pages.overview.spend_details')}`, tone: 'warning' as const })
  }
  return items
})
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <PageHeader :title="t('pages.overview.title')" :subtitle="t('pages.overview.subtitle')" show-kave-logo>
      <LiveStatusBadge :is-live="isLive" />
    </PageHeader>

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
      <!-- Live activity feed -->
      <UCard class="rounded-xl">
        <template #header>
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-base font-semibold">{{ t('pages.overview.live_activity') }}</h3>
              <p class="text-sm text-muted">{{ t('pages.overview.live_activity_hint') }}</p>
            </div>
            <LiveStatusBadge :is-live="isLive" />
          </div>
        </template>

        <div v-if="liveSpans.length === 0" class="grid h-32 place-items-center text-sm text-muted">
          {{ t('pages.overview.waiting_traces') }}
        </div>
        <div v-else class="divide-y divide-border">
          <div
            v-for="span in liveSpans.slice(0, 10)"
            :key="span.id"
            class="flex items-center justify-between py-2.5 text-sm"
          >
            <div class="flex items-center gap-3 min-w-0">
              <UIcon
                :name="span.error ? 'i-lucide-x-circle' : 'i-lucide-check-circle'"
                :class="span.error ? 'text-red-500' : 'text-green-500'"
                class="size-4 shrink-0"
              />
              <span class="font-mono text-muted text-xs truncate">{{ span.run_id.slice(0, 8) }}</span>
              <span v-if="span.model" class="text-xs text-muted truncate">{{ span.model }}</span>
            </div>
            <div class="flex items-center gap-3 text-xs text-muted shrink-0">
              <span v-if="span.duration_ms">{{ span.duration_ms }}ms</span>
              <span v-if="span.input_tokens">{{ span.input_tokens }}→{{ span.output_tokens }} tok</span>
              <span>{{ new Date(span.started_at).toLocaleTimeString() }}</span>
            </div>
          </div>
        </div>
      </UCard>

      <AlertsPanel :items="alerts" />
    </section>

    <section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
      <UCard class="rounded-xl">
        <template #header>
          <div>
            <h3 class="text-base font-semibold">{{ t('pages.overview.spend_by_model') }}</h3>
            <p class="text-sm text-muted">{{ t('pages.overview.spend_by_model_hint') }}</p>
          </div>
        </template>
        <div v-if="cost?.by_model && Object.keys(cost.by_model).length" class="space-y-1">
          <div v-for="(usd, model) in cost.by_model" :key="model" class="flex items-center justify-between text-sm py-1.5">
            <span class="font-mono text-xs text-muted truncate">{{ model }}</span>
            <span class="font-semibold tabular-nums">{{ usd > 0.01 ? `$${usd.toFixed(4)}` : `$${usd.toFixed(6)}` }}</span>
          </div>
        </div>
        <div v-else class="grid h-24 place-items-center text-sm text-muted">
          {{ t('pages.overview.no_spend') }}
        </div>
      </UCard>

      <ConnectorList :items="connectors" />
    </section>
  </div>
</template>
