import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface LocaleOption {
  code: string
  nativeName: string
  englishName: string
  emoji: string | null // null = use flagSrc instead
  flagSrc: string | null
  dir: 'ltr' | 'rtl'
}

export const ALL_LOCALES: LocaleOption[] = [
  {
    code: 'fa',
    nativeName: 'فارسی',
    englishName: 'Persian',
    emoji: null,
    flagSrc: 'https://cdn.jsdelivr.net/gh/twitter/twemoji@v14.0.3/assets/svg/1f1ee-1f1f7.svg',
    dir: 'rtl',
  },
  {
    code: 'en',
    nativeName: 'English',
    englishName: 'English',
    emoji: '🇺🇸',
    flagSrc: null,
    dir: 'ltr',
  },
  {
    code: 'es',
    nativeName: 'Español',
    englishName: 'Spanish',
    emoji: '🇪🇸',
    flagSrc: null,
    dir: 'ltr',
  },
]

export const RTL_CODES = ['fa', 'ar', 'he', 'ur']
const RTL_CODES_SET = new Set(RTL_CODES)

export const useLocaleStore = defineStore('locale', () => {
  const enabledCodes = ref<string[]>(
    JSON.parse(
      localStorage.getItem('enabledLocales') ?? JSON.stringify(ALL_LOCALES.map((l) => l.code)),
    ),
  )

  const enabledLocales = computed(() =>
    ALL_LOCALES.filter((l) => enabledCodes.value.includes(l.code)),
  )

  function isEnabled(code: string) {
    return enabledCodes.value.includes(code)
  }

  function toggle(code: string) {
    if (enabledCodes.value.includes(code)) {
      // prevent disabling last locale
      if (enabledCodes.value.length > 1) {
        enabledCodes.value = enabledCodes.value.filter((c) => c !== code)
        save()
      }
    } else {
      enabledCodes.value = [...enabledCodes.value, code]
      save()
    }
  }

  function save() {
    localStorage.setItem('enabledLocales', JSON.stringify(enabledCodes.value))
  }

  function isRtl(code: string) {
    return RTL_CODES_SET.has(code)
  }

  return { ALL_LOCALES, enabledLocales, isEnabled, toggle, isRtl }
})
