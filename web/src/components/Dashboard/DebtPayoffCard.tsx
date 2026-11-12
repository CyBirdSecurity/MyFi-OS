import useSWR from 'swr'
import { useAccounts } from '../../hooks/useAccounts'
import { useNetWorthCurrent } from '../../hooks/useNetWorth'
import { useMortgages } from '../../hooks/useMortgages'
import { snapshotsApi, LIABILITY_ACCOUNT_TYPES } from '../../api/client'
import { Card } from '../ui/Card'
import { formatCurrency } from '../../utils/currency'
import { payoffPercent } from '../../utils/debt'

function ProgressRow({ name, left, pctPaid }: { name: string; left: number; pctPaid: number }) {
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <span className="text-sm text-text-primary">{name}</span>
        <span className="text-xs text-text-tertiary">{formatCurrency(left)} left</span>
      </div>
      <div className="h-2 bg-muted rounded-full overflow-hidden">
        <div
          className="h-full rounded-full bg-positive transition-all duration-500"
          style={{ width: `${pctPaid}%` }}
        />
      </div>
      <div className="text-xs text-text-tertiary mt-1">{pctPaid.toFixed(0)}% paid off</div>
    </div>
  )
}

function DebtPayoffRow({
  accountId,
  name,
  currentBalance,
}: {
  accountId: string
  name: string
  currentBalance: number
}) {
  const { data: snapshots } = useSWR(`/accounts/${accountId}/snapshots?payoff`, () => snapshotsApi.list(accountId))

  if (!snapshots || snapshots.length === 0) return null

  // Snapshots come back newest-first; the earliest recorded balance is the last entry.
  const startBalance = Math.abs(snapshots[snapshots.length - 1].balance)
  if (startBalance <= 0) return null

  return (
    <ProgressRow
      name={name}
      left={Math.abs(currentBalance)}
      pctPaid={payoffPercent(startBalance, currentBalance)}
    />
  )
}

export function DebtPayoffCard() {
  const { data: accounts } = useAccounts()
  const { data: nw } = useNetWorthCurrent()
  const { data: mortgages } = useMortgages()

  const balanceMap = new Map((nw?.accounts ?? []).map((a) => [a.accountId, a.balance]))
  const liabilityAccounts = (accounts ?? []).filter((a) => LIABILITY_ACCOUNT_TYPES.has(a.accountType))

  if (liabilityAccounts.length === 0 && !(mortgages ?? []).length) return null

  return (
    <Card title="Debt Payoff Progress">
      <div className="space-y-4">
        {liabilityAccounts.map((acc) => (
          <DebtPayoffRow key={acc.id} accountId={acc.id} name={acc.name} currentBalance={balanceMap.get(acc.id) ?? 0} />
        ))}
        {/* Last: a mortgage is usually the biggest balance, and leading with it
            would discourage more than it informs. */}
        {(mortgages ?? []).map((m) => (
          <ProgressRow
            key={m.accountId}
            name={m.name}
            left={m.currentBalance}
            pctPaid={payoffPercent(m.originalLoan, m.currentBalance)}
          />
        ))}
      </div>
    </Card>
  )
}
