import useSWR from 'swr'
import { snapshotsApi } from '../api/client'
import type { BalanceSnapshot } from '../api/client'

export function useSnapshots(accountId: string) {
  return useSWR<BalanceSnapshot[]>(
    accountId ? `/accounts/${accountId}/snapshots` : null,
    () => snapshotsApi.list(accountId),
  )
}
