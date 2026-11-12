import useSWR from 'swr'
import { networthApi } from '../api/client'
import type { NetWorthCurrent, HistoryPoint, AccountHistory } from '../api/client'
import { monthsAgo, today } from '../utils/dates'

export function useNetWorthCurrent() {
  return useSWR<NetWorthCurrent>('/networth/current', () => networthApi.current(), {
    refreshInterval: 5 * 60 * 1000, // refresh every 5 minutes
  })
}

export function useNetWorthHistory(
  from = monthsAgo(12),
  to = today(),
  interval = 'month',
) {
  const key = `/networth/history?from=${from}&to=${to}&interval=${interval}`
  return useSWR<HistoryPoint[]>(key, () => networthApi.history(from, to, interval))
}

/** Net worth history by account. Pass `enabled: false` to skip the request when it isn't shown. */
export function useNetWorthAccountHistory(from: string, to: string, interval: string, enabled = true) {
  const key = `/networth/history/accounts?from=${from}&to=${to}&interval=${interval}`
  return useSWR<AccountHistory>(enabled ? key : null, () => networthApi.accountHistory(from, to, interval))
}
