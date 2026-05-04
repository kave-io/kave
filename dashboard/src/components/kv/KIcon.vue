<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ name: string; size?: number }>(), { size: 14 })

const ALIAS: Record<string, string> = {
  rotate: 'rotate-cw',
}

const lucide = computed(() => `i-lucide-${ALIAS[props.name] ?? props.name}`)
const sz = computed(() => props.size + 'px')
</script>

<template>
  <UIcon :name="lucide" :style="{ width: sz, height: sz, flexShrink: 0 }" />
</template>
