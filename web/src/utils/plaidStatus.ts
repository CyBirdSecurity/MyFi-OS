import type { PlaidItem } from '../api/client'

/**
 * Why a Plaid connection needs the user's attention, as a sentence for a
 * tooltip, or null when it is syncing normally (or we don't know yet).
 */
export function plaidSyncProblem(item: Pick<PlaidItem, 'status' | 'lastSyncError'> | undefined): string | null {
  if (!item || item.status === 'active') return null
  if (item.status === 'login_required') {
    return 'Your bank needs you to sign in again. Open this account to reconnect.'
  }
  return item.lastSyncError
    ? `The last sync failed: ${item.lastSyncError}`
    : 'The last sync failed. Open this account to review it.'
}
