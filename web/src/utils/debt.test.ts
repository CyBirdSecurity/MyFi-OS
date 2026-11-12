import { describe, expect, it } from 'vitest'
import type { HomeDetails } from '../api/client'
import { mortgageFromDetails, payoffPercent } from './debt'

const details = (over: Partial<HomeDetails>): HomeDetails => ({
  accountId: 'h1',
  address: '1 Main St',
  interestRate: 0.06,
  loanTermYears: 30,
  originalLoanValue: 300_000,
  currentLoanBalance: 240_000,
  ...over,
})

describe('payoffPercent', () => {
  it('is the share of the starting balance that has been paid', () => {
    expect(payoffPercent(300_000, 240_000)).toBe(20)
    expect(payoffPercent(1000, 0)).toBe(100)
  })

  it('treats liabilities stored as negative balances the same as positive ones', () => {
    expect(payoffPercent(-1000, -250)).toBe(75)
  })

  it('clamps: a balance that grew is 0% paid, never negative', () => {
    expect(payoffPercent(1000, 1500)).toBe(0)
  })

  it('is 0 when there is no starting balance to measure against', () => {
    expect(payoffPercent(0, 500)).toBe(0)
  })
})

describe('mortgageFromDetails', () => {
  it('describes a home with loan details', () => {
    expect(mortgageFromDetails('h1', 'Primary Residence', details({}))).toEqual({
      accountId: 'h1',
      name: 'Primary Residence (Mortgage)',
      originalLoan: 300_000,
      currentBalance: 240_000,
    })
  })

  it('is null for a home with no loan filled in, a paid-off loan, or no details', () => {
    expect(mortgageFromDetails('h1', 'Home', details({ originalLoanValue: 0, currentLoanBalance: 0 }))).toBeNull()
    expect(mortgageFromDetails('h1', 'Home', details({ currentLoanBalance: 0 }))).toBeNull()
    expect(mortgageFromDetails('h1', 'Home', null)).toBeNull()
  })
})
