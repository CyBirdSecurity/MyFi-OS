import { Bar, CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { AccountHistory } from '../../api/client'
import { formatCurrency } from '../../utils/currency'
import { formatDate, formatMonthYear } from '../../utils/dates'
import { formatAxis, niceTicks } from '../../utils/chartAxis'
import { orderSeries, seriesColors, stackBounds, toRows } from '../../utils/netWorthStack'
import { useChartPalette } from '../../hooks/useChartPalette'
import { RangeButtons } from './NetWorthChart'
import { EmptyState } from '../ui/EmptyState'

/** How many accounts the hover tooltip lists before folding the rest into "N more". */
const TOOLTIP_ROWS = 8

interface Props {
  history?: AccountHistory
  isLoading?: boolean
  selectedRange?: string
  onRangeChange?: (range: string) => void
  interval?: string
}

function AccountsTooltip({ active, payload, label, palette, interval, history, colors }: any) {
  if (!active || !payload?.length) return null
  const balances: Record<string, number> = payload[0].payload
  const dateLabel = interval === 'day' || interval === 'week' ? formatDate(label, 'MMM d, yyyy') : formatMonthYear(label)

  const rows = (history.series as AccountHistory['series'])
    .map((s) => ({ s, value: Number(balances[s.id] ?? 0) }))
    .filter((r) => r.value !== 0)
    .sort((a, b) => Math.abs(b.value) - Math.abs(a.value))
  const netWorth = rows.reduce((sum, r) => sum + r.value, 0)
  const hidden = rows.slice(TOOLTIP_ROWS)
  const hiddenTotal = hidden.reduce((sum, r) => sum + r.value, 0)

  return (
    <div
      className="rounded-lg px-4 py-3 shadow-float text-sm max-w-xs"
      style={{ background: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}` }}
    >
      <div className="mb-2" style={{ color: palette.tooltipMuted }}>{dateLabel}</div>
      <div className="space-y-1">
        {rows.slice(0, TOOLTIP_ROWS).map(({ s, value }) => (
          <div key={s.id} className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-2 min-w-0" style={{ color: palette.tooltipMuted }}>
              <span className="inline-block h-2.5 w-2.5 rounded-sm shrink-0" style={{ background: colors[s.id] }} />
              <span className="truncate">{s.name}</span>
            </span>
            <span style={{ color: value < 0 ? palette.goalLow : palette.tooltipText }}>{formatCurrency(value)}</span>
          </div>
        ))}
        {hidden.length > 0 && (
          <div className="flex justify-between gap-4" style={{ color: palette.tooltipMuted }}>
            <span>{hidden.length} more</span>
            <span>{formatCurrency(hiddenTotal)}</span>
          </div>
        )}
        <div
          className="flex justify-between gap-4 pt-1.5 mt-1.5 font-semibold"
          style={{ borderTop: `1px solid ${palette.tooltipBorder}`, color: palette.tooltipText }}
        >
          <span>Net worth</span>
          <span>{formatCurrency(netWorth)}</span>
        </div>
      </div>
    </div>
  )
}

/**
 * Net worth over time as stacked bars, one segment per account. Assets stack up
 * from zero and liabilities (cards, loans, mortgages) stack down from it, so each
 * bar's segments add up to that date's net worth.
 */
export function NetWorthAccountsChart({ history, isLoading, selectedRange = '1Y', onRangeChange, interval = 'month' }: Props) {
  const palette = useChartPalette()

  const tickFormatter = interval === 'day' || interval === 'week' ? (v: string) => formatDate(v, 'MMM d') : formatMonthYear

  const ordered = history ? orderSeries(history) : []
  const colors = history ? seriesColors(history, ordered, palette.forecast, palette.liability) : {}
  // Each row also carries the date's net worth (the sum of every series), drawn as a line over the bars.
  const rows = (history ? toRows(history) : []).map((row) => ({
    ...row,
    netWorth: (history?.series ?? []).reduce((sum, s) => sum + Number(row[s.id] ?? 0), 0),
  }))
  const { min, max } = history ? stackBounds(history.points) : { min: 0, max: 0 }
  const ticks = niceTicks(min, max)

  return (
    <div>
      {onRangeChange && <RangeButtons selectedRange={selectedRange} onRangeChange={onRangeChange} />}

      {isLoading ? (
        <div className="flex items-center justify-center h-48 text-text-tertiary text-sm">Loading…</div>
      ) : !history || history.points.length === 0 ? (
        <div className="flex items-center justify-center h-48 text-center">
          <EmptyState action={{ label: 'Add an account', to: '/accounts' }}>
            Your net worth is tracked from the day you add an account with its balance.
          </EmptyState>
        </div>
      ) : (
        <>
          <ResponsiveContainer width="100%" height={280}>
            {/* "sign" stacks assets up from zero and liabilities down from it; the default would add the two together. */}
            <ComposedChart data={rows} stackOffset="sign" margin={{ top: 10, right: 10, left: 0, bottom: 0 }} barCategoryGap="12%">
              <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
              <XAxis
                dataKey="date"
                tickFormatter={tickFormatter}
                tick={{ fill: palette.tick, fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                minTickGap={24}
              />
              <YAxis
                tickFormatter={formatAxis}
                tick={{ fill: palette.tick, fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                ticks={ticks}
                domain={[ticks[0], ticks[ticks.length - 1]]}
                width={70}
              />
              <ReferenceLine y={0} stroke={palette.axis} />
              <Tooltip
                cursor={{ fill: palette.grid, opacity: 0.4 }}
                content={(props) => (
                  <AccountsTooltip {...props} palette={palette} interval={interval} history={history} colors={colors} />
                )}
              />
              {ordered.map((s) => (
                <Bar key={s.id} dataKey={s.id} stackId="accounts" fill={colors[s.id]} isAnimationActive={false} />
              ))}
              <Line
                type="monotone"
                dataKey="netWorth"
                stroke={palette.tooltipText}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 3 }}
                isAnimationActive={false}
              />
            </ComposedChart>
          </ResponsiveContainer>

          <ul className="flex flex-wrap gap-x-4 gap-y-1 mt-3 text-xs text-text-secondary">
            <li className="flex items-center gap-1.5 font-medium text-text-primary">
              <span className="inline-block h-0.5 w-3" style={{ background: palette.tooltipText }} />
              Net worth
            </li>
            {ordered.map((s) => (
              <li key={s.id} className="flex items-center gap-1.5">
                <span className="inline-block h-2.5 w-2.5 rounded-sm" style={{ background: colors[s.id] }} />
                {s.name}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}
