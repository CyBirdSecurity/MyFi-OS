import { useState } from 'react'
import { ArrowsClockwise } from '@phosphor-icons/react'
import { plaidApi } from '../../api/client'
import type { Account } from '../../api/client'
import { mutateAfterPlaidSync, usePlaidSyncInfo } from '../../hooks/usePlaid'

/**
 * "Plaid · synced …" badge with an inline refresh action, shown in the
 * account detail header for Plaid-linked accounts. The tooltip surfaces the
 * persisted last-sync status/error (from a scheduled sync too, not just one
 * triggered in this session) and how far back the synced transaction history
 * actually goes, so a "sync succeeded but I don't see more data" question can
 * be answered by hovering rather than digging through server logs.
 */
export function PlaidSyncBadge({ account }: { account: Account }) {
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState('')
  const { data: info } = usePlaidSyncInfo(account)

  if (account.externalProvider !== 'plaid' || !account.externalItemId) return null

  const handleSync = async () => {
    setSyncing(true)
    setError('')
    try {
      const res = await plaidApi.syncItem(account.externalItemId!)
      if (res.errors && res.errors.length > 0) setError(res.errors.join('; '))
      await mutateAfterPlaidSync()
    } catch (e: any) {
      setError(e.message ?? 'Sync failed.')
    } finally {
      setSyncing(false)
    }
  }

  const synced = account.lastSyncedAt
    ? `synced ${new Date(account.lastSyncedAt).toLocaleDateString()}`
    : 'not yet synced'

  // Persisted error takes priority (reflects the real last-sync outcome,
  // scheduled or manual); an error from a sync clicked just now overrides it
  // since it's more current than what the badge has re-fetched yet.
  const persistedError = info?.status !== 'active' ? info?.lastSyncError : undefined
  const displayError = error || persistedError

  const historyLine = info
    ? info.transactionCount > 0 && info.earliestTransactionDate
      ? `Transaction history: ${info.transactionCount} transaction${info.transactionCount !== 1 ? 's' : ''} synced, earliest dated ${new Date(info.earliestTransactionDate).toLocaleDateString()}.`
      : 'No transactions synced for this account yet.'
    : ''

  const tooltip = displayError
    ? `${displayError}${historyLine ? `\n${historyLine}` : ''}`
    : `Linked via Plaid — ${synced}. ${historyLine} Click to sync now.`

  return (
    <span
      className={`inline-flex items-center gap-1.5 text-xs border rounded px-1.5 py-0.5 ${
        displayError
          ? 'bg-negative-tint-bg text-negative border-negative/30'
          : 'bg-accent-tint-bg text-accent-text border-accent-tint-border'
      }`}
      title={tooltip}
    >
      Plaid · {displayError ? 'sync error' : synced}
      <button
        type="button"
        onClick={handleSync}
        disabled={syncing}
        aria-label="Sync now"
        className="hover:opacity-70 disabled:opacity-40"
      >
        <ArrowsClockwise size={12} weight="bold" className={syncing ? 'animate-spin' : ''} />
      </button>
    </span>
  )
}
