<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    value: string
    detail?: string
    tone?: 'default' | 'good' | 'warning' | 'danger'
  }>(),
  { detail: '', tone: 'default' },
)
</script>

<template>
  <article class="metric-card" :class="`metric-${tone}`">
    <span class="metric-label">{{ label }}</span>
    <strong class="metric-value tnum">{{ value }}</strong>
    <span v-if="detail" class="metric-detail">{{ detail }}</span>
  </article>
</template>
