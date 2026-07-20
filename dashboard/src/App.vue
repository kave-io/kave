<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { RouterView } from 'vue-router'
import ConsoleLogo from '@/components/ConsoleLogo.vue'
import { authState, connectCredential } from '@/lib/auth'

const serviceKeyInput = ref('')
const namespaceInput = ref('')
const rememberForTab = ref(false)
const formError = ref('')

async function connect(): Promise<void> {
  formError.value = ''
  try {
    connectCredential(serviceKeyInput.value, namespaceInput.value, rememberForTab.value)
    serviceKeyInput.value = ''
    await nextTick()
  } catch (error) {
    formError.value = error instanceof Error ? error.message : 'The credential could not be used.'
  }
}
</script>

<template>
  <RouterView v-if="authState.connected.value" />
  <main v-else class="signin-shell">
    <section class="signin-card" aria-labelledby="signin-title">
      <header class="signin-brand">
        <ConsoleLogo />
        <div>
          <strong>Kave</strong>
          <span>Operations Console</span>
        </div>
      </header>

      <div class="signin-intro">
        <p class="eyebrow">Kave V2</p>
        <h1 id="signin-title">Connect to a namespace</h1>
        <p>
          Use a least-privilege service key. The console sends it only to this origin and never
          writes it to persistent browser storage.
        </p>
      </div>

      <form class="signin-form" @submit.prevent="connect">
        <label>
          <span>Service key</span>
          <input
            v-model="serviceKeyInput"
            name="service-key"
            type="password"
            required
            autocomplete="new-password"
            autocapitalize="none"
            spellcheck="false"
            placeholder="kv2_••••••••••••••••••••••••.••••••••"
          />
        </label>
        <label>
          <span>Namespace ID <small>needed for configuration inspection</small></span>
          <input
            v-model.trim="namespaceInput"
            name="namespace-id"
            autocomplete="off"
            autocapitalize="none"
            spellcheck="false"
            placeholder="nsp_…"
          />
        </label>
        <label class="check-row">
          <input v-model="rememberForTab" type="checkbox" />
          <span>Keep the service key in session storage until this tab closes</span>
        </label>
        <p v-if="formError" class="form-error" role="alert">{{ formError }}</p>
        <button type="submit" class="button button-primary button-wide">Open console</button>
      </form>

      <footer class="signin-notes">
        <span><i class="security-dot" /> Same-origin requests only</span>
        <span>No prompts or responses</span>
        <span>No local storage</span>
      </footer>
    </section>
  </main>
</template>
