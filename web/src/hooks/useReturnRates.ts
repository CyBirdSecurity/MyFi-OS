import useSWR from 'swr'
import { returnRatesApi } from '../api/client'
import type { ReturnRateEntry } from '../api/client'

export function useReturnRates() {
  return useSWR<ReturnRateEntry[]>('/return-rates', () => returnRatesApi.list())
}
