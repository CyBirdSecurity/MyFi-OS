import useSWR from 'swr'
import { budgetApi } from '../api/client'
import { suggestLimits } from '../utils/budgetLimits'

/** The user's monthly category limits (empty until they set some). */
export function useBudgetLimits() {
  return useSWR('/budget/limits', () => budgetApi.listLimits())
}

/**
 * Suggested limits by category name, from the average of the last three
 * completed months. Uses the same SWR keys as the rest of the Budget tab, so the
 * spending data is shared rather than fetched twice.
 */
export function useLimitSuggestions(): Map<string, number> {
  const year = new Date().getFullYear()
  const { data: thisYear } = useSWR(`/budget/monthly-breakdown?year=${year}`, () => budgetApi.monthlyBreakdown(year))
  const { data: lastYear } = useSWR(`/budget/monthly-breakdown?year=${year - 1}`, () => budgetApi.monthlyBreakdown(year - 1))
  return suggestLimits({ [year]: thisYear, [year - 1]: lastYear }, new Date())
}
