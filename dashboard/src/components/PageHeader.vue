<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
  icon?: string
  showKaveLogo?: boolean
}>()
</script>

<template>
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-3">
      <img v-if="showKaveLogo" src="/icon-128.png" alt="Kave" class="size-7 rounded" />
      <UIcon v-else-if="icon" :name="icon" class="size-6 text-primary" />
      <div>
        <h1 class="text-xl font-semibold tracking-tight">{{ title }}</h1>
        <p v-if="subtitle" class="text-sm text-muted mt-0.5">{{ subtitle }}</p>
      </div>
    </div>
    <slot />
  </div>
</template>
