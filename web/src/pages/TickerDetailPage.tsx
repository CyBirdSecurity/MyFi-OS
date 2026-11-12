import { useParams, Link } from 'react-router-dom'
import useSWR from 'swr'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { tickerApi } from '../api/client'
import type { TickerDetail, TickerHistory, PricePoint } from '../api/client'
import { Card } from '../components/ui/Card'
import { formatCurrency } from '../utils/currency'
import { formatDate } from '../utils/dates'
import { ACCOUNT_TYPE_LABELS } from '../api/client'
import { useChartPalette } from '../hooks/useChartPalette'
import { holdingLabel, isPlaceholderTicker } from '../utils/holdingLabel'

function PriceTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-surface border border-border-strong rounded-lg px-4 py-3 shadow-float text-sm">
      <div className="text-text-secondary mb-1">{formatDate(label)}</div>
      <div className="text-text-primary font-semibold">{formatCurrency(payload[0].value)}</div>
    </div>
  )
}

function PriceChart({ points }: { points: PricePoint[] }) {
  const palette = useChartPalette()
  if (!points.length) return null

  const minVal = Math.min(...points.map((p) => p.close))
  const maxVal = Math.max(...points.map((p) => p.close))
  const pad = (maxVal - minVal) * 0.1 || maxVal * 0.05

  return (
    <ResponsiveContainer width="100%" height={240}>
      <AreaChart data={points} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
        <XAxis
          dataKey="date"
          tickFormatter={(v) => formatDate(v, 'MMM yyyy')}
          tick={{ fill: palette.tick, fontSize: 11 }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          tickFormatter={(v) => formatCurrency(v, { compact: true })}
          tick={{ fill: palette.tick, fontSize: 11 }}
          axisLine={false}
          tickLine={false}
          domain={[minVal - pad, maxVal + pad]}
          width={70}
        />
        <Tooltip content={<PriceTooltip />} />
        <Area
          type="monotone"
          dataKey="close"
          stroke={palette.netWorthLine}
          strokeWidth={2}
          fill={palette.netWorthLine}
          fillOpacity={0.08}
          dot={false}
          activeDot={{ r: 4, fill: palette.netWorthLine }}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}

export function TickerDetailPage() {
  const { ticker = '' } = useParams<{ ticker: string }>()
  const upperTicker = ticker.toUpperCase()

  const { data: detail, isLoading: detailLoading } = useSWR<TickerDetail>(
    `/holdings/ticker/${upperTicker}`,
    () => tickerApi.detail(upperTicker),
  )

  const { data: history, isLoading: historyLoading, error: historyError } = useSWR<TickerHistory>(
    `/holdings/ticker/${upperTicker}/history`,
    () => tickerApi.history(upperTicker),
  )

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex items-center gap-3">
        <Link
          to="/holdings"
          className="text-text-tertiary hover:text-text-primary transition-colors text-sm"
        >
          ← Holdings
        </Link>
      </div>

      {detailLoading ? (
        <div className="text-text-tertiary text-sm">Loading...</div>
      ) : !detail ? (
        <div className="text-text-tertiary text-sm">Ticker not found in your holdings.</div>
      ) : (
        <>
          <div>
            <h1 className="text-2xl font-bold text-text-primary">{holdingLabel(detail.ticker, detail.name)}</h1>
            {detail.name && (
              <p className="text-text-tertiary text-sm mt-1">
                {isPlaceholderTicker(detail.ticker) ? 'No public ticker' : detail.name}
              </p>
            )}
          </div>

          {/* Summary cards */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Card>
              <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">Total Shares</div>
              <div className="text-2xl font-bold text-text-primary">
                {detail.totalShares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
              </div>
            </Card>
            <Card>
              <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">Current Price</div>
              <div className="text-2xl font-bold text-text-primary">
                {detail.pricePerShare != null ? formatCurrency(detail.pricePerShare) : '—'}
              </div>
            </Card>
            <Card>
              <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">Total Value</div>
              <div className="text-2xl font-bold text-positive">
                {formatCurrency(detail.totalValue)}
              </div>
            </Card>
          </div>

          {/* Per-account breakdown */}
          <Card title="Positions by Account">
            <div className="overflow-x-auto">
            <table className="w-full text-sm min-w-[420px]">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-3 font-medium">Account</th>
                  <th className="pb-3 font-medium">Type</th>
                  <th className="pb-3 font-medium text-right">Shares</th>
                  <th className="pb-3 font-medium text-right">Price</th>
                  <th className="pb-3 font-medium text-right">Value</th>
                </tr>
              </thead>
              <tbody>
                {detail.positions.map((pos) => (
                  <tr key={pos.accountId} className="border-b border-border-default/50 hover:bg-subtle/20">
                    <td className="py-3 text-text-primary font-medium">{pos.accountName}</td>
                    <td className="py-3 text-text-tertiary text-xs">
                      {ACCOUNT_TYPE_LABELS[pos.accountType] ?? pos.accountType}
                    </td>
                    <td className="py-3 text-text-secondary text-right">
                      {pos.shares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                    </td>
                    <td className="py-3 text-text-primary text-right">
                      {pos.pricePerShare ? formatCurrency(pos.pricePerShare) : '—'}
                    </td>
                    <td className="py-3 text-text-primary font-medium text-right">
                      {formatCurrency(pos.totalValue)}
                    </td>
                  </tr>
                ))}
                {/* Totals row */}
                <tr className="border-t border-border-strong">
                  <td className="py-3 text-text-secondary font-medium" colSpan={2}>Total</td>
                  <td className="py-3 text-text-primary font-medium text-right">
                    {detail.totalShares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                  </td>
                  <td />
                  <td className="py-3 text-text-primary font-bold text-right">
                    {formatCurrency(detail.totalValue)}
                  </td>
                </tr>
              </tbody>
            </table>
            </div>
          </Card>

          {/* Historical price chart */}
          <Card title="Historical Price">
            {historyLoading ? (
              <div className="text-text-tertiary text-sm">Loading price history...</div>
            ) : historyError ? (
              <div className="flex items-center justify-center h-32 text-text-tertiary text-sm">
                Could not load price history.
              </div>
            ) : history?.available === false ? (
              <div className="flex items-center justify-center h-32 text-text-tertiary text-sm">
                Historical price data is unavailable.
              </div>
            ) : !history?.points.length ? (
              <div className="flex items-center justify-center h-32 text-text-tertiary text-sm">
                No price history available for {upperTicker}.
              </div>
            ) : (
              <PriceChart points={history.points} />
            )}
          </Card>
        </>
      )}
    </div>
  )
}
