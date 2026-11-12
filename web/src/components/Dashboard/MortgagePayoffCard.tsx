import { useMortgages } from '../../hooks/useMortgages'
import { useAmortization } from '../../hooks/useAmortization'
import { Card } from '../ui/Card'
import { formatCurrency } from '../../utils/currency'
import { formatMonthYear } from '../../utils/dates'
import { payoffPercent, type Mortgage } from '../../utils/debt'

/** Whole months from this month until the YYYY-MM-DD date's month. */
function monthsUntil(dateStr: string): number {
  const [year, month] = dateStr.split('-').map(Number)
  const now = new Date()
  return (year - now.getFullYear()) * 12 + (month - 1 - now.getMonth())
}

function countdown(months: number): string {
  if (months <= 0) return 'Paid off'
  const years = Math.floor(months / 12)
  const rest = months % 12
  return [years && `${years} yr`, rest && `${rest} mo`].filter(Boolean).join(' ') + ' to go'
}

function MortgageRow({ mortgage }: { mortgage: Mortgage }) {
  const { data } = useAmortization(mortgage.accountId)
  if (!data) return null

  const { summary, withExtra } = data
  // The schedule with the saved extra payment is the one actually being paid.
  const payoff = withExtra?.payoffDate ?? summary.payoffDate
  const pctPaid = payoffPercent(mortgage.originalLoan, mortgage.currentBalance)

  return (
    <div>
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-sm text-text-primary truncate">{mortgage.name}</span>
        <span className="text-xs text-text-tertiary shrink-0">Paid off {formatMonthYear(payoff)}</span>
      </div>
      <div className="text-2xl font-semibold text-text-primary mt-1">{countdown(monthsUntil(payoff))}</div>
      <div className="h-2 bg-muted rounded-full overflow-hidden mt-2">
        <div className="h-full rounded-full bg-positive transition-all duration-500" style={{ width: `${pctPaid}%` }} />
      </div>
      <div className="flex justify-between text-xs text-text-tertiary mt-1">
        <span>{pctPaid.toFixed(0)}% paid · {formatCurrency(mortgage.currentBalance)} left</span>
        <span>{formatCurrency(summary.totalMonthlyCost)}/mo all-in</span>
      </div>
      {summary.pmiEndDate && (
        <div className="text-xs text-positive mt-2">
          PMI drops off {formatMonthYear(summary.pmiEndDate)}, saving {formatCurrency(summary.pmiMonthly)}/mo
        </div>
      )}
    </div>
  )
}

/** Countdown to each mortgage's payoff, with what it costs each month and when PMI ends. */
export function MortgagePayoffCard() {
  const { data: mortgages } = useMortgages()
  if (!mortgages?.length) return null

  return (
    <Card title="Mortgage Payoff">
      <div className="space-y-5">
        {mortgages.map((m) => (
          <MortgageRow key={m.accountId} mortgage={m} />
        ))}
      </div>
    </Card>
  )
}
