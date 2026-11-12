import useSWR from 'swr'
import { goalsApi } from '../api/client'
import type { Goal } from '../api/client'

export function useGoals() {
  return useSWR<Goal[]>('/goals', () => goalsApi.list())
}
