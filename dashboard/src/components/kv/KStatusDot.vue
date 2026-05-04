<script setup lang="ts">
const props = defineProps<{ status: string }>()

const COLORS: Record<string, string> = {
  completed: 'var(--success)',
  running:   'var(--secondary)',
  failed:    'var(--danger)',
  blocked:   'var(--warning)',
  active:    'var(--success)',
  paused:    'var(--text-faint)',
  enabled:   'var(--success)',
  available: 'var(--text-faint)',
  ok:        'var(--success)',
  error:     'var(--danger)',
}

const color = COLORS[props.status] ?? 'var(--text-faint)'
</script>

<template>
  <span :style="{ width: '6px', height: '6px', borderRadius: '50%', background: color, display: 'inline-block', flexShrink: 0 }" />
</template>
