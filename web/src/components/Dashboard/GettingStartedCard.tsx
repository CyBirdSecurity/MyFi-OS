import { useState } from 'react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { CheckCircle, Circle, X } from '@phosphor-icons/react'
import { useAccounts } from '../../hooks/useAccounts'
import { useGoals } from '../../hooks/useGoals'
import { useProfile } from '../../hooks/useProfile'
import { contributionsApi, acceptsContributions, type Contribution } from '../../api/client'
import { useGuideDismissal } from '../PageGuide'

/**
 * A short setup checklist for new users. Each step is checked off from data
 * the user has already entered, and the card disappears once every step is
 * done or the user dismisses it.
 */
export function GettingStartedCard() {
  const { pending, dismiss } = useGuideDismissal('getting-started')
  const [open, setOpen] = useState<boolean | null>(null)
  const { data: accounts } = useAccounts()
  const { data: goals, isLoading: goalsLoading } = useGoals()
  const { data: profile } = useProfile()
  // Same key the Forecast page uses, so the two share one fetch.
  const { data: contributions, isLoading: contributionsLoading } = useSWR<Contribution[] | null>(
    pending ? '/contributions' : null,
    () => contributionsApi.listAll(),
  )

  // /goals and /contributions return null (not []) when the user has none.
  if (!pending || !accounts || goalsLoading || contributionsLoading || !profile) return null

  // Monthly savings are set per account; point at the first one you'd save into.
  const savingsAccount = accounts.find((a) => acceptsContributions(a.accountType))

  const steps = [
    { label: 'Add your first account', to: '/accounts', done: accounts.length > 0 },
    {
      label: 'Set your retirement spending',
      to: '/retirement',
      done: profile.monthlyExpenses > 0 || profile.useBudgetForExpenses,
    },
    {
      label: 'Add what you save each month',
      to: savingsAccount ? `/accounts/${savingsAccount.id}?tab=contributions` : '/accounts',
      done: (contributions ?? []).length > 0,
    },
    { label: 'Create a goal', to: '/goals', done: (goals ?? []).length > 0 },
  ]
  const completed = steps.filter((s) => s.done).length
  if (completed === steps.length) return null

  const next = steps.find((s) => !s.done)!
  // Open until the user hides it, so the remaining steps aren't missed.
  const isOpen = open ?? true

  return (
    <div className="bg-surface rounded-lg border border-accent-tint-border">
      <div className="flex items-center gap-3 px-4 py-3 text-sm">
        <div className="flex gap-1 shrink-0" aria-label={`${completed} of ${steps.length} setup steps done`}>
          {steps.map((s) => (
            <span key={s.to} className={`h-1.5 w-4 rounded-sm ${s.done ? 'bg-accent' : 'bg-muted'}`} />
          ))}
        </div>
        <span className="text-text-tertiary hidden sm:inline shrink-0">Getting started</span>
        <Link to={next.to} className="text-text-primary hover:text-accent-text truncate min-w-0">
          Next: {next.label} →
        </Link>
        <button
          onClick={() => setOpen(!isOpen)}
          className="ml-auto text-xs text-text-tertiary hover:text-text-primary shrink-0"
          aria-expanded={isOpen}
        >
          {isOpen ? 'Hide' : 'All steps'}
        </button>
        <button
          onClick={dismiss}
          aria-label="Dismiss getting started"
          className="text-text-muted hover:text-text-primary transition-colors shrink-0"
        >
          <X size={16} weight="bold" />
        </button>
      </div>
      {isOpen && (
        <ul className="grid grid-cols-1 sm:grid-cols-2 gap-1 px-2 pb-2">
          {steps.map((s) => (
            <li key={s.to}>
              <Link
                to={s.to}
                className="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-subtle transition-colors"
              >
                {s.done ? (
                  <CheckCircle size={18} weight="fill" className="text-positive shrink-0" />
                ) : (
                  <Circle size={18} weight="bold" className="text-text-muted shrink-0" />
                )}
                <span className={s.done ? 'text-text-tertiary line-through' : 'text-text-primary'}>{s.label}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
