import { useState, useMemo } from 'react'
import { SlidersHorizontal } from '@phosphor-icons/react'
import { useNetWorthCurrent, useNetWorthHistory, useNetWorthAccountHistory } from '../hooks/useNetWorth'
import { useGoals } from '../hooks/useGoals'
import { useHoldings } from '../hooks/useHoldings'
import { useForecast } from '../hooks/useForecast'
import { useAccounts } from '../hooks/useAccounts'
import { useMortgages } from '../hooks/useMortgages'
import { useHousehold } from '../hooks/useHousehold'
import { Card } from '../components/ui/Card'
import { EmptyState } from '../components/ui/EmptyState'
import { ReadinessScoreCard } from '../components/ReadinessScoreCard'
import { NetWorthChart, CHART_RANGES } from '../components/charts/NetWorthChart'
import { NetWorthAccountsChart } from '../components/charts/NetWorthAccountsChart'
import { withLiveBalances } from '../utils/netWorthStack'
import { AllocationPieChart } from '../components/charts/AllocationPieChart'
import { GoalProgressBar } from '../components/charts/GoalProgressBar'
import { DashboardGrid } from '../components/Dashboard/DashboardGrid'
import { CashFlowCard } from '../components/Dashboard/CashFlowCard'
import { PortfolioCheckupCard } from '../components/Dashboard/PortfolioCheckupCard'
import { NetWorthSummaryCard } from '../components/Dashboard/NetWorthSummaryCard'
import { SpendingSummaryCard } from '../components/Dashboard/SpendingSummaryCard'
import { RecentTransactionsCard } from '../components/Dashboard/RecentTransactionsCard'
import { TopMoversCard } from '../components/Dashboard/TopMoversCard'
import { DebtPayoffCard } from '../components/Dashboard/DebtPayoffCard'
import { MortgagePayoffCard } from '../components/Dashboard/MortgagePayoffCard'
import { HouseholdBreakdownCard } from '../components/Dashboard/HouseholdBreakdownCard'
import { useDashboardLayoutStore, useWidgetOption, type DashboardWidgetId } from '../store/dashboardLayoutStore'
import { useCashFlow } from '../hooks/useCashFlow'
import { formatCurrency } from '../utils/currency'
import { monthsAgo, yearsAgo, daysAgo, today } from '../utils/dates'
import { CASH_BALANCE_ACCOUNT_TYPES, LIABILITY_ACCOUNT_TYPES, TAX_TREATMENT_LABELS } from '../api/client'
import type { Goal, AccountBalance } from '../api/client'
import { PageGuide } from '../components/PageGuide'
import { GettingStartedCard } from '../components/Dashboard/GettingStartedCard'
import { ContributionCheckCard } from '../components/Dashboard/ContributionCheckCard'
import { InfoTip } from '../components/ui/InfoTip'

function resolveGoalCurrentValue(
  goal: Goal,
  netWorth: number,
  accountBalances: AccountBalance[],
  assetClassTotals: Record<string, number>,
): number {
  if ((goal.goalType === 'account' || goal.goalType === 'spend') && goal.accountId) {
    return accountBalances.find((a) => a.accountId === goal.accountId)?.balance ?? 0
  }
  if (goal.goalType === 'account_type' && goal.accountTypeFilter) {
    return accountBalances
      .filter((a) => a.accountType === goal.accountTypeFilter)
      .reduce((sum, a) => sum + a.balance, 0)
  }
  if (goal.goalType === 'asset_class' && goal.assetClassFilter) {
    return assetClassTotals[goal.assetClassFilter] ?? 0
  }
  return netWorth
}

export function DashboardPage() {
  const { data: nw, isLoading: nwLoading, error: nwError, mutate: retryNw } = useNetWorthCurrent()
  const { data: forecast } = useForecast()
  const [selectedRange, setSelectedRange] = useState('3Y')
  const [editing, setEditing] = useState(false)
  const range = CHART_RANGES.find((r) => r.label === selectedRange) ?? CHART_RANGES[4]
  const fromDate = range.allTime ? '1900-01-01' : range.years ? yearsAgo(range.years) : range.months ? monthsAgo(range.months) : daysAgo(range.days ?? 14)
  const { data: history } = useNetWorthHistory(fromDate, today(), range.interval)
  // The Detailed view stacks the same history by account; only fetch it when chosen.
  const detailed = useWidgetOption('netWorthChart', 'view') === 'detailed'
  const { data: accountHistory, isLoading: accountHistoryLoading } = useNetWorthAccountHistory(
    fromDate,
    today(),
    range.interval,
    detailed,
  )
  // Replace the last history bucket (current in-progress period) with the live
  // current net worth so the chart always ends at the same value the card shows.
  const chartData = useMemo(() => {
    if (!history?.length || !nw) return history ?? []
    const points = [...history]
    points[points.length - 1] = {
      date: today(),
      assets: nw.totalAssets,
      liabilities: nw.totalLiabilities,
      netWorth: nw.totalNetWorth,
    }
    return points
  }, [history, nw])

  const detailedHistory = useMemo(
    () =>
      accountHistory && nw
        ? withLiveBalances(accountHistory, Object.fromEntries(nw.accounts.map((a) => [a.accountId, a.balance])), today())
        : accountHistory,
    [accountHistory, nw],
  )

  const { data: goals } = useGoals()
  const { data: holdings } = useHoldings()
  const { data: accounts } = useAccounts()
  const { data: householdMembers } = useHousehold()

  const { data: mortgages } = useMortgages()
  // Only fetch cash-flow data when the card is on the dashboard.
  const cashFlowShown = !useDashboardLayoutStore((s) => s.hidden).includes('cashFlow')
  const cashFlow = useCashFlow(cashFlowShown)
  // A mortgage counts as debt too, so someone with only a mortgage still gets the card.
  const hasDebt =
    (accounts ?? []).some((a) => LIABILITY_ACCOUNT_TYPES.has(a.accountType)) || (mortgages ?? []).length > 0
  const hasPartner = (householdMembers ?? []).some((m) => m.relationship === 'partner')

  const accountBalances: AccountBalance[] = nw?.accounts ?? []
  const assetClassTotals = useMemo(() => {
    const totals: Record<string, number> = {}
    for (const h of holdings ?? []) {
      totals[h.assetClass] = (totals[h.assetClass] ?? 0) + h.totalValue
    }
    for (const a of accountBalances) {
      if (CASH_BALANCE_ACCOUNT_TYPES.has(a.accountType) && a.balance > 0) {
        totals['cash_equivalent'] = (totals['cash_equivalent'] ?? 0) + a.balance
      }
    }
    return totals
  }, [holdings, accountBalances])

  const activeGoals = goals?.filter((g) => !g.isAchieved) ?? []

  const widgetContent: Partial<Record<DashboardWidgetId, React.ReactNode>> = {
    summary: nwError && !nw ? (
      <Card>
        <div className="text-sm text-text-secondary">
          Couldn't load your net worth.{' '}
          <button onClick={() => retryNw()} className="text-accent-text hover:text-accent">Retry</button>
        </div>
      </Card>
    ) : (
      <NetWorthSummaryCard nw={nw} loading={nwLoading} />
    ),


    netWorthChart: (
      <Card title="Net Worth Over Time">
        {detailed ? (
          <NetWorthAccountsChart
            history={detailedHistory}
            isLoading={accountHistoryLoading}
            selectedRange={selectedRange}
            onRangeChange={setSelectedRange}
            interval={range.interval}
          />
        ) : (
          <NetWorthChart
            data={chartData}
            selectedRange={selectedRange}
            onRangeChange={setSelectedRange}
            interval={range.interval}
          />
        )}
      </Card>
    ),

    spendingSummary: <SpendingSummaryCard />,

    recentTransactions: <RecentTransactionsCard />,

    allocation: (
      <Card title="Allocation">
        {nw ? <AllocationPieChart data={nw} holdings={holdings} /> : (
          <div className="text-text-tertiary text-sm">Loading...</div>
        )}
      </Card>
    ),

    goals: (
      <Card title="Goals">
        {activeGoals.length === 0 ? (
          <EmptyState action={{ label: 'Create a goal', to: '/goals' }}>
            Set a target, like an emergency fund or a down payment, and track your progress here.
          </EmptyState>
        ) : (
          <div className="space-y-5">
            {activeGoals.slice(0, 3).map((goal) => {
              const currentValue = resolveGoalCurrentValue(
                goal,
                nw?.totalNetWorth ?? 0,
                accountBalances,
                assetClassTotals,
              )
              return (
                <GoalProgressBar
                  key={goal.id}
                  goal={goal}
                  currentValue={currentValue}
                />
              )
            })}
          </div>
        )}
      </Card>
    ),

    topMovers: <TopMoversCard />,

    debtPayoff: hasDebt ? <DebtPayoffCard /> : null,

    mortgagePayoff: (mortgages ?? []).length > 0 ? <MortgagePayoffCard /> : null,

    cashFlow: cashFlow.hasData ? <CashFlowCard /> : null,

    portfolioCheckup: (holdings ?? []).length > 0 ? <PortfolioCheckupCard /> : null,

    household: hasPartner ? <HouseholdBreakdownCard /> : null,

    readiness: forecast?.readinessScore ? (
      <ReadinessScoreCard score={forecast.readinessScore} />
    ) : null,

    taxBreakdown: nw ? (() => {
      const totalAssets = nw.totalAssets || 1
      const buckets = (['taxable', 'tax_deferred', 'tax_free'] as const)
        .map((key) => ({
          key,
          label: TAX_TREATMENT_LABELS[key],
          total: (nw.byTaxTreatment ?? []).filter((t) => t.taxTreatment === key).reduce((s, t) => s + t.total, 0),
        }))
        .filter((b) => b.total > 0)
      // Nothing to split until there are assets.
      if (!buckets.length) return null

      return (
        <Card title={<>Tax treatment breakdown <InfoTip term="taxTreatment" /></>}>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            {buckets.map((b) => (
              <div key={b.key} className="bg-subtle/50 rounded-lg p-4">
                <div className="text-xs text-text-tertiary mb-1">{b.label}</div>
                <div className="text-lg lg:text-xl font-bold text-text-primary truncate">
                  {formatCurrency(b.total)}
                </div>
                <div className="text-xs text-text-tertiary mt-1">
                  {((b.total / totalAssets) * 100).toFixed(1)}% of assets
                </div>
              </div>
            ))}
          </div>
        </Card>
      )
    })() : null,
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text-primary">Dashboard</h1>
            <PageGuide id="dashboard" />
          </div>
          <p className="text-text-tertiary text-sm mt-1">Your financial snapshot</p>
        </div>
        {!editing && (
          <button
            onClick={() => setEditing(true)}
            className="flex items-center gap-1.5 text-sm text-text-tertiary hover:text-text-primary border border-border-default rounded-lg px-3 py-1.5 transition-colors shrink-0"
          >
            <SlidersHorizontal size={16} weight="bold" />
            <span className="hidden sm:inline">Customize</span>
          </button>
        )}
      </div>

      <GettingStartedCard />
      <ContributionCheckCard />

      <DashboardGrid content={widgetContent} editing={editing} onDone={() => setEditing(false)} />
    </div>
  )
}
