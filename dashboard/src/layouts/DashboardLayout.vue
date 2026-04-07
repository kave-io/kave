<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DropdownMenuItem, NavigationMenuItem } from '@nuxt/ui'
import LocaleSelector from '../components/LocaleSelector.vue'
import CurrencySelector from '../components/CurrencySelector.vue'
import { RTL_CODES } from '@/stores/locale'

const { locale, t } = useI18n()
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

const navItems = computed<NavigationMenuItem[][]>(() => [
  [
    { label: t('nav.overview'), icon: 'i-lucide-layout-dashboard', to: '/', exact: true },
    { label: t('nav.traces'), icon: 'i-lucide-waypoints', to: '/traces' },
    { label: t('nav.agents'), icon: 'i-lucide-bot', to: '/agents' },
    { label: t('nav.policies'), icon: 'i-lucide-shield', to: '/policies' },
    { label: t('nav.runs'), icon: 'i-lucide-activity', to: '/runs' },
    { label: t('nav.settings'), icon: 'i-lucide-settings', to: '/settings' },
  ],
])

const externalLinks = [
  { label: 'Docs', icon: 'i-lucide-book-open', url: 'https://docs.kave.io' },
  { label: 'GitHub', icon: 'i-lucide-github', url: 'https://github.com/kave-io/kave' },
  { label: 'Discord', icon: 'i-lucide-send', url: 'https://discord.gg/kave' },
]
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
        <div class="flex items-center gap-2 w-full px-1.5 py-1.5">
          <img src="/icon-128.png" alt="Kave" class="size-6 rounded" />
          <span class="text-sm font-semibold truncate">Kave</span>
        </div>
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

          <!-- External Links Section -->
          <div v-if="state === 'expanded'" class="mt-auto border-t border-default p-3 space-y-2">
            <p class="text-xs font-semibold text-muted uppercase tracking-wide px-1">Resources</p>
            <div class="flex flex-col gap-1.5">
              <a
                v-for="link in externalLinks"
                :key="link.url"
                :href="link.url"
                target="_blank"
                rel="noopener noreferrer"
                class="flex items-center gap-2 px-2.5 py-2 rounded-lg text-sm hover:bg-default/60 transition group"
              >
                <UIcon :name="link.icon" class="size-4 shrink-0 text-muted group-hover:text-foreground" />
                <span class="text-muted group-hover:text-foreground">{{ link.label }}</span>
                <UIcon name="i-lucide-arrow-up-right" class="size-3 ml-auto shrink-0 opacity-0 group-hover:opacity-100 transition" />
              </a>
            </div>
          </div>

          <!-- Collapsed External Links -->
          <div v-else class="mt-auto border-t border-default p-2 flex flex-col gap-1.5 items-center">
            <a
              v-for="link in externalLinks"
              :key="link.url"
              :href="link.url"
              target="_blank"
              rel="noopener noreferrer"
              class="p-2 rounded-lg hover:bg-default/60 transition text-muted hover:text-foreground"
              :title="link.label"
            >
              <UIcon :name="link.icon" class="size-4" />
            </a>
          </div>

          <!-- Settings Section -->
          <div class="border-t border-default p-3">
            <div v-if="state === 'expanded'" class="space-y-3">
              <p class="text-xs font-semibold text-muted uppercase tracking-wide px-1">Preferences</p>
              <div class="space-y-2">
                <div class="flex items-center justify-between px-2.5 py-2 rounded-lg bg-default/40 hover:bg-default/60 transition">
                  <span class="text-xs text-muted flex items-center gap-2">
                    <UIcon name="i-lucide-palette" class="size-4" />
                    Theme
                  </span>
                  <UColorModeSelect variant="ghost" size="sm" />
                </div>
                <div class="flex items-center justify-between px-2.5 py-2 rounded-lg bg-default/40 hover:bg-default/60 transition">
                  <span class="text-xs text-muted flex items-center gap-2">
                    <UIcon name="i-lucide-globe" class="size-4" />
                    Language
                  </span>
                  <LocaleSelector />
                </div>
                <div class="flex items-center justify-between px-2.5 py-2 rounded-lg bg-default/40 hover:bg-default/60 transition">
                  <span class="text-xs text-muted flex items-center gap-2">
                    <UIcon name="i-lucide-dollar-sign" class="size-4" />
                    Currency
                  </span>
                  <CurrencySelector />
                </div>
              </div>
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
        class="h-(--ui-header-height) shrink-0 flex items-center border-b border-default px-4 lg:px-6"
      >
        <UButton
          :icon="toggleIcon"
          color="neutral"
          variant="ghost"
          aria-label="Toggle sidebar"
          @click="open = !open"
        />
      </header>

      <div class="flex-1 overflow-auto scrollbar-hide">
        <RouterView />
      </div>
    </div>
  </div>
</template>
