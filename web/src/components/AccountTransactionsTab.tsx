import { useState } from 'react'
import useSWR, { mutate } from 'swr'
import { Warning } from '@phosphor-icons/react'
import { budgetApi, type BudgetCategory } from '../api/client'
import { Card } from './ui/Card'
import { formatCurrency } from '../utils/currency'

const inputCls =
  'bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'

/**
 * Read-focused view of one account's transactions — for checking/savings
 * accounts, where Plaid sync (or CSV import) is the primary source of
 * transaction data. Category is editable inline; for the fuller edit
 * experience (notes, applying a category to similar unreviewed transactions)
 * use the Budget page's Transactions tab, which this intentionally doesn't
 * duplicate.
 */
export function AccountTransactionsTab({ accountId }: { accountId: string }) {
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')

  const swrKey = `/budget/transactions?accountId=${accountId}&from=${dateFrom}&to=${dateTo}`
  const { data: txns = [], isLoading } = useSWR(swrKey, () =>
    budgetApi.listTransactions({
      accountId,
      from: dateFrom || undefined,
      to: dateTo || undefined,
    }),
  )
  const { data: categories = [] } = useSWR<BudgetCategory[]>('/budget/categories', budgetApi.listCategories)

  const [savingId, setSavingId] = useState<string | null>(null)

  const handleCategoryChange = async (txnId: string, categoryId: string) => {
    setSavingId(txnId)
    try {
      await budgetApi.updateTransaction(txnId, { categoryId: categoryId || undefined })
      await mutate(swrKey)
    } finally {
      setSavingId(null)
    }
  }

  const needsReviewCount = txns.filter((t) => t.needsReview).length

  return (
    <Card title="Transactions">
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <input
            type="date"
            value={dateFrom}
            onChange={(e) => setDateFrom(e.target.value)}
            className={inputCls}
          />
          <span className="text-text-tertiary text-sm">–</span>
          <input
            type="date"
            value={dateTo}
            onChange={(e) => setDateTo(e.target.value)}
            className={inputCls}
          />
          {needsReviewCount > 0 && (
            <a
              href="/budget?flagged=1"
              className="ml-auto text-xs font-medium text-warning underline underline-offset-2"
            >
              {needsReviewCount} need{needsReviewCount === 1 ? 's' : ''} review — fix in Budget →
            </a>
          )}
        </div>

        {isLoading && (
          <div className="text-center text-text-tertiary py-12 text-sm">Loading…</div>
        )}

        {!isLoading && txns.length === 0 && (
          <div className="text-center text-text-muted py-16 text-sm">
            No transactions found for this account{dateFrom || dateTo ? ' in this date range' : ''}.
          </div>
        )}

        {txns.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm min-w-[480px]">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-2 pr-4 font-medium">Date</th>
                  <th className="pb-2 pr-4 font-medium">Merchant</th>
                  <th className="pb-2 pr-4 font-medium text-right">Amount</th>
                  <th className="pb-2 font-medium">Category</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-default">
                {txns.map((txn) => (
                  <tr key={txn.id} className={txn.needsReview ? 'bg-warning-tint-bg' : ''}>
                    <td className="py-2.5 pr-4 text-text-secondary whitespace-nowrap">
                      {txn.transDate.slice(0, 10)}
                    </td>
                    <td className="py-2.5 pr-4 text-text-primary max-w-[240px] truncate" title={txn.rawDescription}>
                      {txn.merchant}
                      {txn.needsReview && <Warning size={12} weight="bold" className="inline ml-1.5 text-warning" />}
                    </td>
                    <td className="py-2.5 pr-4 text-text-primary text-right whitespace-nowrap">
                      {formatCurrency(txn.amount)}
                    </td>
                    <td className="py-2.5">
                      <select
                        value={txn.categoryId ?? ''}
                        onChange={(e) => handleCategoryChange(txn.id, e.target.value)}
                        disabled={savingId === txn.id}
                        className="text-xs bg-transparent border border-border-strong rounded-full px-2 py-0.5 text-text-primary focus:outline-none focus:ring-2 focus:ring-accent disabled:opacity-40"
                      >
                        <option value="">— uncategorized —</option>
                        {categories.map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.name}
                          </option>
                        ))}
                      </select>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Card>
  )
}
