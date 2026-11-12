import useSWR from 'swr'
import { budgetApi } from '../api/client'
import { buildMonths, summarize } from '../utils/cashFlow'

/**
 * Income and spending for the last six months, plus this month's savings rate
 * and the average over the three months before it. The SWR keys match the
 * Budget tab's own, so the data is shared rather than fetched twice. Pass
 * `enabled: false` to skip fetching when nothing is going to show it.
 */
export function useCashFlow(enabled = true) {
  const now = new Date()
  const year = now.getFullYear()
  const key = (path: string, y: number) => (enabled ? `/budget/${path}?year=${y}` : null)

  const spendNow = useSWR(key('monthly-breakdown', year), () => budgetApi.monthlyBreakdown(year))
  const spendPrev = useSWR(key('monthly-breakdown', year - 1), () => budgetApi.monthlyBreakdown(year - 1))
  const incomeNow = useSWR(key('income-breakdown', year), () => budgetApi.incomeBreakdown(year))
  const incomePrev = useSWR(key('income-breakdown', year - 1), () => budgetApi.incomeBreakdown(year - 1))

  const months = buildMonths(
    { [year]: spendNow.data, [year - 1]: spendPrev.data },
    { [year]: incomeNow.data, [year - 1]: incomePrev.data },
    now,
  )
  return {
    months,
    ...summarize(months),
    isLoading: spendNow.isLoading || spendPrev.isLoading || incomeNow.isLoading || incomePrev.isLoading,
  }
}
