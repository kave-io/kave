<script setup lang="ts">
import { computed, useSlots } from 'vue'
import KIcon from './KIcon.vue'

const props = withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  icon?: string
  iconRight?: string
  ariaLabel?: string
  disabled?: boolean
  type?: 'button' | 'submit'
}>(), {
  variant: 'secondary',
  size: 'md',
  type: 'button',
})

defineEmits<{ (e: 'click', ev: MouseEvent): void }>()

const slots = useSlots()
const classes = computed(() => [
  'btn',
  `btn-${props.variant}`,
  props.size === 'sm' ? 'btn-sm' : props.size === 'lg' ? 'btn-lg' : '',
  !slots.default ? 'btn-icon' : '',
].filter(Boolean).join(' '))

const iconSize = computed(() => props.size === 'sm' ? 13 : 14)
</script>

<template>
  <button :type="type" :class="classes" :disabled="disabled" :aria-label="ariaLabel" @click="$emit('click', $event)">
    <KIcon v-if="icon" :name="icon" :size="iconSize" />
    <slot />
    <KIcon v-if="iconRight" :name="iconRight" :size="iconSize" />
  </button>
</template>
