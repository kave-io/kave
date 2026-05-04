<script setup lang="ts">
import KIcon from './KIcon.vue'

withDefaults(defineProps<{
  tone?: 'success' | 'info' | 'danger' | 'warning' | 'neutral' | 'accent'
  dot?: 'live' | 'plain' | null
  icon?: string
}>(), { tone: 'neutral', dot: null })
</script>

<template>
  <span :class="['badge', tone]">
    <span v-if="dot" :class="['dot', dot === 'live' ? 'live' : '']" :style="dot === 'live' ? undefined : { background: 'currentColor' }" />
    <KIcon v-if="icon" :name="icon" :size="11" />
    <slot />
  </span>
</template>
