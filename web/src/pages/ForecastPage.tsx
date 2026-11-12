import { useState } from 'react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { forecastApi, contributionsApi, acceptsContributions } from '../api/client'
import { useProfile } from '../hooks/useProfile'
import { useGoals } from '../hooks/useGoals'
import { Card } from '../components/ui/Card'
import { ForecastChart } from '../components/charts/ForecastChart'
import { ProfileRequired } from '../components/ProfileRequired'
import { ReadinessScoreCard } from '../components/ReadinessScoreCard'
import { HistoricalBacktestPanel } from '../components/HistoricalBacktestPanel'
import { formatCurrency } from '../utils/currency'
import { accountOwner, annualContribution, annualSalary, describeAmount, describeMatch } from '../utils/contributions'
import { findGoalHitYear } from '../utils/goalProjection'
import { findFIYear, yearAtRetirement } from '../utils/fi'
import { useAccounts } from '../hooks/useAccounts'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'
import type { ForecastResult, FIMetrics, Contribution, AccountMeta } from '../api/client'
import { PageGuide } from '../components/PageGuide'
import { InfoTip } from '../components/ui/InfoTip'
import { SetupPrompt } from '../components/SetupPrompt'
import { RetirementSetupForm } from '../components/SetupForms'

type Scenario = 'base' | 'optimistic' | 'pessimistic'
type RangeKey = '10Y' | '20Y' | '30Y' | 'retirement' | 'full'

export function ForecastPage() {
  const { data: profile, isLoading: profileLoading } = useProfile()
  const [scenario, setScenario] = useState<Scenario>('base')
  const [showBands, setShowBands] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [rangeKey, setRangeKey] = useState<RangeKey>('30Y')
  const { data: goals } = useGoals()

  const profileReady = !profileLoading && profile?.birthday != null
  const shouldFetch = profileReady

  const {
    data: forecast,
    error: forecastError,
    isLoading: forecastLoading,
  } = useSWR<ForecastResult>(
    shouldFetch ? '/forecast' : null,
    () => forecastApi.get(),
    { revalidateOnFocus: false },
  )

  const { data: contributions } = useSWR<Contribution[]>(
    shouldFetch ? '/contributions' : null,
    () => contributionsApi.listAll(),
    { revalidateOnFocus: false },
  )

  if (profileLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (!profileReady) {
    return (
      <ProfileRequired
        title="Portfolio Forecast"
        guideId="forecast"
        subtitle="Projected growth through age 100 — all accounts combined."
        what="your net worth projected year by year through age 100"
      />
    )
  }

  if (forecastLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin" />
        <p className="text-sm text-text-tertiary">Building your forecast&hellip;</p>
      </div>
    )
  }

  if (forecastError) {
    return (
      <div className="max-w-xl">
        <div className="bg-warning-tint-bg border border-warning/30 rounded-lg px-5 py-4">
          <div className="text-warning font-medium mb-1">Forecast unavailable</div>
          <div className="text-sm text-text-secondary">{forecastError.message}</div>
          <Link
            to="/settings"
            className="mt-3 inline-block text-sm text-accent-text hover:text-accent-text underline"
          >
            Review your settings &rarr;
          </Link>
        </div>
      </div>
    )
  }

  if (!forecast) return null

  if (forecast.accountMeta.length === 0) {
    return (
      <div className="max-w-xl space-y-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text-primary">Portfolio Forecast</h1>
            <PageGuide id="forecast" />
          </div>
          <p className="text-sm text-text-tertiary mt-1">Projected growth through age 100 &mdash; all accounts combined.</p>
        </div>
        <div className="bg-surface border border-border-default rounded-lg px-5 py-6 space-y-3">
          <div className="text-text-primary font-medium">No accounts to forecast</div>
          <p className="text-sm text-text-tertiary">
            Your forecast needs at least one account with a balance. Add an account to see your projected portfolio growth.
          </p>
          <div className="flex flex-wrap gap-3 pt-1">
            <Link
              to="/accounts"
              className="inline-block text-sm bg-accent hover:bg-accent-hover text-white px-4 py-2 rounded-lg transition-colors"
            >
              Add an account
            </Link>
          </div>
        </div>
      </div>
    )
  }

  const years = forecast[scenario] ?? forecast.base

  const startYear = years[0]?.year ?? new Date().getFullYear()
  const rangeEndYear: number | undefined =
    rangeKey === '10Y' ? startYear + 10
    : rangeKey === '20Y' ? startYear + 20
    : rangeKey === '30Y' ? startYear + 30
    : rangeKey === 'retirement' ? forecast.retirementYear
    : undefined

  const visibleYears = rangeEndYear ? years.filter((y) => y.year <= rangeEndYear) : years

  const retirementYearData = yearAtRetirement(years, forecast.retirementYear)
  const age80Data = years.find((y) => y.age === 80)
  const age100Data = years[years.length - 1]

  // Compute FI year for the selected scenario so the panel and chart stay in sync.
  const scenarioFIMetrics = (() => {
    if (!forecast.fiMetrics) return undefined
    const hit = findFIYear(years, forecast.fiMetrics)
    return {
      ...forecast.fiMetrics,
      timeToFIYear: hit?.year ?? undefined,
      timeToFIAge: hit?.age ?? undefined,
    }
  })()

  const goalProjections = (goals ?? [])
    .filter((g) => !g.isAchieved && g.targetAmount > 0)
    .map((g) => {
      const hit = findGoalHitYear(g, years, forecast.accountMeta)
      return { goal: g, hitYear: hit?.year ?? null, hitAge: hit?.age ?? null }
    })
    .sort((a, b) => {
      if (a.hitYear === null && b.hitYear === null) return a.goal.targetAmount - b.goal.targetAmount
      if (a.hitYear === null) return 1
      if (b.hitYear === null) return -1
      return a.hitYear - b.hitYear
    })

  const RANGES: { label: string; key: RangeKey }[] = [
    { label: '10Y', key: '10Y' },
    { label: '20Y', key: '20Y' },
    { label: '30Y', key: '30Y' },
    { label: 'To Retirement', key: 'retirement' },
    { label: 'To 100', key: 'full' },
  ]

  return (
    <div className={fullscreen ? 'fixed inset-0 z-50 bg-canvas p-4 sm:p-6 pt-[max(1rem,env(safe-area-inset-top))] overflow-auto' : 'space-y-6 max-w-6xl'}>
      {/* Header */}
      <div className="flex items-start sm:items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text-primary">Portfolio Forecast</h1>
            <PageGuide id="forecast" />
          </div>
          <p className="text-sm text-text-tertiary mt-1">
            Projected growth of the savings and investments you can retire on, through age 100. Debts show below the line; cars and other property aren't counted.
          </p>
        </div>
        <button
          onClick={() => setFullscreen((f) => !f)}
          className="shrink-0 text-xs text-text-tertiary hover:text-text-primary border border-border-strong rounded px-3 py-1.5 transition-colors"
        >
          {fullscreen ? 'Exit' : 'Expand'}
        </button>
      </div>

      {/* Without ongoing saving the projection only grows today's balances,
          which is easy to mistake for a broken chart. */}
      {/* undefined while loading; null (not []) when there are none. */}
      {contributions !== undefined && !contributions?.length && (() => {
        const savingsAccount = forecast.accountMeta.find((a) => acceptsContributions(a.accountType))
        return (
          <div className="flex flex-col sm:flex-row sm:items-center gap-3 bg-accent-tint-bg border border-accent-tint-border rounded-lg px-4 py-3">
            <p className="text-sm text-text-primary flex-1">
              {savingsAccount
                ? "This forecast only grows what you have today. Add what you save each month, like 401(k) or IRA contributions, to see where your saving takes you."
                : 'This forecast only grows what you have today, and cash grows slowly. Add your retirement and investment accounts, and what you save into them each month, to see your real trajectory.'}
            </p>
            <Link
              to={savingsAccount ? `/accounts/${savingsAccount.id}?tab=contributions` : '/accounts'}
              className="text-sm font-medium text-accent-text hover:text-accent whitespace-nowrap self-start sm:self-auto"
            >
              {savingsAccount ? 'Add monthly savings' : 'Add an account'} →
            </Link>
          </div>
        )
      })()}

      {/* Scenario + band controls */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-text-tertiary">Scenario:</span>
          {(['base', 'optimistic', 'pessimistic'] as Scenario[]).map((s) => (
            <button
              key={s}
              onClick={() => setScenario(s)}
              className={`text-xs px-3 py-1 rounded-full border transition-colors ${
                scenario === s
                  ? 'bg-accent-tint-bg border-accent text-accent-text'
                  : 'border-border-strong text-text-tertiary hover:text-text-primary hover:border-border-strong'
              }`}
            >
              {s === 'base' ? 'Base' : s === 'optimistic' ? 'Optimistic (+2%)' : 'Pessimistic (−2%)'}
            </button>
          ))}
        </div>
        <button
          onClick={() => setShowBands((b) => !b)}
          className={`text-xs px-3 py-1 rounded-full border transition-colors ${
            showBands
              ? 'bg-accent-tint-bg border-accent text-accent-text'
              : 'border-border-strong text-text-tertiary hover:text-text-primary hover:border-border-strong'
          }`}
        >
          {showBands ? 'Hide probability bands' : 'Show probability bands'}
        </button>
      </div>

      {/* Chart */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2 mb-4">
          <div className="flex flex-wrap gap-1">
            {RANGES.map((r) => (
              <button
                key={r.key}
                onClick={() => setRangeKey(r.key)}
                className={`px-3 py-1 rounded text-xs font-medium transition-colors ${
                  rangeKey === r.key
                    ? 'bg-accent text-white'
                    : 'text-text-secondary hover:text-text-primary hover:bg-subtle'
                }`}
              >
                {r.label}
              </button>
            ))}
          </div>
          <span className="text-xs text-text-muted">
            {visibleYears.length} year{visibleYears.length !== 1 ? 's' : ''}
          </span>
        </div>
        <ForecastChart
          data={forecast}
          scenario={scenario}
          showBands={showBands}
          endYear={rangeEndYear}
          rangeKey={rangeKey}
          goals={goals ?? undefined}
          mortgagePayoffYears={forecast.mortgagePayoffYears}
          fiMetrics={scenarioFIMetrics}
        />
      </Card>

      {/* FI Metrics panel */}
      {scenarioFIMetrics && <FIMetricsPanel metrics={scenarioFIMetrics} />}

      {/* Historical Backtest — real 1928-present rolling windows, vs. the statistical Monte Carlo above */}
      <HistoricalBacktestPanel />

      {/* Retirement Readiness Score — expanded breakdown */}
      {forecast.readinessScore && (
        <ReadinessScoreCard score={forecast.readinessScore} expanded />
      )}

      {/* Summary cards — stack on small, 3-col on sm+ */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <SummaryCard
          label={`At Retirement (${forecast.retirementYear})`}
          value={retirementYearData?.total ?? 0}
          age={retirementYearData && retirementYearData.age + (forecast.retirementYear - retirementYearData.year)}
        />
        <SummaryCard label="At Age 80" value={age80Data?.total ?? 0} age={80} />
        <SummaryCard label="At Age 100" value={age100Data?.total ?? 0} age={100} />
      </div>

      {/* Goal projections */}
      {goalProjections.length > 0 && (
        <Card title="Goal Projections">
          <div className="space-y-3">
            {goalProjections.map(({ goal, hitYear, hitAge }) => (
              <div
                key={goal.id}
                className="flex items-center justify-between gap-4 py-2 border-b border-border-default/50 last:border-0"
              >
                <div className="min-w-0">
                  <div className="text-sm font-medium text-text-primary truncate">{goal.name}</div>
                  <div className="text-xs text-text-tertiary mt-0.5">
                    Target: {formatCurrency(goal.targetAmount)}
                  </div>
                </div>
                {hitYear ? (
                  <div className="text-right shrink-0">
                    <div className="text-sm font-semibold text-positive">{hitYear}</div>
                    <div className="text-xs text-text-tertiary">age {hitAge}</div>
                  </div>
                ) : (
                  <div className="text-xs text-text-muted shrink-0">Not reached</div>
                )}
              </div>
            ))}
          </div>
        </Card>
      )}

      {/* Account breakdown at retirement */}
      {retirementYearData && forecast.accountMeta.length > 1 && (
        <Card title={`Account Breakdown at Retirement (${forecast.retirementYear})`}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm min-w-[400px]">
              <thead>
                <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                  <th className="pb-2 font-medium">Account</th>
                  <th className="pb-2 font-medium text-right">Projected Balance</th>
                  <th className="pb-2 font-medium text-right">% of Portfolio</th>
                </tr>
              </thead>
              <tbody>
                {forecast.accountMeta
                  .filter((m) => (retirementYearData.accounts[m.id] ?? 0) > 0)
                  .sort(
                    (a, b) =>
                      (retirementYearData.accounts[b.id] ?? 0) -
                      (retirementYearData.accounts[a.id] ?? 0),
                  )
                  .map((m) => {
                    const bal = retirementYearData.accounts[m.id] ?? 0
                    const pct =
                      retirementYearData.total > 0
                        ? ((bal / retirementYearData.total) * 100).toFixed(1)
                        : '0.0'
                    return (
                      <tr key={m.id} className="border-b border-border-default/50">
                        <td className="py-3 text-text-primary">{m.name}</td>
                        <td className="py-3 text-text-primary font-medium text-right">
                          {formatCurrency(bal)}
                        </td>
                        <td className="py-3 text-text-tertiary text-right">{pct}%</td>
                      </tr>
                    )
                  })}
                <tr>
                  <td className="py-3 font-semibold text-text-primary">Total</td>
                  <td className="py-3 font-semibold text-text-primary text-right">
                    {formatCurrency(retirementYearData.total)}
                  </td>
                  <td />
                </tr>
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {/* Active contributions used by this model */}
      {contributions && contributions.length > 0 && (
        <ContributionsPanel
          contributions={contributions}
          accountMeta={forecast.accountMeta}
        />
      )}

      {/* Settings nudge */}
      <div className="text-xs text-text-muted">
        Adjust assumptions in{' '}
        <Link to="/settings" className="text-accent-text hover:text-accent-text underline">
          Settings
        </Link>
        . Add contributions to individual accounts on each{' '}
        <Link to="/accounts" className="text-accent-text hover:text-accent-text underline">
          account detail page
        </Link>
        .
      </div>
    </div>
  )
}

function FIMetricsPanel({ metrics: m }: { metrics: FIMetrics }) {
  const pct = Math.min(m.fiProgressPct, 100)

  return (
    <Card title="Financial Independence Metrics">
      <div className="space-y-5">
        {/* FI Number progress bar */}
        {m.fiNumber > 0 && (
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-sm text-text-secondary">FI Number <InfoTip term="fiNumber" /></span>
              <span className="text-sm font-semibold text-text-primary">{formatCurrency(m.fiNumber)}</span>
            </div>
            <div className="h-2 bg-subtle rounded-full overflow-hidden">
              <div
                className="h-full rounded-full bg-accent-hover transition-all"
                style={{ width: `${pct}%` }}
              />
            </div>
            <div className="flex items-center justify-between mt-1">
              <span className="text-xs text-text-tertiary">
                {formatCurrency(m.currentPortfolio)} saved
              </span>
              <span className="text-xs text-text-tertiary">{pct.toFixed(1)}%</span>
            </div>
          </div>
        )}

        {/* Everything but the savings rate is measured against retirement
            spending, so without it there's nothing meaningful to show. */}
        {m.fiNumber <= 0 && (
          <SetupPrompt
            message="Set your retirement spending to see your FI Number, when you could reach it, and how likely your plan is to last."
            buttonLabel="Set retirement spending"
            title="Retirement Spending"
          >
            {(done) => <RetirementSetupForm onDone={done} />}
          </SetupPrompt>
        )}

        <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
          {/* Time to FI */}
          {m.fiNumber > 0 && (
          <div>
            <div className="text-xs text-text-tertiary mb-0.5">Time to FI</div>
            {m.timeToFIYear ? (
              <>
                <div className="text-base font-semibold text-positive">{m.timeToFIYear}</div>
                <div className="text-xs text-text-muted">age {m.timeToFIAge}</div>
              </>
            ) : (
              <div className="text-sm text-text-muted">Not projected</div>
            )}
          </div>
          )}

          {/* Coast FI */}
          {m.fiNumber > 0 && (
          <div>
            <div className="text-xs text-text-tertiary mb-0.5">Coast FI Target <InfoTip term="coastFi" /></div>
            {m.coastFIAmount > 0 ? (
              <>
                <div className={`text-base font-semibold ${m.isCoastFI ? 'text-positive' : 'text-text-primary'}`}>
                  {formatCurrency(m.coastFIAmount)}
                </div>
                <div className="text-xs text-text-muted">
                  {m.isCoastFI ? 'Reached — stop contributing and still FI' : 'Not yet reached'}
                </div>
              </>
            ) : (
              <div className="text-sm text-text-muted">—</div>
            )}
          </div>
          )}

          {/* Savings rate */}
          {m.savingsRate != null && (
            <div>
              <div className="text-xs text-text-tertiary mb-0.5">Savings Rate</div>
              <div className="text-base font-semibold text-text-primary">
                {(m.savingsRate * 100).toFixed(1)}%
              </div>
              <div className="text-xs text-text-muted">of gross income</div>
            </div>
          )}

          {/* Withdrawal rate */}
          {m.withdrawalRate != null && (
            <div>
              <div className="text-xs text-text-tertiary mb-0.5">Withdrawal Rate</div>
              <div className={`text-base font-semibold ${m.withdrawalRateWarn ? 'text-warning' : 'text-text-primary'}`}>
                {(m.withdrawalRate * 100).toFixed(1)}%
              </div>
              {m.withdrawalRateWarn ? (
                <div className="text-xs text-warning">Above 4.5% — consider adjusting</div>
              ) : (
                <div className="text-xs text-text-muted">at retirement</div>
              )}
            </div>
          )}

          {/* Monte Carlo */}
          {m.fiNumber > 0 && (
            <div>
              <div className="text-xs text-text-tertiary mb-0.5">Monte Carlo Success</div>
              <div className={`text-base font-semibold ${m.monteCarloSuccessPct >= 80 ? 'text-positive' : m.monteCarloSuccessPct >= 60 ? 'text-warning' : 'text-negative'}`}>
                {m.monteCarloSuccessPct.toFixed(0)}%
              </div>
              <div className="text-xs text-text-muted">of 500 simulations</div>
            </div>
          )}
        </div>
      </div>
    </Card>
  )
}

function SummaryCard({
  label,
  value,
  age,
}: {
  label: string
  value: number
  age?: number
}) {
  return (
    <div className="bg-surface border border-border-default rounded-lg px-4 py-4">
      <div className="text-xs text-text-tertiary mb-1 truncate">{label}</div>
      <div className="text-lg lg:text-xl font-bold text-text-primary truncate">{formatCurrency(value)}</div>
      {age != null && <div className="text-xs text-text-muted mt-0.5">age {age}</div>}
    </div>
  )
}

function ContributionsPanel({
  contributions,
  accountMeta,
}: {
  contributions: Contribution[]
  accountMeta: AccountMeta[]
}) {
  const { data: accounts } = useAccounts()
  const { data: members } = useHouseholdMembers()
  const accountNameById = Object.fromEntries(accountMeta.map((m) => [m.id, m.name]))
  const salaryOf = (c: Contribution) => {
    const acc = accounts?.find((a) => a.id === c.accountId)
    return acc ? annualSalary(accountOwner(acc, members ?? [])) : 0
  }
  const totalAnnual = contributions.reduce((sum, c) => sum + annualContribution(c, salaryOf(c)).employee, 0)

  return (
    <Card title="Contributions Used in This Model">
      <div className="overflow-x-auto">
        <table className="w-full text-sm min-w-[480px]">
          <thead>
            <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
              <th className="pb-2 font-medium">Account</th>
              <th className="pb-2 font-medium text-right">Amount</th>
              <th className="pb-2 font-medium text-right">Annual&nbsp;+</th>
              <th className="pb-2 font-medium text-right">Match</th>
              <th className="pb-2 font-medium text-right">Period</th>
            </tr>
          </thead>
          <tbody>
            {contributions.map((c) => {
              const accountName = accountNameById[c.accountId] ?? 'Unknown account'
              const startYear = c.startDate ? c.startDate.slice(0, 4) : 'now'
              const endYear = c.endDate ? c.endDate.slice(0, 4) : 'retirement'
              const period =
                startYear === 'now' && endYear === 'retirement'
                  ? 'now → retirement'
                  : `${startYear} → ${endYear}`
              return (
                <tr key={c.id} className="border-b border-border-default/50 last:border-0">
                  <td className="py-3 text-text-primary">
                    <div>{accountName}</div>
                    {c.label && <div className="text-xs text-text-tertiary">{c.label}</div>}
                  </td>
                  <td className="py-3 text-text-primary font-medium text-right">
                    {describeAmount(c, formatCurrency)}
                  </td>
                  <td className="py-3 text-right text-text-secondary">
                    {c.annualIncreasePct > 0 ? `+${(c.annualIncreasePct * 100).toFixed(1)}%` : '—'}
                  </td>
                  <td className="py-3 text-right text-text-secondary">
                    {describeMatch(c.matchTiers)}
                  </td>
                  <td className="py-3 text-right text-text-tertiary text-xs">{period}</td>
                </tr>
              )
            })}
          </tbody>
          <tfoot>
            <tr className="border-t border-border-strong">
              <td className="pt-3 text-text-primary font-semibold">Total</td>
              <td className="pt-3 text-text-primary font-semibold text-right">
                {formatCurrency(totalAnnual)}/yr
              </td>
              <td className="pt-3" colSpan={3} />
            </tr>
          </tfoot>
        </table>
      </div>
    </Card>
  )
}
