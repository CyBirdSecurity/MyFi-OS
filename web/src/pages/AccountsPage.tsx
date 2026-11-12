import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { mutate } from 'swr'
import { Check, X } from '@phosphor-icons/react'
import { useAccounts } from '../hooks/useAccounts'
import { useNetWorthCurrent } from '../hooks/useNetWorth'
import { useHousehold } from '../hooks/useHousehold'
import { useAuthStore } from '../store/authStore'
import {
  accountsApi,
  homeApi,
  vehicleApi,
  snapshotsApi,
  plaidApi,
  ACCOUNT_TYPE_LABELS,
  ACCOUNT_TYPE_DEFAULTS,
  LIABILITY_ACCOUNT_TYPES,
  FREE_CHOICE_TAX_ACCOUNT_TYPES,
  CASH_BALANCE_ACCOUNT_TYPES,
  NO_HOLDINGS_ACCOUNT_TYPES,
  EMPLOYER_MATCH_ACCOUNT_TYPES,
  contributionsApi,
  acceptsContributions,
} from '../api/client'
import type { CreateAccountInput, Account } from '../api/client'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Modal } from '../components/ui/Modal'
import { TaxBadge } from '../components/ui/Badge'
import { InstitutionCombobox } from '../components/ui/InstitutionCombobox'
import { PlaidLinkFlow } from '../components/plaid/PlaidLinkFlow'
import { PlaidLogo } from '../components/plaid/PlaidLogo'
import { plaidSyncProblem } from '../utils/plaidStatus'
import { usePlaidConfig, usePlaidItems, mutateAfterPlaidSync } from '../hooks/usePlaid'
import { formatCurrency } from '../utils/currency'
import { today } from '../utils/dates'
import { PageGuide } from '../components/PageGuide'

const inputCls =
  'w-full bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

// ─── Ownership tag ────────────────────────────────────────────────────────────

type OwnerTag = 'self' | 'partner' | 'joint'

function ownerTag(account: Account): OwnerTag {
  if (account.isJoint) return 'joint'
  return account.isPartnerAccount ? 'partner' : 'self'
}

function OwnershipBadge({ tag, name }: { tag: OwnerTag; name: string }) {
  if (tag === 'joint') {
    return (
      <span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full bg-subtle text-text-secondary border border-border-default">
        <span className="w-1.5 h-1.5 rounded-full bg-text-tertiary shrink-0" />
        {name}
      </span>
    )
  }
  if (tag === 'partner') {
    return (
      <span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full bg-warning-tint-bg text-warning border border-warning/20">
        <span className="w-1.5 h-1.5 rounded-full bg-warning shrink-0" />
        {name}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full bg-accent-tint-bg text-accent-text border border-accent-tint-border">
      <span className="w-1.5 h-1.5 rounded-full bg-accent shrink-0" />
      {name}
    </span>
  )
}

// ─── Account form ─────────────────────────────────────────────────────────────

/** What the form collects beyond the account itself. */
export type AccountExtras = {
  address?: string
  vehicleYear?: number
  vehicleMake?: string
  vehicleModel?: string
  vehicleTrim?: string
  openingBalance?: number
  /** Saved as a monthly contribution so the forecast counts it. */
  monthlyContribution?: number
  employerMatchPct?: number
}

/** Creates an account with its opening balance and home/vehicle details. Also used by onboarding. */
export async function createAccount(data: CreateAccountInput, extra: AccountExtras) {
  const account = await accountsApi.create(data)
  if (extra.openingBalance != null && Number.isFinite(extra.openingBalance)) {
    await snapshotsApi.create(account.id, { snapshotDate: today(), balance: extra.openingBalance })
  }
  if (extra.monthlyContribution && extra.monthlyContribution > 0) {
    await contributionsApi.create(account.id, {
      amount: extra.monthlyContribution,
      frequency: 'monthly',
      annualIncreasePct: 0,
      amountType: 'fixed',
      incomePct: 0,
      matchTiers: extra.employerMatchPct ? [{ matchPct: extra.employerMatchPct, upToPct: null }] : [],
    })
    await Promise.all([mutate('/contributions'), mutate('/forecast')])
  }
  if (data.accountType === 'home' && extra.address) {
    await homeApi.upsertDetails(account.id, { address: extra.address, interestRate: 0, loanTermYears: 30, originalLoanValue: 0, currentLoanBalance: 0 })
  }
  if (data.accountType === 'automobile' && extra.vehicleMake) {
    await vehicleApi.upsertDetails(account.id, {
      vehicleYear: extra.vehicleYear,
      vehicleMake: extra.vehicleMake,
      vehicleModel: extra.vehicleModel,
      vehicleTrim: extra.vehicleTrim,
    })
  }
  await mutate('/accounts')
  // Net worth holds the per-account balances the account list shows (and edits inline).
  await mutate('/networth/current')
}

export function AccountForm({
  partnerName,
  partnerMemberId,
  selfName,
  onSubmit,
  onCancel,
  cancelLabel = 'Cancel',
  submitLabel = 'Create Account',
}: {
  partnerName?: string
  partnerMemberId?: string
  selfName?: string
  onSubmit: (
    data: CreateAccountInput,
    extra: AccountExtras,
  ) => Promise<void>
  onCancel: () => void
  cancelLabel?: string
  submitLabel?: string
}) {
  const [name, setName] = useState('')
  const [accountType, setAccountType] = useState('checking')
  const [taxTreatment, setTaxTreatment] = useState<string>('taxable')
  const [institution, setInstitution] = useState('')
  const [owner, setOwner] = useState<'self' | 'partner' | 'joint'>('self')
  const [address, setAddress] = useState('')
  const [vehicleYear, setVehicleYear] = useState(new Date().getFullYear())
  const [vehicleMake, setVehicleMake] = useState('')
  const [vehicleModel, setVehicleModel] = useState('')
  const [vehicleTrim, setVehicleTrim] = useState('')
  const [balance, setBalance] = useState('')
  const [contribution, setContribution] = useState('')
  const [match, setMatch] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleTypeChange = (t: string) => {
    setAccountType(t)
    setTaxTreatment(ACCOUNT_TYPE_DEFAULTS[t] ?? 'taxable')
  }

  const isLiability = LIABILITY_ACCOUNT_TYPES.has(accountType)
  // A home or car is valued from its details instead.
  const hasBalanceField = accountType !== 'home' && accountType !== 'automobile'

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onSubmit(
        {
          name,
          accountType,
          taxTreatment,
          institution: institution || undefined,
          memberId: owner === 'partner' ? partnerMemberId : undefined,
          isJoint: owner === 'joint' || undefined,
        },
        {
          address: accountType === 'home' ? address : undefined,
          vehicleYear: accountType === 'automobile' ? vehicleYear : undefined,
          vehicleMake: accountType === 'automobile' ? vehicleMake : undefined,
          vehicleModel: accountType === 'automobile' ? vehicleModel : undefined,
          vehicleTrim: accountType === 'automobile' && vehicleTrim ? vehicleTrim : undefined,
          // Liabilities are stored as negative balances; people enter what they owe.
          openingBalance: hasBalanceField && balance.trim() !== ''
            ? Math.abs(parseFloat(balance)) * (isLiability ? -1 : 1)
            : undefined,
          monthlyContribution: acceptsContributions(accountType) ? parseFloat(contribution) || undefined : undefined,
          employerMatchPct: EMPLOYER_MATCH_ACCOUNT_TYPES.has(accountType) && match ? parseFloat(match) / 100 : undefined,
        },
      )
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const isFreeChoice = FREE_CHOICE_TAX_ACCOUNT_TYPES.has(accountType)

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="text-negative text-sm bg-negative-tint-bg rounded px-3 py-2">{error}</div>
      )}

      {partnerName && (
        <div>
          <label className={labelCls}>Owner</label>
          <div className="flex gap-2">
            {(
              [
                { value: 'self',    label: selfName ?? 'Myself', activeClass: 'bg-accent-tint-bg border-accent text-accent-text' },
                { value: 'partner', label: partnerName,          activeClass: 'bg-warning-tint-bg border-warning text-warning' },
                { value: 'joint',   label: 'Joint',              activeClass: 'bg-subtle border-border-strong text-text-primary' },
              ] as const
            ).map(({ value, label, activeClass }) => (
              <button
                key={value}
                type="button"
                onClick={() => setOwner(value)}
                className={`flex-1 text-xs px-3 py-2 rounded-lg border transition-colors ${
                  owner === value
                    ? activeClass
                    : 'border-border-default text-text-tertiary hover:text-text-primary hover:border-border-strong'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
          {owner === 'partner' && (
            <p className="text-xs text-text-tertiary mt-1.5">
              This account will appear under {partnerName}&rsquo;s name.
            </p>
          )}
          {owner === 'joint' && (
            <p className="text-xs text-text-tertiary mt-1.5">
              This account will be shared by you and {partnerName}.
            </p>
          )}
        </div>
      )}

      <div>
        <label className={labelCls}>Account Type</label>
        <select
          value={accountType}
          onChange={(e) => handleTypeChange(e.target.value)}
          className={inputCls}
        >
          <optgroup label="Assets">
            {Object.entries(ACCOUNT_TYPE_LABELS)
              .filter(([v]) => !LIABILITY_ACCOUNT_TYPES.has(v))
              .map(([value, label]) => (
                <option key={value} value={value}>{label}</option>
              ))}
          </optgroup>
          <optgroup label="Liabilities">
            {Object.entries(ACCOUNT_TYPE_LABELS)
              .filter(([v]) => LIABILITY_ACCOUNT_TYPES.has(v))
              .map(([value, label]) => (
                <option key={value} value={value}>{label}</option>
              ))}
          </optgroup>
        </select>
      </div>
      <div>
        <label className={labelCls}>Account Name</label>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          placeholder="e.g. Fidelity 401k"
          className={inputCls}
        />
      </div>

      {!isLiability && (isFreeChoice || !(accountType in ACCOUNT_TYPE_DEFAULTS)) && (
        <div>
          <label className={labelCls}>Tax Treatment</label>
          <select
            value={taxTreatment}
            onChange={(e) => setTaxTreatment(e.target.value)}
            className={inputCls}
          >
            <option value="taxable">Taxable</option>
            <option value="tax_free">Tax-Free</option>
          </select>
        </div>
      )}

      {accountType === 'home' && (
        <div>
          <label className={labelCls}>Property Address</label>
          <input
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="e.g. 123 Main St, Springfield, IL 62701"
            className={inputCls}
          />
        </div>
      )}

      {accountType === 'automobile' && (
        <>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelCls}>Year</label>
              <input
                type="number"
                min={1900}
                max={new Date().getFullYear() + 1}
                value={vehicleYear}
                onChange={(e) => setVehicleYear(Number(e.target.value))}
                required
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Make</label>
              <input
                value={vehicleMake}
                onChange={(e) => setVehicleMake(e.target.value)}
                placeholder="e.g. Toyota"
                required
                className={inputCls}
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelCls}>Model</label>
              <input
                value={vehicleModel}
                onChange={(e) => setVehicleModel(e.target.value)}
                placeholder="e.g. Camry"
                required
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Trim (optional)</label>
              <input
                value={vehicleTrim}
                onChange={(e) => setVehicleTrim(e.target.value)}
                placeholder="e.g. XLE"
                className={inputCls}
              />
            </div>
          </div>
        </>
      )}

      {hasBalanceField && (
        <div>
          <label className={labelCls}>{isLiability ? 'Amount owed' : 'Current balance'}</label>
          <input
            type="number"
            step="0.01"
            min={0}
            value={balance}
            onChange={(e) => setBalance(e.target.value)}
            placeholder="e.g. 2500"
            className={inputCls}
          />
          <p className="text-xs text-text-tertiary mt-1.5">
            {NO_HOLDINGS_ACCOUNT_TYPES.has(accountType) || CASH_BALANCE_ACCOUNT_TYPES.has(accountType) || isLiability
              ? 'Optional. You can update it anytime.'
              : 'Optional. Add individual holdings later for live prices.'}
          </p>
        </div>
      )}

      {acceptsContributions(accountType) && (
        <div className={EMPLOYER_MATCH_ACCOUNT_TYPES.has(accountType) ? 'grid grid-cols-2 gap-3' : ''}>
          <div>
            <label className={labelCls}>You add each month</label>
            <input
              type="number"
              step="1"
              min={0}
              value={contribution}
              onChange={(e) => setContribution(e.target.value)}
              placeholder="e.g. 500"
              className={inputCls}
            />
          </div>
          {EMPLOYER_MATCH_ACCOUNT_TYPES.has(accountType) && (
            <div>
              <label className={labelCls}>Employer match (%)</label>
              <input
                type="number"
                step="1"
                min={0}
                max={200}
                value={match}
                onChange={(e) => setMatch(e.target.value)}
                placeholder="e.g. 50"
                className={inputCls}
              />
            </div>
          )}
          <p className={`text-xs text-text-tertiary mt-1.5 ${EMPLOYER_MATCH_ACCOUNT_TYPES.has(accountType) ? 'col-span-2' : ''}`}>
            Optional, but it's what makes your forecast grow.
            {EMPLOYER_MATCH_ACCOUNT_TYPES.has(accountType) && ' Match is a % of what you put in (100 = dollar for dollar).'}
          </p>
        </div>
      )}

      {accountType !== 'automobile' && (
        <div>
          <label className={labelCls}>Institution (optional)</label>
          <InstitutionCombobox value={institution} onChange={setInstitution} />
        </div>
      )}
      <div className="flex justify-end gap-3 pt-2">
        <Button variant="secondary" onClick={onCancel} type="button">
          {cancelLabel}
        </Button>
        <Button type="submit" loading={loading}>
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

type FilterTag = 'all' | 'self' | 'partner'

export function AccountsPage() {
  const { data: accounts, isLoading } = useAccounts()
  const { data: nw } = useNetWorthCurrent()
  const { data: members } = useHousehold()
  const user = useAuthStore((s) => s.user)
  const { data: appConfig } = usePlaidConfig()
  const plaidEnabled = appConfig?.plaidEnabled ?? false
  const { data: plaidItems } = usePlaidItems(plaidEnabled)
  const hasPlaidItems = (plaidItems?.length ?? 0) > 0
  const [showCreate, setShowCreate] = useState(false)
  // ?connect=1 (from onboarding's first look) opens the Plaid flow straight away.
  const [searchParams] = useSearchParams()
  const [showPlaidLink, setShowPlaidLink] = useState(searchParams.get('connect') === '1')
  const [syncingAll, setSyncingAll] = useState(false)
  const [syncError, setSyncError] = useState('')
  const [filter, setFilter] = useState<FilterTag>('all')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [savingId, setSavingId] = useState<string | null>(null)

  const selfMember = (members ?? []).find((m) => m.relationship === 'self')
  const selfName = selfMember?.name || user?.displayName || 'Me'
  const partnerMember = (members ?? []).find((m) => m.relationship === 'partner')
  const partnerName = partnerMember?.name || (accounts ?? []).find((a) => a.isPartnerAccount)?.ownerName
  const partnerMemberId = partnerMember?.id

  const inHousehold = !!partnerMember

  const balanceMap = new Map(nw?.accounts.map((a) => [a.accountId, a.displayBalance ?? a.balance]) ?? [])

  const visibleAccounts = (accounts ?? []).filter((acc) => {
    if (filter === 'all') return true
    return ownerTag(acc) === filter
  })

  // Grouped by what the money is for, in ACCOUNT_GROUPS order; empty groups are skipped.
  const groups = ACCOUNT_GROUPS.map((g) => {
    const list = visibleAccounts
      .filter((acc) => accountGroupOf(acc.accountType) === g.key)
      .sort((a, b) => Math.abs(balanceMap.get(b.id) ?? 0) - Math.abs(balanceMap.get(a.id) ?? 0))
    return { ...g, accounts: list, total: list.reduce((sum, acc) => sum + (balanceMap.get(acc.id) ?? 0), 0) }
  }).filter((g) => g.accounts.length > 0)

  const all = accounts ?? []
  const counts = {
    self:    all.filter((a) => ownerTag(a) === 'self').length,
    partner: all.filter((a) => ownerTag(a) === 'partner').length,
  }

  const navigate = useNavigate()

  const handleCreate = async (
    data: CreateAccountInput,
    extra: AccountExtras,
  ) => {
    await createAccount(data, extra)
    setShowCreate(false)
  }

  const handleSyncAll = async () => {
    setSyncingAll(true)
    setSyncError('')
    try {
      const res = await plaidApi.syncAll()
      if (res.errors && res.errors.length > 0) setSyncError(res.errors.join('; '))
      await mutateAfterPlaidSync()
    } catch (e: any) {
      setSyncError(e.message ?? 'Sync failed.')
    } finally {
      setSyncingAll(false)
    }
  }

  const startEditBalance = (accountId: string, balance: number) => {
    setEditingId(accountId)
    setEditValue(String(balance))
  }

  const handleSaveBalance = async (e: React.FormEvent, accountId: string) => {
    e.preventDefault()
    const trimmed = editValue.trim()
    if (!/^-?\d+(\.\d+)?$/.test(trimmed)) return
    const value = parseFloat(trimmed)
    if (!Number.isFinite(value)) return
    setSavingId(accountId)
    try {
      await snapshotsApi.create(accountId, { snapshotDate: today(), balance: value })
      await mutate('/networth/current')
      setEditingId((cur) => (cur === accountId ? null : cur))
    } finally {
      setSavingId(null)
    }
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text-primary">Accounts</h1>
            <PageGuide id="accounts" />
          </div>
          <p className="text-text-tertiary text-sm mt-1">{all.length} accounts</p>
        </div>
        <div className="flex gap-2">
          {plaidEnabled && hasPlaidItems && (
            <Button variant="secondary" onClick={handleSyncAll} loading={syncingAll}>
              Sync Now
            </Button>
          )}
          {plaidEnabled && (
            <Button variant="secondary" onClick={() => setShowPlaidLink(true)}>
              Connect Institution
            </Button>
          )}
          <Button onClick={() => setShowCreate(true)}>+ Add Account</Button>
        </div>
      </div>

      {syncError && (
        <div className="text-negative text-sm bg-negative-tint-bg rounded px-3 py-2">{syncError}</div>
      )}

      {inHousehold && all.length > 0 && (
        <div className="flex flex-wrap gap-2">
          <FilterButton active={filter === 'all'} onClick={() => setFilter('all')}>
            All <span className="ml-1 opacity-60">{all.length}</span>
          </FilterButton>
          <FilterButton active={filter === 'self'} onClick={() => setFilter('self')} color="brand">
            <span className="w-1.5 h-1.5 rounded-full bg-accent shrink-0" />
            {selfName} <span className="ml-1 opacity-60">{counts.self}</span>
          </FilterButton>
          {counts.partner > 0 && (
            <FilterButton active={filter === 'partner'} onClick={() => setFilter('partner')} color="warning">
              <span className="w-1.5 h-1.5 rounded-full bg-warning shrink-0" />
              {partnerName} <span className="ml-1 opacity-60">{counts.partner}</span>
            </FilterButton>
          )}
        </div>
      )}

      {isLoading ? (
        <div className="text-text-tertiary text-sm">Loading...</div>
      ) : !all.length ? (
        <Card>
          <div className="text-center py-8 text-text-tertiary">
            <p>No accounts yet.</p>
            <button
              onClick={() => setShowCreate(true)}
              className="text-accent-text hover:text-accent mt-2 text-sm"
            >
              Add your first account
            </button>
          </div>
        </Card>
      ) : visibleAccounts.length === 0 ? (
        <div className="text-text-tertiary text-sm py-4">No accounts match this filter.</div>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-6 items-start">
          {groups.map((group) => {
            return (
              <div key={group.key}>
                <div className="flex items-baseline justify-between mb-2 px-1">
                  <h2 className="text-xs font-semibold text-text-tertiary uppercase tracking-wider">{group.label}</h2>
                  <span className="text-sm font-medium text-text-secondary">{formatCurrency(group.total)}</span>
                </div>
                <div className="space-y-2">
                  {group.accounts.map((acc) => {
                    const balance = balanceMap.get(acc.id)
                    const isLiability = LIABILITY_ACCOUNT_TYPES.has(acc.accountType)
                    const isEditableCash = CASH_BALANCE_ACCOUNT_TYPES.has(acc.accountType)
                    const tag = ownerTag(acc)
                    const ownerDisplayName = tag === 'self' ? selfName : acc.ownerName

                    return (
                      <div
                        key={acc.id}
                        // The whole row opens the account; the balance editor
                        // and links inside it handle their own clicks.
                        onClick={(e) => {
                          if (!(e.target as HTMLElement).closest('a, button, input, form')) navigate(`/accounts/${acc.id}`)
                        }}
                        className="bg-surface border border-border-default rounded-lg px-5 py-4 flex items-center gap-4 hover:border-border-strong hover:bg-subtle/40 cursor-pointer transition-colors"
                      >
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <Link
                              to={`/accounts/${acc.id}`}
                              className="font-medium text-text-primary hover:text-accent-text transition-colors"
                            >
                              {acc.name}
                            </Link>
                            {acc.externalProvider === 'plaid' && (() => {
                              // The mark turns red when this connection's sync is failing.
                              const problem = plaidSyncProblem(plaidItems?.find((i) => i.itemId === acc.externalItemId))
                              return (
                                <span
                                  title={
                                    problem ??
                                    (acc.lastSyncedAt
                                      ? `Synced via Plaid — ${new Date(acc.lastSyncedAt).toLocaleDateString()}`
                                      : 'Synced via Plaid')
                                  }
                                  className={`inline-flex items-center justify-center shrink-0 ${
                                    problem ? 'text-negative' : 'text-accent-text'
                                  }`}
                                >
                                  <PlaidLogo className="w-3.5 h-3.5" />
                                </span>
                              )
                            })()}
                            {inHousehold && (
                              <OwnershipBadge tag={tag} name={ownerDisplayName} />
                            )}
                          </div>
                          <div className="flex items-center gap-2 mt-1">
                            {!isLiability && (
                              <TaxBadge treatment={acc.taxTreatment} />
                            )}
                            <span className="text-xs text-text-tertiary truncate">
                              {[ACCOUNT_TYPE_LABELS[acc.accountType] ?? acc.accountType, acc.institution].filter(Boolean).join(' · ')}
                            </span>
                          </div>
                        </div>
                        <div className="flex items-center gap-4 shrink-0">
                          {isEditableCash && editingId === acc.id ? (
                            <form
                              onSubmit={(e) => handleSaveBalance(e, acc.id)}
                              className="flex items-center gap-1"
                            >
                              <input
                                type="number"
                                step="0.01"
                                autoFocus
                                value={editValue}
                                onChange={(e) => setEditValue(e.target.value)}
                                className="w-28 bg-surface border border-border-strong rounded-lg px-2 py-1 text-sm text-right text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
                              />
                              <button
                                type="submit"
                                disabled={savingId === acc.id}
                                className="p-1.5 text-positive hover:bg-positive-tint-bg rounded disabled:opacity-40"
                                aria-label="Save balance"
                              >
                                <Check size={16} weight="bold" />
                              </button>
                              <button
                                type="button"
                                onClick={() => setEditingId(null)}
                                className="p-1.5 text-text-tertiary hover:bg-subtle rounded"
                                aria-label="Cancel"
                              >
                                <X size={16} weight="bold" />
                              </button>
                            </form>
                          ) : (
                            balance != null && (
                              isEditableCash ? (
                                <button
                                  type="button"
                                  onClick={() => startEditBalance(acc.id, balance)}
                                  title="Update balance"
                                  className="text-sm font-medium text-text-primary underline decoration-dotted decoration-border-strong underline-offset-4 hover:decoration-accent"
                                >
                                  {formatCurrency(balance)}
                                </button>
                              ) : (
                                <span className="text-sm font-medium text-text-primary">
                                  {formatCurrency(balance)}
                                </span>
                              )
                            )
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      )}

      <Modal open={showCreate} onClose={() => setShowCreate(false)} title="Add Account">
        <AccountForm
          partnerName={partnerName}
          partnerMemberId={partnerMemberId}
          selfName={selfName}
          onSubmit={handleCreate}
          onCancel={() => setShowCreate(false)}
        />
      </Modal>

      {plaidEnabled && (
        <PlaidLinkFlow
          open={showPlaidLink}
          onClose={() => setShowPlaidLink(false)}
          accounts={all}
        />
      )}
    </div>
  )
}

// ─── Filter button ────────────────────────────────────────────────────────────

function FilterButton({
  active,
  onClick,
  color,
  children,
}: {
  active: boolean
  onClick: () => void
  color?: 'brand' | 'warning' | 'positive'
  children: React.ReactNode
}) {
  const activeClass =
    color === 'warning'
      ? 'bg-warning-tint-bg border-warning/40 text-warning'
      : color === 'positive'
        ? 'bg-positive-tint-bg border-positive/40 text-positive'
        : color === 'brand'
          ? 'bg-accent-tint-bg border-accent/40 text-accent-text'
          : 'bg-subtle border-border-strong text-text-primary'

  return (
    <button
      onClick={onClick}
      className={`inline-flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-full border transition-colors ${
        active ? activeClass : 'border-border-default text-text-tertiary hover:text-text-primary hover:border-border-strong'
      }`}
    >
      {children}
    </button>
  )
}

// ─── Account groups ───────────────────────────────────────────────────────────

const ACCOUNT_GROUPS = [
  { key: 'cash', label: 'Cash' },
  { key: 'investments', label: 'Investments' },
  { key: 'retirement', label: 'Retirement' },
  { key: 'property', label: 'Property' },
  { key: 'debt', label: 'Debt' },
] as const

type AccountGroupKey = (typeof ACCOUNT_GROUPS)[number]['key']

function accountGroupOf(accountType: string): AccountGroupKey {
  if (LIABILITY_ACCOUNT_TYPES.has(accountType) || accountType === 'mortgage') return 'debt'
  if (accountType === 'checking' || accountType === 'savings' || accountType === 'money_market') return 'cash'
  if (accountType === 'brokerage' || accountType === 'crypto' || accountType === '529') return 'investments'
  if (accountType === 'home' || accountType === 'automobile' || accountType === 'other_asset') return 'property'
  return 'retirement'
}
