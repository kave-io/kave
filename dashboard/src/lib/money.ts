/**
 * Smart monetary amount formatting.
 * Adapts decimal precision based on value magnitude and applies
 * currency-specific formatting (symbol, separators, position).
 */

export interface CurrencyConfig {
  symbol: string
  symbolPosition: 'before' | 'after'
  spaceBetween?: boolean
  decimalSep?: string
  thousandsSep?: string
  fractionDigits?: number
}

/**
 * Compute ideal decimal places for a given absolute value.
 * Shows more precision for smaller values.
 */
export function smartPrecision(abs: number, base: number = 2): number {
  if (abs === 0) return base
  if (abs >= 1) return Math.max(base, 2)
  if (abs >= 0.1) return Math.max(base, 2)
  if (abs >= 0.01) return 4
  if (abs >= 0.001) return 5
  if (abs >= 0.0001) return 6
  if (abs >= 0.000001) return 8
  return 10
}

/**
 * Format a raw decimal string (from API) into a display string
 * with currency symbol and locale-specific separators.
 */
export function formatAmount(raw: string | undefined | null, cfg: CurrencyConfig): string {
  const {
    symbol,
    symbolPosition,
    spaceBetween = false,
    decimalSep = '.',
    thousandsSep = ',',
    fractionDigits = 2,
  } = cfg

  const sep = spaceBetween ? ' ' : ''

  if (!raw || raw === '0') {
    const zero = `0${decimalSep}${'0'.repeat(fractionDigits)}`
    return symbolPosition === 'before' ? `${symbol}${sep}${zero}` : `${zero}${sep}${symbol}`
  }

  const value = parseFloat(raw)
  if (isNaN(value)) return raw

  const abs = Math.abs(value)
  const decimals = smartPrecision(abs, fractionDigits)

  const fixed = abs.toFixed(decimals)
  const parts = fixed.split('.')
  const intRaw = parts[0] || '0'
  const fracRaw = parts[1] || ''

  const intFormatted = intRaw.replace(/\B(?=(\d{3})+(?!\d))/g, thousandsSep)

  // Trim trailing zeros but keep at least fractionDigits
  const trimmed = fracRaw.replace(/0+$/, '')
  const fracLength = Math.max(fractionDigits, trimmed.length)
  const fracPart = fracRaw.slice(0, fracLength).replace(/0+$/, '')

  const sign = value < 0 ? '-' : ''
  const number = fracPart ? `${intFormatted}${decimalSep}${fracPart}` : intFormatted

  const display = `${sign}${number}`
  return symbolPosition === 'before' ? `${symbol}${sep}${display}` : `${display}${sep}${symbol}`
}
