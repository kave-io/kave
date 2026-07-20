<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import MetricCard from '@/components/MetricCard.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusPanel from '@/components/StatusPanel.vue'
import UsageChart from '@/components/UsageChart.vue'
import { bucketUsage, summarizeUsage, usageByModel } from '@/lib/analytics'
import { contextRevision, currentRange, currentScope, hasReportingScope } from '@/lib/context'
import {
  formatCompactInteger,
  formatInteger,
  formatNanoUsd,
  formatTime,
  humanize,
  shortId,
} from '@/lib/format'
import { kernel } from '@/lib/kernel'
import type { TimeRange, UsageEntry } from '@/lib/types'

const entries = ref<UsageEntry[]>([])
const nextPageToken = ref('')
const loading = ref(false)
const loadingMore = ref(false)
const error = ref<Error>()
const agent = ref('')
const metric = ref('')
const loadedRange = ref<TimeRange>()
let generation = 0

async function load(reset = true): Promise<void> {
  const current = ++generation
  if (!hasReportingScope.value) {
    entries.value = []
    nextPageToken.value = ''
    loadedRange.value = undefined
    error.value = undefined
    loading.value = false
    loadingMore.value = false
    return
  }
  if (reset) {
    entries.value = []
    nextPageToken.value = ''
    loadedRange.value = undefined
    loadingMore.value = false
  }
  const isMore = !reset
  if (isMore) loadingMore.value = true
  else loading.value = true
  error.value = undefined
  try {
    const range = reset || !loadedRange.value ? currentRange() : loadedRange.value
    const page = await kernel.queryUsage(currentScope(), range, {
      agent: agent.value.trim() || undefined,
      metric: metric.value || undefined,
      pageToken: isMore ? nextPageToken.value : undefined,
    })
    if (current !== generation) return
    entries.value = isMore ? [...entries.value, ...page.items] : page.items
    nextPageToken.value = page.nextPageToken
    loadedRange.value = range
  } catch (caught) {
    if (current === generation) {
      error.value = caught instanceof Error ? caught : new Error('Usage could not be loaded.')
    }
  } finally {
    if (current === generation) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

onMounted(() => {
  if (hasReportingScope.value) void load()
})
watch(contextRevision, () => void load())

const totals = computed(() => summarizeUsage(entries.value))
const models = computed(() => usageByModel(entries.value))
const buckets = computed(() =>
  loadedRange.value ? bucketUsage(entries.value, loadedRange.value) : [],
)
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="Analyze"
      title="Usage analytics"
      description="Exact settled ledger rows with explicit pagination and no sampled or synthetic data."
    />

    <section class="filter-bar" aria-label="Usage filters">
      <label>
        <span>Agent</span>
        <input
          v-model.trim="agent"
          autocomplete="off"
          placeholder="All agents"
          @keyup.enter="load()"
        />
      </label>
      <label>
        <span>Metric</span>
        <select v-model="metric">
          <option value="">All dimensions</option>
          <option value="requests">Requests</option>
          <option value="input_tokens">Input tokens</option>
          <option value="output_tokens">Output tokens</option>
          <option value="cost_nano_usd">Cost</option>
        </select>
      </label>
      <button type="button" class="button button-primary" :disabled="loading" @click="load()">
        Run query
      </button>
    </section>

    <StatusPanel
      v-if="!hasReportingScope"
      title="Reporting scope required"
      message="Set the tenant and bill-to boundary in the top bar to query analytics."
      tone="warning"
    />
    <StatusPanel
      v-else-if="loading && !entries.length"
      title="Loading usage"
      message="Reading immutable settlement rows."
      busy
    />
    <StatusPanel
      v-else-if="error && !entries.length"
      title="Analytics unavailable"
      :message="error.message"
      tone="error"
      @retry="load()"
    >
      <template #action>Try again</template>
    </StatusPanel>

    <template v-else-if="hasReportingScope">
      <div v-if="!loadedRange" class="notice">Run the query to read usage for this scope.</div>

      <section
        v-if="loadedRange"
        class="metric-grid compact-metrics"
        aria-label="Loaded usage totals"
      >
        <MetricCard
          label="Loaded rows"
          :value="formatInteger(entries.length)"
          :detail="nextPageToken ? 'more rows available' : 'interval complete'"
        />
        <MetricCard label="Requests" :value="formatCompactInteger(totals.requests)" />
        <MetricCard
          label="Tokens"
          :value="formatCompactInteger(totals.inputTokens + totals.outputTokens)"
        />
        <MetricCard
          label="Cost"
          :value="formatNanoUsd(totals.costNanoUsd)"
          :tone="totals.estimatedRows ? 'warning' : 'default'"
        />
      </section>

      <section v-if="loadedRange" class="analytics-grid">
        <article class="panel chart-panel">
          <header class="panel-header">
            <div>
              <h2>Cost cadence</h2>
              <p>Nano-USD settlement totals in loaded rows</p>
            </div>
          </header>
          <UsageChart :buckets="buckets" value="costNanoUsd" />
        </article>

        <article class="panel">
          <header class="panel-header">
            <div>
              <h2>Models</h2>
              <p>Grouped from loaded canonical rows</p>
            </div>
          </header>
          <div v-if="models.length" class="model-list">
            <div
              v-for="model in models.slice(0, 8)"
              :key="`${model.provider}:${model.model}`"
              class="model-row"
            >
              <div>
                <strong class="mono">{{ model.model }}</strong>
                <span>{{ model.provider }}</span>
              </div>
              <div class="model-values">
                <span>{{ formatCompactInteger(model.tokens) }} tok</span>
                <strong>{{ formatNanoUsd(model.costNanoUsd) }}</strong>
              </div>
            </div>
          </div>
          <div v-else class="panel-empty">
            <strong>No models</strong><span>No settled model usage matched.</span>
          </div>
        </article>
      </section>

      <section v-if="loadedRange" class="panel">
        <header class="panel-header">
          <div>
            <h2>Usage ledger</h2>
            <p>Only canonical consume and provider settlement rows are exposed by V2</p>
          </div>
          <span v-if="nextPageToken" class="status-badge warning">Partial</span>
        </header>
        <div v-if="entries.length" class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Time</th>
                <th>Invocation</th>
                <th>Provider / model</th>
                <th>Requests</th>
                <th>Tokens in / out</th>
                <th>Cost</th>
                <th>Evidence</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="entry in entries" :key="entry.id">
                <td>{{ formatTime(entry.createdAtMs) }}</td>
                <td class="mono" :title="entry.invocationId">{{ shortId(entry.invocationId) }}</td>
                <td>
                  <span>{{ entry.provider || 'custom' }}</span
                  ><small class="table-sub mono">{{ entry.model || 'unattributed' }}</small>
                </td>
                <td class="tnum">{{ formatInteger(entry.requestCount) }}</td>
                <td class="tnum">
                  {{ formatInteger(entry.inputTokens) }} / {{ formatInteger(entry.outputTokens) }}
                </td>
                <td class="tnum">{{ formatNanoUsd(entry.costNanoUsd) }}</td>
                <td>
                  <span class="status-badge" :class="entry.estimated ? 'warning' : 'settled'">{{
                    entry.estimated ? 'estimated' : humanize(entry.eventKind)
                  }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="panel-empty">
          <strong>No usage</strong><span>No canonical rows matched this query.</span>
        </div>
        <footer v-if="nextPageToken || (error && entries.length)" class="panel-footer">
          <span v-if="error" class="form-error" role="alert">{{ error.message }}</span>
          <span v-else>{{ entries.length }} rows loaded; totals above are partial.</span>
          <button
            type="button"
            class="button button-secondary"
            :disabled="loadingMore"
            @click="load(false)"
          >
            {{ loadingMore ? 'Loading…' : 'Load next 200' }}
          </button>
        </footer>
      </section>
    </template>
  </div>
</template>
