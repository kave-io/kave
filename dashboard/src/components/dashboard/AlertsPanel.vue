<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  items: Array<{
    title: string
    description: string
    tone: 'success' | 'warning' | 'error'
  }>
}>()

const { t } = useI18n()
</script>

<template>
  <UCard class="rounded-xl">
    <template #header>
      <div>
        <h3 class="text-base font-semibold">{{ t('pages.overview.alerts') }}</h3>
        <p class="text-sm text-muted">{{ items.length === 0 ? t('pages.overview.alerts_clear') : t('pages.overview.alerts_active') }}</p>
      </div>
    </template>

    <div v-if="items.length === 0" class="grid h-24 place-items-center text-sm text-muted">
      {{ t('pages.overview.no_alerts') }}
    </div>
    <div v-else class="space-y-2">
      <div
        v-for="item in items"
        :key="item.title"
        class="rounded-lg border border-default/60 border-l-4 px-3 py-2.5"
        :class="{
          'border-l-amber-500 bg-amber-500/5': item.tone === 'warning',
          'border-l-red-500 bg-red-500/5': item.tone === 'error',
        }"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="font-medium text-sm">{{ item.title }}</p>
            <p class="mt-0.5 text-sm text-muted">{{ item.description }}</p>
          </div>
          <UBadge
            :color="item.tone"
            variant="soft"
            :ui="{ base: 'shrink-0' }"
            class="shrink-0 text-xs"
          >
            {{ item.tone }}
          </UBadge>
        </div>
      </div>
    </div>
  </UCard>
</template>
