<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Span } from '@/types/api'

// Show total context (input + cache hits) → output.
// Cache hits are marked with ⚡ to indicate they were cheap.
function formatTokens(s: Span): string {
  if (s.input_tokens == null) return '—'
  const totalIn = (s.input_tokens ?? 0) + (s.cache_read_tokens ?? 0) + (s.cache_write_tokens ?? 0)
  const cacheHit = (s.cache_read_tokens ?? 0) > 0
  return `${totalIn}${cacheHit ? '⚡' : ''}→${s.output_tokens ?? 0}`
}
import { useSpans } from '@/lib/queries'
import { useSpanStream } from '@/composables/useSpanStream'

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

const columns = [
  { accessorKey: 'started', header: 'Time' },
  { accessorKey: 'run_id', header: 'Run' },
  { accessorKey: 'model', header: 'Model' },
  { accessorKey: 'duration', header: 'Duration' },
  { accessorKey: 'tokens', header: 'Tokens' },
  { accessorKey: 'cost', header: 'Cost' },
  { accessorKey: 'status', header: 'Status' },
]
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold tracking-tight">Traces</h1>
        <p class="text-sm text-muted mt-0.5">Every LLM call, tool use, and action through the proxy.</p>
      </div>

      <!-- LIVE indicator -->
      <div class="flex items-center gap-2">
        <span v-if="isLive" class="flex items-center gap-1.5 rounded-full bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-500">
          <span class="relative flex h-1.5 w-1.5">
            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
            <span class="relative inline-flex h-1.5 w-1.5 rounded-full bg-green-500" />
          </span>
          LIVE
        </span>
        <span v-else class="flex items-center gap-1.5 rounded-full bg-muted/50 px-2.5 py-1 text-xs font-medium text-muted">
          <span class="h-1.5 w-1.5 rounded-full bg-muted" />
          connecting
        </span>
      </div>
    </div>

    <UCard class="rounded-xl">
      <div v-if="isLoading && rows.length === 0" class="grid h-32 place-items-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <div v-else-if="error" class="grid h-32 place-items-center text-sm text-red-500">
        Failed to load spans: {{ error.message }}
      </div>

      <div v-else-if="rows.length === 0" class="grid h-48 place-items-center text-center">
        <div class="space-y-2">
          <UIcon name="i-lucide-activity" class="size-8 text-muted mx-auto" />
          <p class="text-sm font-medium">Waiting for traces</p>
          <p class="text-xs text-muted max-w-xs">
            Point any LLM call at the proxy — no config needed.
          </p>
          <code class="mt-2 block text-xs text-muted bg-muted/20 rounded px-2 py-1">
            OPENAI_BASE_URL=http://localhost:8080/proxy/openai
          </code>
        </div>
      </div>

      <UTable v-else :data="rows" :columns="columns">
        <template #status-cell="{ row }">
          <UBadge :color="row.original.status === 'error' ? 'error' : 'success'" variant="soft">
            {{ row.original.status }}
          </UBadge>
        </template>
      </UTable>
    </UCard>
  </div>
</template>
