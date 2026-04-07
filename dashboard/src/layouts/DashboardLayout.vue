<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { DropdownMenuItem, NavigationMenuItem } from '@nuxt/ui'
import LocaleSelector from '../components/LocaleSelector.vue'
import CurrencySelector from '../components/CurrencySelector.vue'
import { RTL_CODES } from '@/stores/locale'

const { locale, t } = useI18n()
const route = useRoute()
const open = ref(true)

const isRtl = computed(() => RTL_CODES.includes(locale.value))
const sidebarSide = computed(() => (isRtl.value ? 'right' : 'left'))
const toggleIcon = computed(() => (isRtl.value ? 'i-lucide-panel-right' : 'i-lucide-panel-left'))

const workspaceKeys = ['production', 'staging', 'development'] as const
const workspaceIcons: Record<string, string> = {
  production: 'i-lucide-globe',
  staging: 'i-lucide-flask-conical',
  development: 'i-lucide-code',
}
const selectedWorkspaceKey = ref<string>('production')

const workspaceItems = computed<DropdownMenuItem[][]>(() => [
  workspaceKeys.map((key) => ({
    label: t(`workspace.${key}`),
    icon: workspaceIcons[key],
    onSelect() {
      selectedWorkspaceKey.value = key
    },
  })),
  [{ label: t('workspace.add'), icon: 'i-lucide-circle-plus' }],
])

const selectedWorkspace = computed(() => ({
  label: t(`workspace.${selectedWorkspaceKey.value}`),
  icon: workspaceIcons[selectedWorkspaceKey.value],
}))

const navItems = computed<NavigationMenuItem[]>(() => [
  { label: t('nav.overview'), icon: 'i-lucide-layout-dashboard', to: '/' },
  { label: t('nav.traces'), icon: 'i-lucide-waypoints', to: '/traces' },
  { label: t('nav.agents'), icon: 'i-lucide-bot', to: '/agents' },
  { label: t('nav.policies'), icon: 'i-lucide-shield', to: '/policies' },
  { label: t('nav.budgets'), icon: 'i-lucide-wallet-cards', to: '/budgets' },
  { label: t('nav.connectors'), icon: 'i-lucide-plug-zap', to: '/connectors' },
  {
    label: t('nav.settings'),
    icon: 'i-lucide-settings',
    defaultOpen: route.path.startsWith('/settings'),
    children: [
      { label: t('nav.settings_general'), icon: 'i-lucide-house', to: '/settings' },
      { label: t('nav.settings_team'), icon: 'i-lucide-users', to: '/settings/team' },
      { label: t('nav.settings_api_keys'), icon: 'i-lucide-key', to: '/settings/api-keys' },
      { label: t('nav.settings_billing'), icon: 'i-lucide-credit-card', to: '/settings/billing' },
    ],
  },
])
</script>

<template>
  <div class="flex h-full">
    <USidebar
      v-model:open="open"
      variant="inset"
      collapsible="icon"
      :side="sidebarSide"
      :ui="{ container: 'h-full' }"
    >
      <template #header>
        <UDropdownMenu
          :items="workspaceItems"
          :content="{ align: 'start', collisionPadding: 12 }"
          :ui="{ content: 'w-(--reka-dropdown-menu-trigger-width) min-w-48' }"
        >
          <UButton
            :icon="selectedWorkspace.icon"
            :label="selectedWorkspace.label"
            trailing-icon="i-lucide-chevrons-up-down"
            color="neutral"
            variant="ghost"
            square
            class="w-full data-[state=open]:bg-elevated overflow-hidden"
            :ui="{ trailingIcon: 'text-dimmed ms-auto' }"
          />
        </UDropdownMenu>
        <UButton
          icon="i-lucide-x"
          color="neutral"
          variant="ghost"
          square
          class="sm:hidden"
          @click="open = false"
        />
      </template>

      <template #default="{ state }">
        <div class="flex flex-col h-full">
          <UNavigationMenu
            :items="navItems"
            :collapsed="state === 'collapsed'"
            tooltip
            popover
            orientation="vertical"
            :ui="{ link: 'p-1.5 overflow-hidden' }"
          />
          <div class="mt-auto flex flex-col gap-2 border-t border-default p-3">
            <div v-if="state === 'expanded'" class="flex flex-col gap-2">
              <UColorModeSelect variant="ghost" />
              <LocaleSelector />
              <CurrencySelector />
            </div>
            <div v-else class="flex flex-col gap-2 items-center">
              <UColorModeButton variant="ghost" />
            </div>
          </div>
        </div>
      </template>
    </USidebar>

    <div
      class="flex-1 flex flex-col overflow-hidden peer-data-[variant=inset]:m-4 lg:peer-data-[variant=inset]:not-peer-data-[collapsible=offcanvas]:ms-0 peer-data-[variant=inset]:rounded-xl peer-data-[variant=inset]:shadow-sm peer-data-[variant=inset]:ring peer-data-[variant=inset]:ring-default bg-default"
    >
      <header
        class="h-(--ui-header-height) shrink-0 flex items-center justify-between border-b border-default px-4 lg:px-6"
      >
        <UButton
          :icon="toggleIcon"
          color="neutral"
          variant="ghost"
          aria-label="Toggle sidebar"
          @click="open = !open"
        />
        <div class="flex items-center gap-2">
          <UInput icon="i-lucide-search" placeholder="Search..." class="w-64" />
          <UButton icon="i-lucide-plus">New agent</UButton>
        </div>
      </header>

      <div class="flex-1 overflow-auto scrollbar-hide">
        <RouterView />
      </div>
    </div>
  </div>
</template>
