<script setup lang="ts">
withDefaults(defineProps<{ k: string; mono?: boolean }>(), { mono: false })
</script>

<template>
  <div class="kv">
    <div class="k">{{ k }}</div>
    <div :class="['v', mono ? 'mono' : '']" style="display: flex; align-items: center; justify-content: space-between; gap: 8px;">
      <span style="overflow: hidden; text-overflow: ellipsis;"><slot /></span>
      <slot name="action" />
    </div>
  </div>
</template>
