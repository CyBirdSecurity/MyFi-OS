import { useNetWorthHistory } from '../../hooks/useNetWorth'
import { useChartPalette } from '../../hooks/useChartPalette'
import { Card } from '../ui/Card'
import { formatCurrency, formatPercent } from '../../utils/currency'
import { daysAgo, today } from '../../utils/dates'
import type { NetWorthCurrent } from '../../api/client'

/**
 * The dashboard's lead card: net worth, its 30-day change with a sparkline,
 * then assets and liabilities. Balances stay in the primary text color;
 * green and red are kept for the change.
 */
export function NetWorthSummaryCard({ nw, loading }: { nw?: NetWorthCurrent; loading: boolean }) {
  const { data: history } = useNetWorthHistory(daysAgo(30), today(), 'day')

  const points = history && nw ? [...history.map((h) => h.netWorth), nw.totalNetWorth] : []
  const delta = points.length >= 2 ? points[points.length - 1] - points[0] : null
  const deltaPct = delta != null && points[0] !== 0 ? (delta / Math.abs(points[0])) * 100 : 0

  return (
    <Card>
      <div className="flex flex-col sm:flex-row sm:items-end sm:justify-between gap-4">
        <div className="min-w-0">
          <div className="text-sm text-text-tertiary mb-1">Net worth</div>
          <div className="text-3xl lg:text-4xl font-bold text-text-primary truncate">
            {loading ? '—' : formatCurrency(nw?.totalNetWorth ?? 0)}
          </div>
          {delta != null && (
            <div className="mt-1.5 text-sm">
              <span className={`font-medium ${delta >= 0 ? 'text-positive' : 'text-negative'}`}>
                {formatCurrency(delta, { showSign: true })} ({formatPercent(deltaPct)})
              </span>
              <span className="text-text-tertiary"> past 30 days</span>
            </div>
          )}
        </div>
        {points.length >= 2 && <Sparkline values={points} className="w-full h-12 sm:w-56 sm:h-14" />}
      </div>
      <div className="grid grid-cols-2 gap-4 mt-5 pt-4 border-t border-border-subtle">
        <div className="min-w-0">
          <div className="text-xs text-text-tertiary mb-0.5">Assets</div>
          <div className="text-lg font-semibold text-text-primary truncate">
            {loading ? '—' : formatCurrency(nw?.totalAssets ?? 0)}
          </div>
        </div>
        <div className="min-w-0">
          <div className="text-xs text-text-tertiary mb-0.5">Liabilities</div>
          <div className="text-lg font-semibold text-text-primary truncate">
            {loading ? '—' : formatCurrency(Math.abs(nw?.totalLiabilities ?? 0))}
          </div>
        </div>
      </div>
    </Card>
  )
}

function Sparkline({ values, className }: { values: number[]; className?: string }) {
  const palette = useChartPalette()
  const min = Math.min(...values)
  const span = Math.max(...values) - min || 1
  // viewBox units; the SVG stretches to the element's size.
  const path = values
    .map((v, i) => `${i === 0 ? 'M' : 'L'}${((i / (values.length - 1)) * 100).toFixed(2)},${(38 - ((v - min) / span) * 36).toFixed(2)}`)
    .join(' ')
  return (
    <svg viewBox="0 0 100 40" preserveAspectRatio="none" className={className} aria-hidden="true">
      <path d={path} fill="none" stroke={palette.netWorthLine} strokeWidth={2} vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
