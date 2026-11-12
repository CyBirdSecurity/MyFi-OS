import { useEffect, useState } from 'react'
import useSWR, { mutate } from 'swr'
import { allocationTargetsApi, householdMembersApi, profileApi, type AllocationTarget } from '../api/client'
import { useProfile } from '../hooks/useProfile'
import { useAuthStore } from '../store/authStore'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'
import { STATE_TAX_RATES } from '../data/stateTaxRates'
import { Button } from './ui/Button'

// Small forms used by the setup popups on individual pages. They save only
// their own fields; everything else stays editable in Settings.

const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

const refresh = (prefixes: string[]) =>
  mutate((key) => typeof key === 'string' && prefixes.some((p) => key.startsWith(p)))

function FormFooter({ loading, label }: { loading: boolean; label: string }) {
  return (
    <div className="flex items-center justify-between gap-3 pt-2">
      <p className="text-xs text-text-muted">You can change this anytime in Settings.</p>
      <Button type="submit" loading={loading}>{label}</Button>
    </div>
  )
}

// ─── Forecast / Retirement: date of birth and retirement age ────────────────

export function BirthdaySetupForm({ onDone }: { onDone: () => void }) {
  const user = useAuthStore((st) => st.user)
  // /household/members returns null (not []) when the user has none.
  const { data: members } = useHouseholdMembers()
  const self = members?.find((m) => m.relationship === 'self')
  const [birthday, setBirthday] = useState(self?.birthday?.slice(0, 10) ?? '')
  const [retirementAge, setRetirementAge] = useState(String(self?.retirementAge ?? 65))
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const age = parseInt(retirementAge, 10) || 65
      if (self) {
        await householdMembersApi.update(self.id, { birthday, retirementAge: age })
      } else {
        await householdMembersApi.upsert({ relationship: 'self', name: user?.displayName ?? '', birthday, retirementAge: age })
      }
      await refresh(['/profile', '/household/members', '/forecast', '/retirement', 'forecast/'])
      onDone()
    } catch (err: any) {
      setError(err.message ?? 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label className={labelCls}>Date of birth</label>
          <input type="date" required value={birthday} onChange={(e) => setBirthday(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className={labelCls}>Retirement age</label>
          <input
            type="number"
            min={18}
            max={99}
            required
            value={retirementAge}
            onChange={(e) => setRetirementAge(e.target.value)}
            className={inputCls}
          />
        </div>
      </div>
      <FormFooter loading={loading} label="Save" />
    </form>
  )
}

// ─── Taxes: filing status, state, and income ─────────────────────────────────

export function TaxSetupForm({ onDone }: { onDone: () => void }) {
  const { data: profile } = useProfile()
  const { data: members } = useHouseholdMembers()
  const [filingStatus, setFilingStatus] = useState(profile?.filingStatus ?? 'single')
  const [stateCode, setStateCode] = useState(profile?.stateTaxCode ?? '')
  const [incomes, setIncomes] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const state = STATE_TAX_RATES.find((s) => s.code === stateCode)
      await profileApi.upsert({
        filingStatus,
        stateTaxCode: stateCode || null,
        ...(state ? { stateTaxRate: state.rate } : {}),
      })
      for (const [id, value] of Object.entries(incomes)) {
        if (value !== '') await householdMembersApi.update(id, { annualIncome: parseFloat(value) || 0 })
      }
      await refresh(['/profile', '/household/members', '/taxes', '/forecast'])
      onDone()
    } catch (err: any) {
      setError(err.message ?? 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label className={labelCls}>Filing status</label>
          <select value={filingStatus} onChange={(e) => setFilingStatus(e.target.value)} className={inputCls}>
            <option value="single">Single</option>
            <option value="married_filing_jointly">Married Filing Jointly</option>
          </select>
        </div>
        <div>
          <label className={labelCls}>State</label>
          <select value={stateCode} onChange={(e) => setStateCode(e.target.value)} className={inputCls}>
            <option value="">Select state…</option>
            {STATE_TAX_RATES.map((s) => (
              <option key={s.code} value={s.code}>{s.name}</option>
            ))}
          </select>
        </div>
      </div>
      {members?.map((m) => (
        <div key={m.id}>
          <label className={labelCls}>
            {m.relationship === 'self' ? 'Your' : `${m.name || "Partner"}'s`} annual income ($)
          </label>
          <input
            type="number"
            min={0}
            step="1000"
            placeholder={m.monthlyIncome ? String(Math.round(m.monthlyIncome * 12)) : '0'}
            value={incomes[m.id] ?? ''}
            onChange={(e) => setIncomes((prev) => ({ ...prev, [m.id]: e.target.value }))}
            className={inputCls}
          />
        </div>
      ))}
      <FormFooter loading={loading} label="Save" />
    </form>
  )
}

// ─── Retirement: spending and withdrawal rate ────────────────────────────────

export function RetirementSetupForm({ onDone }: { onDone: () => void }) {
  const { data: profile } = useProfile()
  const [spending, setSpending] = useState(profile?.monthlyExpenses ? String(profile.monthlyExpenses) : '')
  const [withdrawalRate, setWithdrawalRate] = useState(
    profile?.withdrawalRate != null ? String(Math.round(profile.withdrawalRate * 1000) / 10) : '',
  )
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await profileApi.upsert({
        monthlyExpenses: parseFloat(spending) || 0,
        withdrawalRate: withdrawalRate !== '' ? parseFloat(withdrawalRate) / 100 : null,
      })
      await refresh(['/profile', '/retirement', '/forecast'])
      onDone()
    } catch (err: any) {
      setError(err.message ?? 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div>
        <label className={labelCls}>Monthly spending in retirement ($)</label>
        <input
          type="number"
          min={0}
          step="100"
          required
          value={spending}
          onChange={(e) => setSpending(e.target.value)}
          className={inputCls}
        />
        <p className="text-xs text-text-muted mt-1">
          What you expect to spend each month once you stop working, in today's dollars.
        </p>
      </div>
      <div>
        <label className={labelCls}>Safe withdrawal rate (%)</label>
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
        <p className="text-xs text-text-muted mt-1">Leave blank to use the standard 4% rule.</p>
      </div>
      <FormFooter loading={loading} label="Save" />
    </form>
  )
}

// ─── Holdings / Settings: allocation targets ─────────────────────────────────

const ASSET_CLASSES: { key: keyof AllocationTarget; label: string }[] = [
  { key: 'equityPct', label: 'Equity' },
  { key: 'bondPct',   label: 'Bond / Fixed Income' },
  { key: 'cryptoPct', label: 'Cryptocurrency' },
  { key: 'cashPct',   label: 'Cash' },
  { key: 'otherPct',  label: 'Other' },
]

export function useAllocationTargets() {
  return useSWR<AllocationTarget>('/allocation-targets', () => allocationTargetsApi.get())
}

export function AllocationTargetsForm({ onSaved }: { onSaved: () => void }) {
  const { data: existing } = useAllocationTargets()
  const [targets, setTargets] = useState<AllocationTarget>({
    equityPct: 0, bondPct: 0, cryptoPct: 0, cashPct: 0, otherPct: 0,
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (existing) setTargets({
      equityPct: existing.equityPct ?? 0,
      bondPct:   existing.bondPct   ?? 0,
      cryptoPct: existing.cryptoPct ?? 0,
      cashPct:   existing.cashPct   ?? 0,
      otherPct:  existing.otherPct  ?? 0,
    })
  }, [existing])

  const total = ASSET_CLASSES.reduce((s, { key }) => s + (targets[key] || 0), 0)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (Math.round(total) !== 100 && total !== 0) {
      setError('Targets must sum to 100% (or leave all at 0 to skip)')
      return
    }
    setError('')
    setLoading(true)
    try {
      await allocationTargetsApi.upsert(targets)
      mutate('/allocation-targets')
      onSaved()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <div className="text-negative text-sm">{error}</div>}
      <div className="space-y-3">
        {ASSET_CLASSES.map(({ key, label }) => (
          <div key={key} className="flex items-center gap-3">
            <label className="w-40 text-sm text-text-secondary shrink-0">{label}</label>
            <div className="relative flex-1 max-w-[120px]">
              <input
                type="number"
                min={0}
                max={100}
                step="1"
                value={targets[key] || ''}
                onChange={(e) => setTargets((prev) => ({ ...prev, [key]: parseFloat(e.target.value) || 0 }))}
                className={inputCls + ' pr-8'}
                placeholder="0"
              />
              <span className="absolute right-3 top-1/2 -translate-y-1/2 text-text-tertiary text-sm">%</span>
            </div>
          </div>
        ))}
      </div>
      <div className={`text-xs ${total === 0 ? 'text-text-muted' : Math.round(total) === 100 ? 'text-positive' : 'text-warning'}`}>
        Total: {total.toFixed(0)}%{total > 0 && Math.round(total) !== 100 ? ' — must equal 100%' : ''}
      </div>
      <div className="pt-1">
        <Button type="submit" loading={loading}>
          Save Targets
        </Button>
      </div>
    </form>
  )
}
