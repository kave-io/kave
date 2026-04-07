<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useLocaleStore } from '../stores/locale'

const { locale } = useI18n()
const localeStore = useLocaleStore()

const items = computed(() =>
  localeStore.enabledLocales.map((l) => ({
    label: l.nativeName,
    value: l.code,
    emoji: l.emoji,
    flagSrc: l.flagSrc,
  })),
)

const modelValue = computed({
  get() {
    return locale.value
  },
  set(value: string) {
    locale.value = value
    localStorage.setItem('locale', value)
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
      <span v-if="selectedItem?.emoji" class="text-lg">{{ selectedItem.emoji }}</span>
      <img
        v-else
        :src="selectedItem?.flagSrc ?? undefined"
        class="size-5 rounded-sm"
        :alt="selectedItem?.label"
      />
    </template>
    <template #item-leading="{ item }">
      <span v-if="item.emoji" class="text-lg">{{ item.emoji }}</span>
      <img v-else :src="item.flagSrc ?? undefined" class="size-5 rounded-sm" :alt="item.label" />
    </template>
  </USelectMenu>
</template>
