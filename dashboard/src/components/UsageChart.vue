<script setup lang="ts">
import { computed } from 'vue'
import type { UsageBucket } from '@/lib/analytics'

const props = defineProps<{
  buckets: UsageBucket[]
  value: 'requests' | 'costNanoUsd'
}>()

const points = computed(() => {
  const values = props.buckets.map((bucket) => bucket[props.value])
  const maximum = values.reduce((max, value) => (value > max ? value : max), 0n)
  if (maximum === 0n || values.length === 0) return ''
  return values
    .map((value, index) => {
      const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100
      const ratio = Number((value * 1000n) / maximum) / 1000
      const y = 92 - ratio * 80
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
})

const hasValues = computed(() => props.buckets.some((bucket) => bucket[props.value] > 0n))
const accessibleLabel = computed(
  () => `${props.value === 'requests' ? 'Requests' : 'Cost'} over the selected reporting interval`,
)
</script>

<template>
  <div class="usage-chart" role="img" :aria-label="accessibleLabel">
    <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
      <line x1="0" y1="92" x2="100" y2="92" class="chart-axis" />
      <line x1="0" y1="52" x2="100" y2="52" class="chart-grid" />
      <line x1="0" y1="12" x2="100" y2="12" class="chart-grid" />
      <polyline v-if="hasValues" :points="points" class="chart-line" />
    </svg>
    <span v-if="!hasValues" class="chart-empty">No measured usage in this interval</span>
  </div>
</template>
