<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import type { Span } from '@/types/api'
import { useSpans } from '@/lib/queries'
import { useSpanStream } from '@/composables/useSpanStream'

const { t } = useI18n()

// Show total context (input + cache hits) → output.
// Cache hits are marked with ⚡ to indicate they were cheap.
function formatTokens(s: Span): string {
  if (s.input_tokens == null) return '—'
  const totalIn = (s.input_tokens ?? 0) + (s.cache_read_tokens ?? 0) + (s.cache_write_tokens ?? 0)
  const cacheHit = (s.cache_read_tokens ?? 0) > 0
  return `${totalIn}${cacheHit ? t('pages.traces.cache_hit') : ''}→${s.output_tokens ?? 0}`
}

const limit = ref(50)

const { data: historicalSpans, isLoading, error } = useSpans({ limit: limit.value })
const { spans: liveSpans, isLive } = useSpanStream()

// Merge live (newest first) + historical, deduplicated by id
const rows = computed(() => {
  const live = liveSpans.value
  const hist = historicalSpans.value ?? []
  const liveIds = new Set(live.map(s => s.id))
  const merged = [...live, ...hist.filter(s => !liveIds.has(s.id))]

  return merged.map(s => ({
    id: s.id.slice(0, 8),
    run_id: s.run_id.slice(0, 8),
    model: s.model ?? '—',
    duration: s.duration_ms != null ? `${s.duration_ms}ms` : '—',
    tokens: formatTokens(s),
    cost: s.cost_usd != null ? `$${s.cost_usd.toFixed(6)}` : '—',
    cached: (s.cache_read_tokens ?? 0) > 0,
    status: s.error ? 'error' : 'ok',
    started: new Date(s.started_at).toLocaleTimeString(),
    _isNew: liveIds.has(s.id),
    _error: s.error,
  }))
})

const columns = computed(() => [
  { accessorKey: 'started', header: t('pages.traces.table_time') },
  { accessorKey: 'run_id', header: t('pages.traces.table_run') },
  { accessorKey: 'model', header: t('pages.traces.table_model') },
  { accessorKey: 'duration', header: t('pages.traces.table_duration') },
  { accessorKey: 'tokens', header: t('pages.traces.table_tokens') },
  { accessorKey: 'cost', header: t('pages.traces.table_cost') },
  { accessorKey: 'status', header: t('pages.traces.table_status') },
])
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <PageHeader :title="t('pages.traces.title')" :subtitle="t('pages.traces.subtitle')" icon="i-lucide-waypoints">
      <!-- LIVE indicator -->
      <div class="flex items-center gap-2">
        <span v-if="isLive" class="flex items-center gap-1.5 rounded-full bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-500">
          <span class="relative flex h-1.5 w-1.5">
            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
            <span class="relative inline-flex h-1.5 w-1.5 rounded-full bg-green-500" />
          </span>
          {{ t('pages.traces.live_indicator') }}
        </span>
        <span v-else class="flex items-center gap-1.5 rounded-full bg-muted/50 px-2.5 py-1 text-xs font-medium text-muted">
          <span class="h-1.5 w-1.5 rounded-full bg-muted" />
          {{ t('pages.traces.connecting_indicator') }}
        </span>
      </div>
    </PageHeader>

    <UCard class="rounded-xl">
      <template #header>
        <p class="text-xs text-muted">{{ rows.length }} {{ t('pages.traces.count_loaded') }}</p>
      </template>

      <div v-if="isLoading && rows.length === 0" class="grid h-32 place-items-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <div v-else-if="error" class="grid h-32 place-items-center text-sm text-red-500">
        {{ t('pages.traces.error_loading') }} {{ error.message }}
      </div>

      <div v-else-if="rows.length === 0" class="grid h-40 place-items-center text-center">
        <div class="space-y-2">
          <UIcon name="i-lucide-activity" class="size-8 text-muted mx-auto" />
          <p class="text-sm font-medium">{{ t('pages.traces.waiting_traces') }}</p>
          <p class="text-xs text-muted max-w-xs">
            {{ t('pages.traces.waiting_hint') }}
          </p>
          <code class="mt-2 block text-xs text-muted bg-muted/20 rounded px-2 py-1">
            {{ t('pages.traces.example_url') }}
          </code>
        </div>
      </div>

      <UTable v-else :data="rows" :columns="columns">
        <template #status-cell="{ row }">
          <UBadge :color="row.original.status === 'error' ? 'error' : 'success'" variant="soft" size="xs">
            {{ row.original.status === 'error' ? t('pages.traces.status_error') : t('pages.traces.status_ok') }}
          </UBadge>
        </template>
        <template #model-cell="{ row }">
          <span class="font-mono text-xs">{{ row.original.model }}</span>
        </template>
      </UTable>
    </UCard>
  </div>
</template>
