import { Link } from 'react-router-dom'
import { useAccounts } from '../../hooks/useAccounts'
import { useContributionProgress } from '../../hooks/useContributions'

/** Lists retirement accounts whose contributions this year have drifted from the plan. */
export function ContributionCheckCard() {
  const { data: progress } = useContributionProgress()
  const { data: accounts } = useAccounts()
  const offPlan = (progress ?? []).filter((p) => p.employeeOffPlan || p.employerOffPlan)
  if (!offPlan.length || !accounts) return null

  const nameOf = (id: string) => accounts.find((a) => a.id === id)?.name ?? 'Account'
  return (
    <div className="bg-warning-tint-bg rounded-lg px-4 py-3 text-sm text-text-primary">
      <span className="font-medium">Contributions don't match your plan:</span>{' '}
      {offPlan.map((p, i) => (
        <span key={p.accountId}>
          {i > 0 && ', '}
          <Link to={`/accounts/${p.accountId}?tab=contributions`} className="underline underline-offset-2 hover:text-accent-text">
            {nameOf(p.accountId)}
          </Link>
        </span>
      ))}
      . Review them so your forecast stays accurate.
    </div>
  )
}
