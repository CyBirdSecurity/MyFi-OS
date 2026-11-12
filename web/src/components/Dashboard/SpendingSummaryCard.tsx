import useSWR from 'swr'
import { Link } from 'react-router-dom'
import { budgetApi } from '../../api/client'
import { Card } from '../ui/Card'
import { EmptyState } from '../ui/EmptyState'
import { formatCurrency } from '../../utils/currency'

export function SpendingSummaryCard() {
  const now = new Date()
  const year = now.getFullYear()
  const month = now.getMonth() + 1

  const { data: summary } = useSWR(
    `budget/summary?year=${year}&month=${month}`,
    () => budgetApi.summary(year, month),
  )

  const categories = summary?.categories ?? []
  const total = categories.reduce((sum, c) => sum + c.total, 0)
  const topCategories = categories.slice(0, 3)

  return (
    <Card title="This Month's Spending" action={<Link to="/budget" className="text-xs text-accent-text hover:text-accent">View Budget →</Link>}>
      <div className="text-2xl font-bold text-text-primary mb-3">{formatCurrency(total)}</div>
      {topCategories.length === 0 ? (
        <EmptyState action={{ label: 'Import a bank statement', to: '/import' }}>
          Nothing logged this month. Your top spending categories show here once transactions come in.
        </EmptyState>
      ) : (
        <div className="space-y-2">
          {topCategories.map((c) => (
            <div key={c.categoryId} className="flex items-center justify-between text-sm">
              <span className="text-text-secondary">{c.categoryName}</span>
              <span className="font-medium text-text-primary">{formatCurrency(c.total)}</span>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
