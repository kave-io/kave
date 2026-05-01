<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import { useLocaleStore, ALL_LOCALES } from '../stores/locale'
import { useCurrencyStore, ALL_CURRENCIES } from '../stores/currency'
import { projectId, envId } from '@/stores/workspace'
import { usePricingBook, useSavePricingBook } from '@/composables/api/useSettings'

const { locale, t } = useI18n()
const localeStore = useLocaleStore()
const currencyStore = useCurrencyStore()
const copiedUrl = ref<string | null>(null)
const pricingEditor = ref('')
const pricingError = ref<string | null>(null)
const pricingQuery = usePricingBook()
const savePricing = useSavePricingBook()

watch(
  () => pricingQuery.data.value,
  (book) => {
    if (book) {
      pricingEditor.value = JSON.stringify(book, null, 2)
    }
  },
  { immediate: true }
)

function handleLocaleToggle(code: string) {
  if (code === locale.value && localeStore.enabledLocales.length > 1) {
    const next = localeStore.enabledLocales.find(l => l.code !== code)
    if (next) {
      locale.value = next.code
      localStorage.setItem('locale', next.code)
    }
  }
  localeStore.toggle(code)
}

const baseUrl = window.location.origin

const proxyUrls = computed(() => [
  { name: t('pages.settings.proxy_openai'), url: `${baseUrl}/frameworks/claude-code/openai`, env: 'OPENAI_BASE_URL' },
  { name: t('pages.settings.proxy_anthropic'), url: `${baseUrl}/frameworks/claude-code/anthropic`, env: 'ANTHROPIC_BASE_URL' },
  { name: t('pages.settings.proxy_gemini'), url: `${baseUrl}/frameworks/claude-code/gemini`, env: 'GOOGLE_GENERATIVEAI_API_BASE' },
  { name: t('pages.settings.proxy_ollama'), url: `${baseUrl}/frameworks/claude-code/ollama`, env: 'OLLAMA_HOST' },
])

async function copyUrl(url: string) {
  await navigator.clipboard.writeText(url)
  copiedUrl.value = url
  setTimeout(() => {
    copiedUrl.value = null
  }, 2000)
}

async function savePricingBook() {
  pricingError.value = null
  try {
    const parsed = JSON.parse(pricingEditor.value)
    await savePricing.mutateAsync(parsed)
  } catch (error) {
    pricingError.value = error instanceof Error ? error.message : 'Failed to save pricing'
  }
}
</script>

<template>
  <div class="p-4 lg:p-6 max-w-5xl space-y-8">
    <PageHeader :title="t('pages.settings.title')" :subtitle="t('pages.settings.subtitle')" icon="i-lucide-settings" />

    <!-- Workspace -->
    <section class="space-y-4">
      <div>
        <h2 class="text-base font-semibold">{{ t('pages.settings.workspace') }}</h2>
        <p class="text-sm text-muted mt-1">
          {{ t('pages.settings.workspace_hint') }}
        </p>
      </div>

      <UCard class="rounded-xl">
        <div class="space-y-4">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-xs text-muted">{{ t('pages.settings.project_id') || 'Project ID' }}</p>
              <p class="text-sm font-mono mt-1">{{ projectId }}</p>
            </div>
            <UButton
              icon="i-lucide-copy"
              color="gray"
              variant="ghost"
              size="sm"
              @click="copyUrl(projectId)"
              :title="`${t('common.copy')} project ID`"
            />
          </div>
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-xs text-muted">{{ t('pages.settings.env_id') || 'Environment ID' }}</p>
              <p class="text-sm font-mono mt-1">{{ envId }}</p>
            </div>
            <UButton
              icon="i-lucide-copy"
              color="gray"
              variant="ghost"
              size="sm"
              @click="copyUrl(envId)"
              :title="`${t('common.copy')} environment ID`"
            />
          </div>
        </div>
      </UCard>
    </section>

    <!-- Proxy URLs -->
    <section class="space-y-4">
      <div>
        <h2 class="text-base font-semibold">{{ t('pages.settings.proxy_urls') }}</h2>
        <p class="text-sm text-muted mt-1">
          {{ t('pages.settings.proxy_hint') }}
        </p>
      </div>

      <UCard class="divide-y divide-default/60 rounded-xl" :ui="{ body: 'p-0' }">
        <div
          v-for="proxy in proxyUrls"
          :key="proxy.name"
          class="px-4 py-3.5"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium">{{ proxy.name }}</p>
              <div class="flex items-center gap-1 text-xs text-muted mt-1">
                <span class="font-mono">{{ proxy.env }}</span>
                <span>=</span>
              </div>
            </div>
            <div class="flex items-center gap-2 ml-2 shrink-0">
              <UButton
                :icon="copiedUrl === proxy.url ? 'i-lucide-check' : 'i-lucide-copy'"
                :color="copiedUrl === proxy.url ? 'green' : 'gray'"
                variant="ghost"
                size="sm"
                :title="`Copy ${proxy.name} proxy URL`"
                @click="copyUrl(proxy.url)"
              />
            </div>
          </div>
          <code class="block text-xs bg-muted/40 rounded px-2.5 py-2 font-mono mt-2 truncate text-muted">{{ proxy.url }}</code>
        </div>
      </UCard>

      <!-- Quick copy command -->
      <div class="space-y-2">
        <p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('pages.settings.export') }}</p>
        <UCard class="rounded-lg">
          <div class="space-y-2">
            <div
              v-for="proxy in proxyUrls.slice(0, 2)"
              :key="proxy.name"
              class="flex items-center gap-2"
            >
              <code class="text-xs bg-muted/40 rounded px-2 py-1 font-mono truncate flex-1">
                export {{ proxy.env }}={{ proxy.url }}
              </code>
              <UButton
                icon="i-lucide-copy"
                color="gray"
                variant="ghost"
                size="sm"
                @click="copyUrl(`export ${proxy.env}=${proxy.url}`)"
                :title="`Copy export command`"
              />
            </div>
          </div>
        </UCard>
      </div>
    </section>

    <!-- Preferences -->
    <section class="space-y-4 border-t border-default pt-8">
      <div>
        <h2 class="text-base font-semibold">{{ t('pages.settings.preferences') }}</h2>
        <p class="text-sm text-muted mt-1">
          {{ t('pages.settings.languages') }} {{ t('pages.settings.currencies') }}
        </p>
      </div>

      <!-- Languages -->
      <div class="space-y-3">
        <h3 class="text-sm font-medium">{{ t('pages.settings.languages') }}</h3>
        <UCard class="divide-y divide-default/50 rounded-xl" :ui="{ body: 'p-0' }">
        <div
          v-for="loc in ALL_LOCALES"
          :key="loc.code"
          class="flex items-center justify-between px-4 py-3"
        >
          <div class="flex items-center gap-3">
            <img
              v-if="loc.flagSrc"
              :src="loc.flagSrc"
              class="size-8 rounded object-cover shrink-0"
              :alt="loc.englishName"
            />
            <span v-else class="text-2xl leading-none">{{ loc.emoji }}</span>
            <div>
              <p class="text-sm font-medium">{{ loc.nativeName }}</p>
              <p class="text-xs text-muted">{{ loc.englishName }} · {{ loc.dir.toUpperCase() }}</p>
            </div>
          </div>
          <USwitch
            :model-value="localeStore.isEnabled(loc.code)"
            :disabled="localeStore.isEnabled(loc.code) && localeStore.enabledLocales.length === 1"
            @update:model-value="() => handleLocaleToggle(loc.code)"
          />
        </div>
      </UCard>
      </div>

      <!-- Currencies -->
      <div class="space-y-3">
        <h3 class="text-sm font-medium">{{ t('pages.settings.currencies') }}</h3>
        <UCard class="divide-y divide-default/50 rounded-xl" :ui="{ body: 'p-0' }">
          <div
            v-for="c in ALL_CURRENCIES"
            :key="c.code"
            class="flex items-center justify-between px-4 py-3"
          >
            <div class="flex items-center gap-3">
              <div class="size-8 rounded bg-muted flex items-center justify-center font-mono text-lg font-semibold shrink-0">
                {{ c.symbol }}
              </div>
              <div>
                <p class="text-sm font-medium">{{ c.nativeName }}</p>
                <p class="text-xs text-muted">{{ c.name }} · {{ c.code }}</p>
              </div>
            </div>
            <USwitch
              :model-value="currencyStore.isEnabled(c.code)"
              :disabled="currencyStore.isEnabled(c.code) && currencyStore.enabledCurrencies.length === 1"
              @update:model-value="() => currencyStore.toggle(c.code)"
            />
          </div>
        </UCard>
      </div>
    </section>

    <section class="space-y-4 border-t border-default pt-8">
      <div>
        <h2 class="text-base font-semibold">Pricing</h2>
        <p class="text-sm text-muted mt-1">
          Edit the active price book used for cost calculation. Changes apply immediately to new spans and budget entries.
        </p>
      </div>

      <UCard class="rounded-xl space-y-4">
        <UTextarea
          v-model="pricingEditor"
          :rows="18"
          autoresize
          class="font-mono"
        />

        <p v-if="pricingError" class="text-sm text-red-500">{{ pricingError }}</p>
        <p v-else-if="savePricing.isSuccess.value" class="text-sm text-green-600">Pricing saved.</p>

        <div class="flex justify-end">
          <UButton
            color="primary"
            :loading="pricingQuery.isLoading.value || savePricing.isPending.value"
            @click="savePricingBook"
          >
            Save Pricing
          </UButton>
        </div>
      </UCard>
    </section>
  </div>
</template>
