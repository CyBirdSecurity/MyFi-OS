import { useEffect, useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import useSWR, { mutate } from 'swr'
import { ShieldCheck } from '@phosphor-icons/react'
import {
  authApi,
  householdMembersApi,
  profileApi,
  contributionsApi,
  acceptsContributions,
  ACCOUNT_TYPE_LABELS,
  type Contribution,
  type HouseholdMember,
} from '../api/client'
import { useAuthStore } from '../store/authStore'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'
import { useAccounts } from '../hooks/useAccounts'
import { useNetWorthCurrent } from '../hooks/useNetWorth'
import { usePlaidConfig } from '../hooks/usePlaid'
import { useGuideDismissal } from '../components/PageGuide'
import { STATE_TAX_RATES } from '../data/stateTaxRates'
import { formatCurrency } from '../utils/currency'
import { Button } from '../components/ui/Button'
import { AccountForm, createAccount } from './AccountsPage'
import { FirstLook } from '../components/FirstLook'

const inputCls =
  'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'
const sectionCls = 'text-sm font-semibold text-text-primary'

type Person = { name: string; birthday: string; retirementAge: string }

const emptyPerson = (name = ''): Person => ({ name, birthday: '', retirementAge: '65' })

const fromMember = (m: HouseholdMember): Person => ({
  name: m.name,
  birthday: m.birthday?.slice(0, 10) ?? '',
  retirementAge: String(m.retirementAge || 65),
})

function PersonFields({
  label,
  value,
  onChange,
}: {
  label: string
  value: Person
  onChange: (p: Person) => void
}) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
      <div>
        <label className={labelCls}>{label} name</label>
        <input
          type="text"
          value={value.name}
          onChange={(e) => onChange({ ...value, name: e.target.value })}
          required
          className={inputCls}
        />
      </div>
      <div>
        <label className={labelCls}>Date of birth</label>
        <input
          type="date"
          value={value.birthday}
          onChange={(e) => onChange({ ...value, birthday: e.target.value })}
          required
          className={inputCls}
        />
      </div>
      <div>
        <label className={labelCls}>Retirement age</label>
        <input
          type="number"
          min={18}
          max={99}
          value={value.retirementAge}
          onChange={(e) => onChange({ ...value, retirementAge: e.target.value })}
          required
          className={inputCls}
        />
      </div>
    </div>
  )
}

type Step = 1 | 2 | 3
/** After step 3: what MyFi worked out from the answers. */
type Stage = Step | 'look'

function StepLabel({ step }: { step: Step }) {
  return (
    <p className="text-xs font-medium uppercase tracking-wider text-text-tertiary">
      Step {step} of 3
    </p>
  )
}

// ─── Step 1 (required): who's in the plan ────────────────────────────────────

function DetailsStep({ members, onDone }: { members: HouseholdMember[]; onDone: () => void }) {
  const user = useAuthStore((s) => s.user)
  const savedSelf = members.find((m) => m.relationship === 'self')
  const savedPartner = members.find((m) => m.relationship === 'partner')
  // Coming back from step 2 (or resuming later) shows what was already saved.
  const [self, setSelf] = useState<Person>(savedSelf ? fromMember(savedSelf) : emptyPerson(user?.displayName ?? ''))
  const [hasPartner, setHasPartner] = useState(!!savedPartner)
  const [partner, setPartner] = useState<Person>(savedPartner ? fromMember(savedPartner) : emptyPerson())
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const people: [Person, 'self' | 'partner'][] = [[self, 'self']]
      if (hasPartner) people.push([partner, 'partner'])
      for (const [p, relationship] of people) {
        await householdMembersApi.upsert({
          relationship,
          name: p.name,
          birthday: p.birthday,
          retirementAge: parseInt(p.retirementAge, 10) || 65,
        })
      }
      // Went back and unticked the partner: remove the one saved earlier.
      if (!hasPartner && savedPartner) {
        await householdMembersApi.delete(savedPartner.id)
      }
      await mutate('/household/members')
      mutate('/profile')
      onDone()
    } catch (err: any) {
      setError(err.message ?? 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="bg-surface border border-border-default rounded-lg p-6 sm:p-8 space-y-6">
      <StepLabel step={1} />
      <p className="text-sm text-text-secondary">
        Your age and retirement age set the timeline for your forecast: how many years you have to save, and how
        long your money needs to last.
      </p>
      {error && <div className="text-negative text-sm">{error}</div>}

      <PersonFields label="Your" value={self} onChange={setSelf} />

      <div className="border-t border-border-default pt-6 space-y-4">
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={hasPartner}
            onChange={(e) => setHasPartner(e.target.checked)}
            className="w-4 h-4 accent-accent"
          />
          <span className="text-sm text-text-primary">I'm planning with a partner</span>
        </label>
        {hasPartner && <PersonFields label="Partner's" value={partner} onChange={setPartner} />}
      </div>

      <Button type="submit" loading={loading} className="w-full justify-center">
        Continue
      </Button>
    </form>
  )
}

// ─── Step 2: accounts ────────────────────────────────────────────────────────

function AccountsStep({
  members,
  onBack,
  onDone,
}: {
  members: HouseholdMember[]
  onBack: () => void
  onDone: () => void
}) {
  const user = useAuthStore((s) => s.user)
  const { data: accounts } = useAccounts()
  const { data: nw } = useNetWorthCurrent()
  const { data: config } = usePlaidConfig()
  const list = accounts ?? []
  const [adding, setAdding] = useState(false)
  const showForm = adding || list.length === 0
  const self = members.find((m) => m.relationship === 'self')
  const partner = members.find((m) => m.relationship === 'partner')
  const balances = new Map(nw?.accounts.map((a) => [a.accountId, a.displayBalance ?? a.balance]) ?? [])

  return (
    <div className="bg-surface border border-border-default rounded-lg p-6 sm:p-8 space-y-6">
      <StepLabel step={2} />
      <p className="text-sm text-text-secondary">
        Add what you own and what you owe: checking, savings, retirement, investments, a home, loans, and cards.
        Your net worth and forecast are built from these. A rough balance is fine; you can refine it later.
      </p>

      {list.length > 0 && (
        <div className="space-y-2">
          {list.map((a) => (
            <div key={a.id} className="flex items-center justify-between rounded-lg border border-border-default px-4 py-2.5 text-sm">
              <div className="min-w-0">
                <span className="font-medium text-text-primary">{a.name}</span>
                <span className="ml-2 text-xs text-text-tertiary">{ACCOUNT_TYPE_LABELS[a.accountType] ?? a.accountType}</span>
              </div>
              <span className="text-text-primary">{balances.has(a.id) ? formatCurrency(balances.get(a.id)!) : ''}</span>
            </div>
          ))}
          {nw && (
            <div className="flex items-center justify-between px-4 pt-2 text-sm">
              <span className="text-text-secondary">Net worth so far</span>
              <span className="text-lg font-semibold text-accent-text">{formatCurrency(nw.totalNetWorth)}</span>
            </div>
          )}
        </div>
      )}

      {showForm ? (
        <div className={list.length > 0 ? 'border-t border-border-default pt-6' : ''}>
          <AccountForm
            partnerName={partner?.name}
            partnerMemberId={partner?.id}
            selfName={self?.name || user?.displayName}
            onSubmit={async (data, extra) => {
              await createAccount(data, extra)
              setAdding(false)
            }}
            onCancel={() => (list.length > 0 ? setAdding(false) : onDone())}
            cancelLabel={list.length > 0 ? 'Cancel' : 'Skip for now'}
            submitLabel="Add account"
          />
        </div>
      ) : (
        <Button variant="secondary" onClick={() => setAdding(true)} className="w-full justify-center">
          + Add another account
        </Button>
      )}

      {config?.plaidEnabled && (
        <p className="text-xs text-text-muted">
          Prefer automatic updates? MyFi can sync banks and brokerages through Plaid. You'll find Connect
          Institution under Accounts once you're done here.
        </p>
      )}

      {!showForm && (
        <div className="flex gap-3">
          <Button type="button" variant="secondary" onClick={onBack} className="justify-center">
            ← Back
          </Button>
          <Button onClick={onDone} className="flex-1 justify-center">
            Continue
          </Button>
        </div>
      )}
    </div>
  )
}

// ─── Step 3 (optional): income, spending, and taxes ──────────────────────────

/** Step 2's answers live in the page so they survive a trip back to step 1. */
type PlanAnswers = {
  incomes: Partial<Record<'self' | 'partner', string>>
  currentMonthlyExpenses: string
  monthlyRetirementExpenses: string
  stateCode: string
  /** Empty until chosen; defaults from whether there's a partner. */
  filingStatus: string
  /** Monthly saving into the first retirement/investment account, when none was entered with it. */
  monthlySavings: string
}

const emptyPlan: PlanAnswers = {
  incomes: {},
  currentMonthlyExpenses: '',
  monthlyRetirementExpenses: '',
  stateCode: '',
  filingStatus: '',
  monthlySavings: '',
}

function PlanStep({
  members,
  plan,
  onChange,
  onBack,
  onDone,
}: {
  members: HouseholdMember[]
  plan: PlanAnswers
  onChange: (p: PlanAnswers) => void
  onBack: () => void
  onDone: () => Promise<void>
}) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  // You first, then your partner.
  const people = [...members].sort((a, b) => Number(a.relationship !== 'self') - Number(b.relationship !== 'self'))
  const hasPartner = members.some((m) => m.relationship === 'partner')
  const filingStatus = plan.filingStatus || (hasPartner ? 'married_filing_jointly' : 'single')
  // Without a contribution the forecast only grows today's balances, so ask
  // here if the accounts step didn't get one. /contributions is null when empty.
  const { data: accounts } = useAccounts()
  const { data: contributions, isLoading: contributionsLoading } = useSWR<Contribution[] | null>(
    '/contributions',
    () => contributionsApi.listAll(),
  )
  const savingsAccount = !contributionsLoading && !contributions?.length
    ? accounts?.find((a) => acceptsContributions(a.accountType))
    : undefined
  const totalIncome = Object.values(plan.incomes).reduce((sum, v) => sum + (parseFloat(v ?? '') || 0), 0)
  const leftOver = Math.round(totalIncome / 12 - (parseFloat(plan.currentMonthlyExpenses) || 0))

  const finish = async (save: boolean) => {
    setError('')
    setLoading(true)
    try {
      if (save) {
        const state = STATE_TAX_RATES.find((s) => s.code === plan.stateCode)
        const retirement = parseFloat(plan.monthlyRetirementExpenses)
        const current = parseFloat(plan.currentMonthlyExpenses)
        await profileApi.upsert({
          filingStatus,
          ...(state ? { stateTaxCode: state.code, stateTaxRate: state.rate } : {}),
          ...(retirement > 0 ? { monthlyExpenses: retirement } : {}),
          ...(current > 0 ? { currentMonthlyExpenses: current } : {}),
        })
        for (const m of people) {
          const income = parseFloat(plan.incomes[m.relationship] ?? '')
          if (income > 0) await householdMembersApi.update(m.id, { annualIncome: income })
        }
        const savings = parseFloat(plan.monthlySavings)
        if (savingsAccount && savings > 0) {
          await contributionsApi.create(savingsAccount.id, {
            amount: savings,
            frequency: 'monthly',
            annualIncreasePct: 0,
            amountType: 'fixed',
            incomePct: 0,
            matchTiers: [],
          })
          mutate('/contributions')
        }
        mutate('/profile')
        mutate('/household/members')
        mutate('/forecast')
      }
      await onDone()
    } catch (err: any) {
      setError(err.message ?? 'Something went wrong')
      setLoading(false)
    }
  }

  return (
    <form
      onSubmit={(e) => { e.preventDefault(); finish(true) }}
      className="bg-surface border border-border-default rounded-lg p-6 sm:p-8 space-y-6"
    >
      <StepLabel step={3} />
      <p className="text-sm text-text-secondary">
        Optional, but this is what turns a balance into a plan: income and spending show how fast you're saving and
        whether your money lasts through retirement, and your state sets your tax estimates.
      </p>
      {error && <div className="text-negative text-sm">{error}</div>}

      <div className="space-y-3">
        <h2 className={sectionCls}>Income</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {people.map((m) => (
            <div key={m.id}>
              <label className={labelCls}>
                {m.relationship === 'self' ? 'Your' : `${m.name || 'Partner'}'s`} annual income ($)
              </label>
              <input
                type="number"
                min={0}
                step="1000"
                placeholder="e.g. 85000"
                value={plan.incomes[m.relationship] ?? ''}
                onChange={(e) => onChange({ ...plan, incomes: { ...plan.incomes, [m.relationship]: e.target.value } })}
                className={inputCls}
              />
            </div>
          ))}
        </div>
        <p className="text-xs text-text-muted">Before taxes. Leave blank if not working.</p>
      </div>

      <div className="space-y-3 border-t border-border-default pt-6">
        <h2 className={sectionCls}>Spending</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className={labelCls}>Current monthly expenses ($)</label>
            <input
              type="number"
              min={0}
              step="100"
              placeholder="e.g. 4500"
              value={plan.currentMonthlyExpenses}
              onChange={(e) => onChange({ ...plan, currentMonthlyExpenses: e.target.value })}
              className={inputCls}
            />
            <p className="text-xs text-text-muted mt-1">What you spend today. Used for your savings rate.</p>
          </div>
          <div>
            <label className={labelCls}>Monthly spending in retirement ($)</label>
            <input
              type="number"
              min={0}
              step="100"
              placeholder="e.g. 5000"
              value={plan.monthlyRetirementExpenses}
              onChange={(e) => onChange({ ...plan, monthlyRetirementExpenses: e.target.value })}
              className={inputCls}
            />
            <p className="text-xs text-text-muted mt-1">Per month once you stop working, in today's dollars.</p>
          </div>
          {savingsAccount && (
            <div>
              <label className={labelCls}>Monthly savings into {savingsAccount.name} ($)</label>
              <input
                type="number"
                min={0}
                step="50"
                placeholder="e.g. 500"
                value={plan.monthlySavings}
                onChange={(e) => onChange({ ...plan, monthlySavings: e.target.value })}
                className={inputCls}
              />
              <p className="text-xs text-text-muted mt-1">
                {leftOver > 0
                  ? `You have about ${formatCurrency(leftOver)}/mo left after spending, before taxes. `
                  : ''}
                This is what makes your forecast grow.
              </p>
            </div>
          )}
        </div>
      </div>

      <div className="space-y-3 border-t border-border-default pt-6">
        <h2 className={sectionCls}>Taxes</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className={labelCls}>State</label>
            <select
              value={plan.stateCode}
              onChange={(e) => onChange({ ...plan, stateCode: e.target.value })}
              className={inputCls}
            >
              <option value="">Select state…</option>
              {STATE_TAX_RATES.map((s) => (
                <option key={s.code} value={s.code}>{s.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className={labelCls}>Filing status</label>
            <select
              value={filingStatus}
              onChange={(e) => onChange({ ...plan, filingStatus: e.target.value })}
              className={inputCls}
            >
              <option value="single">Single</option>
              <option value="married_filing_jointly">Married Filing Jointly</option>
            </select>
          </div>
        </div>
      </div>

      <div className="flex gap-3">
        <Button type="button" variant="secondary" onClick={onBack} disabled={loading} className="justify-center">
          ← Back
        </Button>
        <Button type="submit" loading={loading} className="flex-1 justify-center">
          Finish setup
        </Button>
      </div>
      <p className="text-center text-sm text-text-muted">
        <button type="button" onClick={() => finish(false)} disabled={loading} className="hover:text-text-secondary underline">
          Skip this step
        </button>
      </p>
    </form>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function OnboardingPage() {
  const navigate = useNavigate()
  const { user, setUser } = useAuthStore()
  // /household/members returns null (not []) when the user has none.
  const { data, error: membersError } = useHouseholdMembers()
  const members = data ?? []
  const [step, setStep] = useState<Stage | null>(null)
  const [plan, setPlan] = useState<PlanAnswers>(emptyPlan)
  // The dashboard's own welcome guide would repeat this page; finishing here stands in for it.
  const { dismiss: dismissDashboardGuide } = useGuideDismissal('dashboard')

  // Pick the starting step once members load: someone who finished step 1 but
  // left before the end picks up at step 2. Wait for the response itself (null
  // or a list) rather than isLoading, which reads false right after sign-out
  // clears the cache and before the refetch starts.
  const membersLoaded = data !== undefined || !!membersError
  useEffect(() => {
    if (step === null && membersLoaded) {
      setStep(members.some((m) => m.relationship === 'self' && m.birthday) ? 2 : 1)
    }
  }, [step, membersLoaded, members])

  if (!user) return null
  // Finishing marks the user onboarded; the first look still shows before the dashboard.
  if (user.onboardedAt && step !== 'look') return <Navigate to="/dashboard" replace />
  if (step === null) return null

  const complete = async () => {
    // Show the first look before marking the user onboarded: the store update
    // re-renders synchronously, so the redirect above must already see 'look'.
    setStep('look')
    await authApi.markOnboarded()
    dismissDashboardGuide()
    setUser({ ...useAuthStore.getState().user!, onboardedAt: new Date().toISOString() })
  }

  return (
    <div className="min-h-screen bg-canvas flex flex-col items-center justify-start py-12 px-4">
      <div className="w-full max-w-2xl">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-accent-text">
            {step === 'look' ? 'Your first look' : 'Welcome to MyFi'}
          </h1>
          <p className="text-text-tertiary mt-2 text-sm">
            {step === 1
              ? "Three short steps, about two minutes. Then you'll see your net worth, a forecast to age 100, and when you could retire."
              : step === 2
                ? 'Next, the accounts your numbers are built from.'
                : step === 3
                  ? 'Last step. A little more about your plan.'
                  : 'Here’s what MyFi worked out from your answers. Open any number to dig in.'}
          </p>
        </div>
        {step === 1 ? (
          <DetailsStep members={members} onDone={() => setStep(2)} />
        ) : step === 2 ? (
          <AccountsStep members={members} onBack={() => setStep(1)} onDone={() => setStep(3)} />
        ) : step === 'look' ? (
          <FirstLook onDone={() => navigate('/dashboard')} />
        ) : (
          <PlanStep
            members={members}
            plan={plan}
            onChange={setPlan}
            onBack={() => setStep(2)}
            onDone={complete}
          />
        )}
        <p className="mt-6 flex items-start justify-center gap-2 text-center text-xs text-text-muted">
          <ShieldCheck size={14} weight="bold" className="mt-px shrink-0" />
          <span>
            Your data stays on your own server. There are no trackers.
          </span>
        </p>
      </div>
    </div>
  )
}
