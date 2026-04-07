<script setup lang="ts">
defineProps<{ status: string }>()
</script>

<template>
  <UBadge
    :color="status === 'failed' ? 'error' : status === 'completed' ? 'success' : 'info'"
    variant="soft"
    size="xs"
  >{{ status }}</UBadge>
</template>
