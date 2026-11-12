export function formatCurrency(
  value: number,
  opts: { compact?: boolean; showSign?: boolean } = {},
): string {
  if (!Number.isFinite(value)) return '—'
  const abs = Math.abs(value)
  let formatted: string

  if (opts.compact && abs >= 999_950) {
    formatted = `$${(abs / 1_000_000).toFixed(1)}M`
  } else if (opts.compact && abs >= 1_000) {
    formatted = `$${(abs / 1_000).toFixed(1)}K`
  } else {
    formatted = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      maximumFractionDigits: 0,
    }).format(abs)
  }

  if (value < 0) return `-${formatted}`
  if (opts.showSign && value > 0) return `+${formatted}`
  return formatted
}

export function formatPercent(value: number, decimals = 1): string {
  if (!Number.isFinite(value)) return '—'
  return `${value >= 0 ? '+' : ''}${value.toFixed(decimals)}%`
}
