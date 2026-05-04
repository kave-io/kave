<script setup lang="ts">
import { ref } from 'vue'
import KIcon from './KIcon.vue'

const props = withDefaults(defineProps<{ value: string; label?: string }>(), { label: 'copy' })

const copied = ref(false)
async function onClick(e: MouseEvent) {
  e.stopPropagation()
  try { await navigator.clipboard?.writeText(props.value) } catch (_) { /* noop */ }
  copied.value = true
  setTimeout(() => { copied.value = false }, 1500)
}
</script>

<template>
  <button :class="['copy-btn', copied ? 'copied' : '']" @click="onClick">
    <KIcon :name="copied ? 'check' : 'copy'" :size="11" />
    {{ label }}
  </button>
</template>
