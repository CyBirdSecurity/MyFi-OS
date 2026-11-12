import useSWR from 'swr'
import { accountsApi } from '../api/client'
import type { Account } from '../api/client'

export function useAccounts() {
  return useSWR<Account[]>('/accounts', () => accountsApi.list())
}

export function useAccount(id: string) {
  return useSWR<Account>(id ? `/accounts/${id}` : null, () => accountsApi.get(id))
}
