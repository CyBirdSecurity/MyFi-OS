import type { MonthlyBreakdownRow, MonthlyIncomeRow } from '../api/client'

export interface MonthFlow {
  year: number
  month: number // 1-12
  label: string // "Oct"
  income: number
  spending: number
}

const MONTH_LABELS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export const saved = (m: Pick<MonthFlow, 'income' | 'spending'>) => m.income - m.spending

/**
 * The last `count` months ending at `now`'s month (oldest first), with each
 * month's income and total spending. Rows come per year from the Budget API
 * (spending has one row per category, so they are summed).
 */
export function buildMonths(
  spendingByYear: Record<number, MonthlyBreakdownRow[] | undefined>,
  incomeByYear: Record<number, MonthlyIncomeRow[] | undefined>,
  now: Date,
  count = 6,
): MonthFlow[] {
  const months: MonthFlow[] = []
  for (let offset = count - 1; offset >= 0; offset--) {
    const d = new Date(now.getFullYear(), now.getMonth() - offset, 1)
    const year = d.getFullYear()
    const month = d.getMonth() + 1
    months.push({
      year,
      month,
      label: MONTH_LABELS[month - 1],
      income: (incomeByYear[year] ?? []).filter((r) => r.month === month).reduce((s, r) => s + r.total, 0),
      spending: (spendingByYear[year] ?? []).filter((r) => r.month === month).reduce((s, r) => s + r.total, 0),
    })
  }
  return months
}

/** Share of income left after spending, in percent; null when there is no income to measure against. */
export function savingsRate(income: number, spending: number): number | null {
  return income > 0 ? ((income - spending) / income) * 100 : null
}

/**
 * Savings rate across several months together (total saved over total income),
 * so a big month counts for more than a small one. Months with no income are
 * left out; null if none have any.
 */
export function averageSavingsRate(months: Pick<MonthFlow, 'income' | 'spending'>[]): number | null {
  const withIncome = months.filter((m) => m.income > 0)
  const income = withIncome.reduce((s, m) => s + m.income, 0)
  if (income <= 0) return null
  const spent = withIncome.reduce((s, m) => s + m.spending, 0)
  return ((income - spent) / income) * 100
}

/**
 * The current month so far, plus the average rate over the three months before
 * it. The current month is partial (income may not have landed yet), so the
 * average deliberately uses completed months only.
 */
export function summarize(months: MonthFlow[]) {
  const current = months[months.length - 1]
  const completed = months.slice(0, -1).slice(-3)
  return {
    current,
    currentRate: current ? savingsRate(current.income, current.spending) : null,
    averageRate: averageSavingsRate(completed),
    hasData: months.some((m) => m.income > 0 || m.spending > 0),
  }
}
