<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { consoleContext, setReportingScope } from '@/lib/context'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const tenant = ref('')
const billTo = ref('')
const actor = ref('')
const feature = ref('')
const formError = ref('')
const dialog = ref<HTMLElement>()
let returnFocus: HTMLElement | null = null

watch(
  () => props.open,
  (open) => {
    if (!open) {
      void nextTick(() => returnFocus?.focus())
      return
    }
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    tenant.value = consoleContext.tenant
    billTo.value = consoleContext.billTo
    actor.value = consoleContext.actor
    feature.value = consoleContext.feature
    formError.value = ''
    void nextTick(() => dialog.value?.querySelector<HTMLElement>('input')?.focus())
  },
  { immediate: true },
)

function apply(): void {
  formError.value = ''
  try {
    setReportingScope({
      tenant: tenant.value,
      billTo: billTo.value,
      actor: actor.value || undefined,
      feature: feature.value || undefined,
    })
    close()
  } catch (error) {
    formError.value = error instanceof Error ? error.message : 'The reporting scope is invalid.'
  }
}

function close(): void {
  emit('close')
}

function trapFocus(event: KeyboardEvent): void {
  if (event.key !== 'Tab' || !dialog.value) return
  const focusable = [
    ...dialog.value.querySelectorAll<HTMLElement>('button, input, select, [tabindex]'),
  ].filter((element) => element.tabIndex >= 0 && !element.hasAttribute('disabled'))
  const first = focusable[0]
  const last = focusable.at(-1)
  if (!first || !last) return
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

onBeforeUnmount(() => returnFocus?.focus())
</script>

<template>
  <div v-if="open" class="modal-backdrop" @click.self="close">
    <section
      ref="dialog"
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="scope-title"
      @keydown.esc="close"
      @keydown.tab="trapFocus"
    >
      <header class="modal-header">
        <div>
          <p class="eyebrow">Query boundary</p>
          <h2 id="scope-title">Reporting scope</h2>
        </div>
        <button type="button" class="icon-button" aria-label="Close reporting scope" @click="close">
          ×
        </button>
      </header>
      <p class="modal-copy">
        Kave requires opaque tenant and billing references for every usage query. These values are
        application assertions, not human identity records.
      </p>
      <form class="form-grid" @submit.prevent="apply">
        <label>
          <span>Tenant reference</span>
          <input v-model.trim="tenant" required autocomplete="off" placeholder="tenant_opaque_id" />
        </label>
        <label>
          <span>Bill-to reference</span>
          <input
            v-model.trim="billTo"
            required
            autocomplete="off"
            placeholder="billing_opaque_id"
          />
        </label>
        <label>
          <span>Actor filter <small>optional</small></span>
          <input v-model.trim="actor" autocomplete="off" placeholder="actor_opaque_id" />
        </label>
        <label>
          <span>Feature filter <small>optional</small></span>
          <input v-model.trim="feature" autocomplete="off" placeholder="feature.name" />
        </label>
        <p v-if="formError" class="form-error" role="alert">{{ formError }}</p>
        <div class="modal-actions">
          <button type="button" class="button button-secondary" @click="close">Cancel</button>
          <button type="submit" class="button button-primary">Apply scope</button>
        </div>
      </form>
    </section>
  </div>
</template>
