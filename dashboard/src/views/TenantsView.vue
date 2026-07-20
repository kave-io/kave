<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusPanel from '@/components/StatusPanel.vue'
import { consoleContext, currentRange, selectTenant } from '@/lib/context'
import { formatCompactInteger, formatNanoUsd, formatRelativeTime, humanize } from '@/lib/format'
import { KernelError, kernel } from '@/lib/kernel'
import type { TenantSummary, TimeRange } from '@/lib/types'

const search = ref('')
const tenants = ref<TenantSummary[]>([])
const nextPageToken = ref('')
const loadedRange = ref<TimeRange>()
const loading = ref(false)
const loadingMore = ref(false)
const error = ref<Error>()
let generation = 0

async function load(reset = true): Promise<void> {
  const current = ++generation
  if (reset) {
    tenants.value = []
    nextPageToken.value = ''
    loadedRange.value = undefined
    loadingMore.value = false
  }
  if (reset) loading.value = true
  else loadingMore.value = true
  error.value = undefined
  try {
    const range = reset || !loadedRange.value ? currentRange() : loadedRange.value
    const page = await kernel.listTenants(range, {
      pageToken: reset ? undefined : nextPageToken.value,
    })
    if (current !== generation) return
    tenants.value = reset ? page.tenants : [...tenants.value, ...page.tenants]
    nextPageToken.value = page.nextPageToken
    loadedRange.value = range
  } catch (caught) {
    if (current === generation) {
      error.value =
        caught instanceof Error ? caught : new Error('Tenant directory could not be loaded.')
    }
  } finally {
    if (current === generation) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

onMounted(() => void load())
watch(
  () => consoleContext.range,
  () => void load(),
)

const unavailable = computed(() => error.value instanceof KernelError && error.value.unimplemented)
const filteredTenants = computed(() => {
  const query = search.value.trim().toLowerCase()
  return query
    ? tenants.value.filter(
        (tenant) =>
          tenant.tenant.toLowerCase().includes(query) ||
          tenant.billTo?.toLowerCase().includes(query),
      )
    : tenants.value
})
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="Manage"
      title="Tenant scopes"
      description="Opaque tenant boundaries observed inside this namespace; no user profiles or identity data."
    >
      <template #actions>
        <button type="button" class="button button-secondary" :disabled="loading" @click="load()">
          Refresh
        </button>
      </template>
    </PageHeader>

    <div class="notice notice-security">
      Tenant references are asserted by your application. Kave exposes only operational aggregates
      and admission state; it does not resolve references to people.
    </div>

    <section v-if="tenants.length" class="filter-bar tenant-filter">
      <label>
        <span>Find tenant</span>
        <input
          v-model.trim="search"
          type="search"
          autocomplete="off"
          placeholder="Opaque tenant reference"
        />
      </label>
      <span class="filter-count">{{ filteredTenants.length }} of {{ tenants.length }} loaded</span>
    </section>

    <StatusPanel
      v-if="loading && !tenants.length"
      title="Loading tenant directory"
      message="Reading pseudonymous tenant summaries from this namespace."
      busy
    />
    <StatusPanel
      v-else-if="unavailable"
      title="Tenant directory not enabled"
      message="This Kave server does not yet expose the V2 ListTenants reporting method. You can still set an exact scope in the top bar and use Overview or Analytics."
      tone="warning"
    />
    <StatusPanel
      v-else-if="error && !tenants.length"
      title="Tenant directory unavailable"
      :message="error.message"
      tone="error"
      @retry="load()"
    >
      <template #action>Try again</template>
    </StatusPanel>

    <section v-else class="panel">
      <header class="panel-header">
        <div>
          <h2>Directory</h2>
          <p>Activity in the selected interval, sorted by the server</p>
        </div>
        <span v-if="nextPageToken" class="status-badge warning">Partial</span>
      </header>
      <div v-if="filteredTenants.length" class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Tenant reference</th>
              <th>Status</th>
              <th>Last observed</th>
              <th>Invocations</th>
              <th>Requests</th>
              <th>Cost</th>
              <th>Limits</th>
              <th><span class="sr-only">Action</span></th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="tenant in filteredTenants"
              :key="`${tenant.tenant}\u0000${tenant.billTo ?? ''}`"
            >
              <td>
                <strong class="mono">{{ tenant.tenant }}</strong
                ><small v-if="tenant.billTo" class="table-sub mono"
                  >bill-to: {{ tenant.billTo }}</small
                >
              </td>
              <td>
                <span class="status-badge" :class="tenant.status">{{
                  humanize(tenant.status)
                }}</span>
              </td>
              <td>{{ formatRelativeTime(tenant.lastSeenAtMs) }}</td>
              <td class="tnum">{{ formatCompactInteger(tenant.invocationCount) }}</td>
              <td class="tnum">
                {{
                  tenant.requestCount === undefined
                    ? '—'
                    : formatCompactInteger(tenant.requestCount)
                }}
              </td>
              <td class="tnum">
                {{ tenant.costNanoUsd === undefined ? '—' : formatNanoUsd(tenant.costNanoUsd) }}
              </td>
              <td class="tnum">{{ tenant.activeLimits ?? '—' }}</td>
              <td class="table-action">
                <button
                  type="button"
                  class="button button-secondary button-small"
                  :title="
                    tenant.billTo
                      ? 'Use this exact tenant and bill-to reporting scope'
                      : 'Select this tenant, then supply its exact bill-to reference in the top bar'
                  "
                  @click="selectTenant(tenant.tenant, tenant.billTo)"
                >
                  {{ tenant.billTo ? 'Inspect' : 'Set bill-to' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="panel-empty">
        <strong>{{ search ? 'No matching tenant' : 'No observed tenants' }}</strong>
        <span>{{
          search
            ? 'Change the opaque-reference filter.'
            : 'No tenant activity was reported in this interval.'
        }}</span>
      </div>
      <footer v-if="nextPageToken || (error && tenants.length)" class="panel-footer">
        <span v-if="error" class="form-error" role="alert">{{ error.message }}</span>
        <span v-else>{{ tenants.length }} tenants loaded; directory is partial.</span>
        <button
          v-if="nextPageToken"
          type="button"
          class="button button-secondary"
          :disabled="loadingMore"
          @click="load(false)"
        >
          {{ loadingMore ? 'Loading…' : 'Load next 200' }}
        </button>
      </footer>
    </section>
  </div>
</template>
