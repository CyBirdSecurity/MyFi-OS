import { useState } from 'react'
import { mutate } from 'swr'
import {
  contributionEntriesApi,
  contributionsApi,
  EMPLOYER_MATCH_ACCOUNT_TYPES,
  type Account,
  type Contribution,
  type ContributionEntry,
  type ContributionEntryInput,
  type ContributionProgress,
  type ContributionSource,
} from '../api/client'
import { useContributionEntries, useContributionProgress } from '../hooks/useContributions'
import { formatCurrency } from '../utils/currency'
import { formatDate, toDateInputValue, today } from '../utils/dates'
import { Card } from './ui/Card'
import { Button } from './ui/Button'
import { Modal } from './ui/Modal'

const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

const FREQ_LABEL = { monthly: '/mo', biweekly: '/paycheck', annually: '/yr' } as const

// Entry dates are calendar days; drop the time so they don't shift by timezone.
const day = (iso: string) => iso.slice(0, 10)

/** "18% behind" / "12% ahead of" */
function drift(actual: number, expected: number) {
  const pct = Math.round((Math.abs(actual - expected) / expected) * 100)
  return actual < expected ? `${pct}% behind` : `${pct}% ahead of`
}

function refresh(accountId: string) {
  mutate(`/accounts/${accountId}/contribution-entries`)
  mutate('/contribution-progress')
}

/**
 * This year's actual contributions to a retirement account — synced from
 * Plaid or logged by hand — compared with the plan, with a one-click plan
 * update when they've drifted.
 */
export function ContributionsThisYear({ account, plans }: { account: Account; plans: Contribution[] }) {
  const { data: entries } = useContributionEntries(account.id)
  const { data: allProgress } = useContributionProgress()
  const [logging, setLogging] = useState<ContributionEntry | 'new' | null>(null)
  const progress = allProgress?.find((p) => p.accountId === account.id)
  const isSynced = !!account.externalProvider
  const showEmployer =
    EMPLOYER_MATCH_ACCOUNT_TYPES.has(account.accountType) || !!progress?.employerActual || !!progress?.employerExpected

  const setSource = async (e: ContributionEntry, source: ContributionSource) => {
    await contributionEntriesApi.update(account.id, e.id, {
      entryDate: day(e.entryDate),
      amount: e.amount,
      source,
      description: e.description,
    })
    refresh(account.id)
  }

  const remove = async (e: ContributionEntry) => {
    if (!confirm('Remove this contribution?')) return
    await contributionEntriesApi.delete(account.id, e.id)
    refresh(account.id)
  }

  return (
    <Card
      title="This Year So Far"
      action={
        !isSynced && (
          <Button size="sm" variant="secondary" onClick={() => setLogging('new')}>
            + Log Contribution
          </Button>
        )
      }
    >
      {!progress ? (
        <p className="text-text-tertiary text-sm">
          {isSynced
            ? `${account.institution ?? 'This institution'} hasn't reported any contributions to this account in the past year, so there's nothing to compare with your plan yet. Some providers only share balances.`
            : 'Log your contributions since January 1 to see how they compare with your plan.'}
        </p>
      ) : (
        <>
          <div className={`grid gap-3 ${showEmployer ? 'grid-cols-2' : 'grid-cols-1'}`}>
            <Stat label="From you" actual={progress.employeeActual} expected={progress.employeeExpected} asOf={progress.asOf} />
            {showEmployer && (
              <Stat label="From your employer" actual={progress.employerActual} expected={progress.employerExpected} asOf={progress.asOf} />
            )}
          </div>
          {!isSynced && (
            <p className="text-xs text-text-muted mt-2">
              Compared with your plan since January 1, so log every contribution made this year.
            </p>
          )}
          <OffPlanNotice account={account} plans={plans} progress={progress} />
        </>
      )}

      {!!entries?.length && (
        <div className="overflow-x-auto mt-4">
          <table className="w-full text-sm min-w-[480px]">
            <thead>
              <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                <th className="pb-2 font-medium">Date</th>
                <th className="pb-2 font-medium">Description</th>
                <th className="pb-2 font-medium text-right">Amount</th>
                <th className="pb-2 font-medium pl-4">From</th>
                <th className="pb-2"></th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <tr key={e.id} className="border-b border-border-default/50">
                  <td className="py-2 text-text-secondary whitespace-nowrap">{formatDate(day(e.entryDate), 'MMM d')}</td>
                  <td className="py-2 text-text-secondary">{e.description || '—'}</td>
                  <td className="py-2 text-text-primary font-medium text-right">{formatCurrency(e.amount)}</td>
                  <td className="py-2 pl-4">
                    <select
                      value={e.source}
                      onChange={(ev) => setSource(e, ev.target.value as ContributionSource)}
                      aria-label="Whose contribution"
                      className="bg-transparent text-text-secondary text-sm focus:outline-none"
                    >
                      <option value="employee">You</option>
                      <option value="employer">Employer</option>
                    </select>
                  </td>
                  <td className="py-2 text-right whitespace-nowrap">
                    {!e.isSynced && (
                      <span className="flex gap-3 justify-end">
                        <button onClick={() => setLogging(e)} className="text-xs text-text-secondary hover:text-text-primary">
                          Edit
                        </button>
                        <button onClick={() => remove(e)} className="text-xs text-text-muted hover:text-negative">
                          Delete
                        </button>
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {isSynced && (
            <p className="text-xs text-text-muted mt-2">
              Synced from {account.institution ?? 'your institution'}. Who each contribution came from is a best guess
              from its description; change it if it's wrong.
            </p>
          )}
        </div>
      )}

      <Modal open={logging !== null} onClose={() => setLogging(null)} title={logging === 'new' ? 'Log Contribution' : 'Edit Contribution'}>
        {logging !== null && (
          <EntryForm
            initial={logging === 'new' ? undefined : logging}
            showEmployer={showEmployer}
            onCancel={() => setLogging(null)}
            onSubmit={async (data) => {
              if (logging === 'new') await contributionEntriesApi.create(account.id, data)
              else await contributionEntriesApi.update(account.id, logging.id, data)
              refresh(account.id)
              setLogging(null)
            }}
          />
        )}
      </Modal>
    </Card>
  )
}

function Stat({ label, actual, expected, asOf }: { label: string; actual: number; expected: number; asOf: string }) {
  return (
    <div className="bg-subtle rounded-lg px-4 py-3">
      <div className="text-xs text-text-tertiary">{label}</div>
      <div className="text-lg font-semibold text-text-primary">{formatCurrency(actual)}</div>
      <div className="text-xs text-text-muted">
        Plan by {formatDate(day(asOf), 'MMM d')}: {formatCurrency(expected)}
      </div>
    </div>
  )
}

function OffPlanNotice({ account, plans, progress }: { account: Account; plans: Contribution[]; progress: ContributionProgress }) {
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState('')
  if (!progress.employeeOffPlan && !progress.employerOffPlan) return null

  const s = progress.suggestion
  const plan = s?.contributionId ? plans.find((p) => p.id === s.contributionId) : undefined
  const newAmount =
    s?.amountType === 'percent'
      ? `${+(s.incomePct * 100).toFixed(2)}% of salary`
      : s && `${formatCurrency(s.amount)}${FREQ_LABEL[s.frequency]}`

  const applySuggestion = async () => {
    if (!s) return
    setError('')
    setApplying(true)
    const amount = Math.round(s.amount * 100) / 100
    const incomePct = Math.round(s.incomePct * 10000) / 10000
    try {
      if (plan) {
        await contributionsApi.update(account.id, plan.id, {
          label: plan.label,
          amount: s.amountType === 'percent' ? 0 : amount,
          frequency: plan.frequency,
          annualIncreasePct: plan.annualIncreasePct,
          amountType: s.amountType,
          incomePct: s.amountType === 'percent' ? incomePct : 0,
          matchTiers: plan.matchTiers,
          nextPayDate: toDateInputValue(plan.nextPayDate) || undefined,
          startDate: toDateInputValue(plan.startDate) || undefined,
          endDate: toDateInputValue(plan.endDate) || undefined,
          isActive: plan.isActive,
        })
      } else {
        await contributionsApi.create(account.id, {
          amount,
          frequency: 'monthly',
          annualIncreasePct: 0,
          amountType: 'fixed',
          incomePct: 0,
          matchTiers: [],
        })
      }
      mutate(`/accounts/${account.id}/contributions`)
      mutate('/contributions')
      mutate('/forecast')
      mutate('/contribution-progress')
    } catch (err: any) {
      setError(err.message)
    } finally {
      setApplying(false)
    }
  }

  return (
    <div className="mt-4 rounded-lg bg-warning-tint-bg px-4 py-3 text-sm text-text-primary space-y-2">
      {progress.employeeOffPlan && (
        <p>
          {progress.employeeExpected > 0
            ? `Your contributions are ${drift(progress.employeeActual, progress.employeeExpected)} your plan.`
            : plans.some((p) => p.isActive)
              ? "Your plan doesn't expect any contributions yet this year, but you've made some."
              : "You're contributing to this account, but there's no plan for it, so the forecast leaves these contributions out."}
          {s && ` At this pace you'll put in about ${formatCurrency(s.annualPace)} this year.`}
          {!s && progress.employeeExpected > 0 && ' Review the planned contributions above so the forecast matches.'}
        </p>
      )}
      {progress.employerOffPlan && (
        <p>
          {progress.employerExpected > 0
            ? `Employer contributions are ${drift(progress.employerActual, progress.employerExpected)} what your match should add. Some employers deposit the match quarterly or at year end; if yours doesn't, check the match tiers above.`
            : 'Your employer is contributing, but no match is set on the plan. Add the match tiers above so the forecast includes it.'}
        </p>
      )}
      {s && (
        <div className="flex flex-wrap items-center gap-3">
          <Button size="sm" onClick={applySuggestion} loading={applying}>
            {plan ? `Update plan to ${newAmount}` : `Add a ${newAmount} plan`}
          </Button>
          {error && <span className="text-negative text-xs">{error}</span>}
        </div>
      )}
    </div>
  )
}

function EntryForm({
  initial,
  showEmployer,
  onSubmit,
  onCancel,
}: {
  initial?: ContributionEntry
  showEmployer: boolean
  onSubmit: (d: ContributionEntryInput) => Promise<void>
  onCancel: () => void
}) {
  const [entryDate, setEntryDate] = useState(initial ? day(initial.entryDate) : today())
  const [amount, setAmount] = useState(initial ? String(initial.amount) : '')
  const [source, setSource] = useState<ContributionSource>(initial?.source ?? 'employee')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onSubmit({ entryDate, amount: parseFloat(amount), source, description: description || undefined })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <label className={labelCls}>Date</label>
          <input type="date" value={entryDate} onChange={(e) => setEntryDate(e.target.value)} required className={inputCls} />
        </div>
        <div>
          <label className={labelCls}>Amount ($)</label>
          <input
            type="number"
            step="any"
            min={0.01}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            required
            placeholder="500"
            className={inputCls}
          />
        </div>
      </div>
      {showEmployer && (
        <div>
          <label className={labelCls}>From</label>
          <select value={source} onChange={(e) => setSource(e.target.value as ContributionSource)} className={inputCls}>
            <option value="employee">You</option>
            <option value="employer">Employer</option>
          </select>
        </div>
      )}
      <div>
        <label className={labelCls}>Note (optional)</label>
        <input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="e.g. March paycheck" className={inputCls} />
      </div>
      <div className="flex justify-end gap-3 pt-2">
        <Button variant="secondary" onClick={onCancel} type="button">Cancel</Button>
        <Button type="submit" loading={loading}>{initial ? 'Save Changes' : 'Log Contribution'}</Button>
      </div>
    </form>
  )
}
