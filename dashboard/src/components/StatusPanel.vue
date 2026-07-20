<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    message: string
    tone?: 'neutral' | 'error' | 'warning'
    busy?: boolean
  }>(),
  { tone: 'neutral', busy: false },
)

defineEmits<{ retry: [] }>()
</script>

<template>
  <section
    class="status-panel"
    :class="`status-${tone}`"
    :role="tone === 'error' ? 'alert' : 'status'"
    :aria-live="tone === 'error' ? 'assertive' : 'polite'"
    :aria-busy="busy"
  >
    <span v-if="busy" class="spinner" aria-hidden="true" />
    <span v-else class="status-glyph" aria-hidden="true">{{ tone === 'error' ? '!' : '·' }}</span>
    <div>
      <h2>{{ title }}</h2>
      <p>{{ message }}</p>
      <button
        v-if="$slots.action"
        type="button"
        class="button button-secondary"
        @click="$emit('retry')"
      >
        <slot name="action" />
      </button>
    </div>
  </section>
</template>
