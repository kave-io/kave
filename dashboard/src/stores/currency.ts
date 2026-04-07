import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface CurrencyOption {
  code: string
  name: string
  nativeName: string
  symbol: string
  symbolPosition: 'before' | 'after'
}

export const ALL_CURRENCIES: CurrencyOption[] = [
  {
    code: 'IRT',
    name: 'Iranian Toman',
    nativeName: 'تومان',
    symbol: 'T',
    symbolPosition: 'after',
  },
  {
    code: 'USD',
    name: 'US Dollar',
    nativeName: 'US Dollar',
    symbol: '$',
    symbolPosition: 'before',
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

  function format(amount: number): string {
    const c = selected.value
    const formatted = amount.toLocaleString()
    return c.symbolPosition === 'before'
      ? `${c.symbol}${formatted}`
      : `${formatted} ${c.symbol}`
  }

  function save() {
    localStorage.setItem('enabledCurrencies', JSON.stringify(enabledCodes.value))
  }

  return { ALL_CURRENCIES, selected, enabledCurrencies, isEnabled, select, toggle, format }
})
