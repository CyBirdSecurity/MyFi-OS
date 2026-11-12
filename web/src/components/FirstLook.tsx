import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { ArrowRight } from '@phosphor-icons/react'
import { contributionsApi, taxApi, acceptsContributions, type Contribution, type TaxAnalysis } from '../api/client'
import { useAccounts } from '../hooks/useAccounts'
import { useNetWorthCurrent } from '../hooks/useNetWorth'
import { useProfile } from '../hooks/useProfile'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'
import { useRetirement } from '../hooks/useRetirement'
import { usePlaidConfig, usePlaidItems } from '../hooks/usePlaid'
import { formatCurrency } from '../utils/currency'
import { Button } from './ui/Button'

/**
 * The end of onboarding: what MyFi already worked out from the user's
 * answers, and what each missing piece of data would add.
 */
export function FirstLook({ onDone }: { onDone: () => void }) {
  const { data: accounts } = useAccounts()
  const { data: nw } = useNetWorthCurrent()
  const { data: profile } = useProfile()
  // /household/members and /contributions return null (not []) when empty.
  const { data: members } = useHouseholdMembers()
  const { data: contributions, isLoading: contributionsLoading } = useSWR<Contribution[] | null>(
    '/contributions',
    () => contributionsApi.listAll(),
  )
  const { data: config } = usePlaidConfig()
  const { data: plaidItems } = usePlaidItems(!!config?.plaidEnabled)

  const income = (members ?? []).reduce((sum, m) => sum + (m.monthlyIncome ?? 0) * 12, 0)
  const hasSpending = (profile?.monthlyExpenses ?? 0) > 0
  const hasAccounts = (accounts ?? []).length > 0
  const { data: plan } = useRetirement(undefined, !!profile?.birthday && hasAccounts)
  const { data: tax } = useSWR<TaxAnalysis>(income > 0 ? '/taxes' : null, () => taxApi.analyze())

  if (!profile || !accounts || contributionsLoading) return null

  const savingsAccount = accounts.find((a) => acceptsContributions(a.accountType))
  const b = tax?.bracketAnalysis

  const insights: { label: string; value: string; detail: string; to: string }[] = []
  if (hasAccounts && nw) {
    insights.push({
      label: 'Net worth',
      value: formatCurrency(nw.totalNetWorth),
      detail: `Across ${accounts.length} ${accounts.length === 1 ? 'account' : 'accounts'}, and tracked from today.`,
      to: '/dashboard',
    })
  }
  if (plan) {
    insights.push({
      label: `At retirement (age ${plan.retirementAge})`,
      value: formatCurrency(plan.retirementPortfolio),
      detail: !hasSpending
        ? 'Projected portfolio. Add retirement spending to see how long it lasts.'
        : plan.depletionAge
          ? `At ${formatCurrency(plan.monthlySpending)}/mo, it runs out at age ${plan.depletionAge}.`
          : `At ${formatCurrency(plan.monthlySpending)}/mo, it lasts past age 100.`,
      to: '/retirement',
    })
  }
  if (b) {
    insights.push({
      label: 'Estimated taxes this year',
      value: formatCurrency(b.totalTaxOwed),
      detail: `${Math.round(b.marginalRate * 100)}% bracket, with ${formatCurrency(b.bracketHeadroom)} of room before the next one.`,
      to: '/taxes',
    })
  }

  // What each missing piece adds, most valuable first.
  const nextSteps: { label: string; detail: string; to: string }[] = []
  if (!hasAccounts) {
    nextSteps.push({ label: 'Add your accounts', detail: 'Your net worth, forecast and allocation are all built from them.', to: '/accounts' })
  }
  if (hasAccounts && !contributions?.length) {
    nextSteps.push({
      label: 'Add what you save each month',
      detail: 'Your forecast only grows today’s balances until it knows what you add.',
      to: savingsAccount ? `/accounts/${savingsAccount.id}?tab=contributions` : '/accounts',
    })
  }
  if (!hasSpending) {
    nextSteps.push({ label: 'Set your retirement spending', detail: 'See how long your money lasts and your FI number.', to: '/retirement' })
  }
  if (income <= 0) {
    nextSteps.push({ label: 'Add your income', detail: 'Unlocks tax brackets, Roth conversion planning and your savings rate.', to: '/settings' })
  }
  if (config?.plaidEnabled && plaidItems && plaidItems.length === 0) {
    nextSteps.push({
      label: 'Connect your bank with Plaid',
      detail: 'Balances, holdings and transactions then update on their own, and your Budget fills itself in.',
      to: '/accounts?connect=1',
    })
  } else if (!config?.plaidEnabled) {
    nextSteps.push({ label: 'Import a bank statement', detail: 'Fills in your Budget, spending categories and subscriptions.', to: '/import' })
  }

  return (
    <div className="space-y-6">
      {insights.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {insights.map((i) => (
            <Link
              key={i.label}
              to={i.to}
              className="bg-surface border border-border-default rounded-lg p-4 hover:border-accent-tint-border transition-colors"
            >
              <div className="text-xs text-text-tertiary">{i.label}</div>
              <div className="mt-1 text-2xl font-bold text-text-primary">{i.value}</div>
              <div className="mt-1 text-xs text-text-secondary">{i.detail}</div>
            </Link>
          ))}
        </div>
      )}

      {nextSteps.length > 0 && (
        <div className="bg-surface border border-border-default rounded-lg p-6 space-y-3">
          <h2 className="text-sm font-semibold text-text-primary">Sharpen these numbers</h2>
          <ul className="space-y-1">
            {nextSteps.map((s) => (
              <li key={s.label}>
                <Link
                  to={s.to}
                  className="flex items-start gap-3 rounded-lg px-3 py-2 -mx-3 hover:bg-subtle transition-colors"
                >
                  <ArrowRight size={16} weight="bold" className="mt-0.5 shrink-0 text-accent-text" />
                  <span>
                    <span className="block text-sm font-medium text-text-primary">{s.label}</span>
                    <span className="block text-xs text-text-tertiary">{s.detail}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}

      <Button onClick={onDone} className="w-full justify-center">
        Go to my dashboard
      </Button>
    </div>
  )
}
