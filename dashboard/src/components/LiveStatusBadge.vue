<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{ isLive: boolean }>()

const { t } = useI18n()
</script>

<template>
  <span v-if="isLive" class="flex items-center gap-1.5 rounded-full bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-500">
    <span class="relative flex h-1.5 w-1.5">
      <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
      <span class="relative inline-flex h-1.5 w-1.5 rounded-full bg-green-500" />
    </span>
    {{ t('pages.overview.live_indicator') }}
  </span>
  <span v-else class="flex items-center gap-1.5 rounded-full bg-muted/50 px-2.5 py-1 text-xs font-medium text-muted">
    <span class="h-1.5 w-1.5 rounded-full bg-muted" />
    {{ t('pages.overview.offline_indicator') }}
  </span>
</template>
