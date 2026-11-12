import useSWR from 'swr'
import { useMemo, useState } from 'react'
import { CaretUp, CaretDown, CaretUpDown } from '@phosphor-icons/react'
import { useParams, Link, useNavigate, useSearchParams } from 'react-router-dom'
import { mutate } from 'swr'
import { useAccount } from '../hooks/useAccounts'
import { useSnapshots } from '../hooks/useSnapshots'
import { useHoldingsByAccount } from '../hooks/useHoldings'
import { useContributions } from '../hooks/useContributions'
import {
  snapshotsApi,
  holdingsApi,
  contributionsApi,
  returnRatesApi,
  homeApi,
  vehicleApi,
  creditCardApi,
  loanApi,
  accountsApi,
  ACCOUNT_TYPE_LABELS,
  EMPLOYER_MATCH_ACCOUNT_TYPES,
  RETIREMENT_ACCOUNT_TYPES,
  LIABILITY_ACCOUNT_TYPES,
  NO_HOLDINGS_ACCOUNT_TYPES,
  CASH_BALANCE_ACCOUNT_TYPES,
} from '../api/client'
import type {
  Account,
  Holding,
  UpdateHoldingInput,
  CreateSnapshotInput,
  BalanceSnapshot,
  CreateHoldingInput,
  CreateContributionInput,
  UpdateContributionInput,
  Contribution,
  ContributionAmountType,
  MatchTier,
  AccountEffectiveRate,
  HomeDetails,
  VehicleDetails,
  CreditCardDetails,
  LoanDetails,
} from '../api/client'
import { useHousehold } from '../hooks/useHousehold'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'
import { useAccounts } from '../hooks/useAccounts'
import { useAuthStore } from '../store/authStore'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Modal } from '../components/ui/Modal'
import { TaxBadge } from '../components/ui/Badge'
import { AmortizationTab } from '../components/AmortizationTab'
import { ContributionsThisYear } from '../components/ContributionsThisYear'
import { AccountTransactionsTab } from '../components/AccountTransactionsTab'
import { PlaidSyncBadge } from '../components/plaid/PlaidSyncBadge'
import { PlaidReconnectButton, PlaidGetFullHistoryButton } from '../components/plaid/PlaidLinkFlow'
import { plaidApi } from '../api/client'
import { mutateAfterPlaidSync, usePlaidItemForAccount } from '../hooks/usePlaid'
import { formatCurrency } from '../utils/currency'
import { accountOwner, annualContribution, annualSalary, describeAmount, describeMatch, needsIncome } from '../utils/contributions'
import { formatDate, formatMonthYear, today, toDateInputValue } from '../utils/dates'
import { getTabsForAccountType, TRANSACTIONS_TAB_ACCOUNT_TYPES, type AccountTab } from '../utils/accountTabs'
import { holdingLabel, isPlaceholderTicker } from '../utils/holdingLabel'

type Tab = AccountTab

const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

// ─── Sortable table header ────────────────────────────────────────────────────

function SortableHeader({
  label,
  sortKey,
  current,
  onSort,
  align = 'right',
  className = '',
}: {
  label: string
  sortKey: 'ticker' | 'value' | 'pct'
  current: { key: 'ticker' | 'value' | 'pct'; dir: 'asc' | 'desc' }
  onSort: (key: 'ticker' | 'value' | 'pct') => void
  align?: 'left' | 'right'
  className?: string
}) {
  const active = current.key === sortKey
  return (
    <th className={`pb-2 font-medium ${align === 'right' ? 'text-right' : 'text-left'} ${className}`}>
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

// ─── Holdings grouping ────────────────────────────────────────────────────────

interface HoldingGroup {
  ticker: string
  name?: string
  assetClass: string
  totalShares: number
  avgCostBasis?: number
  totalValue: number
  pricePerShare?: number
  lots: Holding[]
}

function groupHoldings(holdings: Holding[]): HoldingGroup[] {
  const map = new Map<string, HoldingGroup>()
  for (const h of holdings) {
    const existing = map.get(h.ticker)
    if (!existing) {
      map.set(h.ticker, {
        ticker: h.ticker,
        name: h.name,
        assetClass: h.assetClass,
        totalShares: h.shares,
        avgCostBasis: h.costBasis ?? undefined,
        totalValue: h.totalValue,
        pricePerShare: h.pricePerShare,
        lots: [h],
      })
    } else {
      existing.totalShares += h.shares
      existing.totalValue += h.totalValue
      if (h.pricePerShare != null && existing.pricePerShare == null) {
        existing.pricePerShare = h.pricePerShare
      }
      if (h.name && !existing.name) existing.name = h.name
      if (h.costBasis != null) {
        if (existing.avgCostBasis != null) {
          const lotsWithCost = existing.lots.filter((l) => l.costBasis != null)
          const sharesWithCost = lotsWithCost.reduce((s, l) => s + l.shares, 0)
          existing.avgCostBasis =
            (existing.avgCostBasis * sharesWithCost + h.costBasis * h.shares) /
            (sharesWithCost + h.shares)
        } else {
          existing.avgCostBasis = h.costBasis
        }
      }
      existing.lots.push(h)
    }
  }
  return Array.from(map.values()).sort((a, b) => b.totalValue - a.totalValue)
}

// ─── Edit holding modal ───────────────────────────────────────────────────────

const ASSET_CLASS_OPTIONS = [
  { value: 'equity',          label: 'Equity' },
  { value: 'bond',            label: 'Bond' },
  { value: 'crypto',          label: 'Crypto' },
  { value: 'cash_equivalent', label: 'Cash' },
  { value: 'other',           label: 'Other' },
] as const

const ASSET_CLASS_LABELS: Record<string, string> = Object.fromEntries(
  ASSET_CLASS_OPTIONS.map((o) => [o.value, o.label]),
)

function EditHoldingModal({
  accountId,
  holding,
  onClose,
}: {
  accountId: string
  holding: Holding
  onClose: () => void
}) {
  const isCash = holding.assetClass === 'cash_equivalent'
  // For cash, shares stores the dollar amount directly.
  const [amount, setAmount] = useState(String(holding.shares))
  const [shares, setShares] = useState(String(holding.shares))
  const [costBasis, setCostBasis] = useState(
    holding.costBasis != null ? String(holding.costBasis) : '',
  )
  const [assetClass, setAssetClass] = useState(holding.assetClass)
  const [name, setName] = useState(holding.name ?? '')
  const [manualPrice, setManualPrice] = useState(holding.manualPrice)
  const [pricePerShare, setPricePerShare] = useState(
    holding.pricePerShare != null ? String(holding.pricePerShare) : '',
  )
  const [dripEnabled, setDripEnabled] = useState(holding.dripEnabled)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (isCash) {
      const parsedAmount = parseFloat(amount)
      if (!parsedAmount || parsedAmount <= 0) {
        setError('Amount must be a positive number.')
        return
      }
      setSaving(true)
      try {
        await holdingsApi.update(accountId, holding.id, {
          ticker: 'CASH',
          shares: parsedAmount,
          assetClass: 'cash_equivalent',
          name: name.trim() || undefined,
        })
        await mutate(`/accounts/${accountId}/holdings`)
        onClose()
      } catch (err: any) {
        setError(err.message ?? 'Failed to save.')
      } finally {
        setSaving(false)
      }
      return
    }

    const parsedShares = parseFloat(shares)
    if (!parsedShares || parsedShares <= 0) {
      setError('Shares must be a positive number.')
      return
    }
    if (manualPrice && (!pricePerShare || parseFloat(pricePerShare) <= 0)) {
      setError('Unit price must be a positive number when manual price is enabled.')
      return
    }
    const payload: UpdateHoldingInput = {
      ticker: holding.ticker,
      shares: parsedShares,
      assetClass: assetClass || 'equity',
      name: name.trim() || undefined,
      costBasis: costBasis !== '' ? parseFloat(costBasis) : undefined,
      manualPrice,
      pricePerShare: manualPrice && pricePerShare !== '' ? parseFloat(pricePerShare) : undefined,
      dripEnabled,
    }
    setSaving(true)
    try {
      await holdingsApi.update(accountId, holding.id, payload)
      await mutate(`/accounts/${accountId}/holdings`)
      onClose()
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal open onClose={onClose} title={isCash ? 'Edit Cash Position' : `Edit ${holding.ticker} Lot`}>
      <form onSubmit={handleSubmit} className="space-y-4">
        {isCash ? (
          <>
            <div>
              <label className={labelCls}>Amount ($) *</label>
              <input
                type="number"
                step="any"
                min="0"
                required
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Name / Description</label>
              <input
                type="text"
                placeholder="e.g. Money Market Fund"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className={inputCls}
              />
            </div>
          </>
        ) : (
          <>
            <div>
              <label className={labelCls}>Shares *</label>
              <input
                type="number"
                step="any"
                min="0"
                required
                value={shares}
                onChange={(e) => setShares(e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Cost Basis per Share</label>
              <input
                type="number"
                step="any"
                min="0"
                placeholder="Optional"
                value={costBasis}
                onChange={(e) => setCostBasis(e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Asset Class</label>
              <select
                value={assetClass}
                onChange={(e) => setAssetClass(e.target.value)}
                className={inputCls}
              >
                {ASSET_CLASS_OPTIONS.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex items-center gap-3">
              <input
                id="edit-manual-price"
                type="checkbox"
                checked={manualPrice}
                onChange={(e) => setManualPrice(e.target.checked)}
                className="w-4 h-4 accent-accent"
              />
              <label htmlFor="edit-manual-price" className="text-sm text-text-secondary cursor-pointer">
                Manual price (skip auto-sync)
              </label>
            </div>
            {manualPrice && (
              <div>
                <label className={labelCls}>Unit Price ($) *</label>
                <input
                  type="number"
                  step="any"
                  min="0"
                  required={manualPrice}
                  value={pricePerShare}
                  onChange={(e) => setPricePerShare(e.target.value)}
                  placeholder="0.00"
                  className={inputCls}
                />
                <p className="text-xs text-text-tertiary mt-1">
                  Price won&apos;t be updated automatically. Useful for 401k funds not listed on public exchanges.
                </p>
              </div>
            )}
            <div className="flex items-center gap-3">
              <input
                id="edit-drip-enabled"
                type="checkbox"
                checked={dripEnabled}
                onChange={(e) => setDripEnabled(e.target.checked)}
                className="w-4 h-4 accent-accent"
              />
              <label htmlFor="edit-drip-enabled" className="text-sm text-text-secondary cursor-pointer">
                Reinvest dividends (DRIP)
              </label>
            </div>
            <p className="text-xs text-text-tertiary -mt-2">
              When enabled, dividends are automatically added to your share count each month instead of requiring a manual update.
            </p>
            <div>
              <label className={labelCls}>Name / Description</label>
              <input
                type="text"
                placeholder="Optional"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className={inputCls}
              />
            </div>
          </>
        )}
        {error && <p className="text-negative text-sm">{error}</p>}
        <div className="flex justify-end gap-3 pt-1">
          <Button variant="secondary" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={saving}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  )
}

const CASH_HOLDINGS_TABLE_COLS = 5 // expand · ticker · value · % · (spacer for lot actions col)

export function AccountDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: account } = useAccount(id!)
  const { data: snapshots } = useSnapshots(id!)
  const { data: holdings } = useHoldingsByAccount(id!)
  const { data: contributions } = useContributions(id!)
  const { data: homeDetails } = useSWR<HomeDetails>(
    account?.accountType === 'home' ? `${id}/home-details` : null,
    () => homeApi.getDetails(id!),
  )
  const { data: members } = useHousehold()
  const { data: allAccounts } = useAccounts()
  const selfUser = useAuthStore((s) => s.user)
  // ?tab= opens a tab directly (e.g. contributions from the Getting Started checklist).
  const [searchParams] = useSearchParams()
  const [activeTab, setActiveTab] = useState<Tab>((searchParams.get('tab') as Tab | null) ?? 'holdings')
  const [showSnapshot, setShowSnapshot] = useState(false)
  const [editingSnapshot, setEditingSnapshot] = useState<BalanceSnapshot | null>(null)
  const [showHolding, setShowHolding] = useState(false)
  const [showContribution, setShowContribution] = useState(false)
  const [editContribution, setEditContribution] = useState<Contribution | null>(null)
  const [editingHolding, setEditingHolding] = useState<Holding | null>(null)
  const [expandedTickers, setExpandedTickers] = useState<Set<string>>(new Set())
  const [holdingSort, setHoldingSort] = useState<{ key: 'ticker' | 'value' | 'pct'; dir: 'asc' | 'desc' }>({ key: 'value', dir: 'desc' })

  // A home with a known value shows live equity (value less the rolled-forward
  // mortgage), matching net worth, rather than the last saved snapshot.
  const latestBalance = homeDetails?.equity ?? snapshots?.[0]?.balance ?? 0
  const holdingTotal = holdings?.reduce((sum, h) => sum + h.totalValue, 0) ?? 0
  const holdingGroups = useMemo(() => groupHoldings(holdings ?? []), [holdings])

  const sortedHoldingGroups = useMemo(() => {
    return [...holdingGroups].sort((a, b) => {
      let diff = 0
      if (holdingSort.key === 'ticker') {
        diff = a.ticker.localeCompare(b.ticker)
      } else {
        // 'value' and 'pct' are both proportional to totalValue
        diff = a.totalValue - b.totalValue
      }
      return holdingSort.dir === 'asc' ? diff : -diff
    })
  }, [holdingGroups, holdingSort])

  const cycleSort = (key: 'ticker' | 'value' | 'pct') => {
    setHoldingSort(prev =>
      prev.key === key
        ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: key === 'ticker' ? 'asc' : 'desc' }
    )
  }

  const toggleTicker = (ticker: string) =>
    setExpandedTickers((prev) => {
      const next = new Set(prev)
      if (next.has(ticker)) next.delete(ticker)
      else next.add(ticker)
      return next
    })

  const handleCreateSnapshot = async (data: CreateSnapshotInput) => {
    await snapshotsApi.create(id!, data)
    await mutate(`/accounts/${id}/snapshots`)
    await mutate('/networth/current')
    setShowSnapshot(false)
  }

  const handleCreateHolding = async (data: CreateHoldingInput) => {
    await holdingsApi.create(id!, data)
    await mutate(`/accounts/${id}/holdings`)
    setShowHolding(false)
  }

  const handleUpdateSnapshot = async (snapId: string, data: CreateSnapshotInput) => {
    await snapshotsApi.update(id!, snapId, data)
    await mutate(`/accounts/${id}/snapshots`)
    await mutate('/networth/current')
    setEditingSnapshot(null)
  }

  const handleDeleteSnapshot = async (snapId: string) => {
    if (!confirm('Delete this snapshot?')) return
    await snapshotsApi.delete(id!, snapId)
    mutate(`/accounts/${id}/snapshots`)
    mutate('/networth/current')
  }

  const handleDeleteHolding = async (holdingId: string) => {
    if (!confirm('Remove this holding?')) return
    await holdingsApi.delete(id!, holdingId)
    mutate(`/accounts/${id}/holdings`)
  }

  const handleCreateContribution = async (data: CreateContributionInput) => {
    await contributionsApi.create(id!, data)
    await mutate(`/accounts/${id}/contributions`)
    mutate('/forecast')
    setShowContribution(false)
  }

  const handleUpdateContribution = async (cId: string, data: UpdateContributionInput) => {
    await contributionsApi.update(id!, cId, data)
    await mutate(`/accounts/${id}/contributions`)
    mutate('/forecast')
    setEditContribution(null)
  }

  const handleDeleteContribution = async (cId: string) => {
    if (!confirm('Remove this contribution?')) return
    await contributionsApi.delete(id!, cId)
    mutate(`/accounts/${id}/contributions`)
    mutate('/forecast')
  }

  if (!account) return <div className="text-text-tertiary">Loading...</div>

  const tabs = getTabsForAccountType(account.accountType)
  const effectiveTab = tabs.find((t) => t.key === activeTab) ? activeTab : tabs[0]?.key ?? 'snapshots'

  const ownershipTag = account.isJoint ? 'joint' : account.isPartnerAccount ? 'partner' : null

  const partnerMember = (members ?? []).find((m) => m.relationship === 'partner')
  const selfMember = (members ?? []).find((m) => m.relationship === 'self')
  const inHousehold = !!partnerMember
  const selfName = selfMember?.name || selfUser?.displayName || 'Me'
  const partnerName = partnerMember?.name || (allAccounts ?? []).find((a) => a.isPartnerAccount)?.ownerName

  return (
    <div className="space-y-6 max-w-6xl">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2 text-xs text-text-tertiary mb-2">
          <Link to="/accounts" className="hover:text-text-primary">Accounts</Link>
          <span>/</span>
          <span>{account.name}</span>
        </div>
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-bold text-text-primary truncate">{account.name}</h1>
              {ownershipTag === 'partner' && (
                <span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full bg-warning-tint-bg text-warning border border-warning/20 shrink-0">
                  <span className="w-1.5 h-1.5 rounded-full bg-warning" />
                  {account.ownerName}
                </span>
              )}
              {ownershipTag === 'joint' && (
                <span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full bg-subtle text-text-secondary border border-border-default shrink-0">
                  <span className="w-1.5 h-1.5 rounded-full bg-text-tertiary" />
                  {account.ownerName}
                </span>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-2 mt-1">
              {account.institution && (
                <span className="text-sm text-text-tertiary">{account.institution}</span>
              )}
              {!LIABILITY_ACCOUNT_TYPES.has(account.accountType) && (
                <TaxBadge treatment={account.taxTreatment} />
              )}
              <span className="text-xs text-text-tertiary">
                {ACCOUNT_TYPE_LABELS[account.accountType] ?? account.accountType}
              </span>
              {account.returnRateOverride != null && (
                <span className="text-xs bg-accent-tint-bg text-accent-text border border-accent-tint-border rounded px-1.5 py-0.5">
                  {(account.returnRateOverride * 100).toFixed(1)}% custom return rate
                </span>
              )}
              <PlaidSyncBadge account={account} />
            </div>
          </div>
          <div className="sm:text-right shrink-0">
            <div className="text-2xl font-bold text-text-primary">
              {formatCurrency(holdingTotal || latestBalance)}
            </div>
            <div className="text-xs text-text-tertiary">
              {account.accountType === 'home' ? 'Home Equity' :
               account.accountType === 'automobile' ? 'Estimated Value' : 'current balance'}
            </div>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-border-default">
        {tabs.map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setActiveTab(key)}
            className={`px-4 py-2 text-sm font-medium transition-colors -mb-px border-b-2 ${
              effectiveTab === key
                ? 'border-accent text-accent-text'
                : 'border-transparent text-text-tertiary hover:text-text-primary'
            }`}
          >
            {label}
            {key === 'contributions' && contributions?.length ? (
              <span className="ml-1.5 text-xs bg-subtle text-text-secondary rounded-full px-1.5 py-0.5">
                {contributions.filter((c) => c.isActive).length}
              </span>
            ) : null}
          </button>
        ))}
      </div>

      {/* Account-type-specific detail cards */}
      {account.accountType === 'home' && (
        <HomeDetailCard accountId={id!} />
      )}
      {account.accountType === 'automobile' && (
        <VehicleDetailCard accountId={id!} />
      )}

      {/* Holdings tab — cash accounts show a synthetic balance row */}
      {effectiveTab === 'holdings' && CASH_BALANCE_ACCOUNT_TYPES.has(account.accountType) && (
        <Card title="Holdings">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-2 font-medium">Ticker</th>
                  <th className="pb-2 font-medium text-right">Value</th>
                  <th className="pb-2 font-medium text-right">% of Portfolio</th>
                </tr>
              </thead>
              <tbody>
                <tr className="border-b border-border-default/50">
                  <td className="py-3">
                    <div className="font-medium text-text-primary">Cash</div>
                    <div className="text-xs text-text-tertiary">uninvested cash</div>
                  </td>
                  <td className="py-3 text-text-primary text-right font-medium">
                    {formatCurrency(latestBalance)}
                  </td>
                  <td className="py-3 text-text-tertiary text-right text-xs">100%</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p className="text-xs text-text-muted mt-3">
            Balance is tracked via snapshots. {ACCOUNT_TYPE_LABELS[account.accountType]} accounts do not support individual holdings.
          </p>
        </Card>
      )}

      {effectiveTab === 'transactions' && TRANSACTIONS_TAB_ACCOUNT_TYPES.has(account.accountType) && (
        <AccountTransactionsTab accountId={account.id} />
      )}

      {/* Holdings tab — investment accounts */}
      {effectiveTab === 'holdings' && !CASH_BALANCE_ACCOUNT_TYPES.has(account.accountType) && (
        <Card
          title="Holdings"
          action={
            <Button size="sm" onClick={() => setShowHolding(true)}>
              + Add Holding
            </Button>
          }
        >
          {!holdings?.length ? (
            <p className="text-text-tertiary text-sm">No holdings. Add individual positions to track live prices.</p>
          ) : (
            <div className="overflow-x-auto">
            <table className="w-full text-sm min-w-[540px]">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-2 font-medium w-6" />
                  <SortableHeader
                    label="Ticker"
                    sortKey="ticker"
                    current={holdingSort}
                    onSort={cycleSort}
                    align="left"
                  />
                  <th className="pb-2 pl-6 font-medium text-right">Total Shares</th>
                  <th className="pb-2 pl-6 font-medium text-right">Avg Cost</th>
                  <th className="pb-2 pl-6 font-medium text-right">Price</th>
                  <SortableHeader
                    label="Value"
                    sortKey="value"
                    current={holdingSort}
                    onSort={cycleSort}
                    align="right"
                    className="pl-6"
                  />
                  <SortableHeader
                    label="% of Portfolio"
                    sortKey="pct"
                    current={holdingSort}
                    onSort={cycleSort}
                    align="right"
                    className="pl-6"
                  />
                </tr>
              </thead>
              <tbody>
                {sortedHoldingGroups.map((group) => {
                  const expanded = expandedTickers.has(group.ticker)
                  const isCash = group.assetClass === 'cash_equivalent'
                  const gainLoss =
                    !isCash && group.avgCostBasis != null && group.pricePerShare != null
                      ? ((group.pricePerShare - group.avgCostBasis) / group.avgCostBasis) * 100
                      : null
                  return (
                    <>
                      {/* ── Group row ── */}
                      <tr
                        key={`group-${group.ticker}`}
                        className="border-b border-border-default/50 hover:bg-subtle/20 cursor-pointer"
                        onClick={() => toggleTicker(group.ticker)}
                      >
                        <td className="py-3 pr-1">
                          <span className="text-text-tertiary text-xs select-none">
                            {expanded ? '▼' : '▶'}
                          </span>
                        </td>
                        <td className="py-3">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-text-primary">
                              {isCash ? 'Cash' : holdingLabel(group.ticker, group.name)}
                            </span>
                            {!isCash && group.lots.some((l) => l.manualPrice) && (
                              <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-text-secondary">
                                Manual
                              </span>
                            )}
                          </div>
                          {group.name && (
                            <div className="text-xs text-text-tertiary">
                              {isPlaceholderTicker(group.ticker) ? 'No public ticker' : group.name}
                            </div>
                          )}
                        </td>
                        {isCash ? (
                          <>
                            <td className="py-3 pl-6 text-text-tertiary text-right text-xs italic" colSpan={3}>
                              uninvested cash
                            </td>
                          </>
                        ) : (
                          <>
                            <td className="py-3 pl-6 text-text-secondary text-right">
                              {group.totalShares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                            </td>
                            <td className="py-3 pl-6 text-text-secondary text-right">
                              {group.avgCostBasis != null ? formatCurrency(group.avgCostBasis) : '—'}
                            </td>
                            <td className="py-3 pl-6 text-right">
                              <div className="text-text-primary">
                                {group.pricePerShare ? formatCurrency(group.pricePerShare) : '—'}
                              </div>
                              {gainLoss !== null && (
                                <div className={`text-xs ${gainLoss >= 0 ? 'text-positive' : 'text-negative'}`}>
                                  {gainLoss >= 0 ? '+' : ''}{gainLoss.toFixed(1)}%
                                </div>
                              )}
                            </td>
                          </>
                        )}
                        <td className="py-3 pl-6 text-text-primary text-right font-medium">
                          {formatCurrency(group.totalValue)}
                        </td>
                        <td className="py-3 pl-6 text-text-tertiary text-right text-xs">
                          {holdingTotal > 0
                            ? `${((group.totalValue / holdingTotal) * 100).toFixed(1)}%`
                            : '—'}
                        </td>
                      </tr>

                      {/* ── Lot rows (expanded) ── */}
                      {expanded &&
                        group.lots.map((lot, i) => {
                          const lotGainLoss =
                            !isCash && lot.costBasis != null && lot.pricePerShare != null
                              ? ((lot.pricePerShare - lot.costBasis) / lot.costBasis) * 100
                              : null
                          return (
                            <tr
                              key={`lot-${lot.id}`}
                              className="border-b border-border-default/30 bg-surface/50 hover:bg-subtle/30"
                            >
                              <td />
                              <td className="py-2 pl-4 text-xs text-text-tertiary">
                                {isCash ? (lot.name ?? `Cash ${i + 1}`) : `Lot ${i + 1}`}
                              </td>
                              {isCash ? (
                                <td colSpan={3} />
                              ) : (
                                <>
                                  <td className="py-2 pl-6 text-text-tertiary text-right text-xs">
                                    {lot.shares.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                                  </td>
                                  <td className="py-2 pl-6 text-text-tertiary text-right text-xs">
                                    {lot.costBasis != null ? formatCurrency(lot.costBasis) : '—'}
                                  </td>
                                  <td className="py-2 pl-6 text-right text-xs">
                                    <div className="text-text-secondary">
                                      {lot.pricePerShare ? formatCurrency(lot.pricePerShare) : '—'}
                                    </div>
                                    {lotGainLoss !== null && (
                                      <div className={`text-xs ${lotGainLoss >= 0 ? 'text-positive' : 'text-negative'}`}>
                                        {lotGainLoss >= 0 ? '+' : ''}{lotGainLoss.toFixed(1)}%
                                      </div>
                                    )}
                                  </td>
                                </>
                              )}
                              <td className="py-2 pl-6 text-right text-xs text-text-secondary">
                                {formatCurrency(lot.totalValue)}
                              </td>
                              <td />
                              <td className="py-2 text-right">
                                <div className="flex items-center justify-end gap-3">
                                  <button
                                    className="text-xs text-accent-text hover:text-accent-text transition-colors"
                                    onClick={(e) => {
                                      e.stopPropagation()
                                      setEditingHolding(lot)
                                    }}
                                  >
                                    Edit
                                  </button>
                                  <button
                                    onClick={(e) => {
                                      e.stopPropagation()
                                      handleDeleteHolding(lot.id)
                                    }}
                                    className="text-xs text-text-muted hover:text-negative transition-colors"
                                  >
                                    Remove
                                  </button>
                                </div>
                              </td>
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
      )}

      {/* Snapshots tab */}
      {effectiveTab === 'snapshots' && (
        <Card
          title={account.accountType === 'automobile' ? 'Value History' : 'Balance Snapshots'}
          action={
            <Button size="sm" onClick={() => setShowSnapshot(true)}>
              {account.accountType === 'automobile' ? '+ Add Entry' : '+ Add Snapshot'}
            </Button>
          }
        >
          {!snapshots?.length ? (
            <p className="text-text-tertiary text-sm">
              {account.accountType === 'automobile'
                ? 'No entries yet. Record a value to start tracking.'
                : 'No snapshots yet. Record a balance to start tracking.'}
            </p>
          ) : (
            <div className="overflow-x-auto">
            <table className="w-full text-sm min-w-[380px]">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-2 font-medium">Date</th>
                  <th className="pb-2 font-medium text-right">
                    {account.accountType === 'automobile' ? 'Value' : 'Balance'}
                  </th>
                  {account.accountType === 'automobile' && (
                    <th className="pb-2 font-medium text-right">Miles</th>
                  )}
                  <th className="pb-2 font-medium">Notes</th>
                  {account.accountType !== 'automobile' && (
                    <th className="pb-2 font-medium">Source</th>
                  )}
                  <th className="pb-2"></th>
                </tr>
              </thead>
              <tbody>
                {snapshots.map((s) => (
                  <tr key={s.id} className="border-b border-border-default/50">
                    <td className="py-3 text-text-primary">{formatDate(s.snapshotDate)}</td>
                    <td className="py-3 font-medium text-right text-text-primary">
                      {formatCurrency(s.balance)}
                    </td>
                    {account.accountType === 'automobile' && (
                      <td className="py-3 text-text-secondary text-right">
                        {s.miles != null ? s.miles.toLocaleString() : '—'}
                      </td>
                    )}
                    <td className="py-3 text-text-tertiary">{s.notes ?? '—'}</td>
                    {account.accountType !== 'automobile' && (
                      <td className="py-3 text-text-muted text-xs">{s.source}</td>
                    )}
                    <td className="py-3 text-right space-x-3">
                      <button
                        onClick={() => setEditingSnapshot(s)}
                        className="text-xs text-text-muted hover:text-text-primary transition-colors"
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => handleDeleteSnapshot(s.id)}
                        className="text-xs text-text-muted hover:text-negative transition-colors"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          )}
        </Card>
      )}

      {/* Card Details tab (credit cards) */}
      {effectiveTab === 'cardDetails' && account.accountType === 'credit_card' && (
        <CreditCardDetailCard accountId={id!} isPlaidLinked={account.externalProvider === 'plaid'} />
      )}

      {/* Loan Details tab (loans and other liabilities) */}
      {effectiveTab === 'loanDetails' && (
        <LoanDetailCard accountId={id!} />
      )}

      {/* Contributions tab */}
      {effectiveTab === 'contributions' && (
        <Card
          title="Planned Contributions"
          action={
            <Button size="sm" onClick={() => setShowContribution(true)}>
              + Add Contribution
            </Button>
          }
        >
          <p className="text-xs text-text-tertiary mb-4">
            Contributions are used in the{' '}
            <Link to="/forecast" className="text-accent-text hover:text-accent-text">
              portfolio forecast
            </Link>
            . They stop automatically when you reach your retirement age.
          </p>

          {/* Return rate override */}
          <AccountReturnRateOverride accountId={id!} currentOverride={account.returnRateOverride} />

          {!contributions?.length ? (
            <p className="text-text-tertiary text-sm mt-4">
              No contributions set. Add recurring contributions to project future growth.
            </p>
          ) : (
            <div className="overflow-x-auto mt-4">
            <table className="w-full text-sm min-w-[560px]">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-2 font-medium">Label</th>
                  <th className="pb-2 font-medium text-right">Amount</th>
                  <th className="pb-2 font-medium">Frequency</th>
                  <th className="pb-2 font-medium text-right">Annual +%</th>
                  <th className="pb-2 font-medium text-right">Match</th>
                  <th className="pb-2 font-medium">Period</th>
                  <th className="pb-2 font-medium">Active</th>
                  <th className="pb-2"></th>
                </tr>
              </thead>
              <tbody>
                {contributions.map((c) => (
                  <tr
                    key={c.id}
                    className={`border-b border-border-default/50 ${!c.isActive ? 'opacity-50' : ''}`}
                  >
                    <td className="py-3 text-text-primary">{c.label ?? '—'}</td>
                    <td className="py-3 text-text-primary font-medium text-right">
                      {c.amountType === 'percent' ? describeAmount(c, formatCurrency) : formatCurrency(c.amount)}
                    </td>
                    <td className="py-3 text-text-secondary">
                      {c.amountType === 'percent'
                        ? 'Per paycheck'
                        : c.frequency === 'biweekly' ? 'Bi-weekly' : c.frequency === 'monthly' ? 'Monthly' : 'Annually'}
                    </td>
                    <td className="py-3 text-text-secondary text-right">
                      {c.annualIncreasePct > 0
                        ? `${(c.annualIncreasePct * 100).toFixed(1)}%`
                        : '—'}
                    </td>
                    <td className="py-3 px-3 text-text-secondary text-right">
                      {describeMatch(c.matchTiers)}
                    </td>
                    <td className="py-3 text-text-secondary text-sm whitespace-nowrap">
                      {c.startDate || c.endDate ? (
                        <>
                          {c.startDate ? formatMonthYear(c.startDate) : '…'}
                          {' → '}
                          {c.endDate ? formatMonthYear(c.endDate) : '…'}
                        </>
                      ) : '—'}
                    </td>
                    <td className="py-3">
                      <span
                        className={`text-xs ${
                          c.isActive ? 'text-positive' : 'text-text-muted'
                        }`}
                      >
                        {c.isActive ? 'Yes' : 'No'}
                      </span>
                    </td>
                    <td className="py-3 text-right">
                      <div className="flex items-center justify-end gap-3">
                        <button
                          onClick={() => setEditContribution(c)}
                          className="text-xs text-text-tertiary hover:text-accent-text transition-colors"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() => handleDeleteContribution(c.id)}
                          className="text-xs text-text-muted hover:text-negative transition-colors"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          )}
        </Card>
      )}

      {effectiveTab === 'contributions' && RETIREMENT_ACCOUNT_TYPES.has(account.accountType) && (
        <ContributionsThisYear account={account} plans={contributions ?? []} />
      )}

      {/* Amortization tab */}
      {effectiveTab === 'amortization' && account.accountType === 'home' && (
        <AmortizationTab
          accountId={id!}
          homeDetails={homeDetails ?? null}
        />
      )}

      {/* Settings tab */}
      {effectiveTab === 'settings' && (
        <AccountSettings
          account={account}
          inHousehold={inHousehold}
          selfName={selfName}
          partnerName={partnerName}
          partnerMemberId={partnerMember?.id}
          onDelete={() => navigate('/accounts')}
        />
      )}

      {/* Modals */}
      <Modal
        open={showSnapshot}
        onClose={() => setShowSnapshot(false)}
        title={account.accountType === 'automobile' ? 'Add Entry' : 'Add Snapshot'}
      >
        <SnapshotForm
          accountType={account.accountType}
          onSubmit={handleCreateSnapshot}
          onCancel={() => setShowSnapshot(false)}
        />
      </Modal>

      {editingSnapshot && (
        <Modal
          open={!!editingSnapshot}
          onClose={() => setEditingSnapshot(null)}
          title={account.accountType === 'automobile' ? 'Edit Entry' : 'Edit Snapshot'}
        >
          <SnapshotForm
            accountType={account.accountType}
            initial={editingSnapshot}
            onSubmit={(data) => handleUpdateSnapshot(editingSnapshot.id, data)}
            onCancel={() => setEditingSnapshot(null)}
          />
        </Modal>
      )}

      <Modal open={showHolding} onClose={() => setShowHolding(false)} title="Add Holding">
        <HoldingForm onSubmit={handleCreateHolding} onCancel={() => setShowHolding(false)} />
      </Modal>

      <Modal
        open={showContribution}
        onClose={() => setShowContribution(false)}
        title="Add Contribution"
      >
        <ContributionForm
          onSubmit={handleCreateContribution}
          onCancel={() => setShowContribution(false)}
          account={account}
        />
      </Modal>

      {editContribution && (
        <Modal
          open={!!editContribution}
          onClose={() => setEditContribution(null)}
          title="Edit Contribution"
        >
          <ContributionForm
            initial={editContribution}
            onSubmit={(data) =>
              handleUpdateContribution(editContribution.id, {
                ...data,
                isActive: editContribution.isActive,
              })
            }
            onCancel={() => setEditContribution(null)}
            showActive
            initialActive={editContribution.isActive}
            account={account}
          />
        </Modal>
      )}

      {editingHolding && (
        <EditHoldingModal
          accountId={id!}
          holding={editingHolding}
          onClose={() => setEditingHolding(null)}
        />
      )}
    </div>
  )
}

// ─── Settings tab ─────────────────────────────────────────────────────────────

function AccountSettings({
  account,
  inHousehold,
  selfName,
  partnerName,
  partnerMemberId,
  onDelete,
}: {
  account: Account
  inHousehold: boolean
  selfName: string
  partnerName?: string
  partnerMemberId?: string
  onDelete: () => void
}) {
  // Rename
  const [name, setName] = useState(account.name)
  const [renameSaving, setRenameSaving] = useState(false)
  const [renameError, setRenameError] = useState('')

  // Ownership
  const currentOwner: 'self' | 'partner' | 'joint' = account.isJoint ? 'joint' : account.isPartnerAccount ? 'partner' : 'self'
  const [owner, setOwner] = useState<'self' | 'partner' | 'joint'>(currentOwner)
  const [ownerSaving, setOwnerSaving] = useState(false)
  const [ownerError, setOwnerError] = useState('')

  // Delete
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')

  // Plaid unlink
  const plaidItem = usePlaidItemForAccount(account)
  const { data: allAccounts } = useAccounts()
  const [unlinking, setUnlinking] = useState(false)
  const [unlinkError, setUnlinkError] = useState('')
  // Only investment-like accounts (brokerage, IRA/401k, HSA, crypto, ...) get
  // Plaid's holdings-based historical backfill — checking/savings/credit/loan
  // accounts are balance-only and never have backfill requested for them.
  const isHoldingsBased =
    !CASH_BALANCE_ACCOUNT_TYPES.has(account.accountType) &&
    !LIABILITY_ACCOUNT_TYPES.has(account.accountType) &&
    !NO_HOLDINGS_ACCOUNT_TYPES.has(account.accountType)
  // The "get full history" reset only makes sense for banking-transaction
  // accounts (checking/savings/credit/loan) — investment accounts get their
  // history via a separate holdings-based backfill, not transactions.days_requested.
  const isBankTransactionAccount =
    CASH_BALANCE_ACCOUNT_TYPES.has(account.accountType) || LIABILITY_ACCOUNT_TYPES.has(account.accountType)

  const handleRename = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setRenameSaving(true)
    setRenameError('')
    try {
      await accountsApi.update(account.id, {
        name: name.trim(),
        accountType: account.accountType,
        taxTreatment: account.taxTreatment,
        institution: account.institution ?? undefined,
      })
      await mutate(`/accounts/${account.id}`)
      await mutate('/accounts')
    } catch (err: any) {
      setRenameError(err.message ?? 'Failed to rename.')
    } finally {
      setRenameSaving(false)
    }
  }

  const handleOwnerSave = async () => {
    if (owner === currentOwner) return
    setOwnerSaving(true)
    setOwnerError('')
    try {
      await accountsApi.update(account.id, {
        name: account.name,
        accountType: account.accountType,
        taxTreatment: account.taxTreatment,
        institution: account.institution ?? undefined,
        memberId: owner === 'partner' ? partnerMemberId : undefined,
        clearMember: owner === 'self',
        isJoint: owner === 'joint' || undefined,
      })
      await mutate(`/accounts/${account.id}`)
      await mutate('/accounts')
    } catch (err: any) {
      setOwnerError(err.message ?? 'Failed to update ownership.')
    } finally {
      setOwnerSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this account? Its historical snapshots will be preserved.')) return
    setDeleting(true)
    setDeleteError('')
    try {
      await accountsApi.delete(account.id)
      await mutate('/accounts')
      onDelete()
    } catch (err: any) {
      setDeleteError(err.message ?? 'Failed to delete.')
      setDeleting(false)
    }
  }

  const handleUnlinkPlaid = async () => {
    if (!account.externalItemId) return
    if (!confirm(
      'Unlink this account from Plaid? It will revert to manual tracking — all holdings, snapshots, and settings are preserved.',
    )) return
    setUnlinking(true)
    setUnlinkError('')
    try {
      await plaidApi.removeItem(account.externalItemId)
      await mutateAfterPlaidSync()
    } catch (err: any) {
      setUnlinkError(err.message ?? 'Failed to unlink.')
    } finally {
      setUnlinking(false)
    }
  }

  return (
    <div className="space-y-4">
      {/* Rename */}
      <Card title="Account Name">
        <form onSubmit={handleRename} className="space-y-3">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            className={inputCls}
          />
          {renameError && <p className="text-negative text-sm">{renameError}</p>}
          <div className="flex justify-end">
            <Button type="submit" size="sm" loading={renameSaving}>
              Save Name
            </Button>
          </div>
        </form>
      </Card>

      {/* Ownership — only shown when in a household */}
      {inHousehold && partnerName && (
        <Card title="Ownership">
          <p className="text-xs text-text-tertiary mb-3">
            Change who this account belongs to. Both household members can always view each other&apos;s accounts.
          </p>
          <div className="flex gap-2 mb-3">
            {(
              [
                { value: 'self' as const,    label: selfName,    color: 'indigo' },
                { value: 'partner' as const, label: partnerName ?? 'Partner', color: 'amber' },
                { value: 'joint' as const,   label: 'Joint',     color: 'neutral' },
              ]
            ).map(({ value, label, color }) => (
              <button
                key={value}
                type="button"
                onClick={() => setOwner(value)}
                className={`flex-1 text-xs px-3 py-2 rounded-lg border transition-colors ${
                  owner === value
                    ? color === 'indigo'
                      ? 'bg-accent-tint-bg border-accent text-accent-text'
                      : color === 'neutral'
                        ? 'bg-subtle border-border-strong text-text-primary'
                        : 'bg-warning-tint-bg border-warning text-warning'
                    : 'border-border-strong text-text-tertiary hover:text-text-primary hover:border-border-strong'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
          {owner === 'partner' && (
            <p className="text-xs text-text-tertiary mb-3">
              This account will appear under {partnerName}&rsquo;s name.
            </p>
          )}
          {owner === 'joint' && (
            <p className="text-xs text-text-tertiary mb-3">
              This account will be shared by you and {partnerName}.
            </p>
          )}
          {ownerError && <p className="text-negative text-sm mb-2">{ownerError}</p>}
          <div className="flex justify-end">
            <Button
              size="sm"
              loading={ownerSaving}
              onClick={handleOwnerSave}
              disabled={owner === currentOwner}
            >
              Save Ownership
            </Button>
          </div>
        </Card>
      )}

      {/* Plaid connection */}
      {account.externalProvider === 'plaid' && account.externalItemId && (
        <Card title="Plaid Connection">
          <p className="text-sm text-text-secondary mb-1">
            {plaidItem?.institutionName ?? 'Linked institution'}
            {plaidItem?.status === 'login_required' && (
              <span className="ml-2 text-xs font-medium px-2 py-0.5 rounded-full bg-warning-tint-bg text-warning">
                reconnect needed
              </span>
            )}
            {plaidItem?.status === 'error' && (
              <span className="ml-2 text-xs font-medium px-2 py-0.5 rounded-full bg-negative-tint-bg text-negative">
                error
              </span>
            )}
          </p>
          <div className="text-xs text-text-tertiary mb-4 space-y-0.5">
            <p>
              {account.lastSyncedAt
                ? `Last synced ${new Date(account.lastSyncedAt).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}`
                : 'Not yet synced'}
              {plaidItem?.lastSyncError ? ` · ${plaidItem.lastSyncError}` : ''}
            </p>
            {isHoldingsBased && plaidItem?.backfillRequested && (
              <p>
                {plaidItem.backfilledAt
                  ? `History backfilled ${new Date(plaidItem.backfilledAt).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}`
                  : 'Backfilling up to 24 months of history — this can take a few minutes.'}
              </p>
            )}
          </div>
          {unlinkError && <p className="text-negative text-sm mb-2">{unlinkError}</p>}
          <div className="flex flex-wrap gap-2">
            {plaidItem?.status === 'login_required' && (
              <PlaidReconnectButton itemId={account.externalItemId} />
            )}
            {isBankTransactionAccount && (
              <PlaidGetFullHistoryButton
                itemId={account.externalItemId}
                accountId={account.id}
                accounts={allAccounts ?? []}
              />
            )}
            <Button variant="secondary" size="sm" onClick={handleUnlinkPlaid} loading={unlinking}>
              Unlink from Plaid
            </Button>
          </div>
        </Card>
      )}

      {/* Delete */}
      <Card title="Danger Zone">
        <p className="text-sm text-text-secondary mb-4">
          Deleting this account is permanent. Historical balance snapshots are preserved for net worth history, but the account will no longer appear in your dashboard or forecast.
        </p>
        {account.isPartnerAccount && (
          <p className="text-xs text-warning mb-3">
            Only {account.ownerName} can delete this account.
          </p>
        )}
        {deleteError && <p className="text-negative text-sm mb-2">{deleteError}</p>}
        <Button
          variant="danger"
          size="sm"
          onClick={handleDelete}
          loading={deleting}
          disabled={account.isPartnerAccount}
        >
          Delete Account
        </Button>
      </Card>
    </div>
  )
}

// ─── Home Detail Card ─────────────────────────────────────────────────────────

function HomeDetailCard({ accountId }: { accountId: string }) {
  const { data: details, mutate: mutateDetails } = useSWR<HomeDetails>(
    `${accountId}/home-details`,
    () => homeApi.getDetails(accountId),
  )

  const [address, setAddress] = useState('')
  const [editingAddress, setEditingAddress] = useState(false)
  const [editingLoan, setEditingLoan] = useState(false)
  const [editingHomeValue, setEditingHomeValue] = useState(false)
  const [homeValueInput, setHomeValueInput] = useState('')
  const [interestRate, setInterestRate] = useState('')
  const [loanTermYears, setLoanTermYears] = useState('')
  const [originalLoanValue, setOriginalLoanValue] = useState('')
  const [currentLoanBalance, setCurrentLoanBalance] = useState('')
  const [mortgageStartDate, setMortgageStartDate] = useState('')
  const [extraMonthlyPayment, setExtraMonthlyPayment] = useState('')
  const [editingCosts, setEditingCosts] = useState(false)
  const [purchasePrice, setPurchasePrice] = useState('')
  const [propertyTaxAnnual, setPropertyTaxAnnual] = useState('')
  const [homeInsuranceAnnual, setHomeInsuranceAnnual] = useState('')
  const [hoaMonthly, setHoaMonthly] = useState('')
  const [pmiMonthly, setPmiMonthly] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const startEditLoan = () => {
    setInterestRate(details ? String(details.interestRate * 100) : '0')
    setLoanTermYears(details ? String(details.loanTermYears) : '30')
    setOriginalLoanValue(details ? String(details.originalLoanValue) : '0')
    setCurrentLoanBalance(details ? String(details.currentLoanBalance) : '0')
    setMortgageStartDate(toDateInputValue(details?.mortgageStartDate))
    setExtraMonthlyPayment(details?.extraMonthlyPayment ? String(details.extraMonthlyPayment) : '0')
    setEditingLoan(true)
  }

  // upsertDetails always replaces the full row — there's no partial-update
  // support server-side — so every save must resend every field or the
  // omitted ones silently reset to their zero value. This derives the full
  // set from the last-loaded details so each handler only needs to override
  // what it's actually changing.
  const baseHomeFields = () => ({
    address: details?.address ?? '',
    interestRate: details?.interestRate ?? 0,
    loanTermYears: details?.loanTermYears ?? 30,
    originalLoanValue: details?.originalLoanValue ?? 0,
    currentLoanBalance: details?.currentLoanBalance ?? 0,
    mortgageStartDate: toDateInputValue(details?.mortgageStartDate) || undefined,
    extraMonthlyPayment: details?.extraMonthlyPayment ?? 0,
    trackMortgageAsLiability: details?.trackMortgageAsLiability ?? false,
    includeInRetirementCalcs: details?.includeInRetirementCalcs ?? false,
    propertyTaxAnnual: details?.propertyTaxAnnual ?? 0,
    homeInsuranceAnnual: details?.homeInsuranceAnnual ?? 0,
    hoaMonthly: details?.hoaMonthly ?? 0,
    pmiMonthly: details?.pmiMonthly ?? 0,
    purchasePrice: details?.purchasePrice ?? 0,
  })

  const startEditCosts = () => {
    const str = (v?: number) => (v ? String(v) : '')
    setPurchasePrice(str(details?.purchasePrice))
    setPropertyTaxAnnual(str(details?.propertyTaxAnnual))
    setHomeInsuranceAnnual(str(details?.homeInsuranceAnnual))
    setHoaMonthly(str(details?.hoaMonthly))
    setPmiMonthly(str(details?.pmiMonthly))
    setEditingCosts(true)
  }

  const handleSaveCosts = async () => {
    setSaving(true)
    setError(null)
    try {
      await homeApi.upsertDetails(accountId, {
        ...baseHomeFields(),
        purchasePrice: parseFloat(purchasePrice) || 0,
        propertyTaxAnnual: parseFloat(propertyTaxAnnual) || 0,
        homeInsuranceAnnual: parseFloat(homeInsuranceAnnual) || 0,
        hoaMonthly: parseFloat(hoaMonthly) || 0,
        pmiMonthly: parseFloat(pmiMonthly) || 0,
      })
      await mutateDetails()
      await mutate(`/accounts/${accountId}/amortization`)
      setEditingCosts(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  const handleSaveAddress = async () => {
    setSaving(true)
    setError(null)
    try {
      await homeApi.upsertDetails(accountId, { ...baseHomeFields(), address })
      await mutateDetails()
      setEditingAddress(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  const handleSaveHomeValue = async () => {
    const val = parseFloat(homeValueInput)
    if (!val || val <= 0) return
    setSaving(true)
    setError(null)
    try {
      await homeApi.upsertDetails(accountId, { ...baseHomeFields(), homeValue: val })
      await mutateDetails()
      await mutate(`/accounts/${accountId}/snapshots`)
      await mutate('/networth/current')
      setEditingHomeValue(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  const handleSaveLoan = async () => {
    setSaving(true)
    setError(null)
    try {
      await homeApi.upsertDetails(accountId, {
        ...baseHomeFields(),
        interestRate: parseFloat(interestRate) / 100,
        loanTermYears: parseInt(loanTermYears),
        originalLoanValue: parseFloat(originalLoanValue),
        currentLoanBalance: parseFloat(currentLoanBalance),
        mortgageStartDate: mortgageStartDate || undefined,
        extraMonthlyPayment: parseFloat(extraMonthlyPayment) || 0,
      })
      await mutateDetails()
      await mutate(`/accounts/${accountId}/snapshots`)
      await mutate('/networth/current')
      setEditingLoan(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Card title="Property">
        <div className="space-y-4">
          <div>
            <label className={labelCls}>Address</label>
            {editingAddress ? (
              <div className="flex gap-2">
                <input
                  value={address}
                  onChange={(e) => setAddress(e.target.value)}
                  placeholder="123 Main St, City, State 00000"
                  className={`${inputCls} flex-1`}
                />
                <Button size="sm" onClick={handleSaveAddress} loading={saving}>Save</Button>
                <Button size="sm" variant="secondary" onClick={() => setEditingAddress(false)}>Cancel</Button>
              </div>
            ) : (
              <div className="flex items-center gap-3">
                <span className="text-sm text-text-primary">{details?.address || '—'}</span>
                <button
                  onClick={() => { setAddress(details?.address ?? ''); setEditingAddress(true) }}
                  className="text-xs text-text-tertiary hover:text-accent-text"
                >
                  Edit
                </button>
              </div>
            )}
          </div>

          {/* Property Value row */}
          <div>
            <label className={labelCls}>Property Value</label>
            {editingHomeValue ? (
              <div className="flex gap-2">
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={homeValueInput}
                  onChange={(e) => setHomeValueInput(e.target.value)}
                  placeholder="e.g. 350000"
                  className={`${inputCls} flex-1`}
                  autoFocus
                />
                <Button size="sm" onClick={handleSaveHomeValue} loading={saving}>Save</Button>
                <Button size="sm" variant="secondary" onClick={() => setEditingHomeValue(false)}>Cancel</Button>
              </div>
            ) : (
              <div className="flex items-center gap-3">
                <span className="text-sm text-text-primary">
                  {details?.lastZestimate != null
                    ? formatCurrency(details.lastZestimate)
                    : '—'}
                </span>
                {details?.lastZestimateAt && (
                  <span className="text-xs text-text-muted">
                    {formatDate(details.lastZestimateAt)}
                  </span>
                )}
                <button
                  onClick={() => {
                    setHomeValueInput(details?.lastZestimate != null ? String(details.lastZestimate) : '')
                    setEditingHomeValue(true)
                  }}
                  className="text-xs text-text-tertiary hover:text-accent-text"
                >
                  Set manually
                </button>
              </div>
            )}
          </div>

          {error && <p className="text-negative text-sm">{error}</p>}

          <label className="flex items-center gap-2 cursor-pointer select-none">
            <input
              type="checkbox"
              className="w-4 h-4 rounded accent-accent cursor-pointer"
              checked={details?.includeInRetirementCalcs ?? false}
              onChange={async (e) => {
                try {
                  await homeApi.upsertDetails(accountId, {
                    ...baseHomeFields(),
                    includeInRetirementCalcs: e.target.checked,
                  })
                  await mutateDetails()
                  await mutate('/forecast')
                } catch {
                  // silently ignore — toggle snaps back on next fetch
                }
              }}
            />
            <span className="text-xs text-text-secondary">
              Count this home's equity toward FI Number, Forecast, and Retirement projections
            </span>
          </label>
          <p className="text-xs text-text-muted -mt-2">
            Always included in Net Worth and Accounts regardless of this setting.
          </p>

          {details?.equity != null && (
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-2 border-t border-border-default">
              <div>
                <div className="text-xs text-text-tertiary mb-1">Home Value</div>
                <div className="text-sm font-medium text-text-primary">{formatCurrency(details.lastZestimate ?? 0)}</div>
              </div>
              <div>
                <div className="text-xs text-text-tertiary mb-1">Loan Balance</div>
                <div className="text-sm font-medium text-text-primary">{formatCurrency(details.currentLoanBalance)}</div>
              </div>
              <div>
                <div className="text-xs text-text-tertiary mb-1">Equity</div>
                <div className={`text-sm font-medium ${details.equity >= 0 ? 'text-positive' : 'text-negative'}`}>
                  {formatCurrency(details.equity)}
                </div>
              </div>
            </div>
          )}
        </div>
      </Card>

      <Card
        title="Loan"
        action={
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 cursor-pointer select-none">
              <input
                type="checkbox"
                className="w-4 h-4 rounded accent-accent cursor-pointer"
                checked={details?.trackMortgageAsLiability ?? false}
                onChange={async (e) => {
                  if (!details) return
                  try {
                    await homeApi.upsertDetails(accountId, {
                      ...baseHomeFields(),
                      trackMortgageAsLiability: e.target.checked,
                    })
                    await mutateDetails()
                    await mutate('/networth/current')
                  } catch {
                    // silently ignore — toggle snaps back on next fetch
                  }
                }}
              />
              <span className="text-xs text-text-secondary">Track mortgage as liability</span>
            </label>
            {!editingLoan && <Button size="sm" variant="secondary" onClick={startEditLoan}>Edit</Button>}
          </div>
        }
      >
        <div className="space-y-4">
          {editingLoan ? (
            <>
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div>
                  <label className={labelCls}>Interest Rate (%)</label>
                  <input type="number" step="0.01" min={0} value={interestRate}
                    onChange={(e) => setInterestRate(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>Loan Term (years)</label>
                  <input type="number" step="1" min={1} value={loanTermYears}
                    onChange={(e) => setLoanTermYears(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>Original Loan ($)</label>
                  <input type="number" step="0.01" min={0} value={originalLoanValue}
                    onChange={(e) => setOriginalLoanValue(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>Current Balance ($)</label>
                  <input type="number" step="0.01" min={0} value={currentLoanBalance}
                    onChange={(e) => setCurrentLoanBalance(e.target.value)} className={inputCls} />
                </div>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>
                    Mortgage Start Date
                    <span className="text-text-muted ml-1 font-normal">(for forecast accuracy)</span>
                  </label>
                  <input
                    type="date"
                    value={mortgageStartDate}
                    onChange={(e) => setMortgageStartDate(e.target.value)}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>
                    Extra Monthly Principal ($)
                    <span className="text-text-muted ml-1 font-normal">(accelerates payoff)</span>
                  </label>
                  <input
                    type="number"
                    step="50"
                    min={0}
                    value={extraMonthlyPayment}
                    onChange={(e) => setExtraMonthlyPayment(e.target.value)}
                    className={inputCls}
                  />
                </div>
              </div>
              {error && <p className="text-negative text-sm">{error}</p>}
              <div className="flex gap-2">
                <Button size="sm" onClick={handleSaveLoan} loading={saving}>Save</Button>
                <Button size="sm" variant="secondary" onClick={() => setEditingLoan(false)}>Cancel</Button>
              </div>
            </>
          ) : (
            <div className="space-y-4 text-sm">
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                <div>
                  <div className="text-xs text-text-tertiary mb-1">Interest Rate</div>
                  <div className="text-text-primary font-medium">
                    {details ? `${(details.interestRate * 100).toFixed(2)}%` : '—'}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-text-tertiary mb-1">Loan Term</div>
                  <div className="text-text-primary font-medium">{details ? `${details.loanTermYears} yr` : '—'}</div>
                </div>
                <div>
                  <div className="text-xs text-text-tertiary mb-1">Original Loan</div>
                  <div className="text-text-primary font-medium">{details ? formatCurrency(details.originalLoanValue) : '—'}</div>
                </div>
                <div>
                  <div className="text-xs text-text-tertiary mb-1">Current Balance</div>
                  <div className="text-text-primary font-medium">{details ? formatCurrency(details.currentLoanBalance) : '—'}</div>
                  {details?.loanBalanceEstimated && details.loanBalanceAsOf && (
                    <div className="text-xs text-text-muted mt-0.5">
                      Estimated from the balance you entered on {formatDate(toDateInputValue(details.loanBalanceAsOf))}
                    </div>
                  )}
                </div>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <div className="text-xs text-text-tertiary mb-1">Mortgage Start Date</div>
                  <div className="text-text-primary font-medium">
                    {details?.mortgageStartDate
                      ? formatDate(details.mortgageStartDate)
                      : <span className="text-text-muted text-xs">Not set — add for accurate forecast paydown</span>}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-text-tertiary mb-1">Extra Monthly Principal</div>
                  <div className="text-text-primary font-medium">
                    {details?.extraMonthlyPayment
                      ? formatCurrency(details.extraMonthlyPayment) + '/mo'
                      : <span className="text-text-muted text-xs">None — see Amortization tab to model savings</span>}
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </Card>

      <Card
        title="Housing Costs"
        action={!editingCosts ? <Button size="sm" variant="secondary" onClick={startEditCosts}>Edit</Button> : undefined}
      >
        <div className="space-y-4">
          <p className="text-xs text-text-tertiary">
            What you pay on top of principal and interest. PMI stops automatically once the loan reaches 78% of the purchase price.
          </p>
          {editingCosts ? (
            <>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                <div>
                  <label className={labelCls}>Property Tax ($/yr)</label>
                  <input type="number" step="0.01" min={0} value={propertyTaxAnnual}
                    onChange={(e) => setPropertyTaxAnnual(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>Home Insurance ($/yr)</label>
                  <input type="number" step="0.01" min={0} value={homeInsuranceAnnual}
                    onChange={(e) => setHomeInsuranceAnnual(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>HOA Dues ($/mo)</label>
                  <input type="number" step="0.01" min={0} value={hoaMonthly}
                    onChange={(e) => setHoaMonthly(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>PMI ($/mo)</label>
                  <input type="number" step="0.01" min={0} value={pmiMonthly}
                    onChange={(e) => setPmiMonthly(e.target.value)} className={inputCls} />
                </div>
                <div>
                  <label className={labelCls}>Purchase Price ($)</label>
                  <input type="number" step="0.01" min={0} value={purchasePrice}
                    onChange={(e) => setPurchasePrice(e.target.value)} className={inputCls} />
                </div>
              </div>
              {error && <p className="text-negative text-sm">{error}</p>}
              <div className="flex gap-2">
                <Button size="sm" onClick={handleSaveCosts} loading={saving}>Save</Button>
                <Button size="sm" variant="secondary" onClick={() => setEditingCosts(false)}>Cancel</Button>
              </div>
            </>
          ) : (
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 text-sm">
              {([
                ['Property Tax', details?.propertyTaxAnnual, '/yr'],
                ['Home Insurance', details?.homeInsuranceAnnual, '/yr'],
                ['HOA Dues', details?.hoaMonthly, '/mo'],
                ['PMI', details?.pmiMonthly, '/mo'],
                ['Purchase Price', details?.purchasePrice, ''],
              ] as const).map(([label, value, unit]) => (
                <div key={label}>
                  <div className="text-xs text-text-tertiary mb-1">{label}</div>
                  <div className="text-text-primary font-medium">{value ? formatCurrency(value) + unit : '—'}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      </Card>
    </>
  )
}

// ─── Vehicle Detail Card ──────────────────────────────────────────────────────

function VehicleDetailCard({ accountId }: { accountId: string }) {
  const { data: details, mutate: mutateDetails } = useSWR<VehicleDetails>(
    `${accountId}/vehicle-details`,
    () => vehicleApi.getDetails(accountId),
  )
  const [editing, setEditing] = useState(false)
  const [year, setYear] = useState('')
  const [make, setMake] = useState('')
  const [model, setModel] = useState('')
  const [trim, setTrim] = useState('')
  const [value, setValue] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const startEdit = () => {
    setYear(details ? String(details.vehicleYear) : '')
    setMake(details?.vehicleMake ?? '')
    setModel(details?.vehicleModel ?? '')
    setTrim(details?.vehicleTrim ?? '')
    setValue(details?.lastValue != null ? String(details.lastValue) : '')
    setEditing(true)
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await vehicleApi.upsertDetails(accountId, {
        vehicleYear: parseInt(year),
        vehicleMake: make,
        vehicleModel: model,
        vehicleTrim: trim || undefined,
        value: value ? parseFloat(value) : undefined,
      })
      await mutate(`/accounts/${accountId}/snapshots`)
      await mutate('/networth/current')
      await mutateDetails()
      setEditing(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card
      title="Vehicle Details"
      action={!editing ? <Button size="sm" variant="secondary" onClick={startEdit}>Edit</Button> : undefined}
    >
      <div className="space-y-4">
        {editing ? (
          <>
            <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
              <div>
                <label className={labelCls}>Year</label>
                <input
                  type="number"
                  min={1900}
                  max={new Date().getFullYear() + 1}
                  value={year}
                  onChange={(e) => setYear(e.target.value)}
                  required
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Make</label>
                <input
                  value={make}
                  onChange={(e) => setMake(e.target.value)}
                  placeholder="Toyota"
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Model</label>
                <input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="Camry"
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Trim (optional)</label>
                <input
                  value={trim}
                  onChange={(e) => setTrim(e.target.value)}
                  placeholder="XLE"
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Estimated value</label>
                <input
                  type="number"
                  min={0}
                  step="any"
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  placeholder="22000"
                  className={inputCls}
                />
              </div>
            </div>
            {error && <p className="text-negative text-sm">{error}</p>}
            <div className="flex gap-2">
              <Button size="sm" onClick={handleSave} loading={saving}>Save</Button>
              <Button size="sm" variant="secondary" onClick={() => setEditing(false)}>Cancel</Button>
            </div>
          </>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 text-sm">
            <div>
              <div className="text-xs text-text-tertiary mb-1">Year</div>
              <div className="text-text-primary font-medium">{details?.vehicleYear ?? '—'}</div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Make</div>
              <div className="text-text-primary font-medium">{details?.vehicleMake || '—'}</div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Model</div>
              <div className="text-text-primary font-medium">{details?.vehicleModel || '—'}</div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Trim</div>
              <div className="text-text-primary font-medium">{details?.vehicleTrim || '—'}</div>
            </div>
          </div>
        )}

        {!editing && details?.lastValue != null && (
          <div className="text-sm text-text-secondary">
            Estimated value: {formatCurrency(details.lastValue)}
            {details.lastValueAt && (
              <span className="text-text-muted ml-1">({formatDate(details.lastValueAt)})</span>
            )}
          </div>
        )}

        {error && !editing && <p className="text-negative text-sm">{error}</p>}
      </div>
    </Card>
  )
}

function CreditCardDetailCard({ accountId, isPlaidLinked }: { accountId: string; isPlaidLinked: boolean }) {
  const { data: details, mutate: mutateDetails } = useSWR<CreditCardDetails>(
    `${accountId}/credit-card-details`,
    () => creditCardApi.getDetails(accountId),
  )
  const [editing, setEditing] = useState(false)
  const [aprPercentage, setAprPercentage] = useState('')
  const [creditLimit, setCreditLimit] = useState('')
  const [lastStatementBalance, setLastStatementBalance] = useState('')
  const [lastStatementIssueDate, setLastStatementIssueDate] = useState('')
  const [minimumPaymentAmount, setMinimumPaymentAmount] = useState('')
  const [nextPaymentDueDate, setNextPaymentDueDate] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const startEdit = () => {
    setAprPercentage(details?.aprPercentage != null ? String(details.aprPercentage) : '')
    setCreditLimit(details?.creditLimit != null ? String(details.creditLimit) : '')
    setLastStatementBalance(details?.lastStatementBalance != null ? String(details.lastStatementBalance) : '')
    setLastStatementIssueDate(toDateInputValue(details?.lastStatementIssueDate))
    setMinimumPaymentAmount(details?.minimumPaymentAmount != null ? String(details.minimumPaymentAmount) : '')
    setNextPaymentDueDate(toDateInputValue(details?.nextPaymentDueDate))
    setEditing(true)
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await creditCardApi.upsertDetails(accountId, {
        aprPercentage: aprPercentage ? parseFloat(aprPercentage) : undefined,
        creditLimit: creditLimit ? parseFloat(creditLimit) : undefined,
        lastStatementBalance: lastStatementBalance ? parseFloat(lastStatementBalance) : undefined,
        lastStatementIssueDate: lastStatementIssueDate || undefined,
        minimumPaymentAmount: minimumPaymentAmount ? parseFloat(minimumPaymentAmount) : undefined,
        nextPaymentDueDate: nextPaymentDueDate || undefined,
        // Not exposed in this form (Plaid-only fields) — upsertDetails always
        // replaces the full row, so these must be resent or they'd silently
        // reset to null/false on every manual save (see baseHomeFields above).
        // The server's date parser wants bare YYYY-MM-DD, not the RFC3339
        // string GET returns, hence toDateInputValue here too.
        lastPaymentAmount: details?.lastPaymentAmount,
        lastPaymentDate: toDateInputValue(details?.lastPaymentDate) || undefined,
        isOverdue: details?.isOverdue ?? false,
      })
      await mutateDetails()
      setEditing(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card
      title="Credit Card Details"
      action={
        !editing && !isPlaidLinked ? (
          <Button size="sm" variant="secondary" onClick={startEdit}>Edit</Button>
        ) : undefined
      }
    >
      <div className="space-y-4">
        {isPlaidLinked && (
          <p className="text-xs text-text-tertiary">
            Synced from Plaid — updates automatically, not editable here.
          </p>
        )}
        {editing ? (
          <>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
              <div>
                <label className={labelCls}>APR (%)</label>
                <input
                  type="number"
                  step="0.01"
                  value={aprPercentage}
                  onChange={(e) => setAprPercentage(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Credit limit</label>
                <input
                  type="number"
                  step="0.01"
                  value={creditLimit}
                  onChange={(e) => setCreditLimit(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Last statement balance</label>
                <input
                  type="number"
                  step="0.01"
                  value={lastStatementBalance}
                  onChange={(e) => setLastStatementBalance(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Statement issue date</label>
                <input
                  type="date"
                  value={lastStatementIssueDate}
                  onChange={(e) => setLastStatementIssueDate(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Minimum payment due</label>
                <input
                  type="number"
                  step="0.01"
                  value={minimumPaymentAmount}
                  onChange={(e) => setMinimumPaymentAmount(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Payment due date</label>
                <input
                  type="date"
                  value={nextPaymentDueDate}
                  onChange={(e) => setNextPaymentDueDate(e.target.value)}
                  className={inputCls}
                />
              </div>
            </div>
            {error && <p className="text-negative text-sm">{error}</p>}
            <div className="flex gap-2">
              <Button size="sm" onClick={handleSave} loading={saving}>Save</Button>
              <Button size="sm" variant="secondary" onClick={() => setEditing(false)}>Cancel</Button>
            </div>
          </>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 text-sm">
            <div>
              <div className="text-xs text-text-tertiary mb-1">APR</div>
              <div className="text-text-primary font-medium">
                {details?.aprPercentage != null ? `${details.aprPercentage}%` : '—'}
              </div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Credit limit</div>
              <div className="text-text-primary font-medium">
                {details?.creditLimit != null ? formatCurrency(details.creditLimit) : '—'}
              </div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Last statement balance</div>
              <div className="text-text-primary font-medium">
                {details?.lastStatementBalance != null ? formatCurrency(details.lastStatementBalance) : '—'}
                {details?.lastStatementIssueDate && (
                  <span className="text-text-muted ml-1">({formatDate(details.lastStatementIssueDate)})</span>
                )}
              </div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Minimum payment</div>
              <div className={`font-medium ${details?.isOverdue ? 'text-negative' : 'text-text-primary'}`}>
                {details?.minimumPaymentAmount != null ? formatCurrency(details.minimumPaymentAmount) : '—'}
                {details?.isOverdue && <span className="ml-1 text-xs">(overdue)</span>}
              </div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Payment due date</div>
              <div className="text-text-primary font-medium">
                {details?.nextPaymentDueDate ? formatDate(details.nextPaymentDueDate) : '—'}
              </div>
            </div>
          </div>
        )}
      </div>
    </Card>
  )
}

function LoanDetailCard({ accountId }: { accountId: string }) {
  const { data: details, mutate: mutateDetails } = useSWR<LoanDetails>(
    `${accountId}/loan-details`,
    () => loanApi.getDetails(accountId),
  )
  const [editing, setEditing] = useState(false)
  const [aprPercentage, setAprPercentage] = useState('')
  const [monthlyPayment, setMonthlyPayment] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const startEdit = () => {
    setAprPercentage(details?.aprPercentage != null ? String(details.aprPercentage) : '')
    setMonthlyPayment(details?.monthlyPayment != null ? String(details.monthlyPayment) : '')
    setEditing(true)
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await loanApi.upsertDetails(accountId, {
        aprPercentage: aprPercentage ? parseFloat(aprPercentage) : undefined,
        monthlyPayment: monthlyPayment ? parseFloat(monthlyPayment) : undefined,
      })
      await mutateDetails()
      await mutate('/forecast')
      setEditing(false)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card
      title="Loan Details"
      action={!editing ? <Button size="sm" variant="secondary" onClick={startEdit}>Edit</Button> : undefined}
    >
      <div className="space-y-4">
        <p className="text-xs text-text-tertiary">
          The{' '}
          <Link to="/forecast" className="text-accent-text hover:text-accent-text">
            forecast
          </Link>{' '}
          uses these to project when this loan is paid off. Without a monthly payment it stays at today's balance.
        </p>
        {editing ? (
          <>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className={labelCls}>Interest rate (APR %)</label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={aprPercentage}
                  onChange={(e) => setAprPercentage(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>Monthly payment</label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={monthlyPayment}
                  onChange={(e) => setMonthlyPayment(e.target.value)}
                  className={inputCls}
                />
              </div>
            </div>
            {error && <p className="text-negative text-sm">{error}</p>}
            <div className="flex gap-2">
              <Button size="sm" onClick={handleSave} loading={saving}>Save</Button>
              <Button size="sm" variant="secondary" onClick={() => setEditing(false)}>Cancel</Button>
            </div>
          </>
        ) : (
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <div className="text-xs text-text-tertiary mb-1">Interest rate</div>
              <div className="text-text-primary font-medium">
                {details?.aprPercentage != null ? `${details.aprPercentage}%` : '—'}
              </div>
            </div>
            <div>
              <div className="text-xs text-text-tertiary mb-1">Monthly payment</div>
              <div className="text-text-primary font-medium">
                {details?.monthlyPayment != null ? formatCurrency(details.monthlyPayment) : '—'}
              </div>
            </div>
          </div>
        )}
      </div>
    </Card>
  )
}

// ─── Account Return Rate Override ────────────────────────────────────────────

function AccountReturnRateOverride({
  accountId,
  currentOverride,
}: {
  accountId: string
  currentOverride?: number | null
}) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState('')
  const [saving, setSaving] = useState(false)
  const { data: effectiveRate } = useSWR<AccountEffectiveRate>(
    `${accountId}/effective-rate`,
    () => returnRatesApi.effectiveRate(accountId),
  )

  const handleSave = async () => {
    setSaving(true)
    try {
      const rate = value === '' ? null : parseFloat(value) / 100
      await returnRatesApi.setAccountOverride(accountId, rate)
      mutate(`/accounts/${accountId}`)
      mutate('/forecast')
      mutate(`${accountId}/effective-rate`)
      setEditing(false)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-3 bg-subtle/40 rounded-lg px-4 py-3 text-sm">
      <span className="text-text-secondary">Forecast return rate:</span>
      {editing ? (
        <>
          <input
            type="number"
            step="0.1"
            min={0}
            max={100}
            placeholder={currentOverride != null ? String(currentOverride * 100) : 'auto'}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="w-20 bg-subtle border border-border-strong rounded px-2 py-1 text-sm text-text-primary text-right focus:outline-none focus:ring-1 focus:ring-accent"
          />
          <span className="text-text-tertiary">%</span>
          <button
            onClick={handleSave}
            disabled={saving}
            className="text-xs text-positive hover:opacity-75 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            onClick={() => setEditing(false)}
            className="text-xs text-text-muted hover:text-text-secondary"
          >
            Cancel
          </button>
        </>
      ) : (
        <>
          {currentOverride != null ? (
            <span className="text-accent-text font-medium">
              {(currentOverride * 100).toFixed(1)}% (custom)
            </span>
          ) : (
            <span className="text-text-muted">
              {effectiveRate
                ? `auto (${(effectiveRate.rate * 100).toFixed(1)}% based on ${effectiveRate.assetClass ?? 'asset class'})`
                : 'auto (based on asset class)'}
            </span>
          )}
          <button
            onClick={() => {
              setValue(currentOverride != null ? String(currentOverride * 100) : '')
              setEditing(true)
            }}
            className="text-xs text-text-tertiary hover:text-accent-text ml-1"
          >
            {currentOverride != null ? 'Change' : 'Override'}
          </button>
          {currentOverride != null && (
            <button
              onClick={async () => {
                await returnRatesApi.setAccountOverride(accountId, null)
                mutate(`/accounts/${accountId}`)
                mutate('/forecast')
              }}
              className="text-xs text-text-muted hover:text-negative"
            >
              Clear
            </button>
          )}
        </>
      )}
    </div>
  )
}

// ─── Forms ───────────────────────────────────────────────────────────────────

function SnapshotForm({
  accountType,
  initial,
  onSubmit,
  onCancel,
}: {
  accountType: string
  initial?: BalanceSnapshot
  onSubmit: (d: CreateSnapshotInput) => Promise<void>
  onCancel: () => void
}) {
  const isAutomobile = accountType === 'automobile'
  const [date, setDate] = useState(initial ? toDateInputValue(initial.snapshotDate) : today())
  const [balance, setBalance] = useState(initial ? String(initial.balance) : '')
  const [miles, setMiles] = useState(initial?.miles != null ? String(initial.miles) : '')
  const [notes, setNotes] = useState(initial?.notes ?? '')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onSubmit({
        snapshotDate: date,
        balance: parseFloat(balance),
        miles: isAutomobile && miles ? parseInt(miles, 10) : undefined,
        notes: notes || undefined,
      })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div>
        <label className={labelCls}>Date</label>
        <input
          type="date"
          value={date}
          onChange={(e) => setDate(e.target.value)}
          required
          className={inputCls}
        />
      </div>
      <div>
        <label className={labelCls}>{isAutomobile ? 'Value' : 'Balance'}</label>
        <input
          type="number"
          step="0.01"
          value={balance}
          onChange={(e) => setBalance(e.target.value)}
          required
          placeholder={isAutomobile ? '0.00' : '0.00 (negative for liabilities)'}
          className={inputCls}
        />
      </div>
      {isAutomobile && (
        <div>
          <label className={labelCls}>Miles (optional)</label>
          <input
            type="number"
            step="1"
            min="0"
            value={miles}
            onChange={(e) => setMiles(e.target.value)}
            placeholder="e.g. 45000"
            className={inputCls}
          />
        </div>
      )}
      <div>
        <label className={labelCls}>Notes (optional)</label>
        <input
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          className={inputCls}
        />
      </div>
      <div className="flex justify-end gap-3 pt-2">
        <Button variant="secondary" onClick={onCancel} type="button">Cancel</Button>
        <Button type="submit" loading={loading}>{isAutomobile ? 'Save Entry' : 'Save Snapshot'}</Button>
      </div>
    </form>
  )
}

function HoldingForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (d: CreateHoldingInput) => Promise<void>
  onCancel: () => void
}) {
  const [ticker, setTicker] = useState('')
  const [shares, setShares] = useState('')
  const [amount, setAmount] = useState('')
  const [costBasis, setCostBasis] = useState('')
  const [assetClass, setAssetClass] = useState('equity')
  const [manualPrice, setManualPrice] = useState(false)
  const [pricePerShare, setPricePerShare] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const isCash = assetClass === 'cash_equivalent'

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      if (isCash) {
        await onSubmit({
          ticker: 'CASH',
          shares: parseFloat(amount),
          assetClass: 'cash_equivalent',
        })
      } else {
        await onSubmit({
          ticker: ticker.toUpperCase(),
          shares: parseFloat(shares),
          costBasis: costBasis ? parseFloat(costBasis) : undefined,
          assetClass,
          manualPrice: manualPrice || undefined,
          pricePerShare: manualPrice && pricePerShare ? parseFloat(pricePerShare) : undefined,
        })
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div>
        <label className={labelCls}>Asset Class</label>
        <select
          value={assetClass}
          onChange={(e) => setAssetClass(e.target.value)}
          className={inputCls}
        >
          {ASSET_CLASS_OPTIONS.map((c) => (
            <option key={c.value} value={c.value}>{c.label}</option>
          ))}
        </select>
      </div>
      {isCash ? (
        <div>
          <label className={labelCls}>Amount ($) *</label>
          <input
            type="number"
            step="any"
            min="0"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            required
            placeholder="0.00"
            className={inputCls}
          />
          <p className="text-xs text-text-tertiary mt-1">
            Track uninvested cash (e.g. a money market fund) within this account.
          </p>
        </div>
      ) : (
        <>
          <div>
            <label className={labelCls}>Ticker Symbol *</label>
            <input
              value={ticker}
              onChange={(e) => setTicker(e.target.value)}
              required
              placeholder="e.g. AAPL, VTI, BTC"
              className={`${inputCls} uppercase`}
            />
          </div>
          <div>
            <label className={labelCls}>Shares *</label>
            <input
              type="number"
              step="any"
              value={shares}
              onChange={(e) => setShares(e.target.value)}
              required
              min="0"
              className={inputCls}
            />
          </div>
          <div>
            <label className={labelCls}>Cost Basis per Share (optional)</label>
            <input
              type="number"
              step="any"
              value={costBasis}
              onChange={(e) => setCostBasis(e.target.value)}
              placeholder="0.00"
              className={inputCls}
            />
          </div>
          <div className="flex items-center gap-3">
            <input
              id="add-manual-price"
              type="checkbox"
              checked={manualPrice}
              onChange={(e) => setManualPrice(e.target.checked)}
              className="w-4 h-4 accent-accent"
            />
            <label htmlFor="add-manual-price" className="text-sm text-text-secondary cursor-pointer">
              Manual price (skip auto-sync)
            </label>
          </div>
          {manualPrice && (
            <div>
              <label className={labelCls}>Unit Price ($) *</label>
              <input
                type="number"
                step="any"
                min="0"
                required={manualPrice}
                value={pricePerShare}
                onChange={(e) => setPricePerShare(e.target.value)}
                placeholder="0.00"
                className={inputCls}
              />
              <p className="text-xs text-text-tertiary mt-1">
                Price won&apos;t be updated automatically. Useful for 401k funds not listed on public exchanges.
              </p>
            </div>
          )}
        </>
      )}
      <div className="flex justify-end gap-3 pt-2">
        <Button variant="secondary" onClick={onCancel} type="button">Cancel</Button>
        <Button type="submit" loading={loading}>Add Holding</Button>
      </div>
    </form>
  )
}

type TierDraft = { matchPct: string; upToPct: string }

const pctInput = (fraction: number) => String(+(fraction * 100).toFixed(4))

function ContributionForm({
  initial,
  onSubmit,
  onCancel,
  showActive = false,
  initialActive = true,
  account,
}: {
  initial?: Contribution
  onSubmit: (d: CreateContributionInput) => Promise<void>
  onCancel: () => void
  showActive?: boolean
  initialActive?: boolean
  account: Account
}) {
  const { data: members } = useHouseholdMembers()
  const [label, setLabel] = useState(initial?.label ?? '')
  const [amountType, setAmountType] = useState<ContributionAmountType>(initial?.amountType ?? 'fixed')
  const [amount, setAmount] = useState(String(initial?.amount ?? ''))
  const [incomePct, setIncomePct] = useState(initial?.amountType === 'percent' ? pctInput(initial.incomePct) : '')
  const [frequency, setFrequency] = useState<'monthly' | 'annually' | 'biweekly'>(
    initial?.frequency ?? 'monthly',
  )
  const [nextPayDate, setNextPayDate] = useState(toDateInputValue(initial?.nextPayDate))
  const [startDate, setStartDate] = useState(toDateInputValue(initial?.startDate))
  const [endDate, setEndDate] = useState(toDateInputValue(initial?.endDate))
  const [annualIncreasePct, setAnnualIncreasePct] = useState(
    String(initial ? initial.annualIncreasePct * 100 : '0'),
  )
  const [tiers, setTiers] = useState<TierDraft[]>(
    (initial?.matchTiers ?? []).map((t) => ({
      matchPct: pctInput(t.matchPct),
      upToPct: t.upToPct == null ? '' : pctInput(t.upToPct),
    })),
  )
  const [isActive, setIsActive] = useState(initialActive)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const supportsMatch = EMPLOYER_MATCH_ACCOUNT_TYPES.has(account.accountType)
  const isPercent = amountType === 'percent'
  const matchTiers: MatchTier[] = supportsMatch
    ? tiers
        .filter((t) => t.matchPct !== '')
        .map((t) => ({
          matchPct: parseFloat(t.matchPct) / 100,
          upToPct: t.upToPct === '' ? null : parseFloat(t.upToPct) / 100,
        }))
    : []
  const draft = {
    amountType,
    amount: isPercent ? 0 : parseFloat(amount) || 0,
    frequency: isPercent ? ('monthly' as const) : frequency,
    incomePct: isPercent ? (parseFloat(incomePct) || 0) / 100 : 0,
    matchTiers,
  }

  // A percent contribution or capped match needs the owner's salary; the API
  // rejects it otherwise, so say why up front and block saving.
  const owner = accountOwner(account, members ?? [])
  const salary = annualSalary(owner)
  const whose = owner?.relationship === 'partner' && owner.name ? `${owner.name}'s` : 'your'
  const incomeProblem = !members || !needsIncome(draft)
    ? ''
    : account.isJoint
      ? "Joint accounts don't have one salary to base this on. Assign the account to one person in its settings to use a percent of income or a capped match."
      : salary <= 0
        ? `Add ${whose} annual income in Settings to use a percent of income or a capped match.`
        : ''
  const preview = !incomeProblem && (salary > 0 || !needsIncome(draft)) ? annualContribution(draft, salary) : null

  const updateTier = (i: number, patch: Partial<TierDraft>) =>
    setTiers((prev) => prev.map((t, j) => (j === i ? { ...t, ...patch } : t)))

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (incomeProblem) return
    setError('')
    setLoading(true)
    try {
      await onSubmit({
        label: label || undefined,
        ...draft,
        annualIncreasePct: isPercent ? 0 : parseFloat(annualIncreasePct) / 100,
        nextPayDate: !isPercent && frequency === 'biweekly' ? nextPayDate : undefined,
        startDate: startDate || undefined,
        endDate: endDate || undefined,
        ...(showActive ? { isActive } : {}),
      } as CreateContributionInput)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}

      <div>
        <label className={labelCls}>Label (optional)</label>
        <input
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder="e.g. My 401k contributions"
          className={inputCls}
        />
      </div>

      <div className="flex rounded-lg border border-border-default text-sm" role="group" aria-label="Contribution type">
        {([['fixed', 'Dollar amount'], ['percent', '% of income']] as const).map(([value, text]) => (
          <button
            key={value}
            type="button"
            onClick={() => setAmountType(value)}
            aria-pressed={amountType === value}
            className={`flex-1 px-3 py-1.5 first:rounded-l-lg last:rounded-r-lg transition-colors ${
              amountType === value ? 'bg-accent-tint-bg text-accent-text' : 'text-text-tertiary hover:text-text-primary'
            }`}
          >
            {text}
          </button>
        ))}
      </div>

      {isPercent ? (
        <div>
          <label className={labelCls}>Percent of Salary (%)</label>
          <input
            type="number"
            step="any"
            min={0}
            max={100}
            value={incomePct}
            onChange={(e) => setIncomePct(e.target.value)}
            required
            placeholder="6"
            className={inputCls}
          />
          <p className="text-xs text-text-muted mt-1">
            Grows with {whose} salary, using the salary growth set in Settings.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className={labelCls}>Amount ($)</label>
            <input
              type="number"
              step="any"
              min={0}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              required
              placeholder="500"
              className={inputCls}
            />
          </div>
          <div>
            <label className={labelCls}>Frequency</label>
            <select
              value={frequency}
              onChange={(e) => setFrequency(e.target.value as 'monthly' | 'annually' | 'biweekly')}
              className={inputCls}
            >
              <option value="monthly">Monthly</option>
              <option value="biweekly">Bi-weekly</option>
              <option value="annually">Annually</option>
            </select>
          </div>
        </div>
      )}

      {!isPercent && frequency === 'biweekly' && (
        <div>
          <label className={labelCls}>Next Pay Date</label>
          <input
            type="date"
            value={nextPayDate}
            onChange={(e) => setNextPayDate(e.target.value)}
            required
            className={inputCls}
          />
          <p className="text-xs text-text-muted mt-1">
            The upcoming pay date that anchors this bi-weekly schedule (26 payments/year).
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <label className={labelCls}>Start Date (optional)</label>
          <input
            type="date"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
            className={inputCls}
          />
        </div>
        <div>
          <label className={labelCls}>End Date (optional)</label>
          <input
            type="date"
            value={endDate}
            onChange={(e) => setEndDate(e.target.value)}
            className={inputCls}
          />
        </div>
      </div>
      <p className="text-xs text-text-muted -mt-2">
        Leave blank to contribute from the projection start / until retirement.
      </p>

      {!isPercent && (
        <div>
          <label className={labelCls}>Annual Contribution Increase (%)</label>
          <input
            type="number"
            step="0.1"
            min={0}
            max={50}
            value={annualIncreasePct}
            onChange={(e) => setAnnualIncreasePct(e.target.value)}
            placeholder="0"
            className={inputCls}
          />
          <p className="text-xs text-text-muted mt-1">
            Grows your contribution amount each year.
          </p>
        </div>
      )}

      {supportsMatch && (
        <div>
          <label className={labelCls}>Employer Match</label>
          <div className="space-y-2">
            {tiers.map((t, i) => (
              <div key={i} className="flex flex-wrap items-center gap-2 text-sm text-text-secondary">
                <div className="w-20">
                  <input
                    type="number"
                    step="any"
                    min={0}
                    max={200}
                    value={t.matchPct}
                    onChange={(e) => updateTier(i, { matchPct: e.target.value })}
                    aria-label={`Tier ${i + 1} match percent`}
                    placeholder="100"
                    className={inputCls}
                  />
                </div>
                <span>% of the {i === 0 ? 'first' : 'next'}</span>
                <div className="w-24">
                  <input
                    type="number"
                    step="any"
                    min={0}
                    max={100}
                    value={t.upToPct}
                    onChange={(e) => updateTier(i, { upToPct: e.target.value })}
                    aria-label={`Tier ${i + 1} salary cap percent`}
                    placeholder={i === tiers.length - 1 ? 'no cap' : '3'}
                    className={inputCls}
                  />
                </div>
                <span>% of salary</span>
                <button
                  type="button"
                  onClick={() => setTiers((prev) => prev.filter((_, j) => j !== i))}
                  className="text-xs text-text-muted hover:text-negative"
                >
                  Remove
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => setTiers((prev) => [...prev, { matchPct: '', upToPct: '' }])}
              className="text-xs text-accent-text hover:text-accent-text"
            >
              + Add match tier
            </button>
          </div>
          <p className="text-xs text-text-muted mt-1">
            e.g. 100% of the first 3%, then 50% of the next 2%. Leave the last cap blank to match everything you put in.
          </p>
        </div>
      )}

      {incomeProblem ? (
        <div className="text-sm rounded-lg bg-warning-tint-bg text-text-primary px-3 py-2">
          {incomeProblem}{' '}
          {!account.isJoint && (
            <Link to="/settings" className="text-accent-text underline underline-offset-2">Go to Settings</Link>
          )}
        </div>
      ) : preview && (preview.employee > 0 || preview.match > 0) && (
        <p className="text-xs text-text-tertiary">
          About {formatCurrency(preview.employee)}/yr from you
          {preview.match > 0 && <> plus {formatCurrency(preview.match)}/yr from your employer</>}
          {needsIncome(draft) && <> at today's salary of {formatCurrency(salary)}</>}.
        </p>
      )}

      {showActive && (
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={isActive}
            onChange={(e) => setIsActive(e.target.checked)}
            className="w-4 h-4 accent-accent"
          />
          <span className="text-sm text-text-primary">Active (include in forecast)</span>
        </label>
      )}

      <div className="flex justify-end gap-3 pt-2">
        <Button variant="secondary" onClick={onCancel} type="button">Cancel</Button>
        <Button type="submit" loading={loading} disabled={!!incomeProblem}>
          {initial ? 'Save Changes' : 'Add Contribution'}
        </Button>
      </div>
    </form>
  )
}
