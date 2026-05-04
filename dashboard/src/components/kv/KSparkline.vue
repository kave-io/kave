<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ data: number[]; color?: string; height?: number }>(), {
  color: 'var(--accent)',
  height: 28,
})

const w = 100
const h = computed(() => props.height)

const points = computed(() => {
  const max = Math.max(...props.data, 1)
  const min = Math.min(...props.data, 0)
  const range = max - min || 1
  return props.data.map((v, i) => `${(i / (props.data.length - 1)) * w},${h.value - ((v - min) / range) * (h.value - 4) - 2}`).join(' ')
})

const area = computed(() => `0,${h.value} ${points.value} ${w},${h.value}`)
</script>

<template>
  <svg class="spark" :viewBox="`0 0 ${w} ${h}`" preserveAspectRatio="none">
    <polygon :points="area" :fill="color" opacity="0.12" />
    <polyline :points="points" fill="none" :stroke="color" stroke-width="1.4" vector-effect="non-scaling-stroke" />
  </svg>
</template>
