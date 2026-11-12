import useSWR from 'swr'
import { retirementApi } from '../api/client'
import type { RetirementPlan } from '../api/client'

/**
 * Fetches the retirement depletion plan for the given monthly spending amount.
 * Pass `monthlySpending` as a number (or undefined to use the profile default).
 * The SWR key changes when the spending value changes so results auto-refresh.
 */
export function useRetirement(monthlySpending?: number, enabled = true) {
  const key = !enabled ? null : monthlySpending != null
    ? `/retirement?monthlySpending=${monthlySpending}`
    : '/retirement'

  return useSWR<RetirementPlan>(key, () => retirementApi.get(monthlySpending), {
    revalidateOnFocus: false,
  })
}
