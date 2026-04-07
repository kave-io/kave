<script setup lang="ts">
withDefaults(defineProps<{
  label: string
  value?: string
  mono?: boolean
  large?: boolean
}>(), {
  mono: false,
  large: false,
})
</script>

<template>
  <div class="border border-default/60 rounded-lg px-3 py-2.5">
    <p class="text-xs text-muted">{{ label }}</p>
    <slot>
      <p :class="['mt-1', large ? 'text-lg font-semibold' : 'text-sm', mono ? 'font-mono' : '']">{{ value }}</p>
    </slot>
  </div>
</template>
