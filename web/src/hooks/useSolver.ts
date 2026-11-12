import useSWR from 'swr'
import { forecastApi } from '../api/client'
import type { SolveParams, SolveResult } from '../api/client'

/**
 * Solves for the value of one variable (contribution, return rate, or
 * retirement age) needed to reach financial independence, holding the
 * others fixed. Pass null to skip (e.g. while the form is incomplete).
 */
export function useSolver(params: SolveParams | null) {
  const key = params ? `/forecast/solve:${JSON.stringify(params)}` : null

  return useSWR<SolveResult>(
    key,
    () => forecastApi.solve(params as SolveParams),
    { revalidateOnFocus: false },
  )
}
