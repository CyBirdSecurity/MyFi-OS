import type { HomeDetails } from '../api/client'

/** Share of a debt paid off, 0–100, given its starting and current balance. */
export function payoffPercent(startBalance: number, currentBalance: number): number {
  const start = Math.abs(startBalance)
  if (start <= 0) return 0
  const paid = ((start - Math.abs(currentBalance)) / start) * 100
  return Math.min(Math.max(paid, 0), 100)
}

export interface Mortgage {
  accountId: string
  name: string
  originalLoan: number
  currentBalance: number
}

/**
 * The mortgage on a home account, or null when its loan details aren't filled
 * in (or the loan is already paid off).
 */
export function mortgageFromDetails(accountId: string, accountName: string, d: HomeDetails | null): Mortgage | null {
  if (!d || !(d.originalLoanValue > 0) || !(d.currentLoanBalance > 0)) return null
  return {
    accountId,
    // Matches how net worth names the same mortgage.
    name: `${accountName} (Mortgage)`,
    originalLoan: d.originalLoanValue,
    currentBalance: d.currentLoanBalance,
  }
}
