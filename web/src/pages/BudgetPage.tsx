import { useState, useRef, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import useSWR, { mutate } from 'swr'
import { LimitSection } from '../components/Budget/LimitSection'
import { SubscriptionsTab } from '../components/Budget/SubscriptionsTab'
import { useBudgetLimits, useLimitSuggestions } from '../hooks/useBudgetLimits'
import {
  Tooltip as RechartsTooltip, ResponsiveContainer,
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Legend,
} from 'recharts'
import {
  Upload,
  Plus,
  Pencil,
  Trash,
  CaretLeft,
  CaretRight,
  Warning,
  ForkKnife,
  ShoppingCart,
  Car,
  Bus,
  House,
  Heartbeat,
  MusicNotes,
  ShoppingBag,
  Airplane,
  BookOpen,
  Bank,
  Lightning,
  PawPrint,
  Tag,
  ArrowDownLeft,
} from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'

// Categorical icon colors draw from the chart palette's desaturated `pieType`
// ramp (for hues with no semantic meaning) so they stay theme-aware, plus the
// semantic tokens directly where a category already maps onto one (e.g.
// Groceries -> positive).
function getCategoryIconMap(palette: ChartPalette): Record<string, { icon: Icon; color: string }> {
  return {
    'Dining & Restaurants': { icon: ForkKnife,     color: palette.pieType[5] },
    'Groceries':            { icon: ShoppingCart,  color: 'rgb(var(--positive))' },
    'Transportation':       { icon: Bus,           color: palette.pieType[2] },
    'Auto':                 { icon: Car,           color: palette.pieType[0] },
    'Housing':              { icon: House,         color: palette.pieType[6] },
    'Health & Medical':     { icon: Heartbeat,     color: 'rgb(var(--negative))' },
    'Entertainment':        { icon: MusicNotes,    color: palette.pieType[8] },
    'Shopping':             { icon: ShoppingBag,   color: 'rgb(var(--warning))' },
    'Travel':               { icon: Airplane,      color: palette.pieType[1] },
    'Education':            { icon: BookOpen,      color: 'rgb(var(--accent-text))' },
    'Fees & Banking':       { icon: Bank,          color: 'rgb(var(--text-secondary))' },
    'Utilities':            { icon: Lightning,     color: palette.pieType[3] },
    'Pets':                 { icon: PawPrint,      color: palette.pieType[4] },
  }
}
import {
  budgetApi,
  accountsApi,
  type Account,
  type BudgetCategory,
  type BudgetSummaryRow,
  type MonthlyBreakdownRow,
  type Transaction,
} from '../api/client'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Modal } from '../components/ui/Modal'
import { QuickAddTransactionModal } from '../components/QuickAdd/QuickAddTransactionModal'
import { formatCurrency } from '../utils/currency'
import { useChartPalette, type ChartPalette } from '../hooks/useChartPalette'
import { PageGuide } from '../components/PageGuide'
import { refreshReviewCount } from '../hooks/useReviewCount'

// ─── shared styles ───────────────────────────────────────────────────────────
const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

// ─── helpers ─────────────────────────────────────────────────────────────────
function monthLabel(year: number, month: number) {
  return new Date(year, month - 1, 1).toLocaleString('default', {
    month: 'long',
    year: 'numeric',
  })
}

function deltaPct(current: number, avg: number): number | null {
  if (avg <= 0) return null
  return ((current - avg) / avg) * 100
}

// ─── ImportModal ─────────────────────────────────────────────────────────────
function ImportModal({
  onClose,
  onImported,
}: {
  onClose: () => void
  onImported: () => void
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [accountId, setAccountId] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const { data: accounts = [] } = useSWR<Account[]>('/accounts', accountsApi.list)

  const handleStartImport = async () => {
    const f = file
    if (!f) return
    setLoading(true)
    setError('')
    try {
      await budgetApi.importQuick(f, accountId)
      onImported()
      onClose()
    } catch (e: any) {
      setError(e.message ?? 'Import failed.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal open title="Import Transactions" onClose={onClose}>
      <div className="space-y-4">
        {error && <div className="text-negative text-sm">{error}</div>}
        <p className="text-sm text-text-secondary">
          Upload a CSV export from your credit card, checking account, or savings account.
          Transactions are imported immediately — any that can't be auto-classified will appear
          in the <strong>Import</strong> tab for you to review. MyFi suggests categories for them in
          the background over the next few minutes.
        </p>
        <input
          ref={fileRef}
          type="file"
          accept=".csv"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          className="block w-full text-sm text-text-secondary file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:bg-muted file:text-text-primary hover:file:bg-muted"
        />
        {file && (
          <div className="flex items-center gap-2 px-3 py-2 bg-subtle rounded-lg text-sm text-text-secondary">
            <span className="text-positive">✓</span>
            <span>{file.name} selected — ready to import in the background.</span>
          </div>
        )}
        <div>
          <label className={labelCls}>Account <span className="text-text-tertiary">(optional)</span></label>
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
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button onClick={handleStartImport} loading={loading} disabled={!file}>Start Import</Button>
        </div>
      </div>
    </Modal>
  )
}

// ─── EditTransactionModal ─────────────────────────────────────────────────────
function EditTransactionModal({
  txn,
  categories,
  sameVendorCount,
  onClose,
  onSaved,
}: {
  txn: Transaction
  categories: BudgetCategory[]
  sameVendorCount: number
  onClose: () => void
  onSaved: (updated: Transaction) => void
}) {
  const [categoryId, setCategoryId] = useState(txn.categoryId ?? '')
  const [notes, setNotes] = useState(txn.notes ?? '')
  const [accountId, setAccountId] = useState(txn.accountId ?? '')
  const [applyToVendor, setApplyToVendor] = useState(true)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const { data: accounts = [] } = useSWR<Account[]>('/accounts', accountsApi.list)

  const handleSave = async () => {
    setLoading(true)
    setError('')
    try {
      const updated = await budgetApi.updateTransaction(txn.id, {
        categoryId: categoryId || undefined,
        notes: notes || undefined,
        accountId: accountId || undefined,
        applyToVendor: sameVendorCount > 0 && applyToVendor,
      })
      onSaved(updated)
      onClose()
    } catch (e: any) {
      setError(e.message ?? 'Save failed.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal open title="Edit Transaction" onClose={onClose}>
      <div className="space-y-4">
        {error && <div className="text-negative text-sm">{error}</div>}
        <div>
          <p className="text-sm text-text-secondary">{txn.rawDescription}</p>
          <p className="text-lg font-semibold text-text-primary mt-1">{formatCurrency(txn.amount)}</p>
          <p className="text-xs text-text-tertiary">{txn.transDate.slice(0, 10)}</p>
        </div>
        <div>
          <label className={labelCls}>Category</label>
          <select
            value={categoryId}
            onChange={(e) => setCategoryId(e.target.value)}
            className={inputCls}
          >
            <option value="" disabled>— unset —</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </div>
        {sameVendorCount > 0 && (
          <label className="flex items-center gap-2 text-sm text-text-primary cursor-pointer select-none">
            <input
              type="checkbox"
              checked={applyToVendor}
              onChange={(e) => setApplyToVendor(e.target.checked)}
              className="accent-accent"
            />
            Also apply to {sameVendorCount} other unreviewed{' '}
            <span className="font-medium text-text-primary">{txn.merchant}</span>{' '}
            transaction{sameVendorCount !== 1 ? 's' : ''}
          </label>
        )}
        <div>
          <label className={labelCls}>Notes</label>
          <input
            type="text"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Optional note…"
            className={inputCls}
          />
        </div>
        <div>
          <label className={labelCls}>Account</label>
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
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSave} loading={loading}>Save</Button>
        </div>
      </div>
    </Modal>
  )
}

// ─── Chart helpers ───────────────────────────────────────────────────────────
// Category bar colors come from palette.forecast[i] at each usage site
// rather than a fixed array, since the palette itself is theme-dependent.

const MONTH_LABELS = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec']

function ChartTooltip({ active, payload, label, showTotal, palette }: any) {
  if (!active || !payload?.length) return null
  const total = showTotal ? payload.reduce((s: number, p: any) => s + (p.value ?? 0), 0) : null
  return (
    <div style={{
      background: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}`,
      borderRadius: '8px', padding: '10px 14px',
      fontSize: '13px', boxShadow: '0 4px 16px rgb(20 16 10 / 0.08)',
    }}>
      {label && <div style={{ color: palette.tooltipMuted, marginBottom: '6px' }}>{label}</div>}
      {[...payload].reverse().map((p: any) => (
        <div key={p.name} style={{ display: 'flex', gap: '8px', justifyContent: 'space-between', marginBottom: '2px' }}>
          <span style={{ color: p.fill ?? p.color }}>{p.name}</span>
          <span style={{ color: palette.tooltipText, fontWeight: 600 }}>{formatCurrency(p.value)}</span>
        </div>
      ))}
      {total !== null && (
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'space-between', marginTop: '6px', paddingTop: '6px', borderTop: `1px solid ${palette.tooltipBorder}` }}>
          <span style={{ color: palette.tooltipMuted }}>Total</span>
          <span style={{ color: palette.tooltipText, fontWeight: 700 }}>{formatCurrency(total)}</span>
        </div>
      )}
    </div>
  )
}

// Fetches and derives the rolling 12-month breakdown. Returns null while loading.
function useRolling12Months() {
  const now = new Date()
  const thisYear = now.getFullYear()
  const thisMonth = now.getMonth() + 1

  const { data: currYearData = [] } = useSWR(
    `/budget/monthly-breakdown?year=${thisYear}`,
    () => budgetApi.monthlyBreakdown(thisYear),
  )
  const { data: prevYearData = [] } = useSWR(
    `/budget/monthly-breakdown?year=${thisYear - 1}`,
    () => budgetApi.monthlyBreakdown(thisYear - 1),
  )

  // Tag each row with its calendar year then build the 12-month window
  const allRows = [
    ...prevYearData.map(r => ({ ...r, year: thisYear - 1 })),
    ...currYearData.map(r => ({ ...r, year: thisYear })),
  ]

  const window12: { year: number; month: number; label: string }[] = []
  for (let offset = 11; offset >= 0; offset--) {
    const d = new Date(thisYear, thisMonth - 1 - offset, 1)
    window12.push({
      year: d.getFullYear(),
      month: d.getMonth() + 1,
      label: MONTH_LABELS[d.getMonth()],
    })
  }

  // Per-month totals and per-category sums across the window
  const catTotals: Record<string, number> = {}
  const byMonthCat: Record<string, Record<string, number>> = {}

  for (const r of allRows) {
    const key = `${r.year}-${r.month}`
    if (!window12.find(w => w.year === r.year && w.month === r.month)) continue
    catTotals[r.categoryName] = (catTotals[r.categoryName] ?? 0) + r.total
    if (!byMonthCat[key]) byMonthCat[key] = {}
    byMonthCat[key][r.categoryName] = r.total
  }

  const categories = Object.entries(catTotals).sort((a, b) => b[1] - a[1]).map(([n]) => n)

  const chartData = window12.map(({ year, month, label }) => {
    const key = `${year}-${month}`
    const entry: Record<string, any> = { label }
    for (const cat of categories) entry[cat] = byMonthCat[key]?.[cat] ?? 0
    return entry
  })

  // Average monthly total across months that have any spending
  const monthlyTotals = window12.map(({ year, month }) => {
    const key = `${year}-${month}`
    return Object.values(byMonthCat[key] ?? {}).reduce((s, v) => s + v, 0)
  }).filter(t => t > 0)
  const avgMonthly = monthlyTotals.length ? monthlyTotals.reduce((s, v) => s + v, 0) / monthlyTotals.length : 0

  return { categories, chartData, avgMonthly, hasData: allRows.length > 0 }
}

function MonthTotalTile({ total, label, isLoading }: { total: number; label: string; isLoading: boolean }) {
  return (
    <div className="flex-1 min-w-[220px] bg-accent-tint-bg border border-accent-tint-border rounded-lg px-5 py-4 flex items-center justify-between">
      <div>
        <p className="text-xs text-accent-text uppercase tracking-wider mb-1">{label}</p>
        {isLoading
          ? <div className="h-8 w-28 bg-muted/50 rounded animate-pulse mt-1" />
          : <p className="text-2xl font-bold text-text-primary">{formatCurrency(total)}</p>
        }
      </div>
      <p className="text-xs text-accent-text">total spend</p>
    </div>
  )
}

function AvgMonthlyTile({ avg }: { avg: number }) {
  return (
    <div className="flex-1 min-w-[220px] bg-subtle/50 border border-border-strong/50 rounded-lg px-5 py-4 flex items-center justify-between">
      <div>
        <p className="text-xs text-text-tertiary uppercase tracking-wider mb-1">Avg Monthly Spend</p>
        <p className="text-2xl font-bold text-text-primary">{formatCurrency(avg)}</p>
      </div>
      <p className="text-xs text-text-muted">last 12 months</p>
    </div>
  )
}

function MonthIncomeTile({ income, isLoading }: { income: number; isLoading: boolean }) {
  if (income <= 0) return null
  return (
    <div className="flex-1 min-w-[220px] bg-positive-tint-bg border border-positive/30 rounded-lg px-5 py-4 flex items-center justify-between">
      <div>
        <p className="text-xs text-positive uppercase tracking-wider mb-1">Income</p>
        {isLoading
          ? <div className="h-8 w-28 bg-muted/50 rounded animate-pulse mt-1" />
          : <p className="text-2xl font-bold text-text-primary">{formatCurrency(income)}</p>
        }
      </div>
      <ArrowDownLeft size={20} className="text-positive" weight="bold" />
    </div>
  )
}

function Rolling12MonthsChart() {
  const palette = useChartPalette()
  const { categories, chartData, hasData } = useRolling12Months()
  if (!hasData) return null

  return (
    <div>
      <h3 className="text-sm font-medium text-text-secondary mb-4">Spending by Category — Last 12 Months</h3>
      <ResponsiveContainer width="100%" height={300}>
        <BarChart data={chartData} margin={{ top: 0, right: 0, left: 10, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
          <XAxis dataKey="label" tick={{ fill: palette.tick, fontSize: 12 }} axisLine={false} tickLine={false} />
          <YAxis
            tickFormatter={(v) => `$${v >= 1000 ? `${(v / 1000).toFixed(0)}k` : v}`}
            tick={{ fill: palette.tick, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            width={45}
          />
          <RechartsTooltip content={<ChartTooltip showTotal palette={palette} />} cursor={{ fill: 'rgba(127,127,127,0.06)' }} />
          <Legend
            wrapperStyle={{ fontSize: '12px', color: palette.tickMuted, paddingTop: '12px' }}
            formatter={(value) => <span style={{ color: palette.tickMuted }}>{value}</span>}
          />
          {[...categories].reverse().map((cat, reversedIdx) => {
            const originalIdx = categories.length - 1 - reversedIdx
            return (
              <Bar key={cat} dataKey={cat} stackId="a" fill={palette.forecast[originalIdx % palette.forecast.length]} maxBarSize={48} />
            )
          })}
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}

// ─── OverviewTab ──────────────────────────────────────────────────────────────
/** Remembers that the user dismissed the "set monthly limits" prompt. */
const LIMITS_BANNER_KEY = 'myfi-budget-limits-banner-dismissed'

// A category + date range to open the Transactions tab with.
interface TransactionFilter {
  categoryId: string
  from: string
  to: string
}

/** First and last day of a calendar month as YYYY-MM-DD (both inclusive). */
function monthRange(year: number, month: number): { from: string; to: string } {
  const mm = String(month).padStart(2, '0')
  const last = String(new Date(year, month, 0).getDate()).padStart(2, '0')
  return { from: `${year}-${mm}-01`, to: `${year}-${mm}-${last}` }
}

function OverviewTab({
  categories,
  onSwitchToTransactions,
  onSwitchToFlagged,
  onOpenCategory,
  period,
  onPeriodChange,
}: {
  categories: BudgetCategory[]
  onSwitchToTransactions: () => void
  onSwitchToFlagged: () => void
  onOpenCategory: (filter: TransactionFilter) => void
  /** The month shown, owned by the page so it survives switching tabs. */
  period: { year: number; month: number }
  onPeriodChange: (period: { year: number; month: number }) => void
}) {
  const { year, month } = period

  const { avgMonthly } = useRolling12Months()

  const prevMonth = () => onPeriodChange(month === 1 ? { year: year - 1, month: 12 } : { year, month: month - 1 })
  const nextMonth = () => onPeriodChange(month === 12 ? { year: year + 1, month: 1 } : { year, month: month + 1 })

  const { data, isLoading } = useSWR(
    `/budget/summary?year=${year}&month=${month}`,
    () => budgetApi.summary(year, month, false),
  )

  const { data: limitList, mutate: mutateLimits } = useBudgetLimits()
  const suggestions = useLimitSuggestions()
  const limits = new Map((limitList ?? []).map((l) => [l.categoryId, l.monthlyLimit]))
  const [bannerDismissed, setBannerDismissed] = useState(() => {
    try {
      return localStorage.getItem(LIMITS_BANNER_KEY) === '1'
    } catch {
      return false
    }
  })

  const summaryRows = data?.categories ?? []
  // A category with a limit but no spending this month still gets a card, so its
  // empty progress bar is visible.
  const rows = [
    ...summaryRows,
    ...categories
      .filter((c) => limits.has(c.id) && !summaryRows.some((r) => r.categoryId === c.id))
      .map((c) => ({ categoryId: c.id, categoryName: c.name, total: 0, monthAvg: 0, count: 0 })),
  ]
  const reviewCount = data?.reviewCount ?? 0
  const monthlyIncome = data?.monthlyIncome ?? 0
  const grandTotal = rows.reduce((s, r) => s + r.total, 0)

  // Suggestions are keyed by category name; limits attach to real categories only.
  const suggestable = categories
    .filter((c) => !(c.isSystem && (c.name === 'Transfer' || c.name === 'Income')) && suggestions.has(c.name))
    .map((c) => ({ id: c.id, amount: suggestions.get(c.name)! }))
  const showLimitsBanner = !!limitList && limitList.length === 0 && suggestable.length > 0 && !bannerDismissed

  const applySuggestions = async () => {
    await Promise.all(suggestable.map((s) => budgetApi.setLimit(s.id, s.amount)))
    await mutateLimits()
  }
  const dismissBanner = () => {
    setBannerDismissed(true)
    try {
      localStorage.setItem(LIMITS_BANNER_KEY, '1')
    } catch {
      /* ignore: the banner just returns next visit */
    }
  }

  return (
    <div className="space-y-5">
      {/* Tiles row */}
      <div className="flex gap-3 flex-wrap">
        <MonthTotalTile total={grandTotal} label={monthLabel(year, month)} isLoading={isLoading} />
        <MonthIncomeTile income={monthlyIncome} isLoading={isLoading} />
        {avgMonthly > 0 && <AvgMonthlyTile avg={avgMonthly} />}
      </div>

      {/* Month navigation */}
      <div className="flex items-center justify-end gap-1">
        <button
          onClick={prevMonth}
          className="p-1.5 text-text-secondary hover:text-text-primary rounded-lg hover:bg-subtle"
        >
          <CaretLeft size={16} weight="bold" />
        </button>
        <span className="text-sm font-semibold text-text-primary bg-subtle border border-border-strong rounded-lg px-4 py-1.5 w-40 text-center tracking-wide">
          {monthLabel(year, month)}
        </span>
        <button
          onClick={nextMonth}
          className="p-1.5 text-text-secondary hover:text-text-primary rounded-lg hover:bg-subtle"
        >
          <CaretRight size={16} weight="bold" />
        </button>
      </div>

      {reviewCount > 0 && (
        <div className="flex items-center gap-2 p-3 bg-warning-tint-bg border border-warning/30 rounded-lg text-warning text-sm">
          <Warning size={16} weight="bold" />
          {reviewCount} transaction{reviewCount !== 1 ? 's' : ''} couldn't be auto-classified.{' '}
          <button
            className="underline hover:opacity-75 font-medium"
            onClick={onSwitchToFlagged}
          >
            Review flagged transactions →
          </button>
        </div>
      )}

      {showLimitsBanner && (
        <div className="flex flex-wrap items-center gap-3 p-3 bg-accent-tint-bg border border-accent-tint-border rounded-lg text-sm text-text-secondary">
          <span className="flex-1 min-w-[220px]">
            Set monthly limits to see how each category is tracking. Based on your last 3 months, we suggest limits for{' '}
            {suggestable.length} categor{suggestable.length === 1 ? 'y' : 'ies'}.
          </span>
          <button
            type="button"
            onClick={() => void applySuggestions()}
            className="font-medium text-accent-text hover:text-accent"
          >
            Apply suggestions
          </button>
          <button type="button" onClick={dismissBanner} className="text-text-tertiary hover:text-text-primary">
            Not now
          </button>
        </div>
      )}

      {isLoading && (
        <div className="text-center text-text-tertiary py-12 text-sm">Loading…</div>
      )}

      {!isLoading && rows.length === 0 && (
        <div className="text-center text-text-muted py-16 text-sm">
          No spending data for this period. Import a statement on the{' '}
          <button
            className="text-accent-text hover:text-accent-text underline"
            onClick={onSwitchToTransactions}
          >
            Transactions tab
          </button>{' '}
          to get started.
        </div>
      )}

      {rows.length > 0 && (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {rows
              .slice()
              .sort((a, b) => {
                // Categories not seen in recent months float to the end; otherwise alphabetical.
                const aNew = a.monthAvg === 0
                const bNew = b.monthAvg === 0
                if (aNew !== bNew) return aNew ? 1 : -1
                return a.categoryName.localeCompare(b.categoryName)
              })
              .map((row) => {
                // Spending in no category comes back as an "Other" row with a
                // made-up id; there is nothing real to filter by, so it isn't a link.
                const isRealCategory = categories.some((c) => c.id === row.categoryId)
                return (
                  <CategoryCard
                    key={row.categoryId}
                    row={row}
                    isAnnual={false}
                    onClick={
                      isRealCategory
                        ? () => onOpenCategory({ categoryId: row.categoryId, ...monthRange(year, month) })
                        : undefined
                    }
                    limit={
                      isRealCategory
                        ? {
                            amount: limits.get(row.categoryId),
                            suggestion: suggestions.get(row.categoryName),
                            onSave: async (amount) => {
                              await budgetApi.setLimit(row.categoryId, amount)
                              await mutateLimits()
                            },
                            onClear: async () => {
                              await budgetApi.deleteLimit(row.categoryId)
                              await mutateLimits()
                            },
                          }
                        : undefined
                    }
                  />
                )
              })}
          </div>
        </>
      )}

      <div className="pt-2 border-t border-border-default">
        <Rolling12MonthsChart />
      </div>
    </div>
  )
}

/** What a category card needs to show and edit its monthly limit. */
interface CategoryLimit {
  amount?: number
  suggestion?: number
  onSave: (amount: number) => Promise<void>
  onClear: () => Promise<void>
}

function CategoryCard({
  row,
  isAnnual,
  onClick,
  limit,
}: {
  row: BudgetSummaryRow
  isAnnual: boolean
  onClick?: () => void
  limit?: CategoryLimit
}) {
  const palette = useChartPalette()
  const delta = isAnnual ? null : deltaPct(row.total, row.monthAvg)
  const { icon: Icon, color } = getCategoryIconMap(palette)[row.categoryName] ?? { icon: Tag, color: 'rgb(var(--text-tertiary))' }

  // The figures open the category's transactions; the limit controls below are
  // separate buttons, so they can't live inside this one.
  const Figures = onClick ? 'button' : 'div'

  return (
    <div className="bg-subtle/50 border border-border-strong/50 rounded-lg p-4">
      <Figures
        {...(onClick
          ? {
              type: 'button' as const,
              onClick,
              title: `See ${row.categoryName} transactions`,
            }
          : {})}
        className={`block w-full text-left space-y-2 ${
          onClick
            ? '-m-2 p-2 rounded-lg transition-colors hover:bg-accent-tint-bg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent'
            : ''
        }`}
      >
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0">
            <Icon size={16} style={{ color }} className="flex-shrink-0" weight="bold" />
            <p className="text-sm font-medium text-text-primary truncate">{row.categoryName}</p>
          </div>
          {delta !== null && Math.abs(delta) >= 10 && (
            <span
              className={`text-xs px-1.5 py-0.5 rounded font-medium flex-shrink-0 ${
                delta > 0
                  ? 'bg-negative-tint-bg text-negative'
                  : 'bg-positive-tint-bg text-positive'
              }`}
            >
              {delta > 0 ? '↑' : '↓'} {Math.abs(Math.round(delta))}% vs avg
            </span>
          )}
        </div>
        <p className="text-2xl font-bold text-text-primary">{formatCurrency(row.total)}</p>
        {!isAnnual && row.monthAvg > 0 && (
          <p className="text-xs text-text-tertiary">{formatCurrency(row.monthAvg)} avg / mo</p>
        )}
      </Figures>
      {limit && (
        <div className="mt-2">
          <LimitSection
            spent={row.total}
            limit={limit.amount}
            suggestion={limit.suggestion}
            onSave={limit.onSave}
            onClear={limit.onClear}
          />
        </div>
      )}
    </div>
  )
}

// ─── CategoryMultiSelect ──────────────────────────────────────────────────────
function CategoryMultiSelect({
  categories,
  selected,
  onChange,
}: {
  categories: BudgetCategory[]
  selected: string[]
  onChange: (ids: string[]) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const toggle = (id: string) => {
    onChange(selected.includes(id) ? selected.filter((s) => s !== id) : [...selected, id])
  }

  const label =
    selected.length === 0
      ? 'All categories'
      : selected.length === 1
        ? (categories.find((c) => c.id === selected[0])?.name ?? '1 category')
        : `${selected.length} categories`

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-2 bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent min-w-[160px]"
      >
        <span className="flex-1 text-left truncate">{label}</span>
        <CaretRight size={14} weight="bold" className={`text-text-tertiary transition-transform flex-shrink-0 ${open ? 'rotate-90' : ''}`} />
      </button>

      {open && (
        <div className="absolute z-20 mt-1 w-56 bg-surface border border-border-strong rounded-lg shadow-float py-1 max-h-72 overflow-y-auto">
          {selected.length > 0 && (
            <>
              <button
                onClick={() => onChange([])}
                className="w-full text-left px-3 py-1.5 text-xs text-text-tertiary hover:text-text-primary hover:bg-subtle"
              >
                Clear selection
              </button>
              <div className="border-t border-border-default my-1" />
            </>
          )}
          {categories.map((c) => (
            <label
              key={c.id}
              className="flex items-center gap-2.5 px-3 py-1.5 text-sm text-text-primary hover:bg-subtle cursor-pointer"
            >
              <input
                type="checkbox"
                checked={selected.includes(c.id)}
                onChange={() => toggle(c.id)}
                className="w-3.5 h-3.5 accent-accent flex-shrink-0"
              />
              {c.name}
            </label>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── IncomeTab ────────────────────────────────────────────────────────────────
function IncomeTab() {
  const palette = useChartPalette()
  const now = new Date()
  const thisYear = now.getFullYear()
  const thisMonth = now.getMonth() + 1

  const { data: currYearData = [] } = useSWR(
    `/budget/income-breakdown?year=${thisYear}`,
    () => budgetApi.incomeBreakdown(thisYear),
  )
  const { data: prevYearData = [] } = useSWR(
    `/budget/income-breakdown?year=${thisYear - 1}`,
    () => budgetApi.incomeBreakdown(thisYear - 1),
  )

  // Build rolling 12-month window
  const window12: { year: number; month: number; label: string }[] = []
  for (let offset = 11; offset >= 0; offset--) {
    const d = new Date(thisYear, thisMonth - 1 - offset, 1)
    window12.push({ year: d.getFullYear(), month: d.getMonth() + 1, label: MONTH_LABELS[d.getMonth()] })
  }

  const byKey: Record<string, number> = {}
  for (const r of [...prevYearData.map(r => ({ ...r, year: thisYear - 1 })), ...currYearData.map(r => ({ ...r, year: thisYear }))]) {
    byKey[`${r.year}-${r.month}`] = r.total
  }

  const chartData = window12.map(({ year, month, label }) => ({
    label,
    Income: byKey[`${year}-${month}`] ?? 0,
  }))

  const totals = chartData.map(d => d.Income).filter(v => v > 0)
  const ytdTotal = currYearData.reduce((s, r) => s + r.total, 0)
  const avgMonthly = totals.length ? totals.reduce((s, v) => s + v, 0) / totals.length : 0
  const hasData = totals.length > 0

  if (!hasData) {
    return (
      <div className="py-16 text-center text-text-muted text-sm">
        No income recorded yet. Import a bank or checking account CSV to get started.
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Summary tiles */}
      <div className="flex flex-wrap gap-3">
        <div className="flex-1 bg-positive-tint-bg border border-positive/30 rounded-lg px-5 py-4">
          <p className="text-xs text-positive uppercase tracking-wider mb-1">YTD Income</p>
          <p className="text-2xl font-bold text-text-primary">{formatCurrency(ytdTotal)}</p>
          <p className="text-xs text-text-muted mt-0.5">{thisYear} year to date</p>
        </div>
        <div className="flex-1 bg-subtle/50 border border-border-strong/50 rounded-lg px-5 py-4">
          <p className="text-xs text-text-tertiary uppercase tracking-wider mb-1">Avg Monthly</p>
          <p className="text-2xl font-bold text-text-primary">{formatCurrency(avgMonthly)}</p>
          <p className="text-xs text-text-muted mt-0.5">last 12 months</p>
        </div>
      </div>

      {/* Bar chart */}
      <div>
        <p className="text-xs text-text-tertiary uppercase tracking-wider mb-3">Monthly Income — Rolling 12 Months</p>
        <ResponsiveContainer width="100%" height={220}>
          <BarChart data={chartData} barSize={28} margin={{ top: 4, right: 0, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
            <XAxis dataKey="label" tick={{ fill: palette.tick, fontSize: 11 }} axisLine={false} tickLine={false} />
            <YAxis tick={{ fill: palette.tick, fontSize: 11 }} axisLine={false} tickLine={false}
              tickFormatter={(v) => `$${(v / 1000).toFixed(0)}k`} width={44} />
            <RechartsTooltip content={<ChartTooltip showTotal={false} palette={palette} />} cursor={{ fill: 'rgba(127,127,127,0.06)' }} />
            <Bar dataKey="Income" fill={palette.goalHigh} radius={[3, 3, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>

      {/* Monthly table */}
      <div>
        <p className="text-xs text-text-tertiary uppercase tracking-wider mb-3">Month by Month</p>
        <div className="divide-y divide-border-default">
          {[...chartData].reverse().filter(d => d.Income > 0).map((d) => (
            <div key={d.label} className="flex justify-between items-center py-2.5 text-sm">
              <span className="text-text-secondary">{d.label}</span>
              <span className="text-text-primary font-medium">{formatCurrency(d.Income)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ─── TransactionsTab ──────────────────────────────────────────────────────────
const TXN_PAGE_SIZE = 100
const PHONE_QUERY = '(max-width: 639px)' // below Tailwind's sm breakpoint

function TransactionsTab({
  categories,
  initialFlagged = false,
  initialFilter,
}: {
  categories: BudgetCategory[]
  initialFlagged?: boolean
  initialFilter?: TransactionFilter | null
}) {
  const [search, setSearch] = useState('')
  const [filterCategoryIds, setFilterCategoryIds] = useState<string[]>(
    initialFilter ? [initialFilter.categoryId] : [],
  )
  const [filterFlagged, setFilterFlagged] = useState(initialFlagged)
  const [dateFrom, setDateFrom] = useState(initialFilter?.from ?? '')
  const [dateTo, setDateTo] = useState(initialFilter?.to ?? '')
  const [editTxn, setEditTxn] = useState<Transaction | null>(null)
  const [showAdd, setShowAdd] = useState(false)
  const [showImport, setShowImport] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  // Search once typing pauses rather than on every keystroke.
  const [debouncedSearch, setDebouncedSearch] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), 300)
    return () => clearTimeout(t)
  }, [search])

  // Load the newest rows first and more on request; a filter change starts over.
  const filterKey = `from=${dateFrom}&to=${dateTo}&cats=${filterCategoryIds.join(',')}&q=${debouncedSearch}&flagged=${filterFlagged}`
  const [paging, setPaging] = useState({ filterKey, limit: TXN_PAGE_SIZE })
  const limit = paging.filterKey === filterKey ? paging.limit : TXN_PAGE_SIZE
  const loadMore = () => setPaging({ filterKey, limit: limit + TXN_PAGE_SIZE })

  const swrKey = `/budget/transactions?${filterKey}&limit=${limit}`
  const { data: txns = [], isLoading, isValidating } = useSWR(
    swrKey,
    () =>
      budgetApi.listTransactions({
        from: dateFrom || undefined,
        to: dateTo || undefined,
        categoryIds: filterCategoryIds.length ? filterCategoryIds : undefined,
        search: debouncedSearch || undefined,
        needsReview: filterFlagged || undefined,
        limit,
      }),
    { keepPreviousData: true },
  )
  const hasMore = txns.length >= limit

  // Render only the layout for this screen size; both at once doubles the DOM.
  const [isPhone, setIsPhone] = useState(() => window.matchMedia(PHONE_QUERY).matches)
  useEffect(() => {
    const mq = window.matchMedia(PHONE_QUERY)
    const onChange = () => setIsPhone(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const flaggedKey = '/budget/transactions?flagged=true&check'
  const { data: flaggedTxns = [] } = useSWR(flaggedKey, () =>
    budgetApi.listTransactions({ needsReview: true }),
  )
  const hasFlagged = flaggedTxns.length > 0

  const { data: duplicates = [], mutate: mutateDupes } = useSWR('/budget/duplicates', budgetApi.listDuplicates)
  const [showDupes, setShowDupes] = useState(false)
  const [deletingDupes, setDeletingDupes] = useState(false)

  const handleDeleteDuplicates = async () => {
    setDeletingDupes(true)
    try {
      await budgetApi.bulkDeleteTransactions(duplicates.map((t) => t.id))
      await Promise.all([mutateDupes(), refreshList()])
      setShowDupes(false)
    } finally {
      setDeletingDupes(false)
    }
  }

  // Edits can change which transactions need review, so refresh that too.
  const refreshList = () => Promise.all([mutate(swrKey), mutate(flaggedKey), refreshReviewCount()])
  const handleAdded = refreshList
  const handleImported = refreshList

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this transaction?')) return
    setDeletingId(id)
    try {
      await budgetApi.deleteTransaction(id)
      refreshList()
    } finally {
      setDeletingId(null)
    }
  }

  const handleSaved = refreshList

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <input
          type="text"
          placeholder="Search merchant…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent w-44"
        />
        <CategoryMultiSelect
          categories={categories}
          selected={filterCategoryIds}
          onChange={setFilterCategoryIds}
        />
        {hasFlagged && (
          <button
            onClick={() => setFilterFlagged((f) => !f)}
            className={`flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium border transition-colors ${
              filterFlagged
                ? 'bg-warning-tint-bg border-warning/40 text-warning'
                : 'bg-subtle border-border-strong text-text-secondary hover:text-text-primary'
            }`}
          >
            <Warning size={14} weight="bold" />
            Flagged only
          </button>
        )}
        <div className="flex items-center gap-1.5 text-sm text-text-secondary">
          <input
            type="date"
            value={dateFrom}
            onChange={(e) => setDateFrom(e.target.value)}
            className="bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
          />
          <span>–</span>
          <input
            type="date"
            value={dateTo}
            onChange={(e) => setDateTo(e.target.value)}
            className="bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
          />
        </div>
        <div className="flex gap-2 ml-auto">
          <Button size="sm" variant="secondary" onClick={() => setShowImport(true)}>
            <Upload size={15} weight="bold" />
            Import CSV
          </Button>
          <Button size="sm" onClick={() => setShowAdd(true)}>
            <Plus size={15} weight="bold" />
            Add Transaction
          </Button>
        </div>
      </div>

      {duplicates.length > 0 && (
        <div className="flex items-center gap-2 rounded-lg bg-warning-tint-bg border border-warning/40 px-4 py-3 text-sm text-warning">
          <Warning size={16} weight="bold" />
          <span>
            {duplicates.length} transaction{duplicates.length !== 1 ? 's' : ''} imported by CSV{' '}
            {duplicates.length !== 1 ? 'are' : 'is'} also synced from your bank.
          </span>
          <button onClick={() => setShowDupes(true)} className="font-semibold underline underline-offset-2 ml-1">
            Review duplicates →
          </button>
        </div>
      )}

      {isLoading && (
        <div className="text-center text-text-tertiary py-12 text-sm">Loading…</div>
      )}

      {!isLoading && txns.length === 0 && (
        <div className="text-center text-text-muted py-16 text-sm">No transactions found.</div>
      )}

      {/* Phones: a compact list; tap a row to edit. */}
      {txns.length > 0 && isPhone && (
        <ul className="divide-y divide-border-default">
          {txns.map((txn) => (
            <li key={txn.id} className={`flex items-center gap-2 ${txn.needsReview ? 'bg-warning-tint-bg -mx-2 px-2' : ''}`}>
              <button onClick={() => setEditTxn(txn)} className="flex-1 min-w-0 flex items-center gap-3 py-2.5 text-left">
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-text-primary truncate">
                    {txn.merchant}
                    {txn.needsReview && <span className="ml-1.5 text-warning text-xs">⚠</span>}
                  </p>
                  <p className="text-xs text-text-tertiary truncate">
                    {[txn.transDate.slice(0, 10), txn.categoryName ?? 'Uncategorized', txn.accountLabel].filter(Boolean).join(' · ')}
                  </p>
                </div>
                <span className="text-sm text-text-primary whitespace-nowrap">{formatCurrency(txn.amount)}</span>
              </button>
              <button
                onClick={() => handleDelete(txn.id)}
                disabled={deletingId === txn.id}
                className="p-2 text-text-tertiary hover:text-negative rounded disabled:opacity-40"
                aria-label={`Delete ${txn.merchant}`}
              >
                <Trash size={16} weight="bold" />
              </button>
            </li>
          ))}
        </ul>
      )}

      {txns.length > 0 && !isPhone && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm min-w-[560px]">
            <thead>
              <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                <th className="pb-2 pr-4 font-medium">Date</th>
                <th className="pb-2 pr-4 font-medium">Merchant</th>
                <th className="pb-2 pr-4 font-medium text-right">Amount</th>
                <th className="pb-2 pr-4 font-medium">Category</th>
                <th className="pb-2 pr-4 font-medium">Account</th>
                <th className="pb-2 pr-4 font-medium">Notes</th>
                <th className="pb-2 font-medium" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border-default">
              {txns.map((txn) => (
                <tr
                  key={txn.id}
                  className={`${txn.needsReview ? 'bg-warning-tint-bg' : ''}`}
                >
                  <td className="py-2.5 pr-4 text-text-secondary whitespace-nowrap">
                    {txn.transDate.slice(0, 10)}
                  </td>
                  <td className="py-2.5 pr-4 text-text-primary max-w-[220px] truncate" title={txn.rawDescription}>
                    {txn.merchant}
                    {txn.needsReview && (
                      <span className="ml-1.5 text-warning text-xs">⚠</span>
                    )}
                  </td>
                  <td className="py-2.5 pr-4 text-text-primary text-right whitespace-nowrap">
                    {formatCurrency(txn.amount)}
                  </td>
                  <td className="py-2.5 pr-4">
                    {txn.categoryName ? (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-muted text-text-primary">
                        {txn.categoryName}
                      </span>
                    ) : (
                      <span className="text-text-muted text-xs">—</span>
                    )}
                  </td>
                  <td className="py-2.5 pr-4 text-text-tertiary text-xs whitespace-nowrap">
                    {txn.accountLabel ?? ''}
                  </td>
                  <td className="py-2.5 pr-4 text-text-tertiary text-xs max-w-[160px] truncate">
                    {txn.notes ?? ''}
                  </td>
                  <td className="py-2.5">
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => setEditTxn(txn)}
                        className="p-1 text-text-tertiary hover:text-text-primary rounded"
                        title="Edit"
                      >
                        <Pencil size={14} weight="bold" />
                      </button>
                      <button
                        onClick={() => handleDelete(txn.id)}
                        disabled={deletingId === txn.id}
                        className="p-1 text-text-tertiary hover:text-negative rounded disabled:opacity-40"
                        title="Delete"
                      >
                        <Trash size={14} weight="bold" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {hasMore && (
        <div className="flex justify-center">
          <Button size="sm" variant="secondary" disabled={isValidating} onClick={loadMore}>
            {isValidating ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}

      {editTxn && (
        <EditTransactionModal
          txn={editTxn}
          categories={categories}
          sameVendorCount={flaggedTxns.filter(t => t.merchant.toUpperCase() === editTxn.merchant.toUpperCase() && t.id !== editTxn.id).length}
          onClose={() => setEditTxn(null)}
          onSaved={handleSaved}
        />
      )}
      {showAdd && (
        <QuickAddTransactionModal
          onClose={() => setShowAdd(false)}
          onAdded={handleAdded}
        />
      )}
      {showImport && (
        <ImportModal
          onClose={() => setShowImport(false)}
          onImported={handleImported}
        />
      )}
      {showDupes && (
        <Modal open title="Duplicate transactions" onClose={() => setShowDupes(false)}>
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">
              These CSV-imported transactions match a transaction your bank also synced (same date and
              amount). The bank-synced copy is kept and stays up to date — removing these clears the
              duplicates. This can't be undone.
            </p>
            <div className="max-h-72 overflow-y-auto border border-border-default rounded-lg divide-y divide-border-default">
              {duplicates.map((t) => (
                <div key={t.id} className="flex items-center justify-between px-3 py-2 text-sm">
                  <div className="min-w-0">
                    <div className="text-text-primary truncate">{t.merchant}</div>
                    <div className="text-xs text-text-tertiary">
                      {t.transDate.slice(0, 10)}
                      {t.accountLabel ? ` · ${t.accountLabel}` : ''}
                    </div>
                  </div>
                  <div className="text-text-secondary whitespace-nowrap pl-3">
                    {formatCurrency(t.amount)}
                  </div>
                </div>
              ))}
            </div>
            <div className="flex justify-end gap-3">
              <Button variant="ghost" onClick={() => setShowDupes(false)}>
                Cancel
              </Button>
              <Button variant="danger" onClick={handleDeleteDuplicates} loading={deletingDupes}>
                Delete {duplicates.length} duplicate{duplicates.length !== 1 ? 's' : ''}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

// ─── Recurring Tab ────────────────────────────────────────────────────────────

function RecurringTab() {
  const { data: rules = [], isLoading, mutate: mutateRules } = useSWR('/budget/recurring', budgetApi.listRecurring)
  const [showAdd, setShowAdd] = useState(false)
  const [stopping, setStopping] = useState<string | null>(null)

  const handleStop = async (id: string, merchant: string) => {
    if (!window.confirm(`Stop "${merchant}" from repeating? Transactions already added are kept.`)) return
    setStopping(id)
    try {
      await budgetApi.deleteRecurring(id)
      await mutateRules()
    } finally {
      setStopping(null)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <p className="text-sm text-text-tertiary">
          Bills and income that repeat, like a mortgage or paycheck. Each one is added to your transactions automatically on its due date.
        </p>
        <Button size="sm" onClick={() => setShowAdd(true)} className="shrink-0">+ Add Recurring</Button>
      </div>

      {isLoading ? (
        <div className="text-text-tertiary text-sm">Loading…</div>
      ) : rules.length === 0 ? (
        <div className="text-text-tertiary text-sm text-center py-6">No recurring transactions yet.</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm min-w-[560px]">
            <thead>
              <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                <th className="pb-2 pr-4 font-medium">Description</th>
                <th className="pb-2 pr-4 font-medium text-right">Amount</th>
                <th className="pb-2 pr-4 font-medium">Category</th>
                <th className="pb-2 pr-4 font-medium">Repeats</th>
                <th className="pb-2 pr-4 font-medium">Next</th>
                <th className="pb-2 pr-4 font-medium">Account</th>
                <th className="pb-2 font-medium" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border-default">
              {rules.map((r) => (
                <tr key={r.id}>
                  <td className="py-2.5 pr-4 text-text-primary max-w-[220px] truncate">{r.merchant}</td>
                  <td className="py-2.5 pr-4 text-text-primary text-right whitespace-nowrap">{formatCurrency(r.amount)}</td>
                  <td className="py-2.5 pr-4 text-text-secondary">{r.categoryName ?? '—'}</td>
                  <td className="py-2.5 pr-4 text-text-secondary whitespace-nowrap">
                    {r.frequency === 'monthly' ? 'Monthly' : 'Every 2 weeks'}
                  </td>
                  <td className="py-2.5 pr-4 text-text-secondary whitespace-nowrap">{r.nextDate.slice(0, 10)}</td>
                  <td className="py-2.5 pr-4 text-text-secondary">{r.accountLabel ?? '—'}</td>
                  <td className="py-2.5 text-right">
                    <button
                      onClick={() => handleStop(r.id, r.merchant)}
                      disabled={stopping === r.id}
                      className="text-xs text-text-tertiary hover:text-negative transition-colors disabled:opacity-50"
                    >
                      Stop
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showAdd && (
        <QuickAddTransactionModal
          recurring
          onClose={() => setShowAdd(false)}
          onAdded={() => mutateRules()}
        />
      )}
    </div>
  )
}

// ─── BudgetPage ───────────────────────────────────────────────────────────────
type Tab = 'overview' | 'income' | 'transactions' | 'recurring' | 'subscriptions'

export function BudgetPage() {
  const [searchParams] = useSearchParams()
  const startFlagged = searchParams.get('flagged') === '1'
  const [tab, setTab] = useState<Tab>(startFlagged ? 'transactions' : 'overview')
  const [openFlagged, setOpenFlagged] = useState(startFlagged)
  const [categoryFilter, setCategoryFilter] = useState<TransactionFilter | null>(null)
  const [period, setPeriod] = useState(() => {
    const now = new Date()
    return { year: now.getFullYear(), month: now.getMonth() + 1 }
  })
  const { data: categories = [] } = useSWR('/budget/categories', budgetApi.listCategories)

  const tabCls = (t: Tab) =>
    `px-4 py-2 text-sm font-medium rounded-lg transition-colors shrink-0 whitespace-nowrap ${
      tab === t
        ? 'bg-accent-tint-bg text-accent-text'
        : 'text-text-tertiary hover:text-text-primary'
    }`

  const handleSwitchToFlagged = () => {
    setOpenFlagged(true)
    setCategoryFilter(null)
    setTab('transactions')
  }

  const handleOpenCategory = (filter: TransactionFilter) => {
    setOpenFlagged(false)
    setCategoryFilter(filter)
    setTab('transactions')
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex items-center gap-2">
        <h1 className="text-2xl font-bold text-text-primary">Budget</h1>
        <PageGuide id="budget" />
      </div>

      {/* Sub-tab bar */}
      <div className="flex gap-1 border-b border-border-default pb-px overflow-x-auto">
        <button className={tabCls('overview')} onClick={() => setTab('overview')}>
          Overview
        </button>
        <button className={tabCls('income')} onClick={() => setTab('income')}>
          Income
        </button>
        <button className={tabCls('transactions')} onClick={() => { setOpenFlagged(false); setCategoryFilter(null); setTab('transactions') }}>
          Transactions
        </button>
        <button className={tabCls('subscriptions')} onClick={() => setTab('subscriptions')}>
          Subscriptions
        </button>
        <button className={tabCls('recurring')} onClick={() => setTab('recurring')}>
          Recurring
        </button>
      </div>

      <Card>
        {tab === 'overview' && (
          <OverviewTab
            categories={categories}
            onSwitchToTransactions={() => setTab('transactions')}
            onSwitchToFlagged={handleSwitchToFlagged}
            onOpenCategory={handleOpenCategory}
            period={period}
            onPeriodChange={setPeriod}
          />
        )}
        {tab === 'income' && <IncomeTab />}
        {tab === 'transactions' && (
          <TransactionsTab categories={categories} initialFlagged={openFlagged} initialFilter={categoryFilter} />
        )}
        {tab === 'recurring' && <RecurringTab />}
        {tab === 'subscriptions' && <SubscriptionsTab />}
      </Card>
    </div>
  )
}
