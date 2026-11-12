import { CASH_BALANCE_ACCOUNT_TYPES, NO_HOLDINGS_ACCOUNT_TYPES } from '../api/client'

export type AccountTab =
  | 'holdings'
  | 'transactions'
  | 'snapshots'
  | 'contributions'
  | 'cardDetails'
  | 'loanDetails'
  | 'amortization'
  | 'settings'

/** Types where planned contributions make no sense: a car or a home isn't something you fund. */
const NO_CONTRIBUTIONS_ACCOUNT_TYPES = new Set(['automobile', 'home'])

// Account types with a real Transactions tab backed by synced/imported
// transactions rather than manual holdings — checking/savings plus credit
// cards, which get the same Plaid banking-transaction sync. Deliberately not
// folded into CASH_BALANCE_ACCOUNT_TYPES: that set also drives "count this
// balance as cash_equivalent" on the Dashboard/Goals/Holdings pages, which a
// credit card's (liability) balance should never be.
export const TRANSACTIONS_TAB_ACCOUNT_TYPES = new Set([...CASH_BALANCE_ACCOUNT_TYPES, 'credit_card'])

/**
 * The tabs on an account's detail page, in order. A credit card has Card Details
 * (APR, limit, statement and payment info) and a loan has Loan Details (rate and
 * payment) where other accounts have Contributions: you pay debts down rather
 * than make planned contributions to them.
 */
export function getTabsForAccountType(accountType: string): { key: AccountTab; label: string }[] {
  const hideHoldings = NO_HOLDINGS_ACCOUNT_TYPES.has(accountType)
  const isCreditCard = accountType === 'credit_card'
  const isLoan = accountType === 'loan' || accountType === 'other_liability'
  const hideContributions = NO_CONTRIBUTIONS_ACCOUNT_TYPES.has(accountType) || isCreditCard || isLoan
  const snapshotLabel =
    accountType === 'home' ? 'Equity History' : accountType === 'automobile' ? 'Value History' : 'Snapshots'
  return [
    ...(!hideHoldings ? [{ key: 'holdings' as AccountTab, label: 'Holdings' }] : []),
    ...(TRANSACTIONS_TAB_ACCOUNT_TYPES.has(accountType) ? [{ key: 'transactions' as AccountTab, label: 'Transactions' }] : []),
    { key: 'snapshots' as AccountTab, label: snapshotLabel },
    ...(!hideContributions ? [{ key: 'contributions' as AccountTab, label: 'Contributions' }] : []),
    ...(isCreditCard ? [{ key: 'cardDetails' as AccountTab, label: 'Card Details' }] : []),
    ...(isLoan ? [{ key: 'loanDetails' as AccountTab, label: 'Loan Details' }] : []),
    ...(accountType === 'home' ? [{ key: 'amortization' as AccountTab, label: 'Amortization' }] : []),
    { key: 'settings' as AccountTab, label: 'Settings' },
  ]
}
