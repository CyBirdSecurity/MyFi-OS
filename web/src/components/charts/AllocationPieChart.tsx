import { useState } from 'react'
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import type { Holding, NetWorthCurrent } from '../../api/client'
import { formatCurrency } from '../../utils/currency'
import { ACCOUNT_TYPE_LABELS, TAX_TREATMENT_LABELS } from '../../api/client'
import { useChartPalette } from '../../hooks/useChartPalette'
import { allocationByAssetClass } from '../../utils/assetAllocation'
import { EmptyState } from '../ui/EmptyState'

type Mode = 'accountType' | 'assetClass' | 'taxTreatment'

const MODE_LABELS: Record<Mode, string> = {
  accountType: 'By account type',
  assetClass: 'By asset type',
  taxTreatment: 'By tax treatment',
}

interface Props {
  data: NetWorthCurrent
  /** Needed for the asset-type view; until it loads, that view uses account balances alone. */
  holdings?: Holding[]
}

function SliceTooltip({ active, payload, palette }: any) {
  if (!active || !payload?.length) return null
  const { name, value } = payload[0]
  return (
    <div style={{
      background: palette.tooltipBg,
      border: `1px solid ${palette.tooltipBorder}`,
      borderRadius: '8px',
      padding: '12px 16px',
      fontSize: '14px',
      boxShadow: '0 4px 16px rgb(20 16 10 / 0.08)',
    }}>
      <div style={{ color: palette.tooltipMuted, marginBottom: '4px' }}>{name}</div>
      <div style={{ color: palette.tooltipText, fontWeight: 600 }}>{formatCurrency(value)}</div>
    </div>
  )
}

export function AllocationPieChart({ data, holdings }: Props) {
  const palette = useChartPalette()
  const [mode, setMode] = useState<Mode>('accountType')

  const taxData = (['taxable', 'tax_deferred', 'tax_free'] as const)
    .map((key) => ({
      name: TAX_TREATMENT_LABELS[key],
      value: (data.byTaxTreatment ?? []).filter((d) => d.taxTreatment === key).reduce((s, d) => s + d.total, 0),
    }))
    .filter((d) => d.value > 0)

  const typeData = (data.byAccountType ?? [])
    .filter((d) => d.total > 0)
    .map((d) => ({ name: ACCOUNT_TYPE_LABELS[d.accountType] ?? d.accountType, value: d.total }))

  const chartData =
    mode === 'taxTreatment' ? taxData
    : mode === 'assetClass' ? allocationByAssetClass(data.accounts ?? [], holdings ?? [])
    : typeData
  const total = chartData.reduce((s, d) => s + d.value, 0) || 1
  const colors = mode === 'taxTreatment' ? palette.pieTax : palette.pieType

  if (!chartData.length) {
    return (
      <div className="flex items-center justify-center h-40 text-center">
        <EmptyState action={{ label: 'Add an account', to: '/accounts' }}>
          See how your money is split once you add accounts with balances.
        </EmptyState>
      </div>
    )
  }

  return (
    <div>
      {/* Mode toggle */}
      <div className="flex flex-wrap gap-1 mb-4">
        {(Object.keys(MODE_LABELS) as Mode[]).map((m) => (
          <button
            key={m}
            onClick={() => setMode(m)}
            className={`px-3 py-1 rounded text-xs font-medium transition-colors ${
              mode === m
                ? 'bg-accent text-white'
                : 'text-text-secondary hover:text-text-primary hover:bg-subtle'
            }`}
          >
            {MODE_LABELS[m]}
          </button>
        ))}
      </div>

      <div className="flex flex-col sm:flex-row sm:items-center gap-4">
        <ResponsiveContainer width={180} height={180} className="self-center shrink-0">
          <PieChart>
            <Pie
              data={chartData}
              cx="50%"
              cy="50%"
              innerRadius={55}
              outerRadius={85}
              paddingAngle={2}
              dataKey="value"
            >
              {chartData.map((_, i) => (
                <Cell key={i} fill={colors[i % colors.length]} />
              ))}
            </Pie>
            <Tooltip content={(props) => <SliceTooltip {...props} palette={palette} />} />
          </PieChart>
        </ResponsiveContainer>

        {/* Legend */}
        <div className="w-full sm:flex-1 min-w-0 space-y-2">
          {chartData.map((d, i) => (
            <div key={d.name} className="flex items-center justify-between text-sm">
              <div className="flex items-center gap-2 min-w-0">
                <div
                  className="w-2.5 h-2.5 rounded-sm flex-shrink-0"
                  style={{ background: colors[i % colors.length] }}
                />
                <span className="text-text-primary truncate">{d.name}</span>
              </div>
              <span className="text-text-secondary ml-2 flex-shrink-0">
                {formatCurrency(d.value)}
                <span className="inline-block w-12 text-right text-text-tertiary">
                  {((d.value / total) * 100).toFixed(0)}%
                </span>
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
