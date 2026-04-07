<script setup lang="ts">
import { ref, computed } from 'vue'
import { useSpans } from '@/lib/queries'

const limit = ref(50)
const hasError = ref<boolean | undefined>(undefined)

const { data: spans, isLoading, error } = useSpans({ limit: limit.value })

const columns = [
  { accessorKey: 'id', header: 'Span' },
  { accessorKey: 'run_id', header: 'Run' },
  { accessorKey: 'model', header: 'Model' },
  { accessorKey: 'duration', header: 'Duration' },
  { accessorKey: 'tokens', header: 'Tokens' },
  { accessorKey: 'cost', header: 'Cost' },
  { accessorKey: 'status', header: 'Status' },
  { accessorKey: 'started', header: 'Started' },
]

const rows = computed(() =>
  (spans.value ?? []).map(s => ({
    id: s.id.slice(0, 8),
    run_id: s.run_id.slice(0, 8),
    model: s.model ?? '—',
    duration: s.duration_ms ? `${s.duration_ms}ms` : '—',
    tokens: s.input_tokens != null ? `${s.input_tokens}→${s.output_tokens}` : '—',
    cost: s.cost_usd != null ? `$${s.cost_usd.toFixed(6)}` : '—',
    status: s.error ? 'error' : 'ok',
    started: new Date(s.started_at).toLocaleTimeString(),
    _error: s.error,
  }))
)
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold tracking-tight">Traces</h1>
        <p class="text-sm text-muted mt-0.5">Span explorer — every LLM call, tool use, and action.</p>
      </div>
    </div>

    <UCard class="rounded-xl">
      <div v-if="isLoading" class="grid h-32 place-items-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <div v-else-if="error" class="grid h-32 place-items-center text-sm text-red-500">
        Failed to load spans: {{ error.message }}
      </div>

      <div v-else-if="!spans?.length" class="grid h-32 place-items-center text-sm text-muted">
        No spans recorded yet. Point an agent at the proxy to start tracing.
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
