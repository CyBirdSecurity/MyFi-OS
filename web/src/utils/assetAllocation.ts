import { ASSET_CLASS_LABELS, LIABILITY_ACCOUNT_TYPES } from '../api/client'
import type { AccountBalance, Holding } from '../api/client'

export interface AssetSlice {
  key: string
  name: string
  value: number
}

const EXTRA_LABELS: Record<string, string> = {
  real_estate: 'Real estate',
  vehicles: 'Vehicles',
  unitemized: 'Investments without holdings',
}

/** Where an account's balance goes when no holdings explain it. */
function remainderClass(accountType: string): string {
  if (accountType === 'checking' || accountType === 'savings' || accountType === 'money_market') return 'cash_equivalent'
  if (accountType === 'home') return 'real_estate'
  if (accountType === 'automobile') return 'vehicles'
  if (accountType === 'other_asset') return 'other'
  return 'unitemized'
}

/**
 * Assets split by what they're invested in. Holdings count by their asset
 * class; any part of an account's balance its holdings don't cover goes by the
 * account's kind (a home is real estate, a checking account is cash). Holdings
 * are scaled to the account balance so the total matches the other views.
 */
export function allocationByAssetClass(accounts: AccountBalance[], holdings: Holding[]): AssetSlice[] {
  const totals = new Map<string, number>()
  const add = (key: string, value: number) => totals.set(key, (totals.get(key) ?? 0) + value)

  for (const acc of accounts) {
    if (LIABILITY_ACCOUNT_TYPES.has(acc.accountType) || !(acc.balance > 0)) continue
    const held = holdings.filter((h) => h.accountId === acc.accountId && h.totalValue > 0)
    const heldTotal = held.reduce((sum, h) => sum + h.totalValue, 0)
    const scale = heldTotal > acc.balance ? acc.balance / heldTotal : 1
    for (const h of held) add(h.assetClass, h.totalValue * scale)
    const remainder = acc.balance - heldTotal * scale
    if (remainder >= 1) add(remainderClass(acc.accountType), remainder)
  }

  return [...totals]
    .filter(([, value]) => value > 0)
    .map(([key, value]) => ({ key, name: ASSET_CLASS_LABELS[key] ?? EXTRA_LABELS[key] ?? key, value }))
    .sort((a, b) => b.value - a.value)
}
