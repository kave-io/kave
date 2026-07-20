<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusPanel from '@/components/StatusPanel.vue'
import { useResource } from '@/composables/useResource'
import { authState, setNamespaceId } from '@/lib/auth'
import { formatCompactInteger, humanize } from '@/lib/format'
import { KernelError, kernel } from '@/lib/kernel'
import type { LimitSelector } from '@/lib/types'

const namespaceDraft = ref(authState.namespaceId.value)
const namespaceError = ref('')
const resource = useResource(
  async () =>
    authState.namespaceId.value ? kernel.getState(authState.namespaceId.value) : undefined,
  [() => authState.namespaceId.value],
)

const permissionDenied = computed(
  () => resource.error.value instanceof KernelError && resource.error.value.permissionDenied,
)

function loadNamespace(): void {
  namespaceError.value = ''
  try {
    setNamespaceId(namespaceDraft.value)
    void resource.reload()
  } catch (error) {
    namespaceError.value = error instanceof Error ? error.message : 'The namespace ID is invalid.'
  }
}

function selectorLabel(selector?: LimitSelector): string {
  if (!selector) return 'namespace-wide'
  const dimensions = Object.entries(selector)
    .filter(([, value]) => value)
    .map(([key, value]) => `${key}=${value}`)
  return dimensions.length ? dimensions.join(' · ') : 'namespace-wide'
}
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="Configure"
      title="Namespace"
      description="Read-only inspection of the active manifest. Sensitive credential values are never returned or rendered."
    />

    <form
      v-if="!authState.namespaceId.value"
      class="inline-prompt panel"
      @submit.prevent="loadNamespace"
    >
      <div>
        <h2>Namespace ID required</h2>
        <p>
          A service key is namespace-bound, but GetState still requires the explicit namespace ID.
        </p>
      </div>
      <label>
        <span class="sr-only">Namespace ID</span>
        <input v-model.trim="namespaceDraft" required autocomplete="off" placeholder="nsp_…" />
      </label>
      <p v-if="namespaceError" class="form-error" role="alert">{{ namespaceError }}</p>
      <button type="submit" class="button button-primary">Inspect</button>
    </form>

    <StatusPanel
      v-else-if="resource.loading.value && !resource.data.value"
      title="Loading namespace"
      message="Reading the current declarative revision."
      busy
    />
    <StatusPanel
      v-else-if="permissionDenied"
      title="Configuration access not granted"
      message="This service key can use reporting but cannot read the manifest. Reconnect with a key that has config.apply only when configuration inspection is required."
      tone="warning"
    />
    <StatusPanel
      v-else-if="resource.error.value"
      title="Namespace unavailable"
      :message="resource.error.value.message"
      tone="error"
      @retry="resource.reload"
    >
      <template #action>Try again</template>
    </StatusPanel>

    <template v-else-if="resource.data.value?.manifest">
      <section class="namespace-banner">
        <div>
          <p class="eyebrow">Active manifest</p>
          <h2>
            {{ resource.data.value.manifest.namespace?.application || 'application' }}
            <span
              >/ {{ resource.data.value.manifest.namespace?.environment || 'environment' }}</span
            >
          </h2>
          <p class="mono">{{ resource.data.value.namespaceId }}</p>
        </div>
        <div class="revision-block">
          <span>Revision</span>
          <strong class="tnum">{{ formatCompactInteger(resource.data.value.revision) }}</strong>
        </div>
      </section>

      <section class="manifest-counts" aria-label="Manifest resource counts">
        <div>
          <strong>{{ resource.data.value.manifest.agents.length }}</strong
          ><span>Agents</span>
        </div>
        <div>
          <strong>{{ resource.data.value.manifest.routes.length }}</strong
          ><span>Routes</span>
        </div>
        <div>
          <strong>{{ resource.data.value.manifest.limits.length }}</strong
          ><span>Limits</span>
        </div>
      </section>

      <section class="panel">
        <header class="panel-header">
          <div>
            <h2>Agents</h2>
            <p>Static workload identities in this revision</p>
          </div>
        </header>
        <div v-if="resource.data.value.manifest.agents.length" class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Kind</th>
                <th>Route</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="agent in resource.data.value.manifest.agents" :key="agent.name">
                <td>
                  <strong class="mono">{{ agent.name }}</strong>
                </td>
                <td>{{ humanize(String(agent.kind)) }}</td>
                <td class="mono">{{ agent.route }}</td>
                <td>
                  <span class="status-badge" :class="agent.enabled ? 'active' : 'disabled'">{{
                    agent.enabled ? 'enabled' : 'disabled'
                  }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="panel-empty">
          <strong>No agents</strong><span>The active manifest defines no agents.</span>
        </div>
      </section>

      <section class="panel">
        <header class="panel-header">
          <div>
            <h2>Provider routes</h2>
            <p>
              Safe route metadata; base URLs and secret references are withheld from this display
            </p>
          </div>
        </header>
        <div v-if="resource.data.value.manifest.routes.length" class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Provider</th>
                <th>Default model</th>
                <th>Allowed models</th>
                <th>Price revision</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="route in resource.data.value.manifest.routes" :key="route.name">
                <td>
                  <strong class="mono">{{ route.name }}</strong>
                </td>
                <td>{{ route.provider }}</td>
                <td class="mono">{{ route.defaultModel || '—' }}</td>
                <td>{{ route.allowedModels.length }}</td>
                <td class="tnum">{{ formatCompactInteger(route.pricingRevision) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="panel-empty">
          <strong>No routes</strong><span>The active manifest defines no provider routes.</span>
        </div>
      </section>

      <section class="panel">
        <header class="panel-header">
          <div>
            <h2>Limits</h2>
            <p>Declarative admission controls in this revision</p>
          </div>
        </header>
        <div v-if="resource.data.value.manifest.limits.length" class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Key</th>
                <th>Metric</th>
                <th>Selector</th>
                <th>Window</th>
                <th>Soft / hard cap</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="limit in resource.data.value.manifest.limits" :key="limit.key">
                <td>
                  <strong class="mono">{{ limit.key }}</strong>
                </td>
                <td>{{ humanize(limit.metric) }}</td>
                <td class="mono selector-cell">{{ selectorLabel(limit.selector) }}</td>
                <td>{{ humanize(String(limit.window)) }}</td>
                <td class="tnum">
                  {{ limit.softCap === undefined ? '—' : formatCompactInteger(limit.softCap) }} /
                  {{ formatCompactInteger(limit.hardCap) }}
                </td>
                <td>
                  <span class="status-badge" :class="limit.enabled ? 'active' : 'disabled'">{{
                    limit.enabled ? 'active' : 'disabled'
                  }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="panel-empty">
          <strong>No limits</strong><span>The active manifest defines no admission limits.</span>
        </div>
      </section>
    </template>
  </div>
</template>
