<script setup lang="ts">
defineProps<{
  label: string
  value: string
  hint: string
  icon: string
}>()
</script>

<template>
  <UCard class="rounded-xl border-t-2 border-t-primary/30">
    <div class="flex items-start justify-between gap-4">
      <div>
        <p class="text-sm text-muted">{{ label }}</p>
        <p class="mt-2 text-2xl font-semibold tabular-nums">{{ value }}</p>
        <p class="mt-1 text-xs text-muted">{{ hint }}</p>
      </div>
      <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
        <UIcon :name="icon" class="size-5" />
      </div>
    </div>
  </UCard>
</template>
