<script setup lang="ts">
import { computed } from 'vue'
import MetricCard from '@/components/MetricCard.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusPanel from '@/components/StatusPanel.vue'
import UsageChart from '@/components/UsageChart.vue'
import { useResource } from '@/composables/useResource'
import { bucketUsage, rejectionRate, summarizeUsage } from '@/lib/analytics'
import { contextRevision, currentRange, currentScope, hasReportingScope } from '@/lib/context'
import {
  formatCompactInteger,
  formatNanoUsd,
  formatRelativeTime,
  humanize,
  shortId,
} from '@/lib/format'
import { kernel } from '@/lib/kernel'

const resource = useResource(async () => {
  if (!hasReportingScope.value) return undefined
  const range = currentRange()
  const scope = currentScope()
  const [usage, invocations] = await Promise.all([
    kernel.queryUsage(scope, range),
    kernel.queryInvocations(scope, range),
  ])
  return { range, usage, invocations }
}, [contextRevision])

const totals = computed(() => summarizeUsage(resource.data.value?.usage.items ?? []))
const deniedRate = computed(() => rejectionRate(resource.data.value?.invocations.items ?? []))
const buckets = computed(() =>
  resource.data.value
    ? bucketUsage(resource.data.value.usage.items, resource.data.value.range)
    : [],
)
const partial = computed(
  () =>
    Boolean(resource.data.value?.usage.nextPageToken) ||
    Boolean(resource.data.value?.invocations.nextPageToken),
)
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="Observe"
      title="Operations overview"
      description="Settled usage and admission decisions for one exact tenant and billing boundary."
    >
      <template #actions>
        <button
          type="button"
          class="button button-secondary"
          :disabled="resource.loading.value"
          @click="resource.reload"
        >
          Refresh
        </button>
      </template>
    </PageHeader>

    <StatusPanel
      v-if="!hasReportingScope"
      title="Reporting scope required"
      message="Set an opaque tenant and bill-to reference in the top bar before querying the usage ledger."
      tone="warning"
    />
    <StatusPanel
      v-else-if="resource.loading.value && !resource.data.value"
      title="Loading operations"
      message="Reading canonical usage and invocation records from Kave."
      busy
    />
    <StatusPanel
      v-else-if="resource.error.value"
      title="Overview unavailable"
      :message="resource.error.value.message"
      tone="error"
      @retry="resource.reload"
    >
      <template #action>Try again</template>
    </StatusPanel>

    <template v-else-if="resource.data.value">
      <div v-if="partial" class="notice notice-warning" role="status">
        Metrics cover the first 200 canonical rows. Open Analytics to page through the complete
        interval.
      </div>

      <section class="metric-grid" aria-label="Usage totals">
        <MetricCard
          label="Requests"
          :value="formatCompactInteger(totals.requests)"
          detail="settled provider calls"
        />
        <MetricCard
          label="Tokens"
          :value="formatCompactInteger(totals.inputTokens + totals.outputTokens)"
          :detail="`${formatCompactInteger(totals.inputTokens)} in · ${formatCompactInteger(totals.outputTokens)} out`"
        />
        <MetricCard
          label="Cost"
          :value="formatNanoUsd(totals.costNanoUsd)"
          :detail="
            totals.estimatedRows ? `${totals.estimatedRows} estimated row(s)` : 'provider reported'
          "
          :tone="totals.estimatedRows ? 'warning' : 'default'"
        />
        <MetricCard
          label="Rejected"
          :value="`${deniedRate.toFixed(1)}%`"
          :detail="`${resource.data.value.invocations.items.length} decisions loaded`"
          :tone="deniedRate > 10 ? 'danger' : deniedRate > 0 ? 'warning' : 'good'"
        />
      </section>

      <section class="panel chart-panel">
        <header class="panel-header">
          <div>
            <h2>Usage cadence</h2>
            <p>Settled requests across the selected interval</p>
          </div>
          <span class="live-indicator"><i /> Ledger</span>
        </header>
        <UsageChart :buckets="buckets" value="requests" />
      </section>

      <section class="panel">
        <header class="panel-header">
          <div>
            <h2>Recent invocations</h2>
            <p>Admission and settlement state, newest first</p>
          </div>
        </header>
        <div v-if="resource.data.value.invocations.items.length" class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Invocation</th>
                <th>Agent</th>
                <th>Model</th>
                <th>Decision</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="invocation in resource.data.value.invocations.items.slice(0, 12)"
                :key="invocation.id"
              >
                <td class="mono" :title="invocation.id">{{ shortId(invocation.id) }}</td>
                <td>{{ invocation.agent || '—' }}</td>
                <td class="mono">{{ invocation.model || '—' }}</td>
                <td>
                  <span class="status-badge" :class="humanize(String(invocation.decision))">{{
                    humanize(String(invocation.decision))
                  }}</span>
                </td>
                <td>{{ humanize(invocation.status) }}</td>
                <td :title="String(invocation.createdAtMs)">
                  {{ formatRelativeTime(invocation.createdAtMs) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="panel-empty">
          <strong>No invocations</strong>
          <span>No decisions matched this scope and interval.</span>
        </div>
      </section>
    </template>
  </div>
</template>
