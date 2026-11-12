import { LIABILITY_ACCOUNT_TYPES, type AccountHistory, type AccountSeries } from '../api/client'

/**
 * The extent of a stacked chart of `points`: assets stack up from zero and
 * liabilities stack down from it, so the axis needs the largest positive total
 * and the most negative total at any date. Always includes zero.
 */
export function stackBounds(points: AccountHistory['points']): { min: number; max: number } {
  let min = 0
  let max = 0
  for (const p of points) {
    let up = 0
    let down = 0
    for (const v of Object.values(p.balances)) {
      if (v > 0) up += v
      else down += v
    }
    max = Math.max(max, up)
    min = Math.min(min, down)
  }
  return { min, max }
}

/**
 * The series ordered by the size of their latest balance, largest first, so the
 * biggest accounts form the base of each stack and the legend reads in order of
 * importance. Ties keep the server's (name) order.
 */
export function orderSeries(history: AccountHistory): AccountSeries[] {
  const latest = history.points[history.points.length - 1]?.balances ?? {}
  return [...history.series].sort((a, b) => Math.abs(latest[b.id] ?? 0) - Math.abs(latest[a.id] ?? 0))
}

/**
 * Flattens the history into one row per date with a key per series, as the
 * chart wants. A series with no balance at a date (an account opened later) is
 * zero there, so every row has every key.
 */
export function toRows(history: AccountHistory): Record<string, number | string>[] {
  return history.points.map((p) => {
    const row: Record<string, number | string> = { date: p.date }
    for (const s of history.series) row[s.id] = p.balances[s.id] ?? 0
    return row
  })
}

/**
 * Replaces the last (in-progress) date with `today`, using live balances where
 * they are known, so the chart ends at the same net worth the summary card shows.
 * `live` is keyed by series id; a series it doesn't mention (such as the closed
 * accounts) keeps its snapshot balance. Returns the history unchanged when there
 * are no points.
 */
export function withLiveBalances(history: AccountHistory, live: Record<string, number>, today: string): AccountHistory {
  const last = history.points[history.points.length - 1]
  if (!last) return history
  const balances = { ...last.balances }
  for (const s of history.series) {
    if (s.id in live) balances[s.id] = live[s.id]
  }
  return { ...history, points: [...history.points.slice(0, -1), { date: today, balances }] }
}

/**
 * A color per series: liabilities (a liability account type, or anything below
 * zero now, like a home underwater) take the liability shades and everything
 * else the asset colors, each cycling in `ordered` order.
 */
export function seriesColors(
  history: AccountHistory,
  ordered: AccountSeries[],
  assetColors: readonly string[],
  liabilityColors: readonly string[],
): Record<string, string> {
  const latest = history.points[history.points.length - 1]?.balances ?? {}
  let assets = 0
  let liabilities = 0
  const colors: Record<string, string> = {}
  for (const s of ordered) {
    colors[s.id] =
      LIABILITY_ACCOUNT_TYPES.has(s.accountType) || (latest[s.id] ?? 0) < 0
        ? liabilityColors[liabilities++ % liabilityColors.length]
        : assetColors[assets++ % assetColors.length]
  }
  return colors
}
