import type { FIMetrics, ForecastYear } from '../api/client'

/**
 * The first projected year the portfolio reaches the FI number. The FI number
 * is in today's dollars, so in a nominal projection it grows by
 * `fiNumberGrowth` (inflation) each year. Uses the Monte Carlo median when
 * there is one. With no retirement spending the FI number is 0, which never
 * counts as reached.
 */
export function findFIYear(years: ForecastYear[], fi: Pick<FIMetrics, 'fiNumber' | 'fiNumberGrowth'>): ForecastYear | undefined {
  if (fi.fiNumber <= 0) return undefined
  return years.find((y, i) => (y.p50 ?? y.total) >= fi.fiNumber * Math.pow(1 + fi.fiNumberGrowth, i))
}

/**
 * The projected portfolio on the day you retire: the closing balance of the
 * last working year, since the retirement year's own row already has a year of
 * withdrawals taken out. When retiring this year there's no working year left,
 * so the retirement year is used. Mirrors portfolioAtRetirement in the
 * forecast service.
 */
export function yearAtRetirement(years: ForecastYear[], retirementYear: number): ForecastYear | undefined {
  return years.find((y) => y.year === retirementYear - 1) ?? years.find((y) => y.year === retirementYear)
}
