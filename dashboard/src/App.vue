<script setup lang="ts">
import { computed, onMounted, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterView } from 'vue-router'
import { useColorMode } from '@vueuse/core'
import { RTL_CODES } from './stores/locale'
import { initializeWorkspaceContext } from './stores/workspace'

const { locale } = useI18n()
const colorMode = useColorMode()

const dir = computed(() => (RTL_CODES.includes(locale.value) ? 'rtl' : 'ltr'))

watchEffect(() => {
  document.documentElement.lang = locale.value
  document.documentElement.dir = dir.value
})

watchEffect(() => {
  const themeColor = colorMode.value === 'dark' ? '#050403' : '#d9d5cc'
  const metaTag = document.querySelector('meta[name="theme-color"]')
  if (metaTag) {
    metaTag.setAttribute('content', themeColor)
  } else {
    const meta = document.createElement('meta')
    meta.name = 'theme-color'
    meta.content = themeColor
    document.head.appendChild(meta)
  }
})

onMounted(() => {
  void initializeWorkspaceContext()
})
</script>

<template>
  <UApp :dir="dir">
    <RouterView />
  </UApp>
</template>
