import type { Account, Contribution, HouseholdMember, MatchTier } from '../api/client'

// Mirrors internal/services/contributions.go so the form can preview what the
// forecast will use.

type ContributionAmount = Pick<Contribution, 'amountType' | 'amount' | 'frequency' | 'incomePct' | 'matchTiers'>

const PAYMENTS_PER_YEAR = { monthly: 12, biweekly: 26, annually: 1 } as const

/** The member whose salary the account's contributions use: the attributed member, else 'self'. Joint accounts have none. */
export function accountOwner(
  account: Pick<Account, 'memberId' | 'isJoint'>,
  members: HouseholdMember[],
): HouseholdMember | undefined {
  if (account.isJoint) return undefined
  return account.memberId
    ? members.find((m) => m.id === account.memberId)
    : members.find((m) => m.relationship === 'self')
}

/** Yearly income, or 0 when the member isn't employed. */
export function annualSalary(member: HouseholdMember | undefined): number {
  return member?.isEmployed ? member.monthlyIncome * 12 : 0
}

/** A percent contribution or a salary-capped match can't be computed without the owner's salary. */
export function needsIncome(c: Pick<Contribution, 'amountType' | 'matchTiers'>): boolean {
  return c.amountType === 'percent' || c.matchTiers.some((t) => t.upToPct != null)
}

/** Applies each tier to the next slice of the employee contribution. */
export function employerMatch(tiers: MatchTier[], employee: number, salary: number): number {
  let remaining = employee
  let match = 0
  for (const t of tiers) {
    if (remaining <= 0) break
    const portion = t.upToPct == null ? remaining : Math.min(remaining, t.upToPct * salary)
    match += portion * t.matchPct
    remaining -= portion
  }
  return match
}

/** This year's employee contribution and employer match, in dollars. */
export function annualContribution(c: ContributionAmount, salary: number): { employee: number; match: number } {
  const employee = c.amountType === 'percent' ? c.incomePct * salary : c.amount * PAYMENTS_PER_YEAR[c.frequency]
  return { employee, match: employerMatch(c.matchTiers, employee, salary) }
}

const pctLabel = (fraction: number) => `${+(fraction * 100).toFixed(2)}%`

/** e.g. "100% of the first 3%, then 50% of the next 2%". */
export function describeMatch(tiers: MatchTier[]): string {
  if (tiers.length === 0) return '—'
  return tiers
    .map((t, i) => {
      const rate = pctLabel(t.matchPct)
      if (t.upToPct == null) return i === 0 ? `${rate} of contributions` : `${rate} of the rest`
      return `${rate} of the ${i === 0 ? 'first' : 'next'} ${pctLabel(t.upToPct)}`
    })
    .join(', then ')
}

/** "6% of salary" for a percent contribution, else "$500/mo". */
export function describeAmount(c: ContributionAmount, formatCurrency: (n: number) => string): string {
  if (c.amountType === 'percent') return `${pctLabel(c.incomePct)} of salary`
  const freq = c.frequency === 'monthly' ? '/mo' : c.frequency === 'biweekly' ? '/2wk' : '/yr'
  return `${formatCurrency(c.amount)}${freq}`
}
