import { useMemo, useState } from 'react'
import { Warning, X } from '@phosphor-icons/react'
import useSWR from 'swr'
import { budgetApi } from '../../api/client'
import { detectSubscriptions, subscriptionTotals, type DuplicateCharge } from '../../utils/subscriptions'
import { formatCurrency } from '../../utils/currency'

/** How far back to look. Two years lets a yearly charge appear twice. */
const LOOKBACK_MONTHS = 26

function fromDate(): string {
  const d = new Date()
  d.setMonth(d.getMonth() - LOOKBACK_MONTHS)
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${mm}-${dd}`
}

// Subscriptions are priced to the cent ($15.49), unlike the whole-dollar budget figures.
const withCents = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' })

function shortDate(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number)
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

function monthLabel(iso: string): string {
  const [y, m] = iso.split('-').map(Number)
  return new Date(y, m - 1, 1).toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
}

/** "Charged 2× in Oct 2026", plus how many other cycles had extra charges. */
function DuplicateFlag({ duplicates }: { duplicates: DuplicateCharge[] }) {
  const [latest, ...earlier] = duplicates
  return (
    <div className="mt-1 inline-flex items-center gap-1 text-xs text-warning">
      <Warning size={12} weight="bold" className="shrink-0" />
      <span>
        Charged {latest.charges}× in {monthLabel(latest.date)} ({withCents.format(latest.total)})
        {earlier.length > 0 && `, and in ${earlier.length} earlier month${earlier.length === 1 ? '' : 's'}`}
      </span>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex-1 min-w-[150px] bg-subtle/50 border border-border-strong/50 rounded-lg px-4 py-3">
      <p className="text-xs text-text-tertiary uppercase tracking-wider mb-1">{label}</p>
      <p className="text-xl font-bold text-text-primary">{value}</p>
    </div>
  )
}

/** Recurring charges found in the user's transactions, with what they cost. */
export function SubscriptionsTab() {
  const { data: txns, isLoading } = useSWR('/budget/subscriptions', () => budgetApi.listTransactions({ from: fromDate() }))

  // Charges the user marked "not a subscription"; saved on the server so it holds on every device.
  const { data: dismissedKeys = [], mutate: mutateDismissed } = useSWR(
    '/budget/subscriptions/dismissed',
    budgetApi.listDismissedSubscriptions,
  )
  const [showHidden, setShowHidden] = useState(false)

  const found = useMemo(() => detectSubscriptions(txns ?? [], new Date()), [txns])
  const dismissed = new Set(dismissedKeys)
  const subs = found.filter((s) => !dismissed.has(s.key))
  const hidden = found.filter((s) => dismissed.has(s.key))
  const totals = subscriptionTotals(subs)
  const withDuplicates = subs.filter((s) => s.duplicates.length > 0).length

  // Update the list straight away, then save; a failed save puts it back.
  const setDismissed = (key: string, dismiss: boolean) =>
    mutateDismissed(
      async () => {
        await (dismiss ? budgetApi.dismissSubscription(key) : budgetApi.restoreSubscription(key))
        return budgetApi.listDismissedSubscriptions()
      },
      {
        optimisticData: dismiss ? [...dismissedKeys, key] : dismissedKeys.filter((k) => k !== key),
        rollbackOnError: true,
        revalidate: false,
      },
    )

  if (isLoading) {
    return <div className="text-center text-text-tertiary py-12 text-sm">Looking for recurring charges…</div>
  }

  if (found.length === 0) {
    return (
      <div className="text-center text-text-muted py-12 text-sm space-y-1">
        <p>No recurring charges found yet.</p>
        <p className="text-xs">
          A charge shows up here once the same merchant has billed you a similar amount three times about a month apart,
          or twice a year apart.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <div className="flex gap-3 flex-wrap">
        <Stat label="Subscriptions" value={String(subs.length)} />
        <Stat label="Per month" value={formatCurrency(totals.monthly)} />
        <Stat label="Per year" value={formatCurrency(totals.yearly)} />
      </div>

      {withDuplicates > 0 && (
        <div className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning-tint-bg px-4 py-3 text-sm text-text-primary">
          <Warning size={16} weight="bold" className="text-warning shrink-0 mt-0.5" />
          <span>
            {withDuplicates} subscription{withDuplicates === 1 ? ' was' : 's were'} charged more than once in a billing
            period. That can be a double charge to dispute, or two plans with the same company.
          </span>
        </div>
      )}

      <div className="overflow-x-auto">
        <table className="w-full text-sm min-w-[560px]">
          <thead>
            <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
              <th className="pb-3 font-medium">Merchant</th>
              <th className="pb-3 pr-6 font-medium text-right">Charge</th>
              <th className="pb-3 font-medium">Next expected</th>
              <th className="pb-3 font-medium">Last charged</th>
              <th className="pb-3 font-medium text-right">Per year</th>
              <th className="pb-3 w-10" />
            </tr>
          </thead>
          <tbody>
            {subs.map((s) => (
              <tr key={`${s.merchant}-${s.cadence}`} className="border-b border-border-default/50">
                <td className="py-3">
                  <div className="font-medium text-text-primary">{s.merchant}</div>
                  {s.categoryName && <div className="text-xs text-text-tertiary">{s.categoryName}</div>}
                  {s.duplicates.length > 0 && <DuplicateFlag duplicates={s.duplicates} />}
                </td>
                <td className="py-3 pr-6 text-right text-text-primary whitespace-nowrap">
                  {withCents.format(s.amount)} <span className="text-xs text-text-tertiary">{s.cadence}</span>
                </td>
                <td className="py-3 text-text-secondary whitespace-nowrap">{shortDate(s.nextDate)}</td>
                <td className="py-3 text-text-tertiary whitespace-nowrap">{shortDate(s.lastDate)}</td>
                <td className="py-3 text-right font-semibold text-text-primary whitespace-nowrap">
                  {withCents.format(s.yearlyCost)}
                </td>
                <td className="py-3 text-right">
                  <button
                    onClick={() => void setDismissed(s.key, true)}
                    title="Not a subscription — hide it"
                    aria-label={`${s.merchant} is not a subscription`}
                    className="p-1.5 rounded text-text-muted hover:text-text-primary hover:bg-subtle"
                  >
                    <X size={14} weight="bold" />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {subs.length === 0 && (
        <p className="text-sm text-text-tertiary">Every recurring charge found here has been hidden.</p>
      )}

      {hidden.length > 0 && (
        <div className="text-sm">
          <button onClick={() => setShowHidden((v) => !v)} className="text-text-secondary hover:text-text-primary">
            {hidden.length} hidden as not a subscription · {showHidden ? 'Hide' : 'Show'}
          </button>
          {showHidden && (
            <ul className="mt-2 divide-y divide-border-default/50">
              {hidden.map((s) => (
                <li key={s.key} className="flex items-center justify-between gap-3 py-2">
                  <span className="text-text-tertiary">
                    {s.merchant} · {withCents.format(s.amount)} {s.cadence}
                  </span>
                  <button
                    onClick={() => void setDismissed(s.key, false)}
                    className="text-xs text-accent-text hover:text-accent"
                  >
                    Restore
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <p className="text-xs text-text-tertiary">
        Found automatically from your transactions. A charge that stops for a full cycle drops off the list. Use × to
        hide one that isn't a subscription.
      </p>
    </div>
  )
}
