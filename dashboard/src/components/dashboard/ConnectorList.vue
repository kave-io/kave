<script setup lang="ts">
defineProps<{
  items: Array<{
    name: string
    status: string
    latency: string
  }>
}>()
</script>

<template>
  <UCard class="rounded-xl">
    <template #header>
      <div>
        <h3 class="text-base font-semibold">Connector health</h3>
        <p class="text-sm text-muted">Proxy, MCP, and telemetry surfaces.</p>
      </div>
    </template>

    <div class="space-y-3">
      <div
        v-for="item in items"
        :key="item.name"
        class="flex items-center justify-between rounded-lg border border-default p-3"
      >
        <div class="flex items-center gap-3">
          <div
            class="size-2 rounded-full"
            :class="{
              'bg-green-500': item.status === 'Healthy',
              'bg-amber-500': item.status === 'Warning',
              'bg-red-500': item.status !== 'Healthy' && item.status !== 'Warning',
            }"
          />
          <div>
            <p class="font-medium text-sm">{{ item.name }}</p>
            <p class="text-xs text-muted">{{ item.latency }}</p>
          </div>
        </div>

        <UBadge :color="item.status === 'Healthy' ? 'success' : 'warning'" variant="soft">
          {{ item.status }}
        </UBadge>
      </div>
    </div>
  </UCard>
</template>
