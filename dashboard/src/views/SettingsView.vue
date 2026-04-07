<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useLocaleStore, ALL_LOCALES } from '../stores/locale'
import { useCurrencyStore, ALL_CURRENCIES } from '../stores/currency'

const { locale } = useI18n()
const localeStore = useLocaleStore()
const currencyStore = useCurrencyStore()

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
</script>

<template>
  <div class="p-4 lg:p-6 max-w-2xl space-y-8">
    <div>
      <h1 class="text-lg font-semibold">Settings</h1>
      <p class="text-sm text-muted mt-1">Manage your workspace preferences.</p>
    </div>

    <!-- Languages -->
    <section class="space-y-4">
      <div>
        <h2 class="text-base font-semibold">Languages</h2>
        <p class="text-sm text-muted mt-1">
          Choose which languages appear in the locale switcher.
        </p>
      </div>

      <UCard class="divide-y divide-default" :ui="{ body: 'p-0' }">
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
    </section>

    <!-- Currencies -->
    <section class="space-y-4">
      <div>
        <h2 class="text-base font-semibold">Currencies</h2>
        <p class="text-sm text-muted mt-1">
          Choose which currencies appear in the currency switcher.
        </p>
      </div>

      <UCard class="divide-y divide-default" :ui="{ body: 'p-0' }">
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
    </section>
  </div>
</template>
