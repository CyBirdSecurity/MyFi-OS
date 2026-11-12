import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import type { HistoryPoint } from '../../api/client'
import { formatCurrency } from '../../utils/currency'
import { formatAxis, niceTicks } from '../../utils/chartAxis'
import { formatMonthYear, formatDate } from '../../utils/dates'
import { useChartPalette } from '../../hooks/useChartPalette'
import { EmptyState } from '../ui/EmptyState'

export interface ChartRange {
  label: string
  days?: number
  months: number
  years: number
  interval: string
  allTime?: boolean
}

export const CHART_RANGES: ChartRange[] = [
  { label: '3M', months: 3, years: 0, interval: 'week' },
  { label: '6M', months: 6, years: 0, interval: 'week' },
  { label: '1Y', months: 12, years: 0, interval: 'month' },
  { label: '3Y', months: 0, years: 3, interval: 'month' },
  { label: '5Y', months: 0, years: 5, interval: 'month' },
  { label: 'All Time', months: 0, years: 0, interval: 'month', allTime: true },
]

/** The 3M / 6M / 1Y ... range switch, shared by the simple and detailed net worth charts. */
export function RangeButtons({
  selectedRange,
  onRangeChange,
}: {
  selectedRange: string
  onRangeChange: (range: string) => void
}) {
  return (
    <div className="flex gap-1 mb-4">
      {CHART_RANGES.map((r) => (
        <button
          key={r.label}
          onClick={() => onRangeChange(r.label)}
          className={`px-3 py-1 rounded text-xs font-medium transition-colors ${
            selectedRange === r.label
              ? 'bg-accent text-white'
              : 'text-text-secondary hover:text-text-primary hover:bg-subtle'
          }`}
        >
          {r.label}
        </button>
      ))}
    </div>
  )
}

interface NetWorthChartProps {
  data: HistoryPoint[]
  selectedRange?: string
  onRangeChange?: (range: string) => void
  interval?: string
}

function CustomTooltip({ active, payload, label, palette, interval }: any) {
  if (!active || !payload?.length) return null
  const d = payload[0]?.payload as HistoryPoint
  // Day/week buckets are specific dates, not whole months — show the exact
  // date so two different weekly points don't both read "May 2026" and look
  // like the same value when they're actually different underlying dates.
  const dateLabel = interval === 'day' || interval === 'week'
    ? formatDate(label, 'MMM d, yyyy')
    : formatMonthYear(label)
  return (
    <div className="rounded-lg px-4 py-3 shadow-float text-sm" style={{ background: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}` }}>
      <div className="mb-2" style={{ color: palette.tooltipMuted }}>{dateLabel}</div>
      <div className="space-y-1">
        <div className="flex justify-between gap-6">
          <span style={{ color: palette.tooltipMuted }}>Net Worth</span>
          <span className="font-semibold" style={{ color: palette.tooltipText }}>{formatCurrency(d.netWorth)}</span>
        </div>
        <div className="flex justify-between gap-6">
          <span style={{ color: palette.tooltipMuted }}>Assets</span>
          <span style={{ color: palette.goalHigh }}>{formatCurrency(d.assets)}</span>
        </div>
        {d.liabilities < 0 && (
          <div className="flex justify-between gap-6">
            <span style={{ color: palette.tooltipMuted }}>Liabilities</span>
            <span style={{ color: palette.goalLow }}>{formatCurrency(d.liabilities)}</span>
          </div>
        )}
      </div>
    </div>
  )
}

export function NetWorthChart({
  data,
  selectedRange = '1Y',
  onRangeChange,
  interval = 'month',
}: NetWorthChartProps) {
  const palette = useChartPalette()

  const tickFormatter = interval === 'day' || interval === 'week'
    ? (v: string) => formatDate(v, 'MMM d')
    : formatMonthYear

  // Evenly spaced, round-numbered labels that cover the data.
  const values = data.map((d) => d.netWorth)
  const ticks = values.length ? niceTicks(Math.min(...values), Math.max(...values)) : undefined

  return (
    <div>
      {onRangeChange && <RangeButtons selectedRange={selectedRange} onRangeChange={onRangeChange} />}

      {!data.length ? (
        <div className="flex items-center justify-center h-48 text-center">
          <EmptyState action={{ label: 'Add an account', to: '/accounts' }}>
            Your net worth is tracked from the day you add an account with its balance.
          </EmptyState>
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={280}>
          <AreaChart data={data} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
            <XAxis
              dataKey="date"
              tickFormatter={tickFormatter}
              tick={{ fill: palette.tick, fontSize: 11 }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              tickFormatter={formatAxis}
              tick={{ fill: palette.tick, fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              ticks={ticks}
              domain={ticks ? [ticks[0], ticks[ticks.length - 1]] : ['auto', 'auto']}
              width={70}
            />
            <Tooltip content={(props) => <CustomTooltip {...props} palette={palette} interval={interval} />} />
            <Area
              type="monotone"
              dataKey="netWorth"
              stroke={palette.netWorthLine}
              strokeWidth={2}
              fill={palette.netWorthLine}
              fillOpacity={0.08}
              dot={false}
              activeDot={{ r: 4, fill: palette.netWorthLine }}
            />
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}
