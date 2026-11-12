import { describe, expect, it } from 'vitest'
import { getTabsForAccountType } from './accountTabs'

const labels = (type: string) => getTabsForAccountType(type).map((t) => t.label)
const keys = (type: string) => getTabsForAccountType(type).map((t) => t.key)

describe('getTabsForAccountType', () => {
  it('gives a credit card Card Details instead of Contributions', () => {
    expect(labels('credit_card')).toEqual(['Transactions', 'Snapshots', 'Card Details', 'Settings'])
    expect(keys('credit_card')).not.toContain('contributions')
  })

  it('gives loans Loan Details instead of Contributions', () => {
    for (const type of ['loan', 'other_liability']) {
      expect(keys(type), type).toContain('loanDetails')
      expect(keys(type), type).not.toContain('contributions')
    }
  })

  it('leaves Contributions on the accounts you do fund', () => {
    for (const type of ['checking', 'savings', 'brokerage', 'roth_ira', '401k']) {
      expect(labels(type), type).toContain('Contributions')
      expect(labels(type), type).not.toContain('Card Details')
    }
  })

  it('keeps homes and vehicles without Contributions, and homes with Amortization', () => {
    expect(labels('home')).toEqual(['Equity History', 'Amortization', 'Settings'])
    expect(labels('automobile')).toEqual(['Value History', 'Settings'])
  })

  it('keeps the rest of a credit card unchanged: no Holdings, Transactions first', () => {
    expect(keys('credit_card')[0]).toBe('transactions')
    expect(keys('credit_card')).not.toContain('holdings')
  })

  it('keeps Holdings and Transactions as they were for a checking account', () => {
    expect(labels('checking')).toEqual(['Holdings', 'Transactions', 'Snapshots', 'Contributions', 'Settings'])
  })
})
