import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { formatAmount, type CurrencyConfig } from '@/lib/money'

export interface CurrencyOption extends CurrencyConfig {
  code: string
  name: string
  nativeName: string
}

export const ALL_CURRENCIES: CurrencyOption[] = [
  {
    code: 'IRT',
    name: 'Iranian Toman',
    nativeName: 'تومان',
    symbol: 'T',
    symbolPosition: 'after',
    fractionDigits: 0,
    decimalSep: '.',
    thousandsSep: ',',
    spaceBetween: true,
  },
  {
    code: 'USD',
    name: 'US Dollar',
    nativeName: 'US Dollar',
    symbol: '$',
    symbolPosition: 'before',
    fractionDigits: 2,
    decimalSep: '.',
    thousandsSep: ',',
    spaceBetween: false,
  },
  {
    code: 'EUR',
    name: 'Euro',
    nativeName: 'Euro',
    symbol: '€',
    symbolPosition: 'after',
    fractionDigits: 2,
    decimalSep: ',',
    thousandsSep: '.',
    spaceBetween: true,
  },
  {
    code: 'GBP',
    name: 'Pound Sterling',
    nativeName: 'Pound Sterling',
    symbol: '£',
    symbolPosition: 'before',
    fractionDigits: 2,
    decimalSep: '.',
    thousandsSep: ',',
    spaceBetween: false,
  },
  {
    code: 'CHF',
    name: 'Swiss Franc',
    nativeName: 'Franken',
    symbol: 'CHF',
    symbolPosition: 'after',
    fractionDigits: 2,
    decimalSep: '.',
    thousandsSep: "'",
    spaceBetween: true,
  },
  {
    code: 'IRR',
    name: 'Iranian Rial',
    nativeName: 'ریال',
    symbol: '﷼',
    symbolPosition: 'after',
    fractionDigits: 0,
    decimalSep: '.',
    thousandsSep: ',',
    spaceBetween: true,
  },
]

export const useCurrencyStore = defineStore('currency', () => {
  const selectedCode = ref<string>(
    localStorage.getItem('currency') ?? 'IRT'
  )

  const enabledCodes = ref<string[]>(
    JSON.parse(localStorage.getItem('enabledCurrencies') ?? JSON.stringify(ALL_CURRENCIES.map(c => c.code)))
  )

  const selected = computed(() =>
    ALL_CURRENCIES.find(c => c.code === selectedCode.value) ?? ALL_CURRENCIES[0]!
  )

  const enabledCurrencies = computed(() =>
    ALL_CURRENCIES.filter(c => enabledCodes.value.includes(c.code))
  )

  function select(code: string) {
    selectedCode.value = code
    localStorage.setItem('currency', code)
  }

  function isEnabled(code: string) {
    return enabledCodes.value.includes(code)
  }

  function toggle(code: string) {
    if (enabledCodes.value.includes(code)) {
      if (enabledCodes.value.length > 1) {
        enabledCodes.value = enabledCodes.value.filter(c => c !== code)
        // switch away if currently selected
        if (selectedCode.value === code) {
          const next = enabledCodes.value[0]
          if (next) select(next)
        }
        save()
      }
    } else {
      enabledCodes.value = [...enabledCodes.value, code]
      save()
    }
  }

  function format(amount: string | undefined | null): string {
    return formatAmount(amount, selected.value)
  }

  function save() {
    localStorage.setItem('enabledCurrencies', JSON.stringify(enabledCodes.value))
  }

  return { ALL_CURRENCIES, selected, enabledCurrencies, isEnabled, select, toggle, format }
})
