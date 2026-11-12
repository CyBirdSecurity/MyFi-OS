import { useState, useDeferredValue } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts'
import { useRetirement } from '../hooks/useRetirement'
import { useProfile } from '../hooks/useProfile'
import { Card } from '../components/ui/Card'
import { formatCurrency } from '../utils/currency'
import type { RetirementScenario } from '../api/client'
import { useChartPalette } from '../hooks/useChartPalette'
import { PageGuide } from '../components/PageGuide'
import { SetupPrompt } from '../components/SetupPrompt'
import { InfoTip } from '../components/ui/InfoTip'
import { ProfileRequired } from '../components/ProfileRequired'
import { RetirementSetupForm } from '../components/SetupForms'

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="bg-surface border border-border-default rounded-lg px-4 py-4">
      <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">{label}</div>
      <div className="text-xl font-bold text-text-primary">{value}</div>
      {sub && <div className="text-xs text-text-tertiary mt-0.5">{sub}</div>}
    </div>
  )
}

function RateScenariosTable({ scenarios }: { scenarios: RetirementScenario[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm min-w-[480px]">
        <thead>
          <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
            <th className="pb-2 font-medium">Strategy</th>
            <th className="pb-2 font-medium">Withdrawal Rate</th>
            <th className="pb-2 font-medium">Monthly Income (today's $)</th>
            <th className="pb-2 font-medium">Portfolio Lasts Until</th>
          </tr>
        </thead>
        <tbody>
          {scenarios.map((s) => {
            const safe = s.depletionAge == null
            return (
              <tr key={s.label} className="border-b border-border-default/50">
                <td className="py-3 text-text-primary font-medium">{s.label}</td>
                <td className="py-3 text-text-secondary">{(s.rate * 100).toFixed(0)}%</td>
                <td className="py-3 text-text-primary font-semibold">{formatCurrency(s.monthlyIncome)}/mo</td>
                <td className={`py-3 font-semibold ${safe ? 'text-positive' : 'text-warning'}`}>
                  {safe ? 'Age 100+' : `Age ${s.depletionAge}`}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function DrawdownTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-surface border border-border-strong rounded-lg px-3 py-2 text-xs shadow-float">
      <div className="text-text-secondary mb-1">Age {label}</div>
      {payload.map((p: { name: string; value: number; color: string }) => (
        <div key={p.name} style={{ color: p.color }} className="flex gap-2 items-center">
          <span>{p.name}:</span>
          <span className="font-semibold">{formatCurrency(p.value)}</span>
        </div>
      ))}
    </div>
  )
}

const EXAMPLE_SPENDING = 5000

export function RetirementPage() {
  const palette = useChartPalette()
  const { data: profile } = useProfile()

  // Until a retirement spending target is set, show an example figure rather
  // than $0, which would read as a plan that lasts forever.
  const needsSpending = !!profile && !profile.monthlyExpenses && !profile.useBudgetForExpenses
  const defaultSpending = needsSpending ? EXAMPLE_SPENDING : profile?.monthlyExpenses ?? EXAMPLE_SPENDING
  const [rawSpending, setRawSpending] = useState<number | null>(null)
  const spending = rawSpending ?? defaultSpending

  // Debounce via React's deferred value so the slider doesn't fire a network
  // request on every pixel.
  const deferredSpending = useDeferredValue(spending)

  // Projections need a date of birth; skip the request until one is set.
  const { data: plan, isLoading } = useRetirement(deferredSpending, !!profile?.birthday)

  const chartData = (plan?.drawdownSchedule ?? []).map((d) => ({
    age: d.age,
    Balance: Math.max(0, d.balance),
  }))

  const depletionAge = plan?.depletionAge

  if (profile && !profile.birthday) {
    return (
      <ProfileRequired
        title="Retirement"
        guideId="retirement"
        subtitle="See how long your portfolio lasts based on your retirement spending."
        what="how long your portfolio lasts once you retire"
      />
    )
  }

  return (
    <div className="space-y-6 max-w-6xl">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold text-text-primary">Retirement</h1>
          <PageGuide id="retirement" />
        </div>
        <p className="text-sm text-text-tertiary mt-1">
          See how long your portfolio lasts based on your retirement spending.
        </p>
      </div>

      {needsSpending && (
        <SetupPrompt
          message={`These numbers use an example of ${formatCurrency(EXAMPLE_SPENDING)}/mo. Tell MyFi what you expect to spend in retirement so they reflect your plan.`}
          buttonLabel="Set retirement spending"
          title="Retirement Spending"
        >
          {(done) => <RetirementSetupForm onDone={done} />}
        </SetupPrompt>
      )}

      {/* Spending slider */}
      <Card title="Monthly Spending in Retirement">
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-2xl font-bold text-text-primary">{formatCurrency(spending)}/mo</span>
            <span className="text-xs text-text-tertiary">
              = {formatCurrency(spending * 12)}/yr
            </span>
          </div>
          <input
            type="range"
            min={500}
            max={20000}
            step={100}
            value={spending}
            onChange={(e) => setRawSpending(Number(e.target.value))}
            className="w-full accent-accent cursor-pointer"
          />
          <div className="flex justify-between text-xs text-text-muted">
            <span>$500</span>
            <span>$10,000</span>
            <span>$20,000</span>
          </div>
        </div>
      </Card>

      {/* Loading state */}
      {isLoading && (
        <div className="flex justify-center py-12">
          <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin" />
        </div>
      )}

      {plan && (
        <>
          {/* Key stats */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <StatCard
              label="Retirement Age"
              value={String(plan.retirementAge)}
              sub={`Year ${plan.retirementYear}`}
            />
            <StatCard
              label="Portfolio at Retirement"
              value={formatCurrency(plan.retirementPortfolio)}
            />
            <StatCard
              label="Sustainable Monthly"
              value={formatCurrency(plan.sustainableMonthly)}
              sub="In today's dollars"
            />
            <StatCard
              label="Portfolio Lasts Until"
              value={plan.survivesToHundred ? 'Age 100+' : `Age ${plan.depletionAge}`}
              sub={plan.survivesToHundred ? 'Fully sustainable' : 'Then depleted'}
            />
          </div>

          {/* Social Security notice */}
          {plan.annualSocialSecurity > 0 && (
            <div className="text-xs text-text-tertiary bg-surface/50 border border-border-default rounded-lg px-4 py-3">
              Social Security of{' '}
              <span className="text-text-primary font-medium">
                {formatCurrency(plan.annualSocialSecurity / 12)}/mo
              </span>{' '}
              ({formatCurrency(plan.annualSocialSecurity)}/yr) is included from each person's claiming age.
            </div>
          )}

          {/* Depletion warning */}
          {!plan.survivesToHundred && depletionAge != null && (
            <div className="bg-warning-tint-bg border border-warning/30 rounded-lg px-5 py-4">
              <div className="text-warning font-medium mb-1">Portfolio Depletion Warning</div>
              <p className="text-sm text-text-secondary">
                At {formatCurrency(spending)}/mo your portfolio runs out at age{' '}
                <span className="text-warning font-semibold">{depletionAge}</span>. Consider
                reducing spending, retiring later, or increasing contributions.
              </p>
            </div>
          )}

          {/* Drawdown chart */}
          <Card title="Portfolio Drawdown Over Retirement">
            <ResponsiveContainer width="100%" height={320}>
              <LineChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} />
                <XAxis
                  dataKey="age"
                  tick={{ fill: palette.tick, fontSize: 11 }}
                  tickLine={false}
                  label={{ value: 'Age', position: 'insideBottom', offset: -2, fill: palette.tick, fontSize: 11 }}
                />
                <YAxis
                  tickFormatter={(v) => formatCurrency(v, { compact: true })}
                  tick={{ fill: palette.tick, fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                  width={70}
                />
                <Tooltip content={<DrawdownTooltip />} />
                {depletionAge != null && (
                  <ReferenceLine
                    x={depletionAge}
                    stroke={palette.goalLow}
                    strokeDasharray="4 4"
                    label={{ value: 'Depleted', fill: palette.goalLow, fontSize: 10, position: 'top' }}
                  />
                )}
                <Line
                  type="monotone"
                  dataKey="Balance"
                  stroke={palette.netWorthLine}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              </LineChart>
            </ResponsiveContainer>
          </Card>

          {/* Rate scenarios table */}
          {plan.rateScenarios.length > 0 && (
            <Card title={<>Withdrawal Rate Scenarios <InfoTip term="withdrawalRate" /></>}>
              <p className="text-xs text-text-tertiary mb-4">
                How different withdrawal rates affect your monthly income and how long your
                portfolio lasts starting with {formatCurrency(plan.retirementPortfolio)} at retirement.
              </p>
              <RateScenariosTable scenarios={plan.rateScenarios} />
            </Card>
          )}
        </>
      )}

      {/* The plan failed to load (missing birthday is handled above) */}
      {!isLoading && !plan && (
        <div className="bg-surface border border-border-default rounded-lg px-5 py-6 text-sm text-text-tertiary">
          Your retirement plan couldn't be calculated right now. Try reloading the page.
        </div>
      )}
    </div>
  )
}
