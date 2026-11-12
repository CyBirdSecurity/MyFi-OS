import { useState } from 'react'
import useSWR, { mutate } from 'swr'
import { budgetApi, type RecurringFrequency } from '../../api/client'
import { today } from '../../utils/dates'
import { Modal } from '../ui/Modal'
import { Button } from '../ui/Button'
import { useAccounts } from '../../hooks/useAccounts'

const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

export function QuickAddTransactionModal({
  onClose,
  onAdded,
  recurring = false,
}: {
  onClose: () => void
  onAdded?: () => void
  /** Set up a repeating transaction instead of a one-off (Budget → Recurring tab only). */
  recurring?: boolean
}) {
  const { data: categories = [] } = useSWR('/budget/categories', budgetApi.listCategories)
  const { data: accounts = [] } = useAccounts()

  const [date, setDate] = useState(today())
  const [merchant, setMerchant] = useState('')
  const [amount, setAmount] = useState('')
  const [categoryId, setCategoryId] = useState('')
  const [accountId, setAccountId] = useState('')
  const [notes, setNotes] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [added, setAdded] = useState('')
  const [repeat, setRepeat] = useState<RecurringFrequency | 'none'>(recurring ? 'monthly' : 'none')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    const amt = parseFloat(amount)
    if (!merchant.trim()) { setError('Merchant is required.'); return }
    if (isNaN(amt) || amt <= 0) { setError('Enter a valid positive amount.'); return }
    if (!categoryId) { setError('Please select a category.'); return }
    setLoading(true)
    try {
      const fields = {
        merchant: merchant.trim(),
        amount: amt,
        categoryId,
        accountId: accountId || undefined,
        notes: notes.trim() || undefined,
      }
      if (repeat === 'none') {
        await budgetApi.createTransaction({ transDate: date, ...fields })
        setAdded('Transaction added.')
      } else {
        const res = await budgetApi.createRecurring({ startDate: date, frequency: repeat, ...fields })
        setAdded(
          res.added > 0
            ? `Recurring transaction saved. ${res.added} payment${res.added === 1 ? '' : 's'} already due ${res.added === 1 ? 'was' : 'were'} added.`
            : 'Recurring transaction saved. It will be added automatically on each due date.',
        )
        // Back-filled occurrences can land anywhere in the budget.
        mutate((key) => typeof key === 'string' && key.startsWith('/budget'))
      }
      setMerchant('')
      setAmount('')
      setNotes('')
      onAdded?.()
    } catch (e: any) {
      setError(e.message ?? 'Failed to add transaction.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal open title={recurring ? 'Add Recurring Transaction' : 'Add Transaction'} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && <div className="text-negative text-sm">{error}</div>}
        {added && (
          <div className="text-positive text-sm bg-positive-tint-bg rounded px-3 py-2">
            {added}
          </div>
        )}
        {recurring && (
          <div>
            <label className={labelCls}>Repeats</label>
            <select
              value={repeat}
              onChange={(e) => { setRepeat(e.target.value as RecurringFrequency); setAdded('') }}
              className={inputCls}
            >
              <option value="monthly">Monthly</option>
              <option value="biweekly">Every 2 weeks</option>
            </select>
          </div>
        )}
        <div>
          <label className={labelCls}>{repeat === 'none' ? 'Date' : 'Start date'}</label>
          <input
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            required
            className={inputCls}
          />
          {repeat !== 'none' && (
            <p className="text-xs text-text-muted mt-1">
              The first payment. Payments dated today or earlier (up to a year back) are added right away; later ones are added on their due dates.
            </p>
          )}
        </div>
        <div>
          <label className={labelCls}>Merchant / Description</label>
          <input
            type="text"
            value={merchant}
            onChange={(e) => setMerchant(e.target.value)}
            placeholder="e.g. Whole Foods"
            required
            autoFocus
            className={inputCls}
          />
        </div>
        <div>
          <label className={labelCls}>Amount ($)</label>
          <input
            type="number"
            min="0.01"
            step="0.01"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="0.00"
            required
            className={inputCls}
          />
        </div>
        <div>
          <label className={labelCls}>Category</label>
          <select
            value={categoryId}
            onChange={(e) => setCategoryId(e.target.value)}
            required
            className={inputCls}
          >
            <option value="">— select a category —</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </div>
        <div>
          <label className={labelCls}>Account <span className="text-text-muted">(optional)</span></label>
          <select
            value={accountId}
            onChange={(e) => setAccountId(e.target.value)}
            className={inputCls}
          >
            <option value="">— none —</option>
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
        </div>
        <div>
          <label className={labelCls}>Notes <span className="text-text-muted">(optional)</span></label>
          <input
            type="text"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Optional note…"
            className={inputCls}
          />
        </div>
        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="secondary" onClick={onClose}>
            {added ? 'Done' : 'Cancel'}
          </Button>
          <Button type="submit" loading={loading}>{repeat === 'none' ? 'Add Transaction' : 'Save Recurring'}</Button>
        </div>
      </form>
    </Modal>
  )
}
