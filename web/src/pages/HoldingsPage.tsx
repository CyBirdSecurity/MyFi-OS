import { useRef, useMemo, useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useSWR, { mutate } from 'swr'
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import { useHoldings } from '../hooks/useHoldings'
import { useNetWorthCurrent } from '../hooks/useNetWorth'
import { holdingsApi, allocationTargetsApi, costBasisImportApi, CASH_BALANCE_ACCOUNT_TYPES, ASSET_CLASS_LABELS } from '../api/client'
import type { AllocationTarget, Holding, CostBasisPreviewResult } from '../api/client'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Modal } from '../components/ui/Modal'
import { ConcentrationPanel } from '../components/holdings/ConcentrationPanel'
import { FundTypeSizeCard } from '../components/holdings/FundTypeSizeCard'
import { CaretUp, CaretDown, CaretUpDown } from '@phosphor-icons/react'
import { formatCurrency } from '../utils/currency'
import { useChartPalette } from '../hooks/useChartPalette'
import { PageGuide } from '../components/PageGuide'
import { SetupPrompt } from '../components/SetupPrompt'
import { InfoTip } from '../components/ui/InfoTip'
import { AllocationTargetsForm } from '../components/SetupForms'
import { aggregateHoldings, type AggRow } from '../utils/aggregateHoldings'
import { holdingLabel, isPlaceholderTicker } from '../utils/holdingLabel'

type HoldingsSortKey = 'ticker' | 'value' | 'pct'


type DriftRow = {
  assetClass: string
  label: string
  actualPct: number
  actualValue: number
  targetPct: number
  deviation: number       // actual - target
  targetKey: keyof AllocationTarget
  // Best account to rebalance into (largest account of the matching class that needs more)
  recommendAccount?: string
  recommendAmount?: number
}

function driftPill(dev: number) {
  const abs = Math.abs(dev)
  if (abs <= 5) return { cls: 'bg-positive-tint-bg text-positive', label: `${dev >= 0 ? '+' : ''}${dev.toFixed(1)}%` }
  if (abs <= 10) return { cls: 'bg-warning-tint-bg text-warning', label: `${dev >= 0 ? '+' : ''}${dev.toFixed(1)}%` }
  return { cls: 'bg-negative-tint-bg text-negative', label: `${dev >= 0 ? '+' : ''}${dev.toFixed(1)}%` }
}

function AllocationDriftPanel({
  aggregated,
  totalValue,
  targets,
  accounts,
}: {
  aggregated: { ticker: string; assetClass: string; totalValue: number }[]
  totalValue: number
  targets: AllocationTarget
  accounts: { id: string; name: string; accountType: string; balance: number }[]
}) {
  const TARGET_KEYS: { assetClass: string; key: keyof AllocationTarget }[] = [
    { assetClass: 'equity',          key: 'equityPct' },
    { assetClass: 'bond',            key: 'bondPct' },
    { assetClass: 'crypto',          key: 'cryptoPct' },
    { assetClass: 'cash_equivalent', key: 'cashPct' },
    { assetClass: 'other',           key: 'otherPct' },
  ]

  const hasTargets = TARGET_KEYS.some(({ key }) => (targets[key] || 0) > 0)
  if (!hasTargets) return null

  const valueByClass: Record<string, number> = {}
  for (const h of aggregated) {
    valueByClass[h.assetClass] = (valueByClass[h.assetClass] ?? 0) + h.totalValue
  }

  const rows: DriftRow[] = TARGET_KEYS.map(({ assetClass, key }) => {
    const actualValue = valueByClass[assetClass] ?? 0
    const actualPct = totalValue > 0 ? (actualValue / totalValue) * 100 : 0
    const targetPct = targets[key] ?? 0
    const deviation = actualPct - targetPct

    // Recommendation: if we're under-weight, find the largest account whose type maps to this asset class
    let recommendAccount: string | undefined
    let recommendAmount: number | undefined
    if (deviation < -1 && totalValue > 0) {
      const neededValue = (targetPct / 100) * totalValue - actualValue
      const bestAccount = accounts
        .filter((a) => {
          const { ACCOUNT_TYPE_TO_CLASS } = { ACCOUNT_TYPE_TO_CLASS: {
            checking: 'cash_equivalent', savings: 'cash_equivalent',
            brokerage: 'equity', traditional_401k: 'equity', roth_401k: 'equity',
            traditional_ira: 'equity', roth_ira: 'equity', traditional_403b: 'equity',
            roth_403b: 'equity', sep_ira: 'equity', simple_ira: 'equity',
            '457b': 'equity', hsa: 'equity', '529': 'equity',
            crypto: 'crypto',
          } as Record<string, string> }
          return ACCOUNT_TYPE_TO_CLASS[a.accountType] === assetClass
        })
        .sort((a, b) => b.balance - a.balance)[0]
      if (bestAccount) {
        recommendAccount = bestAccount.name
        recommendAmount = neededValue
      }
    }

    return {
      assetClass,
      label: ASSET_CLASS_LABELS[assetClass] ?? assetClass,
      actualPct,
      actualValue,
      targetPct,
      deviation,
      targetKey: key,
      recommendAccount,
      recommendAmount,
    }
  }).filter(({ targetPct }) => targetPct > 0)

  return (
    <Card title="Asset Allocation">
      <div className="overflow-x-auto">
        <table className="w-full text-sm min-w-[520px]">
          <thead>
            <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
              <th className="pb-3 font-medium">Asset Class</th>
              <th className="pb-3 font-medium text-right">Actual</th>
              <th className="pb-3 font-medium text-right">Target</th>
              <th className="pb-3 font-medium text-right">Drift <InfoTip term="drift" /></th>
              <th className="pb-3 font-medium">Recommendation</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const pill = driftPill(row.deviation)
              return (
                <tr key={row.assetClass} className="border-b border-border-default/50">
                  <td className="py-3 text-text-primary">{row.label}</td>
                  <td className="py-3 text-right text-text-secondary">
                    {row.actualPct.toFixed(1)}%
                    <div className="text-xs text-text-muted">{formatCurrency(row.actualValue)}</div>
                  </td>
                  <td className="py-3 text-right text-text-secondary">{row.targetPct.toFixed(0)}%</td>
                  <td className="py-3 text-right">
                    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${pill.cls}`}>
                      {pill.label}
                    </span>
                  </td>
                  <td className="py-3 text-xs text-text-tertiary">
                    {row.recommendAccount && row.recommendAmount != null ? (
                      <span>
                        Add ~{formatCurrency(row.recommendAmount)} to{' '}
                        <span className="text-text-primary">{row.recommendAccount}</span>
                      </span>
                    ) : row.deviation > 5 ? (
                      <span className="text-text-muted">Consider reducing {row.label.toLowerCase()} exposure</span>
                    ) : (
                      <span className="text-text-muted">On target</span>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-text-muted mt-3">
        Green = within ±5% · Yellow = ±5–10% · Red = &gt;10% from target
      </p>
    </Card>
  )
}

// Slice colors come from palette.forecast[i] at each usage site rather
// than a fixed array, since the palette itself is theme-dependent.

function HoldingsSliceTooltip({ active, payload, palette }: any) {
  if (!active || !payload?.length) return null
  const { name, value, payload: data } = payload[0]
  return (
    <div style={{
      background: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}`,
      borderRadius: '8px', padding: '12px 16px', fontSize: '14px',
      boxShadow: '0 4px 16px rgb(20 16 10 / 0.08)',
    }}>
      <div style={{ color: palette.tooltipMuted, marginBottom: '4px' }}>{name}</div>
      <div style={{ color: palette.tooltipText, fontWeight: 600 }}>{formatCurrency(value)}</div>
      <div style={{ color: palette.tick, fontSize: '12px' }}>{data.pct.toFixed(1)}% of portfolio</div>
    </div>
  )
}

function HoldingsPieChart({
  aggregated,
  totalValue,
}: {
  aggregated: { ticker: string; name?: string; totalValue: number }[]
  totalValue: number
}) {
  const palette = useChartPalette()
  const data = aggregated
    .filter((h) => h.totalValue > 0)
    .map((h) => ({
      name: holdingLabel(h.ticker, h.name),
      value: h.totalValue,
      pct: totalValue > 0 ? (h.totalValue / totalValue) * 100 : 0,
    }))
    .sort((a, b) => b.value - a.value)

  if (!data.length) return null

  // Collapse holdings beyond top 14 into "Other"
  const TOP_N = 14
  const top = data.slice(0, TOP_N)
  const rest = data.slice(TOP_N)
  const chartData = rest.length > 0
    ? [...top, {
        name: 'Other',
        value: rest.reduce((s, d) => s + d.value, 0),
        pct: rest.reduce((s, d) => s + d.pct, 0),
      }]
    : top
  const colors = palette.pieType

  // A one-slice donut says nothing; show the single position as a bar.
  if (chartData.length === 1) {
    return (
      <Card title="Holdings breakdown">
        <div className="flex items-center justify-between text-sm mb-2">
          <span className="text-text-primary">{chartData[0].name}</span>
          <span className="text-text-tertiary">100%</span>
        </div>
        <div className="h-2 rounded-sm" style={{ background: colors[0] }} />
      </Card>
    )
  }

  return (
    <Card title="Holdings breakdown">
      <div className="flex flex-col sm:flex-row items-center gap-6">
        <ResponsiveContainer width={200} height={200}>
          <PieChart>
            <Pie
              data={chartData}
              cx="50%"
              cy="50%"
              innerRadius={60}
              outerRadius={95}
              paddingAngle={2}
              dataKey="value"
            >
              {chartData.map((_, i) => (
                <Cell key={i} fill={colors[i % colors.length]} />
              ))}
            </Pie>
            <Tooltip content={<HoldingsSliceTooltip palette={palette} />} />
          </PieChart>
        </ResponsiveContainer>

        <div className="w-full sm:w-auto grid grid-cols-2 sm:grid-cols-[repeat(2,10rem)] gap-x-10 gap-y-1.5">
          {chartData.map((d, i) => (
            <div key={d.name} className="flex items-center justify-between text-sm min-w-0">
              <div className="flex items-center gap-2 min-w-0">
                <div
                  className="w-2.5 h-2.5 rounded-sm flex-shrink-0"
                  style={{ background: colors[i % colors.length] }}
                />
                <span className="text-text-primary truncate text-xs">{d.name}</span>
              </div>
              <span className="text-text-tertiary text-xs ml-2 flex-shrink-0">{d.pct.toFixed(1)}%</span>
            </div>
          ))}
        </div>
      </div>
    </Card>
  )
}

function SortableHeader({
  label,
  sortKey,
  current,
  onSort,
  align = 'right',
  className = '',
}: {
  label: string
  sortKey: HoldingsSortKey
  current: { key: HoldingsSortKey; dir: 'asc' | 'desc' }
  onSort: (key: HoldingsSortKey) => void
  align?: 'left' | 'right'
  className?: string
}) {
  const active = current.key === sortKey
  return (
    <th className={`pb-3 font-medium ${align === 'right' ? 'text-right' : 'text-left'} ${className}`}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className={`inline-flex items-center gap-0.5 transition-colors hover:text-text-primary ${active ? 'text-accent-text' : ''}`}
      >
        {label}
        {active
          ? current.dir === 'asc'
            ? <CaretUp size={12} weight="bold" />
            : <CaretDown size={12} weight="bold" />
          : <CaretUpDown size={12} weight="bold" className="opacity-40" />
        }
      </button>
    </th>
  )
}

function formatPriceAge(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  const diffMin = Math.floor(diffMs / 60_000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDays = Math.floor(diffHr / 24)
  return `${diffDays}d ago`
}

// ─── CostBasisImportModal ─────────────────────────────────────────────────────
function CostBasisImportModal({ onClose, onApplied }: { onClose: () => void; onApplied: () => void }) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<'upload' | 'preview'>('upload')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<CostBasisPreviewResult | null>(null)
  // accepted[holdingId] = costBasis value the user wants to apply
  const [accepted, setAccepted] = useState<Record<string, number>>({})

  const handleUpload = async () => {
    const file = fileRef.current?.files?.[0]
    if (!file) return
    setLoading(true)
    setError('')
    try {
      const result = await costBasisImportApi.preview(file)
      setPreview(result)
      // Pre-populate accepted: use auto-selected match if available, else all matches
      const initial: Record<string, number> = {}
      for (const row of result.rows) {
        if (row.costBasis == null) continue
        const targets = row.autoSelectedMatchId
          ? row.matches.filter((m) => m.id === row.autoSelectedMatchId)
          : row.matches
        for (const m of targets) {
          initial[m.id] = row.costBasis
        }
      }
      setAccepted(initial)
      setStep('preview')
    } catch (e: any) {
      setError(e.message ?? 'Failed to parse file.')
    } finally {
      setLoading(false)
    }
  }

  const handleApply = async () => {
    const rows = Object.entries(accepted)
      .filter(([, v]) => v > 0)
      .map(([holdingId, costBasis]) => ({ holdingId, costBasis }))
    if (!rows.length) { onClose(); return }
    setLoading(true)
    setError('')
    try {
      await costBasisImportApi.apply(rows)
      onApplied()
      onClose()
    } catch (e: any) {
      setError(e.message ?? 'Apply failed.')
    } finally {
      setLoading(false)
    }
  }

  const inputCls = 'w-24 bg-muted border border-border-strong rounded px-2 py-1 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent text-right'

  return (
    <Modal open title="Import Cost Basis" onClose={onClose} size="lg">
      <div className="space-y-4">
        {error && <div className="text-negative text-sm">{error}</div>}

        {step === 'upload' && (
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">
              Upload a portfolio positions CSV from your broker (Vanguard, Fidelity, Schwab, etc.).
              The file should include a column for ticker/symbol and average cost basis per share.
            </p>
            <input
              ref={fileRef}
              type="file"
              accept=".csv,.txt"
              className="block w-full text-sm text-text-secondary file:mr-3 file:py-1.5 file:px-3 file:rounded file:border-0 file:text-sm file:bg-muted file:text-text-primary hover:file:bg-muted"
            />
            {preview === null && (
              <div className="flex justify-end gap-2">
                <Button variant="secondary" onClick={onClose}>Cancel</Button>
                <Button onClick={handleUpload} loading={loading}>Parse File</Button>
              </div>
            )}
          </div>
        )}

        {step === 'preview' && preview && (
          <div className="space-y-3">
            <p className="text-xs text-text-tertiary">
              Detected columns — Ticker: <span className="text-text-primary">{preview.columns.ticker}</span>
              {' · '}Cost Basis: <span className="text-text-primary">{preview.columns.costBasis}</span>
              {preview.columns.account && <>{' · '}Account: <span className="text-text-primary">{preview.columns.account}</span></>}
            </p>
            <div className="overflow-x-auto overflow-y-auto max-h-96">
              <table className="w-full text-sm min-w-[420px]">
                <thead>
                  <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                    <th className="pb-2 font-medium">Ticker</th>
                    <th className="pb-2 font-medium">Account</th>
                    <th className="pb-2 font-medium text-right">Current Basis</th>
                    <th className="pb-2 font-medium text-right">New Basis / Share</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.rows.map((row) =>
                    row.matches.length === 0 ? (
                      <tr key={row.ticker} className="border-b border-border-default/40 opacity-40">
                        <td className="py-2 text-text-tertiary">{row.ticker}</td>
                        <td className="py-2 text-xs text-text-muted" colSpan={3}>no matching holding found</td>
                      </tr>
                    ) : (
                      row.matches.map((m) => {
                        const isAutoSelected = row.autoSelectedMatchId === m.id
                        const sharesMismatch = row.sharesInCsv != null
                          && Math.abs(row.sharesInCsv - m.shares) / Math.max(m.shares, 0.0001) > 0.01
                        return (
                          <tr key={m.id} className={`border-b border-border-default/40 ${!isAutoSelected && row.autoSelectedMatchId ? 'opacity-50' : ''}`}>
                            <td className="py-2 text-text-primary">{row.ticker}</td>
                            <td className="py-2 text-xs text-text-tertiary">
                              {m.accountName}
                              {sharesMismatch && (
                                <span
                                  className="ml-2 text-warning"
                                  title={`CSV shows ${row.sharesInCsv?.toFixed(3)} shares, holding has ${m.shares.toFixed(3)}`}
                                >
                                  ⚠ shares mismatch
                                </span>
                              )}
                            </td>
                            <td className="py-2 text-right text-text-tertiary text-xs">
                              {m.currentBasis != null ? formatCurrency(m.currentBasis) : '—'}
                            </td>
                            <td className="py-2 text-right">
                              <input
                                type="number"
                                step="0.01"
                                min="0"
                                className={inputCls}
                                value={accepted[m.id] ?? row.costBasis ?? ''}
                                onChange={(e) => {
                                  const v = parseFloat(e.target.value)
                                  setAccepted((prev) => ({ ...prev, [m.id]: isNaN(v) ? 0 : v }))
                                }}
                              />
                            </td>
                          </tr>
                        )
                      })
                    )
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex justify-between items-center pt-1">
              <button className="text-sm text-text-tertiary hover:text-text-primary" onClick={() => setStep('upload')}>
                ← Re-upload
              </button>
              <div className="flex gap-2">
                <Button variant="secondary" onClick={onClose}>Cancel</Button>
                <Button onClick={handleApply} loading={loading}>
                  Apply {Object.values(accepted).filter((v) => v > 0).length} Updates
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>
    </Modal>
  )
}

function ImportDropdown({
  onCostBasis,
  onEditBasis,
  onOFX,
}: {
  onCostBasis: () => void
  onEditBasis: () => void
  onOFX: () => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div className="relative" ref={ref}>
      <Button variant="secondary" onClick={() => setOpen(o => !o)}>
        Import ▾
      </Button>
      {open && (
        <div className="absolute right-0 mt-1 w-48 bg-surface border border-border-strong rounded-lg shadow-float z-20 overflow-hidden">
          <button
            className="w-full text-left px-4 py-2.5 text-sm text-text-primary hover:bg-subtle transition-colors"
            onClick={() => { setOpen(false); onOFX() }}
          >
            OFX Holdings
          </button>
          <button
            className="w-full text-left px-4 py-2.5 text-sm text-text-primary hover:bg-subtle transition-colors"
            onClick={() => { setOpen(false); onCostBasis() }}
          >
            Cost Basis CSV
          </button>
          <button
            className="w-full text-left px-4 py-2.5 text-sm text-text-primary hover:bg-subtle transition-colors"
            onClick={() => { setOpen(false); onEditBasis() }}
          >
            Edit Cost Basis Manually
          </button>
        </div>
      )}
    </div>
  )
}

export function HoldingsPage() {
  const navigate = useNavigate()
  const { data: holdings, isLoading } = useHoldings()
  const { data: nw } = useNetWorthCurrent()
  const { data: allocationTargets } = useSWR<AllocationTarget>('/allocation-targets', () => allocationTargetsApi.get())
  const [refreshing, setRefreshing] = useState(false)
  const [sort, setSort] = useState<{ key: HoldingsSortKey; dir: 'asc' | 'desc' }>({ key: 'value', dir: 'desc' })
  const [expandedTickers, setExpandedTickers] = useState<Set<string>>(new Set())
  const [editingBasis, setEditingBasis] = useState(false)
  const [basisEdits, setBasisEdits] = useState<Record<string, string>>({}) // lotId → input string
  const [savingBasis, setSavingBasis] = useState(false)
  const [showCostBasisImport, setShowCostBasisImport] = useState(false)

  const cycleSort = (key: HoldingsSortKey) => {
    setSort(prev =>
      prev.key === key
        ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: key === 'ticker' ? 'asc' : 'desc' }
    )
  }

  const toggleTicker = (ticker: string) =>
    setExpandedTickers(prev => {
      const next = new Set(prev)
      next.has(ticker) ? next.delete(ticker) : next.add(ticker)
      return next
    })

  // One row per ticker across accounts; all cash (cash holdings of any ticker plus
  // checking/savings balances) is a single Cash row. See aggregateHoldings.
  const aggregated = useMemo(
    () => aggregateHoldings(holdings ?? [], nw?.accounts ?? [], CASH_BALANCE_ACCOUNT_TYPES),
    [holdings, nw],
  )

  const accountNameMap = useMemo(() => {
    const m: Record<string, string> = {}
    for (const a of nw?.accounts ?? []) m[a.accountId] = a.name
    return m
  }, [nw])

  const totalValue = aggregated.reduce((sum, h) => sum + h.totalValue, 0)

  const sorted = useMemo(() => {
    return [...aggregated].sort((a, b) => {
      let diff = 0
      if (sort.key === 'ticker') diff = a.ticker.localeCompare(b.ticker)
      else diff = a.totalValue - b.totalValue // value and pct both map to totalValue
      return sort.dir === 'asc' ? diff : -diff
    })
  }, [aggregated, sort])

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      const result = await holdingsApi.refreshPrices()
      await mutate('/holdings')
      await mutate('/networth/current')
      alert(`Updated ${result.updated} holdings with live prices.`)
    } catch (err: any) {
      alert(err.message)
    } finally {
      setRefreshing(false)
    }
  }

  const handleSaveBasis = async () => {
    const updates = Object.entries(basisEdits)
      .map(([id, raw]) => ({ id, costBasis: raw === '' ? null : parseFloat(raw) }))
      .filter(({ costBasis }) => costBasis === null || !isNaN(costBasis))
    if (!updates.length) { setEditingBasis(false); return }
    setSavingBasis(true)
    try {
      await holdingsApi.bulkUpdateCostBasis(updates)
      await mutate('/holdings')
      setEditingBasis(false)
      setBasisEdits({})
    } catch (err: any) {
      alert(err.message ?? 'Save failed.')
    } finally {
      setSavingBasis(false)
    }
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text-primary">Holdings</h1>
            <PageGuide id="holdings" />
          </div>
          <p className="text-text-tertiary text-sm mt-1">
            {aggregated.length} position{aggregated.length !== 1 ? 's' : ''} · Total:{' '}
            <span className="text-text-primary font-medium">{formatCurrency(totalValue)}</span>
          </p>
        </div>
        <div className="flex gap-2">
          {editingBasis ? (
            <>
              <Button variant="secondary" onClick={() => { setEditingBasis(false); setBasisEdits({}) }}>
                Cancel
              </Button>
              <Button onClick={handleSaveBasis} loading={savingBasis}>Save Cost Basis</Button>
            </>
          ) : (
            <>
              <ImportDropdown
                onCostBasis={() => setShowCostBasisImport(true)}
                onEditBasis={() => { setEditingBasis(true); setExpandedTickers(new Set(sorted.map(h => h.ticker))) }}
                onOFX={() => navigate('/import/ofx')}
              />
              <Button variant="secondary" onClick={handleRefresh} loading={refreshing}>
                Refresh Prices
              </Button>
            </>
          )}
        </div>
      </div>

      {aggregated.length > 0 && totalValue > 0 && (
        <HoldingsPieChart aggregated={aggregated} totalValue={totalValue} />
      )}

      {totalValue > 0 && <FundTypeSizeCard holdings={holdings ?? []} />}

      {allocationTargets && totalValue > 0 && (
        <AllocationDriftPanel
          aggregated={aggregated}
          totalValue={totalValue}
          targets={allocationTargets}
          accounts={(nw?.accounts ?? []).map((a) => ({
            id: a.accountId,
            name: a.name,
            accountType: a.accountType,
            balance: a.balance,
          }))}
        />
      )}

      {allocationTargets && totalValue > 0 && !Object.values(allocationTargets).some((v) => typeof v === 'number' && v > 0) && (
        <SetupPrompt
          message="Set a target mix of stocks, bonds, and cash to see how far your portfolio has drifted from it."
          buttonLabel="Set allocation targets"
          title="Allocation Targets"
        >
          {(done) => <AllocationTargetsForm onSaved={done} />}
        </SetupPrompt>
      )}

      <ConcentrationPanel />

      <Card>
        {isLoading ? (
          <div className="text-text-tertiary text-sm">Loading...</div>
        ) : !aggregated.length ? (
          <div className="text-center py-8 text-text-tertiary text-sm">
            No holdings yet. Add positions from an account detail page.
          </div>
        ) : (
          <div className="overflow-x-auto">
          <table className="w-full text-sm min-w-[680px]">
            <thead>
              <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                <th className="pb-3 w-6" />
                <SortableHeader label="Ticker" sortKey="ticker" current={sort} onSort={cycleSort} align="left" />
                <th className="pb-3 pl-6 font-medium">Class</th>
                <th className="pb-3 pl-6 font-medium text-right">Shares</th>
                <th className="pb-3 pl-6 font-medium text-right">
                  Avg Cost{editingBasis && <span className="ml-1 text-accent-text text-xs">(editing)</span>}
                </th>
                <th className="pb-3 pl-6 font-medium text-right">Price</th>
                <SortableHeader label="Value" sortKey="value" current={sort} onSort={cycleSort} className="pl-6" />
                <th className="pb-3 pl-6 font-medium text-right">Return</th>
                <SortableHeader label="%" sortKey="pct" current={sort} onSort={cycleSort} className="pl-6" />
              </tr>
            </thead>
            <tbody>
              {sorted.map((h) => {
                const isCash = h.assetClass === 'cash_equivalent'
                const pct = totalValue > 0 ? (h.totalValue / totalValue) * 100 : 0
                const returnPct =
                  !isCash && h.avgCostBasis != null && h.pricePerShare != null
                    ? ((h.pricePerShare - h.avgCostBasis) / h.avgCostBasis) * 100
                    : null
                const unrealizedGain =
                  !isCash && h.avgCostBasis != null && h.pricePerShare != null
                    ? (h.pricePerShare - h.avgCostBasis) * h.shares
                    : null
                const assetLabel = isCash ? 'Cash' : h.assetClass.replace(/_/g, ' ')
                const expanded = expandedTickers.has(h.ticker)
                const hasLots = h.lots.length > 0 && !isCash

                return (
                  <>
                    {/* ── Ticker row ── */}
                    <tr
                      key={h.ticker}
                      className={`border-b border-border-default/50 hover:bg-subtle/20 ${hasLots ? 'cursor-pointer' : ''}`}
                      onClick={hasLots ? () => toggleTicker(h.ticker) : undefined}
                    >
                      <td className="py-3 pr-1 text-text-muted text-xs select-none">
                        {hasLots && (expanded ? '▼' : '▶')}
                      </td>
                      <td className="py-3">
                        {isCash ? (
                          <div className="font-semibold text-text-primary">Cash</div>
                        ) : (
                          <Link
                            to={`/holdings/${h.ticker}`}
                            onClick={(e) => e.stopPropagation()}
                          >
                            <div className="font-semibold text-accent-text hover:text-accent-text transition-colors">
                              {holdingLabel(h.ticker, h.name)}
                            </div>
                            {isPlaceholderTicker(h.ticker) && h.name ? (
                              <div className="text-xs text-text-tertiary">No public ticker</div>
                            ) : (
                              h.name && <div className="text-xs text-text-tertiary">{h.name}</div>
                            )}
                          </Link>
                        )}
                      </td>
                      <td className="py-3 pl-6 text-text-tertiary text-xs capitalize">{assetLabel}</td>
                      <td className="py-3 pl-6 text-text-secondary text-right">
                        {isCash ? '—' : h.shares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                      </td>
                      <td className="py-3 pl-6 text-text-secondary text-right">
                        {isCash || h.avgCostBasis == null ? '—' : formatCurrency(h.avgCostBasis)}
                      </td>
                      <td className="py-3 pl-6 text-right">
                        {isCash ? (
                          <span className="text-text-muted text-xs">—</span>
                        ) : (
                          <>
                            <div className="text-text-primary">
                              {h.pricePerShare ? formatCurrency(h.pricePerShare) : '—'}
                            </div>
                            {h.priceFetchedAt && (
                              <div className="text-xs text-text-muted" title={new Date(h.priceFetchedAt).toLocaleString()}>
                                {formatPriceAge(h.priceFetchedAt)}
                              </div>
                            )}
                          </>
                        )}
                      </td>
                      <td className="py-3 pl-6 text-right font-medium text-text-primary">
                        {formatCurrency(h.totalValue)}
                      </td>
                      <td className="py-3 pl-6 text-right">
                        {unrealizedGain != null && returnPct != null ? (
                          <div className={unrealizedGain >= 0 ? 'text-positive' : 'text-negative'}>
                            <div className="font-medium">
                              {unrealizedGain >= 0 ? '+' : ''}{formatCurrency(unrealizedGain)}
                            </div>
                            <div className="text-xs">
                              {returnPct >= 0 ? '+' : ''}{returnPct.toFixed(1)}%
                            </div>
                          </div>
                        ) : (
                          <span className="text-text-muted text-xs">—</span>
                        )}
                      </td>
                      <td className="py-3 pl-6 text-right text-text-tertiary">{pct.toFixed(1)}%</td>
                    </tr>

                    {/* ── Lot rows (expanded) ── */}
                    {expanded && h.lots.map((lot, i) => {
                      const lotReturnPct =
                        lot.costBasis != null && lot.pricePerShare != null
                          ? ((lot.pricePerShare - lot.costBasis) / lot.costBasis) * 100
                          : null
                      const lotGain =
                        lot.costBasis != null && lot.pricePerShare != null
                          ? (lot.pricePerShare - lot.costBasis) * lot.shares
                          : null
                      const accountLabel = accountNameMap[lot.accountId] ?? `Lot ${i + 1}`
                      return (
                        <tr
                          key={lot.id}
                          className="border-b border-border-default/30 bg-surface/40 hover:bg-subtle/20"
                        >
                          <td />
                          <td className="py-2 pl-4 text-xs text-text-tertiary col-span-1">
                            {accountLabel}
                          </td>
                          <td />
                          <td className="py-2 pl-6 text-xs text-text-tertiary text-right">
                            {lot.shares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                          </td>
                          <td className="py-2 pl-6 text-xs text-text-tertiary text-right">
                            {editingBasis ? (
                              <input
                                type="number"
                                step="0.01"
                                min="0"
                                placeholder={lot.costBasis != null ? String(lot.costBasis) : '0.00'}
                                value={basisEdits[lot.id] ?? (lot.costBasis != null ? String(lot.costBasis) : '')}
                                onChange={(e) => setBasisEdits((prev) => ({ ...prev, [lot.id]: e.target.value }))}
                                className="w-24 bg-muted border border-border-strong rounded px-2 py-0.5 text-xs text-text-primary focus:outline-none focus:ring-1 focus:ring-accent text-right"
                              />
                            ) : (
                              lot.costBasis != null ? formatCurrency(lot.costBasis) : '—'
                            )}
                          </td>
                          <td className="py-2 pl-6 text-xs text-text-tertiary text-right">
                            {lot.pricePerShare ? formatCurrency(lot.pricePerShare) : '—'}
                          </td>
                          <td className="py-2 pl-6 text-xs text-text-secondary text-right">
                            {formatCurrency(lot.totalValue)}
                          </td>
                          <td className="py-2 pl-6 text-right">
                            {lotGain != null && lotReturnPct != null ? (
                              <div className={`text-xs ${lotGain >= 0 ? 'text-positive' : 'text-negative'}`}>
                                <div>{lotGain >= 0 ? '+' : ''}{formatCurrency(lotGain)}</div>
                                <div>{lotReturnPct >= 0 ? '+' : ''}{lotReturnPct.toFixed(1)}%</div>
                              </div>
                            ) : (
                              <span className="text-text-muted text-xs">—</span>
                            )}
                          </td>
                          <td />
                        </tr>
                      )
                    })}
                  </>
                )
              })}
            </tbody>
          </table>
          </div>
        )}
      </Card>

      {showCostBasisImport && (
        <CostBasisImportModal
          onClose={() => setShowCostBasisImport(false)}
          onApplied={() => { mutate('/holdings'); mutate('/networth/current') }}
        />
      )}
    </div>
  )
}
