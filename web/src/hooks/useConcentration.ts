import useSWR from 'swr'
import { holdingsApi } from '../api/client'
import type { ConcentrationResult } from '../api/client'

export function useConcentration() {
  return useSWR<ConcentrationResult>('/holdings/concentration', () => holdingsApi.concentration())
}
