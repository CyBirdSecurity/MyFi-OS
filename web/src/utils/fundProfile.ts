import type { Holding } from '../api/client'

export type FundType = 'stock' | 'etf' | 'mutual_fund' | 'cash' | 'crypto' | 'unknown'
export type SizeBucket = 'large' | 'mid' | 'small' | 'total' | 'stocks' | 'unknown'

export const FUND_TYPE_LABELS: Record<FundType, string> = {
  stock: 'Individual stocks',
  etf: 'ETFs',
  mutual_fund: 'Mutual funds',
  cash: 'Cash',
  crypto: 'Crypto',
  unknown: 'Unknown',
}

export const SIZE_LABELS: Record<SizeBucket, string> = {
  large: 'Large cap',
  mid: 'Mid cap',
  small: 'Small cap',
  total: 'Total market (all sizes)',
  stocks: 'Individual stocks',
  unknown: 'Not in the name',
}

/** What kind of investment a holding is, from the live quote's instrument type. */
export function fundTypeOf(h: Pick<Holding, 'ticker' | 'assetClass' | 'instrumentType'>): FundType {
  if (h.assetClass === 'cash_equivalent' || h.ticker.toUpperCase() === 'CASH') return 'cash'
  if (h.instrumentType === 'CRYPTOCURRENCY' || h.assetClass === 'crypto') return 'crypto'
  if (h.instrumentType === 'ETF') return 'etf'
  if (h.instrumentType === 'MUTUALFUND') return 'mutual_fund'
  if (h.instrumentType === 'EQUITY') return 'stock'
  return 'unknown'
}

// Checked in order; the first match wins ("Small-Mid Cap" is mid, not small).
const SIZE_PATTERNS: [SizeBucket, RegExp][] = [
  ['mid', /\bsmid\b|small[\s-]*mid|mid[\s-]*cap|\bmidcap\b|s&p (mid ?cap )?400|extended market|completion/],
  ['small', /small[\s-]*cap|\bsmallcap\b|russell 2000|s&p (small ?cap )?600|\bsmall\b/],
  ['total', /total (stock|market|world|international)|\ball[\s-]*cap\b|broad market|\btotal\b/],
  ['large', /large[\s-]*cap|\blargecap\b|mega[\s-]*cap|\b500\b|nasdaq[\s-]*100|\bdow\b/],
]

/**
 * A company-size guess for a stock holding, from the fund's name ("Small Cap
 * Value", "500 Index", "Total Stock Market"). Only stock holdings get a size;
 * others return null. Individual stocks can't be sized without market-cap data.
 */
export function sizeOf(h: Pick<Holding, 'ticker' | 'assetClass' | 'instrumentType' | 'name' | 'quoteName'>): SizeBucket | null {
  if (h.assetClass !== 'equity') return null
  const type = fundTypeOf(h)
  if (type === 'stock') return 'stocks'
  const name = `${h.quoteName ?? ''} ${h.name ?? ''}`.toLowerCase()
  return SIZE_PATTERNS.find(([, re]) => re.test(name))?.[0] ?? 'unknown'
}

export interface Breakdown<K extends string> {
  key: K
  label: string
  value: number
  pct: number
}

function breakdown<K extends string>(entries: [K, number][], labels: Record<K, string>): Breakdown<K>[] {
  const totals = new Map<K, number>()
  for (const [k, v] of entries) totals.set(k, (totals.get(k) ?? 0) + v)
  const total = [...totals.values()].reduce((a, b) => a + b, 0)
  return [...totals]
    .filter(([, v]) => v > 0)
    .map(([key, value]) => ({ key, label: labels[key], value, pct: total > 0 ? (value / total) * 100 : 0 }))
    .sort((a, b) => b.value - a.value)
}

/** Holdings value split by fund type, largest first. */
export function fundTypeBreakdown(holdings: Holding[]): Breakdown<FundType>[] {
  return breakdown(holdings.filter((h) => h.totalValue > 0).map((h) => [fundTypeOf(h), h.totalValue]), FUND_TYPE_LABELS)
}

/** Stock holdings value split by estimated company size, largest first. */
export function sizeBreakdown(holdings: Holding[]): Breakdown<SizeBucket>[] {
  const entries: [SizeBucket, number][] = []
  for (const h of holdings) {
    const size = sizeOf(h)
    if (size && h.totalValue > 0) entries.push([size, h.totalValue])
  }
  return breakdown(entries, SIZE_LABELS)
}
