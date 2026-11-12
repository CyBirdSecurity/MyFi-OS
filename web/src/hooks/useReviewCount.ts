import useSWR, { mutate } from 'swr'
import { budgetApi } from '../api/client'

const KEY = 'budget/review-count'

/**
 * Transactions waiting for review (nav badges, dashboard, import page). SWR
 * refreshes it on mount and when the tab regains focus; budget edits call
 * refreshReviewCount, and a slow poll catches background syncs.
 */
export function useReviewCount() {
  const { data } = useSWR(KEY, () => budgetApi.getReviewCount(), { refreshInterval: 5 * 60 * 1000 })
  return data?.count ?? 0
}

export const refreshReviewCount = () => mutate(KEY)
