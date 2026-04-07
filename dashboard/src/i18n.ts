import { createI18n } from 'vue-i18n'
import type { I18n, I18nOptions } from 'vue-i18n'
import fa from './locales/fa.json'
import en from './locales/en.json'
import es from './locales/es.json'

const messages = {
  fa,
  en,
  es
}

const savedLocale = (typeof localStorage !== 'undefined' ? localStorage.getItem('locale') : null) || 'fa'

const options: I18nOptions = {
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'en',
  messages
}

const i18n: I18n = createI18n(options)

export default i18n
