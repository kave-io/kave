<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 26 })
</script>

<template>
  <div
    :style="{
      width: size + 'px',
      height: size + 'px',
      borderRadius: '6px',
      display: 'grid',
      placeItems: 'center',
      background: 'linear-gradient(135deg, var(--teal-500), var(--teal-700))',
      boxShadow: '0 1px 0 rgba(255,255,255,0.15) inset, 0 1px 2px rgba(20,15,8,0.18)',
      color: 'white',
      fontFamily: 'var(--font-sans)',
      fontWeight: 700,
      fontSize: (size * 0.5) + 'px',
      letterSpacing: '-0.04em',
      flexShrink: 0,
    }"
  >K</div>
</template>
