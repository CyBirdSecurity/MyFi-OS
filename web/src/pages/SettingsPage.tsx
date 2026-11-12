import { useState, useEffect } from 'react'
import { mutate } from 'swr'
import { parseISO } from 'date-fns'
import { Sun, Moon } from '@phosphor-icons/react'
import { authApi, profileApi, returnRatesApi, householdMembersApi, snapshotsApi, plaidApi, type HouseholdMember, type UpsertMemberInput } from '../api/client'
import { STATE_TAX_RATES } from '../data/stateTaxRates'
import { useProfile } from '../hooks/useProfile'
import { useReturnRates } from '../hooks/useReturnRates'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'
import { usePlaidConfig, usePlaidItems, mutateAfterPlaidSync } from '../hooks/usePlaid'
import { useAuthStore } from '../store/authStore'
import { useThemeStore } from '../store/themeStore'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { AllocationTargetsForm } from '../components/SetupForms'
import { InfoTip } from '../components/ui/InfoTip'

const ASSET_CLASS_LABELS: Record<string, string> = {
  equity: 'Equity',
  bond: 'Bond / Fixed Income',
  crypto: 'Cryptocurrency',
  cash_equivalent: 'Cash',
  other: 'Other',
}

const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

// Currency input: shows $X,XXX.XX when blurred, raw number when focused.
function CurrencyInput({
  value,
  onChange,
  className,
  placeholder = '$0.00',
}: {
  value: string
  onChange: (rawNumericString: string) => void
  className?: string
  placeholder?: string
}) {
  const [focused, setFocused] = useState(false)
  const n = parseFloat(value) || 0
  const display = focused || !n
    ? (value === '0' ? '' : value)
    : '$' + n.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

  return (
    <input
      type="text"
      inputMode="decimal"
      value={display}
      placeholder={placeholder}
      onFocus={() => setFocused(true)}
      onChange={(e) => onChange(e.target.value.replace(/[^0-9.]/g, ''))}
      onBlur={(e) => {
        setFocused(false)
        const stripped = e.target.value.replace(/[^0-9.]/g, '')
        const rounded = Math.round(parseFloat(stripped) * 100) / 100
        onChange(isNaN(rounded) ? '0' : String(rounded))
      }}
      className={className}
    />
  )
}

export function SettingsPage() {
  return (
    <div className="space-y-8 max-w-6xl">
      <div>
        <h1 className="text-2xl font-bold text-text-primary">Settings</h1>
        <p className="text-sm text-text-tertiary mt-1">
          Configure your profile and forecast assumptions.
        </p>
      </div>
      {/* Two balanced columns on wide screens: CSS columns split the cards by
          height, so the layout evens out however tall each one is. Cards are
          kept whole (break-inside-avoid) and read top-to-bottom, left column
          first. */}
      <div className="xl:columns-2 xl:gap-8 [&>*]:mb-8 [&>*]:break-inside-avoid">
        <ProfileSection />
        <HouseholdSection />
        <AppearanceSection />
        <PasswordSection />
        <SnapshotSection />
        <PlaidConnectionsSection />
        <ReturnRatesSection />
        <AllocationTargetsSection />
      </div>
    </div>
  )
}

// ─── Appearance Section ───────────────────────────────────────────────────────

function AppearanceSection() {
  const { theme, setTheme } = useThemeStore()
  return (
    <Card title="Appearance">
      <div className="flex items-center gap-3">
        <span className="text-sm text-text-secondary">Theme</span>
        <div className="inline-flex rounded-lg border border-border-default bg-subtle p-0.5">
          <button
            type="button"
            onClick={() => setTheme('light')}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              theme === 'light'
                ? 'bg-accent-tint-bg text-accent-text'
                : 'text-text-tertiary hover:text-text-primary'
            }`}
          >
            <Sun size={16} weight="bold" /> Light
          </button>
          <button
            type="button"
            onClick={() => setTheme('dark')}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              theme === 'dark'
                ? 'bg-accent-tint-bg text-accent-text'
                : 'text-text-tertiary hover:text-text-primary'
            }`}
          >
            <Moon size={16} weight="bold" /> Dark
          </button>
        </div>
      </div>
    </Card>
  )
}

// ─── Password Section ────────────────────────────────────────────────────────

function PasswordSection() {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (newPassword !== confirmPassword) {
      setError('The passwords do not match.')
      return
    }
    setSaving(true)
    setError('')
    setDone(false)
    try {
      await authApi.changePassword(currentPassword, newPassword)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setDone(true)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card title="Password">
      {done && (
        <div className="text-positive text-sm mb-3">
          Password changed. Any other devices have been signed out.
        </div>
      )}
      {error && <div className="text-negative text-sm mb-3">{error}</div>}
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className={labelCls}>Current password</label>
          <input
            className={inputCls}
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            required
          />
        </div>
        <div>
          <label className={labelCls}>New password</label>
          <input
            className={inputCls}
            type="password"
            autoComplete="new-password"
            minLength={8}
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            required
          />
        </div>
        <div>
          <label className={labelCls}>Confirm new password</label>
          <input
            className={inputCls}
            type="password"
            autoComplete="new-password"
            minLength={8}
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            required
          />
        </div>
        <div className="flex justify-end">
          <Button type="submit" loading={saving}>
            Change password
          </Button>
        </div>
      </form>
    </Card>
  )
}

// ─── Snapshot Section ────────────────────────────────────────────────────────

function SnapshotSection() {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState('')
  const [error, setError] = useState('')

  const handleTrigger = async () => {
    setLoading(true)
    setError('')
    setResult('')
    try {
      const { accountsUpdated } = await snapshotsApi.triggerNow()
      mutate('/networth/current')
      mutate('/networth/history')
      setResult(
        accountsUpdated > 0
          ? `Updated ${accountsUpdated} account${accountsUpdated === 1 ? '' : 's'}.`
          : 'No investment accounts with holdings to snapshot.',
      )
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title="Snapshots">
      <p className="text-xs text-text-tertiary mb-4">
        Investment account balances normally snapshot automatically once a week.
        If you've just updated your holdings and don't want to wait, snapshot
        them now.
      </p>
      {error && <div className="text-negative text-sm mb-3">{error}</div>}
      <div className="flex items-center gap-4">
        <Button variant="secondary" onClick={handleTrigger} loading={loading}>
          Update Snapshots Now
        </Button>
        {result && <span className="text-sm text-positive">{result}</span>}
      </div>
    </Card>
  )
}

// ─── Plaid Connections Section ───────────────────────────────────────────────

function PlaidConnectionsSection() {
  const { data: config } = usePlaidConfig()
  const { data: items } = usePlaidItems(!!config?.plaidEnabled)
  const [removing, setRemoving] = useState<string | null>(null)
  const [error, setError] = useState('')

  if (!config?.plaidEnabled || !items || items.length === 0) return null

  const handleRemove = async (itemId: string, name: string, linkedCount: number) => {
    const msg = linkedCount > 0
      ? `Disconnect ${name}? Its ${linkedCount} linked account${linkedCount === 1 ? '' : 's'} will revert to manual tracking with all data kept.`
      : `Remove the unused ${name} connection?`
    if (!confirm(msg)) return
    setRemoving(itemId)
    setError('')
    try {
      await plaidApi.removeItem(itemId)
      await mutateAfterPlaidSync()
    } catch (err: any) {
      setError(err.message ?? 'Failed to remove.')
    } finally {
      setRemoving(null)
    }
  }

  return (
    <Card title="Plaid Connections">
      <p className="text-xs text-text-tertiary mb-4">
        Each bank login is one connection, and counts toward your Plaid plan&rsquo;s connection limit.
        Remove connections you no longer use to free up room. Connections with no linked accounts are
        cleaned up automatically after a couple of hours.
      </p>
      {error && <div className="text-negative text-sm mb-3">{error}</div>}
      <ul className="divide-y divide-border-default">
        {items.map((item) => {
          const linked = item.accounts ?? []
          const name = item.institutionName ?? 'Bank'
          return (
            <li key={item.itemId} className="flex items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <div className="text-sm text-text-primary truncate">
                  {name}
                  {item.status !== 'active' && (
                    <span className="ml-2 text-xs text-warning">
                      {item.status === 'login_required' ? 'needs reconnecting' : 'error'}
                    </span>
                  )}
                </div>
                <div className="text-xs text-text-tertiary truncate">
                  {linked.length > 0 ? linked.map((a) => a.name).join(', ') : 'No linked accounts'}
                </div>
              </div>
              <Button
                variant="secondary"
                loading={removing === item.itemId}
                onClick={() => handleRemove(item.itemId, name, linked.length)}
              >
                Remove
              </Button>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

// ─── Profile Section ─────────────────────────────────────────────────────────

function ProfileSection() {
  const { data: profile } = useProfile()
  const [useInflation, setUseInflation] = useState(false)
  const [inflationRate, setInflationRate] = useState('3')
  const [monthlyExpenses, setMonthlyExpenses] = useState('0')
  const [currentMonthlyExpenses, setCurrentMonthlyExpenses] = useState('0')
  const [useBudgetForExpenses, setUseBudgetForExpenses] = useState(false)
  const [withdrawalRate, setWithdrawalRate] = useState('')
  const [filingStatus, setFilingStatus] = useState('single')
  const [selectedState, setSelectedState] = useState('')
  const [stateTaxRate, setStateTaxRate] = useState('')
  const [dependentsCount, setDependentsCount] = useState('0')
  const [expFreq, setExpFreq] = useState<'monthly' | 'annual'>('monthly')
  const [loading, setLoading] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!profile) return
    setUseInflation(profile.useInflationAdjustment)
    setInflationRate(String(Math.round(profile.inflationRate * 100)))
    setMonthlyExpenses(String(profile.monthlyExpenses ?? 0))
    setCurrentMonthlyExpenses(String(profile.currentMonthlyExpenses ?? 0))
    setUseBudgetForExpenses(profile.useBudgetForExpenses ?? false)
    setWithdrawalRate(profile.withdrawalRate != null ? String(Math.round(profile.withdrawalRate * 1000) / 10) : '')
    setFilingStatus(profile.filingStatus ?? 'single')
    if (profile.stateTaxRate != null) {
      setStateTaxRate(String(Math.round(profile.stateTaxRate * 1000) / 10))
    }
    if (profile.stateTaxCode) {
      setSelectedState(profile.stateTaxCode)
    }
    setDependentsCount(String(profile.dependentsCount ?? 0))
  }, [profile])

  const handleStateChange = (code: string) => {
    setSelectedState(code)
    if (code) {
      const s = STATE_TAX_RATES.find((s) => s.code === code)
      if (s) setStateTaxRate(String(Math.round(s.rate * 1000) / 10))
    }
  }

  const toDisplay = (monthly: string) => {
    const v = parseFloat(monthly) || 0
    return expFreq === 'annual' ? String(Math.round(v * 12)) : monthly
  }
  const fromDisplay = (display: string) => {
    const v = parseFloat(display) || 0
    return expFreq === 'annual' ? String(v / 12) : display
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await profileApi.upsert({
        useInflationAdjustment: useInflation,
        inflationRate: parseFloat(inflationRate) / 100,
        monthlyExpenses: parseFloat(monthlyExpenses) || 0,
        currentMonthlyExpenses: parseFloat(currentMonthlyExpenses) || 0,
        useBudgetForExpenses,
        withdrawalRate: withdrawalRate !== '' ? parseFloat(withdrawalRate) / 100 : null,
        filingStatus,
        stateTaxRate: stateTaxRate !== '' ? parseFloat(stateTaxRate) / 100 : null,
        stateTaxCode: selectedState || null,
        dependentsCount: parseInt(dependentsCount, 10) || 0,
      })
      mutate('/profile')
      mutate('/forecast')
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title="Profile">
      <form onSubmit={handleSubmit} className="space-y-5">
        {error && <div className="text-negative text-sm">{error}</div>}

        {/* Inflation */}
        <div className="space-y-3">
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={useInflation}
              onChange={(e) => setUseInflation(e.target.checked)}
              className="w-4 h-4 accent-accent"
            />
            <span className="text-sm text-text-primary">
              Show inflation-adjusted returns (real dollars)
            </span>
          </label>
          {useInflation && (
            <div>
              <label className={labelCls}>Annual Inflation Rate (%)</label>
              <input
                type="number"
                min={0}
                max={20}
                step="0.1"
                value={inflationRate}
                onChange={(e) => setInflationRate(e.target.value)}
                className={inputCls}
              />
            </div>
          )}
        </div>

        {/* Safe Withdrawal Rate */}
        <div>
          <label className={labelCls}>Safe Withdrawal Rate (%) <InfoTip term="withdrawalRate" /></label>
          <input
            type="number"
            min={1}
            max={10}
            step="0.1"
            placeholder="4"
            value={withdrawalRate}
            onChange={(e) => setWithdrawalRate(e.target.value)}
            className={inputCls}
          />
          <p className="text-xs text-text-muted mt-1">
            Used to calculate your FI Number. Leave blank for the standard 4% rule.
          </p>
        </div>

        {/* Expenses */}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm text-text-secondary">Expenses in Retirement</span>
            <div className="flex items-center gap-1 bg-subtle rounded-lg p-0.5">
              {(['monthly', 'annual'] as const).map((f) => (
                <button
                  key={f}
                  type="button"
                  onClick={() => setExpFreq(f)}
                  className={`px-2.5 py-1 text-xs rounded-md transition-colors ${
                    expFreq === f
                      ? 'bg-muted text-text-primary'
                      : 'text-text-tertiary hover:text-text-primary'
                  }`}
                >
                  {f === 'monthly' ? 'Monthly' : 'Annual'}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className={labelCls}>
              {expFreq === 'monthly' ? 'Monthly' : 'Annual'} Expenses in Retirement
            </label>
            <CurrencyInput
              value={toDisplay(monthlyExpenses)}
              onChange={(raw) => setMonthlyExpenses(fromDisplay(raw))}
              className={inputCls}
            />
          </div>
          <p className="text-xs text-text-muted">
            Used to calculate your FI Number and Coast FI, and as your assumed retirement spending
            on the Forecast and Retirement pages.
          </p>
          <label className="flex items-start gap-3 cursor-pointer select-none">
            <div className="relative mt-0.5">
              <input
                type="checkbox"
                className="sr-only"
                checked={useBudgetForExpenses}
                onChange={(e) => setUseBudgetForExpenses(e.target.checked)}
              />
              <div className={`w-9 h-5 rounded-full transition-colors ${useBudgetForExpenses ? 'bg-accent' : 'bg-muted'}`} />
              <div className={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform ${useBudgetForExpenses ? 'translate-x-4' : 'translate-x-0'}`} />
            </div>
            <div>
              <p className="text-sm text-text-primary">Use budget data for expenses</p>
              <p className="text-xs text-text-muted mt-0.5">
                Uses your 3-month average from imported transactions instead of the value above.
              </p>
            </div>
          </label>
          <div>
            <label className={labelCls}>Current Monthly Expenses</label>
            <CurrencyInput
              value={currentMonthlyExpenses}
              onChange={setCurrentMonthlyExpenses}
              className={inputCls}
            />
            <p className="text-xs text-text-muted mt-1">
              What you actually spend today — used only for your savings rate. Doesn't affect your
              FI Number or retirement spending.
            </p>
          </div>
        </div>

        {/* Tax Profile */}
        <div className="space-y-3">
          <span className="text-sm text-text-secondary">Tax Profile</span>
          <p className="text-xs text-text-muted">
            Used by the Taxes page for bracket analysis, Roth conversion recommendations, and ACA subsidy tracking.
          </p>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label className={labelCls}>Filing Status</label>
              <select
                value={filingStatus}
                onChange={(e) => setFilingStatus(e.target.value)}
                className={inputCls}
              >
                <option value="single">Single</option>
                <option value="married_filing_jointly">Married Filing Jointly</option>
              </select>
            </div>
            <div>
              <label className={labelCls}>State</label>
              <select
                value={selectedState}
                onChange={(e) => handleStateChange(e.target.value)}
                className={inputCls}
              >
                <option value="">Select state…</option>
                {STATE_TAX_RATES.map((s) => (
                  <option key={s.code} value={s.code}>{s.name}</option>
                ))}
              </select>
              {(selectedState || stateTaxRate) && (
                <p className="text-xs text-text-muted mt-1">
                  Top marginal rate:{' '}
                  <input
                    type="number"
                    min={0}
                    max={20}
                    step="0.1"
                    value={stateTaxRate}
                    onChange={(e) => { setStateTaxRate(e.target.value); setSelectedState('') }}
                    className="inline-block w-16 bg-subtle border border-border-strong rounded px-1 py-0.5 text-xs text-text-primary text-right focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                  % — edit to override
                </p>
              )}
            </div>
            <div>
              <label className={labelCls}>Additional Dependents</label>
              <input
                type="number"
                min={0}
                max={10}
                step="1"
                value={dependentsCount}
                onChange={(e) => setDependentsCount(e.target.value)}
                className={inputCls}
              />
              <p className="text-xs text-text-muted mt-1">Children or other dependents beyond household members.</p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-4 pt-1">
          <Button type="submit" loading={loading}>
            Save Profile
          </Button>
          {saved && <span className="text-sm text-positive">Saved!</span>}
        </div>
      </form>
    </Card>
  )
}

// ─── Return Rates Section ─────────────────────────────────────────────────────

function ReturnRatesSection() {
  const { data: rates } = useReturnRates()
  const [editing, setEditing] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState<string | null>(null)

  const handleEdit = (assetClass: string, current: number) => {
    setEditing((prev) => ({
      ...prev,
      [assetClass]: String(Math.round(current * 100 * 10) / 10),
    }))
  }

  const handleSave = async (assetClass: string) => {
    setSaving(assetClass)
    try {
      const pct = parseFloat(editing[assetClass] ?? '0')
      await returnRatesApi.upsert(assetClass, pct / 100)
      mutate('/return-rates')
      mutate('/forecast')
      setEditing((prev) => {
        const next = { ...prev }
        delete next[assetClass]
        return next
      })
    } finally {
      setSaving(null)
    }
  }

  const handleReset = async (assetClass: string) => {
    setSaving(assetClass)
    try {
      await returnRatesApi.delete(assetClass)
      mutate('/return-rates')
      mutate('/forecast')
    } finally {
      setSaving(null)
    }
  }

  return (
    <Card title="Return Rate Assumptions">
      <p className="text-xs text-text-tertiary mb-4">
        Customize the expected annual return used for each asset class.
        System defaults are based on long-term historical averages.
        Enter a custom rate to override (e.g., <strong className="text-text-secondary">10</strong> for 10%).
      </p>
      <div className="overflow-x-auto">
      <table className="w-full text-sm min-w-[380px]">
        <thead>
          <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
            <th className="pb-2 font-medium">Asset Class</th>
            <th className="pb-2 font-medium text-right">System Default</th>
            <th className="pb-2 font-medium text-right">Your Rate</th>
            <th className="pb-2"></th>
          </tr>
        </thead>
        <tbody>
          {rates?.map((r) => {
            const editVal = editing[r.assetClass]
            const hasCustom = r.customRate != null

            return (
              <tr key={r.assetClass} className="border-b border-border-default/50">
                <td className="py-3 text-text-primary">
                  {ASSET_CLASS_LABELS[r.assetClass] ?? r.assetClass}
                </td>
                <td className="py-3 text-text-tertiary text-right">
                  {(r.defaultRate * 100).toFixed(1)}%
                </td>
                <td className="py-3 text-right">
                  {editVal !== undefined ? (
                    <input
                      type="number"
                      step="0.1"
                      min={0}
                      max={100}
                      value={editVal}
                      onChange={(e) =>
                        setEditing((prev) => ({
                          ...prev,
                          [r.assetClass]: e.target.value,
                        }))
                      }
                      className="w-20 bg-subtle border border-border-strong rounded px-2 py-1 text-sm text-text-primary text-right focus:outline-none focus:ring-1 focus:ring-accent"
                    />
                  ) : (
                    <span className={hasCustom ? 'text-accent-text font-medium' : 'text-text-muted'}>
                      {hasCustom
                        ? `${(r.customRate! * 100).toFixed(1)}%`
                        : '—'}
                    </span>
                  )}
                </td>
                <td className="py-3 text-right">
                  <div className="flex items-center justify-end gap-2">
                    {editVal !== undefined ? (
                      <>
                        <button
                          onClick={() => handleSave(r.assetClass)}
                          disabled={saving === r.assetClass}
                          className="text-xs text-positive hover:opacity-75 disabled:opacity-50"
                        >
                          {saving === r.assetClass ? 'Saving…' : 'Save'}
                        </button>
                        <button
                          onClick={() =>
                            setEditing((prev) => {
                              const next = { ...prev }
                              delete next[r.assetClass]
                              return next
                            })
                          }
                          className="text-xs text-text-muted hover:text-text-secondary"
                        >
                          Cancel
                        </button>
                      </>
                    ) : (
                      <>
                        <button
                          onClick={() => handleEdit(r.assetClass, r.effectiveRate)}
                          className="text-xs text-text-tertiary hover:text-accent-text"
                        >
                          Edit
                        </button>
                        {hasCustom && (
                          <button
                            onClick={() => handleReset(r.assetClass)}
                            disabled={saving === r.assetClass}
                            className="text-xs text-text-muted hover:text-negative disabled:opacity-50"
                          >
                            Reset
                          </button>
                        )}
                      </>
                    )}
                  </div>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      </div>
    </Card>
  )
}

// ─── Allocation Targets Section ───────────────────────────────────────────────

function AllocationTargetsSection() {
  const [saved, setSaved] = useState(false)
  return (
    <Card title="Asset Allocation Targets">
      <p className="text-xs text-text-tertiary mb-4">
        Set target percentages for each asset class. The Holdings page will show how far you are from each target.
        Leave all at 0 to skip.
      </p>
      <AllocationTargetsForm onSaved={() => { setSaved(true); setTimeout(() => setSaved(false), 2500) }} />
      {saved && <p className="text-sm text-positive mt-3">Saved!</p>}
    </Card>
  )
}

// ─── Household Members Section ────────────────────────────────────────────────

function MemberForm({
  member,
  onSave,
  onCancel,
  isNew,
  relationship,
}: {
  member?: HouseholdMember
  onSave: (data: Partial<UpsertMemberInput>) => Promise<void>
  onCancel: () => void
  isNew?: boolean
  relationship: 'self' | 'partner'
}) {
  const [name, setName] = useState(member?.name ?? '')
  const [birthday, setBirthday] = useState(member?.birthday?.slice(0, 10) ?? '')
  const [isEmployed, setIsEmployed] = useState(member?.isEmployed ?? true)
  const [monthlyIncome, setMonthlyIncome] = useState(String(member?.monthlyIncome ?? 0))
  const [salaryGrowthPct, setSalaryGrowthPct] = useState(String(+((member?.salaryGrowthPct ?? 0) * 100).toFixed(2)))
  const [retirementAge, setRetirementAge] = useState(String(member?.retirementAge ?? 65))
  const [useSS, setUseSS] = useState(member?.useSocialSecurity ?? false)
  const [ssMonthly, setSsMonthly] = useState(String(member?.socialSecurityMonthly ?? 0))
  const [ssStartAge, setSsStartAge] = useState(String(member?.socialSecurityStartAge ?? 67))
  const [incomeFreq, setIncomeFreq] = useState<'monthly' | 'annual'>('monthly')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const toDisplay = (monthly: string) => {
    const v = parseFloat(monthly) || 0
    return incomeFreq === 'annual' ? String(Math.round(v * 12)) : monthly
  }
  const fromDisplay = (display: string) => {
    const v = parseFloat(display) || 0
    return incomeFreq === 'annual' ? String(v / 12) : display
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onSave({
        relationship,
        name,
        birthday: birthday || undefined,
        isEmployed,
        monthlyIncome: parseFloat(monthlyIncome) || 0,
        salaryGrowthPct: (parseFloat(salaryGrowthPct) || 0) / 100,
        retirementAge: parseInt(retirementAge, 10) || 65,
        useSocialSecurity: useSS,
        socialSecurityMonthly: parseFloat(ssMonthly) || 0,
        socialSecurityStartAge: parseInt(ssStartAge, 10) || 67,
      })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4 pt-2">
      {error && <div className="text-negative text-sm">{error}</div>}

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label className={labelCls}>{relationship === 'self' ? 'Your Name' : "Partner's Name"}</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder={relationship === 'self' ? 'Your name' : "Partner's name"}
            className={inputCls}
          />
        </div>
        <div>
          <label className={labelCls}>Date of Birth</label>
          <input
            type="date"
            value={birthday}
            onChange={(e) => setBirthday(e.target.value)}
            className={inputCls}
          />
        </div>
      </div>

      <div className="space-y-3">
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={isEmployed}
            onChange={(e) => setIsEmployed(e.target.checked)}
            className="w-4 h-4 accent-accent"
          />
          <span className="text-sm text-text-primary">Currently employed</span>
        </label>
        {isEmployed && (
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs text-text-tertiary">Income</span>
              <div className="flex items-center gap-1 bg-subtle rounded-lg p-0.5">
                {(['monthly', 'annual'] as const).map((f) => (
                  <button
                    key={f}
                    type="button"
                    onClick={() => setIncomeFreq(f)}
                    className={`px-2 py-0.5 text-xs rounded-md transition-colors ${incomeFreq === f ? 'bg-muted text-text-primary' : 'text-text-tertiary hover:text-text-primary'}`}
                  >
                    {f === 'monthly' ? 'Monthly' : 'Annual'}
                  </button>
                ))}
              </div>
            </div>
            <CurrencyInput
              value={toDisplay(monthlyIncome)}
              onChange={(raw) => setMonthlyIncome(fromDisplay(raw))}
              className={inputCls}
            />
            <div>
              <label className={labelCls}>Yearly Salary Growth (%)</label>
              <input
                type="number"
                step="0.1"
                min={0}
                max={50}
                value={salaryGrowthPct}
                onChange={(e) => setSalaryGrowthPct(e.target.value)}
                className={inputCls}
              />
              <p className="text-xs text-text-muted mt-1">
                Expected raise each year. Grows percent-of-income contributions and employer match caps in the forecast.
              </p>
            </div>
          </div>
        )}
      </div>

      <div>
        <label className={labelCls}>Target Retirement Age</label>
        <input
          type="number"
          min={18}
          max={99}
          value={retirementAge}
          onChange={(e) => setRetirementAge(e.target.value)}
          className={inputCls}
        />
      </div>

      <div className="space-y-3">
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={useSS}
            onChange={(e) => setUseSS(e.target.checked)}
            className="w-4 h-4 accent-accent"
          />
          <span className="text-sm text-text-primary">Plan for Social Security income</span>
        </label>
        {useSS && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pl-1">
            <div>
              <label className={labelCls}>Monthly Benefit</label>
              <CurrencyInput
                value={ssMonthly}
                onChange={setSsMonthly}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Start Age</label>
              <input
                type="number"
                min={62}
                max={70}
                value={ssStartAge}
                onChange={(e) => setSsStartAge(e.target.value)}
                className={inputCls}
              />
            </div>
          </div>
        )}
      </div>

      <div className="flex gap-3 pt-1">
        <Button type="submit" loading={loading} size="sm">
          {isNew ? 'Add Member' : 'Save Changes'}
        </Button>
        <Button type="button" variant="secondary" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}

function HouseholdSection() {
  const { data: members, mutate: mutateMembers } = useHouseholdMembers()
  const [editingId, setEditingId] = useState<string | null>(null)
  const [addingPartner, setAddingPartner] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  const selfMember = members?.find((m) => m.relationship === 'self')
  const partnerMember = members?.find((m) => m.relationship === 'partner')

  const handleSave = async (id: string | undefined, relationship: 'self' | 'partner', data: Partial<UpsertMemberInput>) => {
    if (id) {
      await householdMembersApi.update(id, data)
    } else {
      await householdMembersApi.upsert({ ...data as UpsertMemberInput, relationship })
    }
    await mutateMembers()
    mutate('/profile')
    mutate('/forecast')
    setEditingId(null)
    setAddingPartner(false)
  }

  const handleDeletePartner = async () => {
    if (!partnerMember) return
    if (!confirm('Remove your partner from the household? Their accounts will remain but will no longer be attributed to them.')) return
    setDeletingId(partnerMember.id)
    try {
      await householdMembersApi.delete(partnerMember.id)
      await mutateMembers()
      mutate('/forecast')
    } finally {
      setDeletingId(null)
    }
  }

  const formatMemberSummary = (m: HouseholdMember) => {
    const parts: string[] = []
    if (m.birthday) {
      const today = new Date()
      const dob = parseISO(m.birthday)
      let age = today.getFullYear() - dob.getFullYear()
      if (today.getMonth() < dob.getMonth() ||
          (today.getMonth() === dob.getMonth() && today.getDate() < dob.getDate())) {
        age--
      }
      parts.push(`Age ${age}`)
    }
    if (m.isEmployed) parts.push(`$${Math.round(m.monthlyIncome).toLocaleString()}/mo income`)
    parts.push(`Retires at ${m.retirementAge}`)
    return parts.join(' · ')
  }

  return (
    <Card title="Household Members">
      <p className="text-xs text-text-tertiary mb-4">
        Household members power the forecast. Income, retirement ages, and Social Security are tracked per person.
      </p>

      <div className="space-y-4">
        {/* Self member */}
        <div className="rounded-lg border border-border-default p-4">
          <div className="flex items-center justify-between mb-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-text-primary">{selfMember?.name || 'You'}</span>
              <span className="text-xs px-1.5 py-0.5 rounded bg-accent-tint-bg text-accent-text">You</span>
            </div>
            {editingId !== (selfMember?.id ?? 'self') && (
              <button
                onClick={() => setEditingId(selfMember?.id ?? 'self')}
                className="text-xs text-text-tertiary hover:text-accent-text"
              >
                Edit
              </button>
            )}
          </div>
          {selfMember && editingId !== selfMember.id && (
            <p className="text-xs text-text-tertiary">{formatMemberSummary(selfMember)}</p>
          )}
          {(editingId === (selfMember?.id ?? 'self')) && (
            <MemberForm
              member={selfMember}
              relationship="self"
              onSave={(data) => handleSave(selfMember?.id, 'self', data)}
              onCancel={() => setEditingId(null)}
            />
          )}
        </div>

        {/* Partner member */}
        {partnerMember ? (
          <div className="rounded-lg border border-border-default p-4">
            <div className="flex items-center justify-between mb-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-text-primary">{partnerMember.name}</span>
                <span className="text-xs px-1.5 py-0.5 rounded bg-warning-tint-bg text-warning">Partner</span>
              </div>
              <div className="flex gap-3">
                {editingId !== partnerMember.id && (
                  <button
                    onClick={() => setEditingId(partnerMember.id)}
                    className="text-xs text-text-tertiary hover:text-accent-text"
                  >
                    Edit
                  </button>
                )}
                <button
                  onClick={handleDeletePartner}
                  disabled={deletingId === partnerMember.id}
                  className="text-xs text-text-muted hover:text-negative disabled:opacity-40"
                >
                  {deletingId === partnerMember.id ? 'Removing…' : 'Remove'}
                </button>
              </div>
            </div>
            {editingId !== partnerMember.id && (
              <p className="text-xs text-text-tertiary">{formatMemberSummary(partnerMember)}</p>
            )}
            {editingId === partnerMember.id && (
              <MemberForm
                member={partnerMember}
                relationship="partner"
                onSave={(data) => handleSave(partnerMember.id, 'partner', data)}
                onCancel={() => setEditingId(null)}
              />
            )}
          </div>
        ) : addingPartner ? (
          <div className="rounded-lg border border-border-default p-4">
            <p className="text-sm font-medium text-text-primary mb-2">Add Partner</p>
            <MemberForm
              relationship="partner"
              isNew
              onSave={(data) => handleSave(undefined, 'partner', data)}
              onCancel={() => setAddingPartner(false)}
            />
          </div>
        ) : (
          <button
            onClick={() => setAddingPartner(true)}
            className="w-full rounded-lg border border-dashed border-border-strong py-3 text-sm text-text-tertiary hover:text-text-secondary hover:border-border-strong/80 transition-colors"
          >
            + Add Partner
          </button>
        )}
      </div>
    </Card>
  )
}

