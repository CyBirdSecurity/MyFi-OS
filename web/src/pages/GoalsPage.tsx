import { useMemo, useState } from 'react'
import { mutate } from 'swr'
import { parseISO } from 'date-fns'
import { useGoals } from '../hooks/useGoals'
import { useNetWorthCurrent } from '../hooks/useNetWorth'
import { useAccounts } from '../hooks/useAccounts'
import { useHoldings } from '../hooks/useHoldings'
import { goalsApi, ACCOUNT_TYPE_LABELS, ASSET_CLASS_LABELS, CASH_BALANCE_ACCOUNT_TYPES } from '../api/client'
import type { Goal, CreateGoalInput, AccountBalance } from '../api/client'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Modal } from '../components/ui/Modal'
import { GoalProgressBar } from '../components/charts/GoalProgressBar'
import { formatCurrency } from '../utils/currency'
import { toDateInputValue } from '../utils/dates'
import { PageGuide } from '../components/PageGuide'

const LIABILITY_TYPES = new Set(['credit_card', 'loan', 'other_liability'])

const ASSET_ACCOUNT_TYPE_OPTIONS = Object.entries(ACCOUNT_TYPE_LABELS)
  .filter(([key]) => !LIABILITY_TYPES.has(key))
  .map(([key, label]) => ({ key, label }))

const ASSET_CLASS_OPTIONS = Object.entries(ASSET_CLASS_LABELS)
  .map(([key, label]) => ({ key, label }))

const inputCls = 'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

type GoalType = 'net_worth' | 'account' | 'account_type' | 'asset_class' | 'spend'

const GOAL_TYPE_OPTIONS: { value: GoalType; label: string; description: string }[] = [
  { value: 'net_worth',    label: 'Net Worth',       description: 'Track your total net worth' },
  { value: 'account',      label: 'Account',          description: 'Track a single specific account' },
  { value: 'account_type', label: 'Account Type',     description: 'Track the combined total across all accounts of a type (e.g. all savings accounts)' },
  { value: 'asset_class',  label: 'Asset Type',       description: 'Track the total value of all holdings in an asset class (e.g. all equity, all cash)' },
  { value: 'spend',        label: 'Large Purchase',   description: 'Plan a one-time expense — link an account to track your savings progress, and the forecast will deduct this from your portfolio on the target date' },
]

function GoalForm({
  accounts,
  onSubmit,
  onCancel,
  initialValues,
}: {
  accounts: AccountBalance[]
  onSubmit: (d: CreateGoalInput) => Promise<void>
  onCancel: () => void
  initialValues?: Goal
}) {
  const [name, setName] = useState(initialValues?.name ?? '')
  const [goalType, setGoalType] = useState<GoalType>((initialValues?.goalType as GoalType) ?? 'net_worth')
  const [accountId, setAccountId] = useState(initialValues?.accountId ?? '')
  const [accountTypeFilter, setAccountTypeFilter] = useState(initialValues?.accountTypeFilter ?? '')
  const [assetClassFilter, setAssetClassFilter] = useState(initialValues?.assetClassFilter ?? '')
  const [targetAmount, setTargetAmount] = useState(initialValues?.targetAmount != null ? String(initialValues.targetAmount) : '')
  const [targetDate, setTargetDate] = useState(toDateInputValue(initialValues?.targetDate))
  const [notes, setNotes] = useState(initialValues?.notes ?? '')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const assetAccounts = accounts.filter((a) =>
    !['credit_card', 'loan', 'other_liability'].includes(a.accountType),
  )

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onSubmit({
        name,
        goalType,
        accountId: goalType === 'account' ? accountId : goalType === 'spend' ? (accountId || undefined) : undefined,
        accountTypeFilter: goalType === 'account_type' ? accountTypeFilter : undefined,
        assetClassFilter: goalType === 'asset_class' ? assetClassFilter : undefined,
        targetAmount: parseFloat(targetAmount),
        targetDate: targetDate || undefined,
        notes: notes || undefined,
      })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const selectedTypeOption = GOAL_TYPE_OPTIONS.find((o) => o.value === goalType)

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div>
        <label className={labelCls}>Goal Type</label>
        <select
          value={goalType}
          onChange={(e) => setGoalType(e.target.value as GoalType)}
          className={inputCls}
        >
          {GOAL_TYPE_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
        {selectedTypeOption && (
          <p className="text-xs text-text-tertiary mt-1">{selectedTypeOption.description}</p>
        )}
      </div>

      {goalType === 'account' && (
        <div>
          <label className={labelCls}>Account</label>
          <select
            value={accountId}
            onChange={(e) => setAccountId(e.target.value)}
            required
            className={inputCls}
          >
            <option value="">Select an account…</option>
            {assetAccounts.map((a) => (
              <option key={a.accountId} value={a.accountId}>
                {a.name} ({formatCurrency(a.balance)})
              </option>
            ))}
          </select>
        </div>
      )}

      {goalType === 'spend' && (
        <div>
          <label className={labelCls}>Savings Account (optional)</label>
          <select
            value={accountId}
            onChange={(e) => setAccountId(e.target.value)}
            className={inputCls}
          >
            <option value="">No account linked — show as planned expense only</option>
            {assetAccounts.map((a) => (
              <option key={a.accountId} value={a.accountId}>
                {a.name} ({formatCurrency(a.balance)})
              </option>
            ))}
          </select>
          <p className="text-xs text-text-tertiary mt-1">
            Link an account to track your savings progress toward this purchase.
          </p>
        </div>
      )}

      {goalType === 'account_type' && (
        <div>
          <label className={labelCls}>Account Type</label>
          <select
            value={accountTypeFilter}
            onChange={(e) => setAccountTypeFilter(e.target.value)}
            required
            className={inputCls}
          >
            <option value="">Select an account type…</option>
            {ASSET_ACCOUNT_TYPE_OPTIONS.map((o) => (
              <option key={o.key} value={o.key}>{o.label}</option>
            ))}
          </select>
        </div>
      )}

      {goalType === 'asset_class' && (
        <div>
          <label className={labelCls}>Asset Class</label>
          <select
            value={assetClassFilter}
            onChange={(e) => setAssetClassFilter(e.target.value)}
            required
            className={inputCls}
          >
            <option value="">Select an asset class…</option>
            {ASSET_CLASS_OPTIONS.map((o) => (
              <option key={o.key} value={o.key}>{o.label}</option>
            ))}
          </select>
        </div>
      )}

      <div>
        <label className={labelCls}>Goal Name</label>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          placeholder={
            goalType === 'account'      ? 'e.g. Emergency Fund' :
            goalType === 'account_type' ? 'e.g. $100k in Savings' :
            goalType === 'asset_class'  ? 'e.g. $500k Invested' :
            goalType === 'spend'        ? 'e.g. New Car' :
            'e.g. Financial Independence'
          }
          className={inputCls}
        />
      </div>
      <div>
        <label className={labelCls}>{goalType === 'spend' ? 'Purchase Amount' : 'Target Amount'}</label>
        <input
          type="number"
          step="any"
          value={targetAmount}
          onChange={(e) => setTargetAmount(e.target.value)}
          required
          min="0"
          placeholder="100000"
          className={inputCls}
        />
      </div>
      <div>
        <label className={labelCls}>{goalType === 'spend' ? 'Purchase Date' : 'Target Date (optional)'}</label>
        <input
          type="date"
          value={targetDate}
          onChange={(e) => setTargetDate(e.target.value)}
          required={goalType === 'spend'}
          className={inputCls}
        />
      </div>
      <div>
        <label className={labelCls}>Notes (optional)</label>
        <input
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          className={inputCls}
        />
      </div>
      <div className="flex justify-end gap-3 pt-2">
        <Button variant="secondary" onClick={onCancel} type="button">Cancel</Button>
        <Button type="submit" loading={loading}>{initialValues ? 'Save Changes' : 'Create Goal'}</Button>
      </div>
    </form>
  )
}

function resolveCurrentValue(
  goal: Goal,
  netWorth: number,
  accountBalances: AccountBalance[],
  assetClassTotals: Record<string, number>,
): number {
  if ((goal.goalType === 'account' || goal.goalType === 'spend') && goal.accountId) {
    return accountBalances.find((a) => a.accountId === goal.accountId)?.balance ?? 0
  }
  if (goal.goalType === 'account_type' && goal.accountTypeFilter) {
    return accountBalances
      .filter((a) => a.accountType === goal.accountTypeFilter)
      .reduce((sum, a) => sum + a.balance, 0)
  }
  if (goal.goalType === 'asset_class' && goal.assetClassFilter) {
    return assetClassTotals[goal.assetClassFilter] ?? 0
  }
  return netWorth
}

export function GoalsPage() {
  const { data: goals, isLoading } = useGoals()
  const { data: nw } = useNetWorthCurrent()
  const { data: accounts } = useAccounts()
  const { data: holdings } = useHoldings()
  const [showCreate, setShowCreate] = useState(false)
  const [editingGoal, setEditingGoal] = useState<Goal | null>(null)

  const currentNetWorth = nw?.totalNetWorth ?? 0
  const accountBalances: AccountBalance[] = nw?.accounts ?? []

  // Build a name lookup from the full accounts list for display
  const accountNameMap = new Map(accounts?.map((a) => [a.id, a.name]) ?? [])

  // Sum holdings value by asset class; cash-type account balances fold into cash_equivalent
  const assetClassTotals = useMemo(() => {
    const totals: Record<string, number> = {}
    for (const h of holdings ?? []) {
      totals[h.assetClass] = (totals[h.assetClass] ?? 0) + h.totalValue
    }
    for (const a of accountBalances) {
      if (CASH_BALANCE_ACCOUNT_TYPES.has(a.accountType) && a.balance > 0) {
        totals['cash_equivalent'] = (totals['cash_equivalent'] ?? 0) + a.balance
      }
    }
    return totals
  }, [holdings, accountBalances])

  const handleCreate = async (data: CreateGoalInput) => {
    await goalsApi.create(data)
    await mutate('/goals')
    setShowCreate(false)
  }

  const handleUpdate = async (data: CreateGoalInput) => {
    if (!editingGoal) return
    await goalsApi.update(editingGoal.id, { ...data, isAchieved: editingGoal.isAchieved })
    await mutate('/goals')
    setEditingGoal(null)
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this goal?')) return
    await goalsApi.delete(id)
    mutate('/goals')
  }

  const handleToggleAchieved = async (goal: Goal) => {
    await goalsApi.update(goal.id, {
      name: goal.name,
      goalType: goal.goalType,
      accountId: goal.accountId,
      accountTypeFilter: goal.accountTypeFilter,
      assetClassFilter: goal.assetClassFilter,
      targetAmount: goal.targetAmount,
      targetDate: toDateInputValue(goal.targetDate) || undefined,
      notes: goal.notes,
      isAchieved: !goal.isAchieved,
    })
    mutate('/goals')
  }

  const activeGoals = goals?.filter((g) => !g.isAchieved) ?? []
  const achievedGoals = goals?.filter((g) => g.isAchieved) ?? []

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text-primary">Goals</h1>
            <PageGuide id="goals" />
          </div>
          <p className="text-text-tertiary text-sm mt-1">
            Current net worth: <span className="text-text-primary font-medium">{formatCurrency(currentNetWorth)}</span>
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)}>+ Add Goal</Button>
      </div>

      {isLoading ? (
        <div className="text-text-tertiary text-sm">Loading...</div>
      ) : (
        <>
          {activeGoals.length > 0 && (
            <Card title="Active Goals">
              <div className="space-y-6">
                {activeGoals.map((goal) => {
                  const currentValue = resolveCurrentValue(goal, currentNetWorth, accountBalances, assetClassTotals)
                  const isSpend = goal.goalType === 'spend'
                  const spendHasAccount = isSpend && !!goal.accountId
                  return (
                    <div key={goal.id} className="group">
                      {isSpend && (
                        <div className="flex items-center gap-2 mb-1">
                          <span className="text-xs font-medium px-1.5 py-0.5 rounded bg-negative/10 text-negative">
                            Planned Purchase
                          </span>
                          {goal.targetDate && (
                            <span className="text-xs text-text-tertiary">
                              {parseISO(goal.targetDate).toLocaleDateString('en-US', { year: 'numeric', month: 'short' })}
                            </span>
                          )}
                        </div>
                      )}
                      {!isSpend && goal.goalType === 'account' && goal.accountId && (
                        <div className="text-xs text-text-tertiary mb-1">
                          {accountNameMap.get(goal.accountId) ?? 'Account'}
                        </div>
                      )}
                      {goal.goalType === 'account_type' && goal.accountTypeFilter && (
                        <div className="text-xs text-text-tertiary mb-1">
                          All {ACCOUNT_TYPE_LABELS[goal.accountTypeFilter] ?? goal.accountTypeFilter} accounts
                        </div>
                      )}
                      {goal.goalType === 'asset_class' && goal.assetClassFilter && (
                        <div className="text-xs text-text-tertiary mb-1">
                          {ASSET_CLASS_LABELS[goal.assetClassFilter] ?? goal.assetClassFilter} holdings
                        </div>
                      )}
                      {(!isSpend || spendHasAccount) ? (
                        <>
                          {spendHasAccount && (
                            <div className="text-xs text-text-tertiary mb-1">
                              Saving in: {accountNameMap.get(goal.accountId!) ?? 'Account'}
                            </div>
                          )}
                          <GoalProgressBar goal={goal} currentValue={currentValue} />
                        </>
                      ) : (
                        <div className="flex items-center justify-between py-1">
                          <span className="text-sm font-medium text-text-primary">{goal.name}</span>
                          <span className="text-sm font-semibold text-negative">{formatCurrency(goal.targetAmount)}</span>
                        </div>
                      )}
                      <div className="flex gap-3 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          onClick={() => setEditingGoal(goal)}
                          className="text-xs text-text-secondary hover:text-text-primary"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() => handleToggleAchieved(goal)}
                          className="text-xs text-positive hover:text-positive"
                        >
                          Mark Achieved
                        </button>
                        <button
                          onClick={() => handleDelete(goal.id)}
                          className="text-xs text-text-muted hover:text-negative"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  )
                })}
              </div>
            </Card>
          )}

          {activeGoals.length === 0 && !isLoading && (
            <Card>
              <div className="text-center py-8 text-text-tertiary text-sm">
                No active goals yet.
                <button
                  onClick={() => setShowCreate(true)}
                  className="block mx-auto mt-2 text-accent-text hover:text-accent-text"
                >
                  Create your first goal
                </button>
              </div>
            </Card>
          )}

          {achievedGoals.length > 0 && (
            <Card title="Achieved">
              <div className="space-y-3">
                {achievedGoals.map((goal) => (
                  <div key={goal.id} className="flex items-center justify-between py-2 border-b border-border-default last:border-0">
                    <div>
                      <div className="text-sm text-text-secondary line-through">{goal.name}</div>
                      <div className="text-xs text-positive">{formatCurrency(goal.targetAmount)}</div>
                    </div>
                    <div className="flex gap-3">
                      <button
                        onClick={() => setEditingGoal(goal)}
                        className="text-xs text-text-tertiary hover:text-text-primary"
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => handleToggleAchieved(goal)}
                        className="text-xs text-text-tertiary hover:text-text-primary"
                      >
                        Reopen
                      </button>
                      <button
                        onClick={() => handleDelete(goal.id)}
                        className="text-xs text-text-muted hover:text-negative"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </Card>
          )}
        </>
      )}

      <Modal open={showCreate} onClose={() => setShowCreate(false)} title="New Goal">
        <GoalForm
          accounts={accountBalances}
          onSubmit={handleCreate}
          onCancel={() => setShowCreate(false)}
        />
      </Modal>

      <Modal open={!!editingGoal} onClose={() => setEditingGoal(null)} title="Edit Goal">
        {editingGoal && (
          <GoalForm
            key={editingGoal.id}
            accounts={accountBalances}
            onSubmit={handleUpdate}
            onCancel={() => setEditingGoal(null)}
            initialValues={editingGoal}
          />
        )}
      </Modal>
    </div>
  )
}
