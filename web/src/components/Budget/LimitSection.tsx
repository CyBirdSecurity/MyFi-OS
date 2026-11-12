import { useState } from 'react'
import { formatCurrency } from '../../utils/currency'
import { limitProgress, type LimitState } from '../../utils/budgetLimits'

const BAR: Record<LimitState, string> = {
  ok: 'bg-positive',
  warn: 'bg-warning',
  over: 'bg-negative',
}
const TEXT: Record<LimitState, string> = {
  ok: 'text-text-tertiary',
  warn: 'text-warning',
  over: 'text-negative',
}

const linkCls = 'text-xs text-accent-text hover:text-accent'

/**
 * The monthly-limit part of a category card: a progress bar against the limit
 * once one is set, a suggested limit (the user's recent average) when there is
 * none, and an inline editor for either.
 */
export function LimitSection({
  spent,
  limit,
  suggestion,
  onSave,
  onClear,
}: {
  spent: number
  limit?: number
  suggestion?: number
  onSave: (amount: number) => Promise<void>
  onClear: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
      setEditing(false)
    } catch (e: any) {
      setError(e.message ?? 'Could not save the limit.')
    } finally {
      setBusy(false)
    }
  }

  const startEditing = () => {
    setValue(String(limit ?? suggestion ?? ''))
    setError('')
    setEditing(true)
  }

  const save = () => {
    const amount = Number(value)
    if (!Number.isFinite(amount) || amount <= 0) {
      setError('Enter an amount above $0.')
      return
    }
    void run(() => onSave(amount))
  }

  if (editing) {
    return (
      <div className="pt-2 mt-1 border-t border-border-default/60 space-y-2">
        <label className="block text-xs text-text-tertiary" htmlFor="limit-input">
          Monthly limit
        </label>
        <div className="flex items-center gap-2">
          <span className="text-sm text-text-secondary">$</span>
          <input
            id="limit-input"
            type="number"
            min="1"
            step="1"
            inputMode="decimal"
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') save()
              if (e.key === 'Escape') setEditing(false)
            }}
            className="w-full bg-surface border border-border-default rounded-lg px-2 py-1 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
          />
        </div>
        {error && <p className="text-xs text-negative">{error}</p>}
        <div className="flex items-center gap-3">
          <button type="button" onClick={save} disabled={busy} className={`${linkCls} font-medium disabled:opacity-50`}>
            Save
          </button>
          <button type="button" onClick={() => setEditing(false)} disabled={busy} className={linkCls}>
            Cancel
          </button>
          {limit !== undefined && (
            <button
              type="button"
              onClick={() => void run(onClear)}
              disabled={busy}
              className="text-xs text-negative hover:opacity-75 ml-auto disabled:opacity-50"
            >
              Remove limit
            </button>
          )}
        </div>
      </div>
    )
  }

  if (limit !== undefined) {
    const { pct, state, remaining } = limitProgress(spent, limit)
    return (
      <div className="pt-2 mt-1 border-t border-border-default/60 space-y-1">
        <div className="flex items-baseline justify-between gap-2">
          <span className="text-xs text-text-secondary">
            {formatCurrency(spent)} of {formatCurrency(limit)}
          </span>
          <span className={`text-xs font-medium ${TEXT[state]}`}>
            {remaining >= 0 ? `${formatCurrency(remaining)} left` : `${formatCurrency(-remaining)} over`}
          </span>
        </div>
        <div
          className="h-2 bg-muted rounded-full overflow-hidden"
          role="progressbar"
          aria-label="Spent against monthly limit"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(pct)}
        >
          <div className={`h-full rounded-full transition-all duration-500 ${BAR[state]}`} style={{ width: `${pct}%` }} />
        </div>
        <button type="button" onClick={startEditing} className={linkCls}>
          Edit limit
        </button>
      </div>
    )
  }

  return (
    <div className="pt-2 mt-1 border-t border-border-default/60 space-y-1">
      {suggestion !== undefined ? (
        <>
          {/* One line plus a faint bar against the suggestion; the page banner explains suggestions. */}
          <div className="flex items-baseline justify-between gap-2">
            <span className="text-xs text-text-tertiary">Suggested {formatCurrency(suggestion)}/mo</span>
            <span className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => void run(() => onSave(suggestion))}
                disabled={busy}
                className={`${linkCls} font-medium disabled:opacity-50`}
              >
                Use
              </button>
              <button type="button" onClick={startEditing} className={linkCls}>
                Edit
              </button>
            </span>
          </div>
          <div className="h-1.5 bg-muted rounded-full overflow-hidden">
            <div
              className="h-full rounded-full bg-text-muted/50"
              style={{ width: `${Math.min(100, suggestion > 0 ? (spent / suggestion) * 100 : 0)}%` }}
            />
          </div>
          {error && <p className="text-xs text-negative">{error}</p>}
        </>
      ) : (
        <button type="button" onClick={startEditing} className={linkCls}>
          Set a monthly limit
        </button>
      )}
    </div>
  )
}
