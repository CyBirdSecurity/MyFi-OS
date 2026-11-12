import useSWR, { mutate } from 'swr'
import { configApi, plaidApi } from '../api/client'
import type { Account, PlaidItem, PlaidSyncInfo } from '../api/client'
import { refreshReviewCount } from './useReviewCount'

export function usePlaidConfig() {
  return useSWR('/config', () => configApi.get(), { revalidateOnFocus: false })
}

export function usePlaidItems(enabled = true) {
  return useSWR<PlaidItem[]>(enabled ? '/plaid/items' : null, () => plaidApi.listItems(), {
    // Poll while a backfill is actually in flight so "Backfilling…" flips to
    // "History backfilled" on its own, without the user reloading the page.
    refreshInterval: (items) =>
      items?.some((i) => i.backfillRequested && !i.backfilledAt) ? 15000 : 0,
  })
}

/** The Plaid item backing a linked account, or undefined if not Plaid-linked. */
export function usePlaidItemForAccount(account?: Account) {
  const linked = account?.externalProvider === 'plaid' && !!account.externalItemId
  const { data: items } = usePlaidItems(linked)
  if (!linked) return undefined
  return items?.find((i) => i.itemId === account!.externalItemId)
}

/** Revalidates every SWR key affected by a Plaid sync. */
export async function mutateAfterPlaidSync() {
  await Promise.all([
    mutate('/plaid/items'),
    mutate('/accounts'),
    mutate('/holdings'),
    refreshReviewCount(), // synced transactions may need review
    mutate((key) => typeof key === 'string' && (
      key.startsWith('/accounts/') || key.startsWith('/networth')
    )),
  ])
}

/**
 * Sync diagnostics for one Plaid-linked account — the persisted last-sync
 * status/error (scheduled or manual) plus the actual date range of synced
 * transaction data, so "did it fail, only get a few months, or something
 * else" can be answered directly instead of guessing from logs.
 */
export function usePlaidSyncInfo(account?: Account) {
  const linked = account?.externalProvider === 'plaid' && !!account.externalItemId
  return useSWR<PlaidSyncInfo>(
    linked ? `/accounts/${account!.id}/plaid-sync-info` : null,
    () => plaidApi.syncInfo(account!.id),
  )
}
