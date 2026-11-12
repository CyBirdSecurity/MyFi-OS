import { describe, expect, it } from 'vitest'
import type { HouseholdMember, MatchTier } from '../api/client'
import { accountOwner, annualContribution, annualSalary, describeMatch, employerMatch, needsIncome } from './contributions'

const safeHarbor: MatchTier[] = [
  { matchPct: 1, upToPct: 0.03 },
  { matchPct: 0.5, upToPct: 0.02 },
]

const member = (over: Partial<HouseholdMember>): HouseholdMember => ({
  id: 'm',
  userId: 'u',
  relationship: 'self',
  name: 'Sam',
  isEmployed: true,
  monthlyIncome: 10_000,
  salaryGrowthPct: 0,
  retirementAge: 65,
  useSocialSecurity: false,
  socialSecurityMonthly: 0,
  socialSecurityStartAge: 67,
  createdAt: '',
  updatedAt: '',
  ...over,
})

describe('employerMatch', () => {
  it('fills each tier in order and stops at the caps', () => {
    expect(employerMatch(safeHarbor, 2_000, 100_000)).toBe(2_000)
    expect(employerMatch(safeHarbor, 4_000, 100_000)).toBe(3_500)
    expect(employerMatch(safeHarbor, 10_000, 100_000)).toBe(4_000)
  })

  it('matches everything on an uncapped tier', () => {
    expect(employerMatch([{ matchPct: 0.5, upToPct: null }], 10_000, 0)).toBe(5_000)
  })
})

describe('annualContribution', () => {
  it('takes a percent of salary', () => {
    const c = { amountType: 'percent' as const, amount: 0, frequency: 'monthly' as const, incomePct: 0.06, matchTiers: safeHarbor }
    expect(annualContribution(c, 100_000)).toEqual({ employee: 6_000, match: 4_000 })
  })

  it('annualizes a fixed amount', () => {
    const c = { amountType: 'fixed' as const, amount: 100, frequency: 'biweekly' as const, incomePct: 0, matchTiers: [] }
    expect(annualContribution(c, 0)).toEqual({ employee: 2_600, match: 0 })
  })
})

describe('needsIncome', () => {
  it('is true for a percent contribution or a capped match only', () => {
    expect(needsIncome({ amountType: 'percent', matchTiers: [] })).toBe(true)
    expect(needsIncome({ amountType: 'fixed', matchTiers: safeHarbor })).toBe(true)
    expect(needsIncome({ amountType: 'fixed', matchTiers: [{ matchPct: 1, upToPct: null }] })).toBe(false)
  })
})

describe('accountOwner', () => {
  const self = member({ id: 's' })
  const partner = member({ id: 'p', relationship: 'partner' })

  it('is the attributed member, else self, and nobody for a joint account', () => {
    expect(accountOwner({ memberId: 'p', isJoint: false }, [self, partner])).toBe(partner)
    expect(accountOwner({ isJoint: false }, [partner, self])).toBe(self)
    expect(accountOwner({ isJoint: true }, [self, partner])).toBeUndefined()
  })

  it('has no salary when not employed', () => {
    expect(annualSalary(member({ isEmployed: false }))).toBe(0)
    expect(annualSalary(member({}))).toBe(120_000)
  })
})

describe('describeMatch', () => {
  it('reads like an employer plan', () => {
    expect(describeMatch(safeHarbor)).toBe('100% of the first 3%, then 50% of the next 2%')
    expect(describeMatch([{ matchPct: 0.5, upToPct: null }])).toBe('50% of contributions')
    expect(describeMatch([])).toBe('—')
  })
})
