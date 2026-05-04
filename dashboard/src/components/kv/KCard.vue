<script setup lang="ts">
import { useSlots } from 'vue'

defineProps<{
  title?: string
  subtitle?: string
  flush?: boolean
}>()

const slots = useSlots()
</script>

<template>
  <section class="card">
    <header v-if="title || slots.action" class="card-h">
      <div>
        <h3 v-if="title">{{ title }}</h3>
        <div v-if="subtitle" class="sub">{{ subtitle }}</div>
      </div>
      <slot name="action" />
    </header>
    <div :class="['card-b', flush ? 'flush' : '']">
      <slot />
    </div>
  </section>
</template>
