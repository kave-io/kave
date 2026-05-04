<script setup lang="ts">
withDefaults(defineProps<{ label: string; value: string | number; mono?: boolean; small?: boolean }>(), {
  mono: false,
  small: false,
})
</script>

<template>
  <div>
    <div :style="{ fontSize: '10.5px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.07em', color: 'var(--text-dim)' }">{{ label }}</div>
    <div
      :style="{
        fontSize: small ? '12px' : '15px',
        fontWeight: 500,
        fontFamily: (mono || small) ? 'var(--font-mono)' : 'inherit',
        marginTop: '3px',
        fontVariantNumeric: 'tabular-nums',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
      }"
    >{{ value }}</div>
  </div>
</template>
