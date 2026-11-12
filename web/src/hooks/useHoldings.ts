import useSWR from 'swr'
import { holdingsApi } from '../api/client'
import type { Holding } from '../api/client'

export function useHoldings() {
  return useSWR<Holding[]>('/holdings', () => holdingsApi.listAll())
}

export function useHoldingsByAccount(accountId: string) {
  return useSWR<Holding[]>(
    accountId ? `/accounts/${accountId}/holdings` : null,
    () => holdingsApi.listByAccount(accountId),
  )
}
