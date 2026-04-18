<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import RunStatusBadge from '../components/RunStatusBadge.vue'
import DetailRow from '../components/DetailRow.vue'
import type { Run } from '@/types/api'
import { useRuns, useRunSpans } from '@/lib/queries'
import { projectId, envId } from '@/stores/workspace'
import { useCurrencyStore } from '@/stores/currency'

const { t } = useI18n()
const currencyStore = useCurrencyStore()

const limit = ref(100)
const { data: runs, isLoading, error } = useRuns({ projectId, envId, limit: limit.value })
const search = ref('')
const statusFilter = ref<'all' | 'completed' | 'failed' | 'pending' | 'active'>('all')

const filteredRuns = computed(() => {
  return (runs.value ?? [])
    .filter(r => {
      if (statusFilter.value !== 'all' && r.status !== statusFilter.value) return false
      if (search.value && !r.id.includes(search.value) && !r.agent_id?.includes(search.value)) return false
      return true
    })
    .map(r => ({
      id: r.id.slice(0, 8),
      run_id: r.id,
      agent_id: r.agent_id?.slice(0, 8) || '—',
      agent_full_id: r.agent_id || '—',
      status: r.status,
      duration: r.ended_at && r.started_at
        ? `${Math.round((new Date(r.ended_at).getTime() - new Date(r.started_at).getTime()) / 1000)}s`
        : '—',
      cost: r.spent ? currencyStore.format(r.spent) : '—',
      started: new Date(r.started_at).toLocaleString(),
      error: r.error_message,
      budget: r.budget_cap,
    }))
})

const selectedRun = ref<typeof filteredRuns.value[0] | null>(null)
const { data: runSpans } = useRunSpans(computed(() => selectedRun.value?.run_id || ''), 100)
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <PageHeader :title="t('pages.runs.title')" :subtitle="t('pages.runs.subtitle')" icon="i-lucide-activity" />

    <UCard class="rounded-xl">
      <template #header>
        <div class="flex items-center justify-between gap-4">
          <UInput v-model="search" icon="i-lucide-search" placeholder="Filter by run or agent…" class="w-64" />
          <USegmentControl
            v-model="statusFilter"
            :options="[
              { value: 'all', label: 'All' },
              { value: 'completed', label: 'Completed' },
              { value: 'failed', label: 'Failed' },
              { value: 'active', label: 'Active' },
            ]"
          />
        </div>
      </template>

      <div v-if="isLoading" class="grid h-32 place-items-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <div v-else-if="error" class="grid h-32 place-items-center text-sm text-red-500">
        Failed to load runs: {{ error.message }}
      </div>

      <div v-else-if="filteredRuns.length === 0" class="grid h-32 place-items-center text-sm text-muted">
        No runs found.
      </div>

      <template v-else>
        <div class="divide-y divide-border/50">
          <div
            v-for="run in filteredRuns"
            :key="run.run_id"
            class="flex items-center justify-between px-4 py-2.5 hover:bg-muted/40 cursor-pointer transition"
            @click="selectedRun = run"
          >
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2.5 mb-1">
                <RunStatusBadge :status="run.status" />
                <span class="text-sm font-mono">{{ run.id }}</span>
                <span class="text-xs text-muted">{{ run.agent_id }}</span>
              </div>
              <p class="text-xs text-muted">{{ run.started }}</p>
            </div>
            <div class="flex items-center gap-6 ml-4 shrink-0 text-xs text-muted">
              <span class="tabular-nums">{{ run.duration }}</span>
              <span class="font-semibold tabular-nums">{{ run.cost }}</span>
            </div>
          </div>
        </div>
      </template>
    </UCard>

    <!-- Run detail drawer -->
    <USlideover v-model="selectedRun" title="">
      <template v-if="selectedRun" #header>
        <div class="space-y-1">
          <div class="flex items-center gap-2">
            <h2 class="text-base font-semibold">Run</h2>
            <RunStatusBadge :status="selectedRun.status" />
          </div>
          <p class="text-xs text-muted font-mono">{{ selectedRun.run_id }}</p>
        </div>
      </template>
      <div v-if="selectedRun" class="space-y-6">
        <!-- Status & Timing -->
        <section class="space-y-3">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Execution</h3>
          <div class="grid grid-cols-2 gap-3">
            <DetailRow label="Agent" :value="selectedRun.agent_full_id" mono />
            <DetailRow label="Duration" :value="selectedRun.duration" />
            <div class="col-span-2">
              <DetailRow label="Started" :value="selectedRun.started" />
            </div>
          </div>
        </section>

        <!-- Cost & Budget -->
        <section class="space-y-3 border-t border-default pt-4">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Cost</h3>
          <div class="grid grid-cols-2 gap-3">
            <DetailRow label="Total Spend" :value="selectedRun.cost" large />
            <DetailRow label="Budget Cap" :value="selectedRun.budget ? currencyStore.format(selectedRun.budget) : '—'" />
          </div>
        </section>

        <!-- Error if present -->
        <div v-if="selectedRun.error" class="space-y-2 border-t border-default pt-4">
          <p class="text-xs font-medium uppercase tracking-wide text-red-600">Error</p>
          <div class="bg-red-500/10 rounded px-3 py-2.5 text-xs text-red-600 font-mono overflow-auto max-h-32">
            {{ selectedRun.error }}
          </div>
        </div>

        <!-- Actions in this run -->
        <section class="border-t border-default pt-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Actions</h3>
            <span class="text-xs text-muted tabular-nums">{{ runSpans?.length ?? 0 }}</span>
          </div>
          <div v-if="!runSpans?.length" class="text-xs text-muted py-2">
            No actions recorded.
          </div>
          <div v-else class="space-y-1.5 max-h-56 overflow-y-auto">
            <div
              v-for="span in runSpans"
              :key="span.id"
              class="text-xs py-2 px-2.5 rounded hover:bg-muted/30 border border-default/50 transition"
            >
              <div class="flex items-center justify-between gap-2">
                <span class="font-mono truncate">{{ span.name }}</span>
                <span v-if="span.model" class="text-muted shrink-0">{{ span.model }}</span>
              </div>
              <div class="flex items-center justify-between text-muted mt-1">
                <span>{{ span.duration_ms }}ms</span>
                <span v-if="span.input_tokens" class="tabular-nums"
                  >{{ span.input_tokens }}{{ (span.cache_read_tokens ?? 0) > 0 ? '⚡' : '' }}→{{ span.output_tokens ?? 0 }}</span
                >
                <span class="tabular-nums">{{ span.cost ? currencyStore.format(span.cost) : '—' }}</span>
              </div>
            </div>
          </div>
        </section>
      </div>
    </USlideover>
  </div>
</template>
