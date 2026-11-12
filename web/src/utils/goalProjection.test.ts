import { describe, expect, it } from 'vitest'
import type { AccountMeta, ForecastYear, Goal } from '../api/client'
import { findGoalHitYear } from './goalProjection'

const goal = (over: Partial<Goal>): Goal => ({
  id: 'g1',
  name: 'Goal',
  goalType: 'net_worth',
  targetAmount: 50_000,
  isAchieved: false,
  createdAt: '2026-01-01',
  ...over,
})

const meta: AccountMeta[] = [
  { id: 'a', name: 'Savings', accountType: 'hysa', ownerUserId: 'u', countsTowardPortfolio: true },
  { id: 'b', name: '401k', accountType: 'traditional_401k', ownerUserId: 'u', countsTowardPortfolio: true },
  { id: 'c', name: 'Other 401k', accountType: 'traditional_401k', ownerUserId: 'u', countsTowardPortfolio: true },
]

// The 401k is large enough that the portfolio total passes 50k immediately,
// while the savings account alone only gets there in 2029.
const years: ForecastYear[] = [2026, 2027, 2028, 2029].map((year, i) => ({
  year,
  age: 40 + i,
  accounts: { a: 20_000 + i * 10_000, b: 200_000, c: 10_000 },
  total: 230_000 + i * 10_000,
}))

describe('findGoalHitYear', () => {
  it('measures an account goal against that account, not the portfolio total', () => {
    expect(findGoalHitYear(goal({ goalType: 'account', accountId: 'a' }), years, meta)?.year).toBe(2029)
  })

  it('is never reached when the account is missing from the forecast', () => {
    expect(findGoalHitYear(goal({ goalType: 'account', accountId: 'zzz' }), years, meta)).toBeUndefined()
  })

  it('sums every account of the type for an account-type goal', () => {
    const g = goal({ goalType: 'account_type', accountTypeFilter: 'traditional_401k', targetAmount: 210_000 })
    expect(findGoalHitYear(g, years, meta)?.year).toBe(2026)
    expect(findGoalHitYear({ ...g, targetAmount: 210_001 }, years, meta)).toBeUndefined()
  })

  it('uses the portfolio total for net worth goals', () => {
    expect(findGoalHitYear(goal({ targetAmount: 245_000 }), years, meta)?.year).toBe(2028)
  })
})
