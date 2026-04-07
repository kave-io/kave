<script setup lang="ts">
import { computed } from 'vue'
import { useCurrencyStore } from '../stores/currency'

const currencyStore = useCurrencyStore()

const currencySymbols: Record<string, string> = {
  USD: '$',
  IRT: 'T',
}

const items = computed(() =>
  currencyStore.enabledCurrencies.map((c) => ({
    label: c.nativeName,
    value: c.code,
    symbol: currencySymbols[c.code] || c.code,
  })),
)

const modelValue = computed({
  get() {
    return currencyStore.selected?.code
  },
  set(value: string) {
    currencyStore.select(value)
  },
})

const selectedItem = computed(
  () => items.value.find((item) => item.value === modelValue.value) || items.value[0],
)
</script>

<template>
  <USelectMenu
    v-model="modelValue"
    :items="items"
    value-key="value"
    label-key="label"
    variant="ghost"
  >
    <template #leading>
      <span class="font-mono font-bold text-sm">{{ selectedItem?.symbol }}</span>
    </template>
    <template #item-leading="{ item }">
      <span class="font-mono font-bold text-sm">{{ item.symbol }}</span>
    </template>
  </USelectMenu>
</template>
