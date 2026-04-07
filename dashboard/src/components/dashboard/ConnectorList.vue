<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  items: Array<{
    name: string
    status: string
    latency: string
  }>
}>()

const { t } = useI18n()
</script>

<template>
  <UCard class="rounded-xl">
    <template #header>
      <div>
        <h3 class="text-base font-semibold">{{ t('pages.overview.connector_health') }}</h3>
        <p class="text-sm text-muted">{{ t('pages.overview.connector_health_hint') }}</p>
      </div>
    </template>

    <div class="space-y-2">
      <div
        v-for="item in items"
        :key="item.name"
        class="flex items-center justify-between rounded-lg border border-default/60 px-3 py-2.5"
      >
        <div class="flex items-center gap-2.5">
          <div
            class="size-2 rounded-full"
            :class="{
              'bg-green-500': item.status === 'Healthy',
              'bg-amber-500': item.status === 'Warning',
              'bg-red-500': item.status !== 'Healthy' && item.status !== 'Warning',
            }"
          />
          <div class="min-w-0">
            <p class="font-medium text-sm">{{ item.name }}</p>
            <p class="text-xs text-muted">{{ item.latency }}</p>
          </div>
        </div>

        <UBadge
          :color="item.status === 'Healthy' ? 'success' : item.status === 'Warning' ? 'warning' : 'error'"
          variant="soft"
          size="xs"
          class="shrink-0 ml-2"
        >
          {{ item.status }}
        </UBadge>
      </div>
    </div>
  </UCard>
</template>
