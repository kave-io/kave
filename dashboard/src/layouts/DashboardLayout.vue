<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute, RouterView } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useColorMode } from '@vueuse/core'
import { KIcon, KBtn, KBadge, KLogo } from '@/components/kv'
import { RTL_CODES } from '@/stores/locale'
import { useDaemonStatus } from '@/composables/api/useControl'

const router = useRouter()
const route = useRoute()
const colorMode = useColorMode()
const { locale, t } = useI18n()
void t
const daemonStatus = useDaemonStatus()

const collapsed = ref(false)
const isRtl = computed(() => RTL_CODES.includes(locale.value))

interface NavItem { id: string; label: string; icon: string; to: string }
interface NavGroup { group: string; items: NavItem[] }

const NAV: NavGroup[] = [
  { group: 'Observe', items: [
    { id: 'overview', label: 'Overview', icon: 'layout-dashboard', to: '/'         },
    { id: 'monitor',  label: 'Monitor',  icon: 'activity',         to: '/monitor'  },
    { id: 'runs',     label: 'Runs',     icon: 'file-text',        to: '/runs'     },
    { id: 'traces',   label: 'Traces',   icon: 'waypoints',        to: '/traces'   },
    { id: 'audit',    label: 'Audit',    icon: 'archive',          to: '/audit'    },
  ]},
  { group: 'Control', items: [
    { id: 'agents',     label: 'Agents',     icon: 'bot',   to: '/agents'     },
    { id: 'policies',   label: 'Policies',   icon: 'shield', to: '/policies'  },
    { id: 'connectors', label: 'Connectors', icon: 'plug',  to: '/connectors' },
  ]},
  { group: 'Spend', items: [
    { id: 'budgets', label: 'Budgets', icon: 'wallet', to: '/budgets' },
  ]},
  { group: 'System', items: [
    { id: 'settings', label: 'Settings', icon: 'settings', to: '/settings' },
  ]},
]

const isActive = (to: string) => to === '/' ? route.path === '/' : route.path.startsWith(to)

function toggleTheme() {
  colorMode.value = colorMode.value === 'dark' ? 'light' : 'dark'
}

const paletteOpen = ref(false)
function onKey(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); paletteOpen.value = !paletteOpen.value }
  if (e.key === 'Escape') paletteOpen.value = false
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

const palQ = ref('')
const palItems = computed(() => {
  const base = NAV.flatMap(g => g.items.map(it => ({ ...it, group: g.group, kind: 'page' as const })))
  const actions = [
    { id: 'create-agent',    label: 'Create agent',                icon: 'plus', group: 'Actions', kind: 'action' as const, to: '/agents'     },
    { id: 'create-policy',   label: 'Create policy',               icon: 'plus', group: 'Actions', kind: 'action' as const, to: '/policies'   },
    { id: 'issue-token',     label: 'Issue agent token',           icon: 'key',  group: 'Actions', kind: 'action' as const, to: '/agents'     },
    { id: 'copy-openai',     label: 'Copy OpenAI proxy URL',       icon: 'copy', group: 'Actions', kind: 'action' as const, to: '/connectors' },
    { id: 'copy-github',     label: 'Copy GitHub tool URL',        icon: 'copy', group: 'Actions', kind: 'action' as const, to: '/connectors' },
  ]
  const all = [...base, ...actions]
  const q = palQ.value.toLowerCase()
  return q ? all.filter(i => i.label.toLowerCase().includes(q)) : all
})
const palGroups = computed(() => Array.from(new Set(palItems.value.map(i => i.group))))

function palGo(item: { to: string }) {
  router.push(item.to)
  paletteOpen.value = false
}
</script>

<template>
  <div class="app-shell" :dir="isRtl ? 'rtl' : 'ltr'">
    <aside
      :class="['app-sidebar', collapsed ? 'is-collapsed' : '']"
      :style="{
        width: collapsed ? 'var(--sidebar-w-collapsed)' : 'var(--sidebar-w)',
        flexShrink: 0,
        background: 'transparent',
        display: 'flex',
        flexDirection: 'column',
        padding: '12px 10px',
        gap: '8px',
        transition: 'width 180ms',
      }"
    >
      <div class="app-brand" style="display: flex; align-items: center; gap: 10px; padding: 4px 8px 8px;">
        <KLogo :size="26" />
        <span v-if="!collapsed" style="font-weight: 600; font-size: 15px; letter-spacing: -0.01em;">Kave</span>
        <span v-if="!collapsed" style="margin-left: auto; font-size: 10.5px; color: var(--text-faint); font-family: var(--font-mono); text-transform: uppercase; letter-spacing: 0.05em;">v0.7.2</span>
      </div>

      <nav class="app-nav" style="display: flex; flex-direction: column; gap: 12px; flex: 1; margin-top: 4px;">
        <div v-for="g in NAV" :key="g.group" class="app-nav-group">
          <div v-if="!collapsed" class="sh" style="padding: 0 10px 6px; font-size: 10.5px;">{{ g.group }}</div>
          <div class="app-nav-items" style="display: flex; flex-direction: column; gap: 1px;">
            <button
              v-for="it in g.items"
              :key="it.id"
              :title="it.label"
              :style="{
                display: 'flex',
                alignItems: 'center',
                gap: '10px',
                padding: collapsed ? '8px' : '7px 10px',
                width: '100%',
                justifyContent: collapsed ? 'center' : 'flex-start',
                background: isActive(it.to) ? 'var(--surface)' : 'transparent',
                border: '1px solid ' + (isActive(it.to) ? 'var(--border)' : 'transparent'),
                color: isActive(it.to) ? 'var(--text)' : 'var(--text-muted)',
                borderRadius: '6px',
                fontSize: '13px',
                fontWeight: isActive(it.to) ? 500 : 400,
                cursor: 'pointer',
                textAlign: 'left',
                fontFamily: 'inherit',
                boxShadow: isActive(it.to) ? 'var(--shadow-sm)' : 'none',
              }"
              @click="router.push(it.to)"
            >
              <KIcon :name="it.icon" :size="15" :style="{ color: isActive(it.to) ? 'var(--accent)' : 'currentColor' }" />
              <span v-if="!collapsed">{{ it.label }}</span>
            </button>
          </div>
        </div>
      </nav>

      <div v-if="!collapsed" class="app-sidebar-links" style="padding: 10px; border-top: 1px solid var(--border-soft); display: flex; flex-direction: column; gap: 6px;">
        <a href="https://docs.kave.io" target="_blank" rel="noopener" style="display: flex; align-items: center; gap: 8px; padding: 5px 8px; border-radius: 4px; font-size: 12px; color: var(--text-dim); text-decoration: none;">
          <KIcon name="book" :size="13" />Docs<KIcon name="arrow-up-right" :size="11" :style="{ marginLeft: 'auto' }" />
        </a>
        <a href="https://github.com/kave-io/kave" target="_blank" rel="noopener" style="display: flex; align-items: center; gap: 8px; padding: 5px 8px; border-radius: 4px; font-size: 12px; color: var(--text-dim); text-decoration: none;">
          <KIcon name="github" :size="13" />GitHub<KIcon name="arrow-up-right" :size="11" :style="{ marginLeft: 'auto' }" />
        </a>
      </div>
    </aside>

    <main
      class="app-main"
      :style="{
        flex: 1,
        minWidth: 0,
        display: 'flex',
        flexDirection: 'column',
        margin: '8px 10px 10px 0',
        background: 'var(--surface)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-lg)',
        overflow: 'hidden',
        boxShadow: 'var(--shadow-sm)',
      }"
    >
      <header
        class="app-header"
        :style="{
          height: 'var(--header-h)',
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          gap: '10px',
          padding: '0 14px',
          borderBottom: '1px solid var(--border-soft)',
          background: 'var(--surface-2)',
        }"
      >
        <KBtn variant="ghost" size="sm" icon="panel-left" aria-label="Toggle sidebar" @click="collapsed = !collapsed" />

        <div style="display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--text-muted); padding: 4px 8px; background: var(--surface); border: 1px solid var(--border); border-radius: 5px;">
          <KIcon name="box" :size="13" :style="{ color: 'var(--text-dim)' }" />
          <span class="mono">default</span>
          <KIcon name="chevron-right" :size="11" :style="{ color: 'var(--text-faint)' }" />
          <span class="mono" style="color: var(--text);">dev</span>
        </div>

        <select class="input select" defaultValue="1h" :style="{ height: '26px', padding: '0 22px 0 8px', fontSize: '12px', width: 'auto', background: 'transparent', border: '1px solid var(--border)' }">
          <option value="15m">last 15m</option>
          <option value="1h">last 1h</option>
          <option value="24h">last 24h</option>
          <option value="7d">last 7d</option>
        </select>

        <button
          class="app-search"
          :style="{ display: 'flex', alignItems: 'center', gap: '8px', height: '28px', padding: '0 10px', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: '6px', color: 'var(--text-faint)', fontSize: '12px', cursor: 'pointer', minWidth: '220px', marginLeft: '8px', fontFamily: 'inherit' }"
          @click="paletteOpen = true"
        >
          <KIcon name="search" :size="13" />
          <span style="flex: 1; text-align: left;">Search runs, traces, agents…</span>
          <span class="kbd">⌘K</span>
        </button>

        <div class="app-header-actions" style="margin-left: auto; display: flex; align-items: center; gap: 8px;">
          <KBadge :tone="daemonStatus.isError.value ? 'danger' : 'success'" :dot="daemonStatus.isError.value ? null : 'live'">
            Daemon · {{ daemonStatus.isError.value ? 'Offline' : (daemonStatus.data.value?.version || 'Live') }}
          </KBadge>
          <KBtn variant="ghost" size="sm" :icon="colorMode === 'dark' ? 'sun' : 'moon'" aria-label="Toggle theme" @click="toggleTheme" />
        </div>
      </header>

      <div class="app-content" style="flex: 1; overflow: auto; min-height: 0; position: relative;">
        <RouterView />
      </div>
    </main>

    <!-- Command palette -->
    <template v-if="paletteOpen">
      <div class="drawer-overlay" :style="{ zIndex: 80 }" @click="paletteOpen = false" />
      <div role="dialog" class="command-palette" :style="{ position: 'fixed', top: '20%', left: '50%', transform: 'translateX(-50%)', width: '540px', background: 'var(--surface)', border: '1px solid var(--border-strong)', borderRadius: '12px', boxShadow: 'var(--shadow-lg)', zIndex: 81, overflow: 'hidden' }">
        <div style="display: flex; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--border-soft);">
          <KIcon name="search" :size="16" :style="{ color: 'var(--text-dim)' }" />
          <input v-model="palQ" autofocus placeholder="Search pages, runs, agents, actions…" :style="{ flex: 1, border: 0, background: 'transparent', fontSize: '14px', color: 'var(--text)', outline: 'none', fontFamily: 'inherit' }" />
          <span class="kbd">esc</span>
        </div>
        <div style="max-height: 360px; overflow: auto; padding: 6px 0;">
          <div v-if="palItems.length === 0" style="padding: 24px; text-align: center; color: var(--text-faint); font-size: 13px;">No results</div>
          <div v-for="g in palGroups" :key="g">
            <div class="sh" style="padding: 8px 14px 4px;">{{ g }}</div>
            <button
              v-for="i in palItems.filter(p => p.group === g)"
              :key="i.id"
              :style="{ display: 'flex', alignItems: 'center', gap: '10px', width: '100%', padding: '8px 14px', border: 0, background: 'transparent', cursor: 'pointer', fontSize: '13px', color: 'var(--text)', textAlign: 'left', fontFamily: 'inherit' }"
              @click="palGo(i)"
            >
              <KIcon :name="i.icon" :size="14" :style="{ color: 'var(--text-dim)' }" />
              <span>{{ i.label }}</span>
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
