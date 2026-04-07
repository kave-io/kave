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
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="text-xs font-medium uppercase tracking-wide text-muted">{{ label }}</p>
        <p class="mt-1.5 text-xl font-semibold tabular-nums">{{ value }}</p>
        <p class="mt-1 text-xs text-muted/80">{{ hint }}</p>
      </div>
      <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
        <UIcon :name="icon" class="size-4" />
      </div>
    </div>
  </UCard>
</template>
