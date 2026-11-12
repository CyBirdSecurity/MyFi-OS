import useSWR from 'swr'
import { homeApi } from '../api/client'
import type { AmortizationResult } from '../api/client'

export function useAmortization(accountId: string, extra?: number) {
  const key = extra != null && extra > 0
    ? `/accounts/${accountId}/amortization?extra=${extra}`
    : `/accounts/${accountId}/amortization`

  return useSWR<AmortizationResult>(
    accountId ? key : null,
    () => homeApi.getAmortization(accountId, extra),
    { revalidateOnFocus: false },
  )
}
