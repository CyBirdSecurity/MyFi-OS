import { Bar, BarChart, Cell, ReferenceLine, ResponsiveContainer, Tooltip, XAxis } from 'recharts'
import { Card } from '../ui/Card'
import { useCashFlow } from '../../hooks/useCashFlow'
import { formatCurrency } from '../../utils/currency'
import { saved } from '../../utils/cashFlow'

function Line({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className="flex items-baseline justify-between">
      <span className="text-sm text-text-secondary">{label}</span>
      <span className={strong ? 'text-lg font-bold' : 'text-sm font-semibold text-text-primary'}>{value}</span>
    </div>
  )
}

export function CashFlowCard() {
  const { months, current, currentRate, averageRate, isLoading } = useCashFlow()

  if (isLoading || !current) {
    return (
      <Card title="Cash Flow">
        <div className="text-text-tertiary text-sm">Loading…</div>
      </Card>
    )
  }

  const monthName = new Date(current.year, current.month - 1, 1).toLocaleString('default', { month: 'long' })
  const left = saved(current)
  const chart = months.map((m) => ({ label: m.label, saved: saved(m) }))

  return (
    <Card title="Cash Flow">
      <div className="text-xs text-text-tertiary mb-2">{monthName} so far</div>
      <div className="space-y-1">
        <Line label="Income" value={formatCurrency(current.income)} />
        <Line label="Spending" value={formatCurrency(current.spending)} />
        <div className={left < 0 ? 'text-negative' : 'text-positive'}>
          <Line label="Saved" value={formatCurrency(left, { showSign: true })} strong />
        </div>
      </div>

      <p className="text-xs text-text-tertiary mt-2">
        {currentRate === null
          ? 'No income recorded yet this month.'
          : currentRate >= 0
            ? `${currentRate.toFixed(0)}% of this month's income saved so far.`
            : "Spending is ahead of this month's income so far."}
      </p>

      <div className="h-24 mt-3" aria-label="Amount saved each month">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={chart} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
            <XAxis
              dataKey="label"
              tickLine={false}
              axisLine={false}
              tick={{ fontSize: 11, fill: 'rgb(var(--text-tertiary))' }}
            />
            <ReferenceLine y={0} stroke="rgb(var(--border-default))" />
            <Tooltip
              cursor={false}
              formatter={(v: number) => [formatCurrency(v, { showSign: true }), 'Saved']}
              contentStyle={{
                background: 'rgb(var(--bg-surface))',
                border: '1px solid rgb(var(--border-default))',
                borderRadius: 8,
                fontSize: 12,
              }}
            />
            <Bar dataKey="saved" radius={[3, 3, 0, 0]}>
              {chart.map((d) => (
                <Cell key={d.label} fill={d.saved < 0 ? 'rgb(var(--negative))' : 'rgb(var(--positive))'} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>

      {averageRate !== null && (
        <p className="text-xs text-text-tertiary mt-2">
          Last 3 full months: {averageRate.toFixed(0)}% of income saved on average.
        </p>
      )}
    </Card>
  )
}
