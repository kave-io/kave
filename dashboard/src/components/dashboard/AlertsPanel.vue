<script setup lang="ts">
defineProps<{
  items: Array<{
    title: string
    description: string
    tone: 'success' | 'warning' | 'error'
  }>
}>()
</script>

<template>
  <UCard class="rounded-xl">
    <template #header>
      <div>
        <h3 class="text-base font-semibold">Alerts & recommendations</h3>
        <p class="text-sm text-muted">Budget, policy, and routing signals.</p>
      </div>
    </template>

    <div class="space-y-3">
      <div
        v-for="item in items"
        :key="item.title"
        class="rounded-lg border border-default border-l-4 p-4"
        :class="{
          'border-l-green-500': item.tone === 'success',
          'border-l-amber-500': item.tone === 'warning',
          'border-l-red-500': item.tone === 'error',
        }"
      >
        <div class="flex items-start justify-between gap-3">
          <div>
            <p class="font-medium text-sm">{{ item.title }}</p>
            <p class="mt-0.5 text-sm text-muted">{{ item.description }}</p>
          </div>
          <UBadge :color="item.tone" variant="soft" class="shrink-0">{{ item.tone }}</UBadge>
        </div>
      </div>
    </div>
  </UCard>
</template>
