import type { Holding } from '../../api/client'
import { Card } from '../ui/Card'
import { InfoTip } from '../ui/InfoTip'
import { formatCurrency } from '../../utils/currency'
import { useChartPalette } from '../../hooks/useChartPalette'
import { fundTypeBreakdown, sizeBreakdown, type Breakdown } from '../../utils/fundProfile'

function BreakdownBar<K extends string>({ title, rows, colors }: { title: string; rows: Breakdown<K>[]; colors: readonly string[] }) {
  return (
    <div>
      <h3 className="text-sm font-medium text-text-primary mb-2">{title}</h3>
      <div className="flex h-2.5 rounded-sm overflow-hidden gap-px bg-muted" aria-hidden="true">
        {rows.map((r, i) => (
          <div key={r.key} style={{ width: `${r.pct}%`, background: colors[i % colors.length] }} />
        ))}
      </div>
      <div className="mt-3 space-y-1.5">
        {rows.map((r, i) => (
          <div key={r.key} className="flex items-center justify-between gap-3 text-sm">
            <div className="flex items-center gap-2 min-w-0">
              <span className="w-2.5 h-2.5 rounded-sm shrink-0" style={{ background: colors[i % colors.length] }} />
              <span className="text-text-primary truncate">{r.label}</span>
            </div>
            <span className="text-text-secondary shrink-0">
              {formatCurrency(r.value)}
              <span className="inline-block w-12 text-right text-text-tertiary">{r.pct.toFixed(0)}%</span>
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * What the portfolio is held in (stocks, ETFs, mutual funds) and, for stock
 * holdings, an estimate of company size taken from fund names.
 */
export function FundTypeSizeCard({ holdings }: { holdings: Holding[] }) {
  const palette = useChartPalette()
  const types = fundTypeBreakdown(holdings)
  const sizes = sizeBreakdown(holdings)
  if (!types.length) return null

  return (
    <Card title={<>Fund types & company size <InfoTip term="fundSize" /></>}>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
        <BreakdownBar title="By fund type" rows={types} colors={palette.pieType} />
        {sizes.length > 0 && <BreakdownBar title="Stocks by company size" rows={sizes} colors={palette.pieType} />}
      </div>
      <p className="mt-4 text-xs text-text-tertiary">
        Size is estimated from fund names (for example "Small Cap" or "500 Index"). Individual stocks, and funds whose
        name doesn't say, aren't sized.
      </p>
    </Card>
  )
}
