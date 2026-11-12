import type { MonthlyBreakdownRow } from '../api/client'

export type LimitState = 'ok' | 'warn' | 'over'

/** Fraction of a limit at which the progress bar turns to a warning. */
export const WARN_AT = 0.8

/**
 * How spending compares to a monthly limit. `pct` is for the bar and is capped
 * at 100; `remaining` goes negative once the limit is passed.
 */
export function limitProgress(spent: number, limit: number): { pct: number; state: LimitState; remaining: number } {
  const ratio = limit > 0 ? spent / limit : 0
  return {
    pct: Math.min(Math.max(ratio * 100, 0), 100),
    state: spent > limit ? 'over' : ratio >= WARN_AT ? 'warn' : 'ok',
    remaining: limit - spent,
  }
}

/** Rounds an average up to a tidy limit: to $5 under $100, $10 under $1,000, else $50. */
export function roundLimit(amount: number): number {
  const step = amount < 100 ? 5 : amount < 1000 ? 10 : 50
  return Math.ceil(amount / step) * step
}

/**
 * Suggested monthly limits by category name: each category's average over the
 * three completed months before `now`'s month (the current month is partial, so
 * it is left out). A month only counts if there is spending data for it at all,
 * so someone with two months of history is averaged over two, not three.
 * Categories averaging under a dollar get no suggestion.
 */
export function suggestLimits(
  spendingByYear: Record<number, MonthlyBreakdownRow[] | undefined>,
  now: Date,
): Map<string, number> {
  const window = [3, 2, 1].map((offset) => {
    const d = new Date(now.getFullYear(), now.getMonth() - offset, 1)
    return { year: d.getFullYear(), month: d.getMonth() + 1 }
  })

  const byCategory = new Map<string, number>()
  let monthsWithData = 0
  for (const { year, month } of window) {
    const rows = (spendingByYear[year] ?? []).filter((r) => r.month === month && r.total > 0)
    if (rows.length === 0) continue
    monthsWithData += 1
    for (const r of rows) byCategory.set(r.categoryName, (byCategory.get(r.categoryName) ?? 0) + r.total)
  }

  const suggestions = new Map<string, number>()
  if (monthsWithData === 0) return suggestions
  for (const [name, total] of byCategory) {
    const average = total / monthsWithData
    if (average >= 1) suggestions.set(name, roundLimit(average))
  }
  return suggestions
}
