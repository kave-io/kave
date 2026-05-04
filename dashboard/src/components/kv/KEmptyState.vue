<script setup lang="ts">
import KIcon from './KIcon.vue'

withDefaults(defineProps<{ icon?: string; title: string }>(), { icon: 'box' })
</script>

<template>
  <div class="empty">
    <div class="icon"><KIcon :name="icon" :size="20" /></div>
    <h4>{{ title }}</h4>
    <p><slot /></p>
    <slot name="action" />
  </div>
</template>
