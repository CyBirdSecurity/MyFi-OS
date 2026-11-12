import type { AccountMeta, ForecastYear, Goal } from '../api/client'

/**
 * The projected value a goal is measured against in one forecast year: the
 * goal's own account for account goals, the sum of matching accounts for
 * account-type goals, and the portfolio total otherwise.
 */
export function projectedGoalValue(goal: Goal, year: ForecastYear, accountMeta: AccountMeta[]): number {
  if (goal.goalType === 'account' && goal.accountId) {
    return year.accounts[goal.accountId] ?? 0
  }
  if (goal.goalType === 'account_type' && goal.accountTypeFilter) {
    return accountMeta
      .filter((m) => m.accountType === goal.accountTypeFilter)
      .reduce((sum, m) => sum + (year.accounts[m.id] ?? 0), 0)
  }
  return year.p50 ?? year.total
}

/** The first forecast year in which the goal's projected value reaches its target. */
export function findGoalHitYear(goal: Goal, years: ForecastYear[], accountMeta: AccountMeta[]): ForecastYear | undefined {
  return years.find((y) => projectedGoalValue(goal, y, accountMeta) >= goal.targetAmount)
}
