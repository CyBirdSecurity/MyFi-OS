import useSWR from 'swr'
import { Link } from 'react-router-dom'
import { budgetApi } from '../../api/client'
import { Card } from '../ui/Card'
import { EmptyState } from '../ui/EmptyState'
import { formatCurrency } from '../../utils/currency'
import { formatDate } from '../../utils/dates'

export function RecentTransactionsCard() {
  const { data: recent = [] } = useSWR('/budget/transactions?recent', () => budgetApi.listTransactions({ limit: 5 }))

  return (
    <Card title="Recent Transactions" action={<Link to="/budget" className="text-xs text-accent-text hover:text-accent">View all →</Link>}>
      {recent.length === 0 ? (
        <EmptyState action={{ label: 'Import a bank statement', to: '/import' }}>
          Your latest purchases and deposits show here. Log one with Add transaction, or bring in a few months at once.
        </EmptyState>
      ) : (
        <div className="space-y-3">
          {recent.map((t) => (
            <div key={t.id} className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-sm text-text-primary truncate">{t.merchant}</div>
                <div className="text-xs text-text-tertiary">{formatDate(t.transDate, 'MMM d')}{t.categoryName ? ` · ${t.categoryName}` : ''}</div>
              </div>
              <div className="text-sm font-medium text-text-primary shrink-0">{formatCurrency(t.amount)}</div>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
