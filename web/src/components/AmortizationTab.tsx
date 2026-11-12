import { useState, useMemo } from 'react'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import { useAmortization } from '../hooks/useAmortization'
import { Card } from './ui/Card'
import { formatCurrency } from '../utils/currency'
import { useChartPalette } from '../hooks/useChartPalette'
import type { HomeDetails, AmortizationMonth } from '../api/client'

interface Props {
  accountId: string
  homeDetails: HomeDetails | null
}

const PAGE_SIZE = 50

export function AmortizationTab({ accountId, homeDetails }: Props) {
  const palette = useChartPalette()
  const [whatIfExtra, setWhatIfExtra] = useState<number | ''>('')
  const [viewMode, setViewMode] = useState<'monthly' | 'yearly'>('yearly')
  const [page, setPage] = useState(0)

  const extra = whatIfExtra !== '' && whatIfExtra > 0 ? whatIfExtra : undefined
  const { data, isLoading } = useAmortization(accountId, extra)

  const homeValue = homeDetails?.lastZestimate ?? 0

  const chartData = useMemo(() => {
    if (!data?.schedule.length) return []
    const points: { label: string; balance: number; equity: number; homeValue: number }[] = []
    let prevYear = ''
    for (const row of data.schedule) {
      const year = row.date.slice(0, 4)
      if (year !== prevYear) {
        const equity = Math.max(0, homeValue - row.balance)
        points.push({ label: year, balance: row.balance, equity, homeValue })
        prevYear = year
      }
    }
    return points
  }, [data?.schedule, homeValue])

  const tableRows = useMemo(() => {
    if (!data?.schedule.length) return []
    if (viewMode === 'monthly') return data.schedule

    const years: AmortizationMonth[] = []
    let chunk: AmortizationMonth[] = []
    let prevYear = ''
    for (const row of data.schedule) {
      const year = row.date.slice(0, 4)
      if (year !== prevYear && chunk.length > 0) {
        years.push(rollupYear(chunk))
        chunk = []
      }
      chunk.push(row)
      prevYear = year
    }
    if (chunk.length > 0) years.push(rollupYear(chunk))
    return years
  }, [data?.schedule, viewMode])

  const totalPages = Math.ceil(tableRows.length / PAGE_SIZE)
  const visibleRows = tableRows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16 text-text-tertiary text-sm">
        Loading amortization schedule…
      </div>
    )
  }

  if (!data) {
    return (
      <div className="flex items-center justify-center py-16 text-text-tertiary text-sm">
        No mortgage data available. Set loan details to see the amortization schedule.
      </div>
    )
  }

  const { summary, withExtra } = data

  return (
    <div className="space-y-6">
      {/* What-if extra payment input */}
      <Card title="Extra Principal Payment (What-If)">
        <div className="flex flex-col sm:flex-row sm:items-end gap-3">
          <div className="flex flex-col gap-1">
            <label className="text-xs text-text-secondary">
              Additional monthly principal ($)
            </label>
            <input
              type="number"
              min={0}
              step={50}
              value={whatIfExtra}
              onChange={e => {
                setPage(0)
                setWhatIfExtra(e.target.value === '' ? '' : Number(e.target.value))
              }}
              placeholder="e.g. 200"
              className="w-40 bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent"
            />
          </div>
          {homeDetails?.extraMonthlyPayment != null && homeDetails.extraMonthlyPayment > 0 && (
            <p className="text-xs text-text-tertiary pb-2">
              Saved default: {formatCurrency(homeDetails.extraMonthlyPayment)}/mo
            </p>
          )}
        </div>
      </Card>

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
        <SummaryCard label="Monthly Payment" value={formatCurrency(summary.monthlyPayment)} />
        <SummaryCard label="Total Interest" value={formatCurrency(summary.totalInterest)} />
        <SummaryCard label="Payoff Date" value={formatDate(summary.payoffDate)} />
        {withExtra && (
          <>
            <SummaryCard
              label="Interest w/ Extra"
              value={formatCurrency(withExtra.totalInterest)}
              delta={`-${formatCurrency(withExtra.interestSaved)} saved`}
              deltaPositive
            />
            <SummaryCard
              label="New Payoff Date"
              value={formatDate(withExtra.payoffDate)}
              delta={`${withExtra.monthsSaved} months sooner`}
              deltaPositive
            />
            <SummaryCard
              label="Total Months Saved"
              value={`${withExtra.monthsSaved}`}
            />
          </>
        )}
      </div>

      {/* Full monthly cost, when there's anything on top of principal and interest */}
      {summary.totalMonthlyCost > summary.monthlyPayment && (
        <Card title="Monthly Cost">
          <div className="space-y-2 text-sm">
            {([
              ['Principal & interest', summary.monthlyPayment],
              ['Property tax', summary.propertyTaxMonthly],
              ['Home insurance', summary.insuranceMonthly],
              ['HOA dues', summary.hoaMonthly],
              [summary.pmiEndDate ? `PMI (until ${formatDate(summary.pmiEndDate)})` : 'PMI', summary.pmiMonthly],
            ] as const).filter(([, v]) => v > 0).map(([label, v]) => (
              <div key={label} className="flex justify-between">
                <span className="text-text-secondary">{label}</span>
                <span className="text-text-primary">{formatCurrency(v)}</span>
              </div>
            ))}
            <div className="flex justify-between pt-2 border-t border-border-default font-semibold">
              <span className="text-text-primary">Total</span>
              <span className="text-text-primary">{formatCurrency(summary.totalMonthlyCost)}</span>
            </div>
          </div>
        </Card>
      )}

      {/* Equity growth chart */}
      {chartData.length > 0 && (
        <Card title="Equity Over Time">
          <ResponsiveContainer width="100%" height={260}>
            <AreaChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} />
              <XAxis
                dataKey="label"
                tick={{ fill: palette.tick, fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                interval="preserveStartEnd"
              />
              <YAxis
                tick={{ fill: palette.tick, fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={v => formatCurrency(v, { compact: true })}
                width={60}
              />
              <Tooltip
                contentStyle={{ backgroundColor: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}`, borderRadius: 8 }}
                labelStyle={{ color: palette.tooltipMuted, fontSize: 12 }}
                formatter={(v: number, name: string) => [formatCurrency(v), name]}
              />
              <Legend wrapperStyle={{ fontSize: 12, color: palette.tooltipMuted }} />
              <Area type="monotone" dataKey="homeValue" name="Home Value"  stroke={palette.forecast[0]} strokeWidth={1.5} fill="none" dot={false} />
              <Area type="monotone" dataKey="balance"   name="Loan Balance" stroke={palette.goalLow}    strokeWidth={1.5} fill={palette.goalLow} fillOpacity={0.06} dot={false} />
              <Area type="monotone" dataKey="equity"    name="Equity"       stroke={palette.goalHigh}   strokeWidth={2}   fill={palette.goalHigh} fillOpacity={0.1} dot={false} />
            </AreaChart>
          </ResponsiveContainer>
        </Card>
      )}

      {/* Amortization table */}
      <Card
        title="Payment Schedule"
        action={
          <div className="flex gap-1">
            {(['yearly', 'monthly'] as const).map(m => (
              <button
                key={m}
                onClick={() => { setViewMode(m); setPage(0) }}
                className={`px-3 py-1 rounded-lg text-xs font-medium transition-colors ${
                  viewMode === m
                    ? 'bg-accent text-white'
                    : 'text-text-secondary hover:text-text-primary hover:bg-subtle'
                }`}
              >
                {m.charAt(0).toUpperCase() + m.slice(1)}
              </button>
            ))}
          </div>
        }
      >
        <div className="overflow-x-auto">
          <table className="w-full min-w-[700px] text-sm">
            <thead>
              <tr className="text-xs text-text-tertiary border-b border-border-default">
                <th className="text-left pb-2 pr-4">{viewMode === 'monthly' ? '#' : 'Year'}</th>
                <th className="text-left pb-2 pr-4">Date</th>
                <th className="text-right pb-2 pr-4">Payment</th>
                <th className="text-right pb-2 pr-4">Principal</th>
                <th className="text-right pb-2 pr-4">Interest</th>
                <th className="text-right pb-2 pr-4">Extra</th>
                <th className="text-right pb-2 pr-4">Balance</th>
                <th className="text-right pb-2">Cum. Interest</th>
              </tr>
            </thead>
            <tbody>
              {visibleRows.map((row, i) => (
                <tr key={i} className="border-b border-border-subtle hover:bg-subtle/50">
                  <td className="py-2 pr-4 text-text-secondary">{row.month}</td>
                  <td className="py-2 pr-4 text-text-primary">{formatDate(row.date)}</td>
                  <td className="py-2 pr-4 text-right text-text-primary">{formatCurrency(row.payment)}</td>
                  <td className="py-2 pr-4 text-right text-positive">{formatCurrency(row.principal)}</td>
                  <td className="py-2 pr-4 text-right text-negative">{formatCurrency(row.interest)}</td>
                  <td className="py-2 pr-4 text-right text-accent-text">
                    {row.extraPrincipal > 0 ? formatCurrency(row.extraPrincipal) : '—'}
                  </td>
                  <td className="py-2 pr-4 text-right text-text-primary">{formatCurrency(row.balance)}</td>
                  <td className="py-2 text-right text-text-tertiary">{formatCurrency(row.cumulativeInterest)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between mt-4 pt-4 border-t border-border-default">
            <span className="text-xs text-text-tertiary">
              Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, tableRows.length)} of {tableRows.length} {viewMode === 'monthly' ? 'months' : 'years'}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(0, p - 1))}
                disabled={page === 0}
                className="px-3 py-1 text-xs rounded-lg bg-subtle text-text-primary disabled:opacity-40 hover:bg-muted transition-colors"
              >
                Prev
              </button>
              <button
                onClick={() => setPage(p => Math.min(totalPages - 1, p + 1))}
                disabled={page >= totalPages - 1}
                className="px-3 py-1 text-xs rounded-lg bg-subtle text-text-primary disabled:opacity-40 hover:bg-muted transition-colors"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

// ─── helpers ──────────────────────────────────────────────────────────────────

interface SummaryCardProps {
  label: string
  value: string
  delta?: string
  deltaPositive?: boolean
}

function SummaryCard({ label, value, delta, deltaPositive }: SummaryCardProps) {
  return (
    <div className="bg-surface rounded-lg border border-border-default p-4">
      <p className="text-xs text-text-tertiary uppercase tracking-wider mb-1">{label}</p>
      <p className="text-xl font-semibold text-text-primary">{value}</p>
      {delta && (
        <p className={`text-xs mt-1 ${deltaPositive ? 'text-positive' : 'text-negative'}`}>
          {delta}
        </p>
      )}
    </div>
  )
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '—'
  const [year, month] = dateStr.split('-')
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  return `${months[parseInt(month, 10) - 1]} ${year}`
}

function rollupYear(rows: AmortizationMonth[]): AmortizationMonth {
  const last = rows[rows.length - 1]
  const totalPayment   = rows.reduce((s, r) => s + r.payment, 0)
  const totalPrincipal = rows.reduce((s, r) => s + r.principal, 0)
  const totalInterest  = rows.reduce((s, r) => s + r.interest, 0)
  const totalExtra     = rows.reduce((s, r) => s + r.extraPrincipal, 0)
  return {
    month: parseInt(last.date.slice(0, 4), 10),
    date: last.date.slice(0, 7) + '-01',
    payment: totalPayment,
    principal: totalPrincipal,
    interest: totalInterest,
    extraPrincipal: totalExtra,
    balance: last.balance,
    cumulativeInterest: last.cumulativeInterest,
  }
}
