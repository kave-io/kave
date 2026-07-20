<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import ConsoleLogo from '@/components/ConsoleLogo.vue'
import ScopeDialog from '@/components/ScopeDialog.vue'
import { authState, disconnectCredential } from '@/lib/auth'
import {
  consoleContext,
  hasReportingScope,
  RANGE_OPTIONS,
  setRange,
  type RangeKey,
} from '@/lib/context'
import { shortId } from '@/lib/format'

const route = useRoute()
const mobileNavigationOpen = ref(false)
const scopeDialogOpen = ref(false)
const narrowLayout = ref(false)
let narrowQuery: MediaQueryList | undefined

const navigation = [
  { to: '/', label: 'Overview', mark: 'O' },
  { to: '/analytics', label: 'Analytics', mark: 'A' },
  { to: '/tenants', label: 'Tenants', mark: 'T' },
  { to: '/namespace', label: 'Namespace', mark: 'N' },
  { to: '/audit', label: 'Audit', mark: 'L' },
]

const scopeLabel = computed(() =>
  hasReportingScope.value
    ? `${shortId(consoleContext.tenant, 14)} / ${shortId(consoleContext.billTo, 10)}`
    : 'Set scope',
)

function active(to: string): boolean {
  return to === '/' ? route.path === '/' : route.path.startsWith(to)
}

function onRangeChange(event: Event): void {
  setRange((event.target as HTMLSelectElement).value as RangeKey)
}

function onGlobalKey(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    mobileNavigationOpen.value = false
    scopeDialogOpen.value = false
  }
}

function syncLayout(event?: MediaQueryListEvent): void {
  narrowLayout.value = event?.matches ?? narrowQuery?.matches ?? false
  if (!narrowLayout.value) mobileNavigationOpen.value = false
}

onMounted(() => {
  window.addEventListener('keydown', onGlobalKey)
  narrowQuery = window.matchMedia('(max-width: 760px)')
  syncLayout()
  narrowQuery.addEventListener('change', syncLayout)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKey)
  narrowQuery?.removeEventListener('change', syncLayout)
})
</script>

<template>
  <div class="console-shell">
    <a class="skip-link" href="#main-content">Skip to content</a>
    <aside
      class="sidebar"
      :class="{ 'sidebar-open': mobileNavigationOpen }"
      :aria-hidden="narrowLayout && !mobileNavigationOpen ? 'true' : undefined"
      :inert="narrowLayout && !mobileNavigationOpen ? true : undefined"
    >
      <div class="brand-row">
        <ConsoleLogo />
        <div class="brand-copy">
          <strong>Kave</strong>
          <span>console</span>
        </div>
        <span class="version-pill">V2</span>
      </div>

      <nav aria-label="Primary navigation">
        <RouterLink
          v-for="item in navigation"
          :key="item.to"
          :to="item.to"
          class="nav-link"
          :class="{ active: active(item.to) }"
          @click="mobileNavigationOpen = false"
        >
          <span class="nav-mark" aria-hidden="true">{{ item.mark }}</span>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>

      <div class="sidebar-security">
        <span class="security-dot" />
        <div>
          <strong>Namespace-bound</strong>
          <span>{{
            authState.rememberedForTab.value ? 'Key kept for this tab' : 'Key held in memory'
          }}</span>
        </div>
      </div>
    </aside>

    <section class="console-main">
      <header class="topbar">
        <button
          type="button"
          class="icon-button mobile-menu-button"
          :aria-expanded="mobileNavigationOpen"
          aria-label="Toggle navigation"
          @click="mobileNavigationOpen = !mobileNavigationOpen"
        >
          ☰
        </button>
        <button type="button" class="scope-button" @click="scopeDialogOpen = true">
          <span class="scope-indicator" :class="{ ready: hasReportingScope }" />
          <span>
            <small>Tenant / bill-to</small>
            <strong>{{ scopeLabel }}</strong>
          </span>
        </button>
        <label class="range-control">
          <span class="sr-only">Time range</span>
          <select :value="consoleContext.range" @change="onRangeChange">
            <option v-for="option in RANGE_OPTIONS" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>
        <div
          class="namespace-chip"
          :title="authState.namespaceId.value || 'No namespace ID supplied'"
        >
          <small>Namespace</small>
          <strong>{{ shortId(authState.namespaceId.value, 12) }}</strong>
        </div>
        <button
          type="button"
          class="button button-secondary disconnect-button"
          @click="disconnectCredential"
        >
          Disconnect
        </button>
      </header>

      <main id="main-content" class="content" tabindex="-1">
        <RouterView />
      </main>
    </section>

    <button
      v-if="mobileNavigationOpen"
      type="button"
      class="navigation-backdrop"
      aria-label="Close navigation"
      @click="mobileNavigationOpen = false"
    />
    <ScopeDialog :open="scopeDialogOpen" @close="scopeDialogOpen = false" />
  </div>
</template>
