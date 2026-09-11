// Formatting helpers. Prices in this app span ~13 orders of magnitude
// (a 0.00003 BTC ratio and a 233,525,000 IRT coin price live on the same
// screen), so significant digits matter more than a fixed decimal count.

export function formatNumber(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  const abs = Math.abs(value)
  if (abs === 0) return '0'

  let decimals: number
  if (abs >= 1_000_000) decimals = 0
  else if (abs >= 1000) decimals = 2
  else if (abs >= 1) decimals = 4
  else if (abs >= 0.01) decimals = 6
  else decimals = 8

  return value.toLocaleString('en-US', {
    minimumFractionDigits: 0,
    maximumFractionDigits: decimals,
  })
}

export function formatPriceString(price: string | undefined): string {
  if (!price) return '—'
  const n = Number(price)
  return Number.isNaN(n) ? price : formatNumber(n)
}

export function formatCompact(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  const abs = Math.abs(value)
  if (abs >= 1000) {
    return value.toLocaleString('en-US', {
      notation: 'compact',
      maximumFractionDigits: 2,
    })
  }
  return formatNumber(value)
}

export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  const sign = value > 0 ? '+' : ''
  return `${sign}${value.toFixed(3)}%`
}

/** Seconds since an ISO timestamp, or null if unparseable. */
export function ageSeconds(iso: string | undefined, now: number): number | null {
  if (!iso) return null
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return null
  return Math.max(0, (now - t) / 1000)
}

export function formatAge(seconds: number | null): string {
  if (seconds === null) return '—'
  if (seconds < 1) return 'now'
  if (seconds < 60) return `${Math.floor(seconds)}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  return `${Math.floor(seconds / 3600)}h ago`
}

export function formatClock(iso: string | undefined): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  return new Date(t).toLocaleTimeString('en-GB')
}
