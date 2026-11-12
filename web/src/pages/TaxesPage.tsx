import { useState, useMemo } from 'react'
import useSWR from 'swr'
import { parseISO } from 'date-fns'
import { taxApi, TaxAnalysis, LadderRung, RMDYear, WithdrawalBucket, ACAThreshold } from '../api/client'
import { useProfile } from '../hooks/useProfile'
import { PageGuide } from '../components/PageGuide'
import { SetupPrompt } from '../components/SetupPrompt'
import { InfoTip } from '../components/ui/InfoTip'
import { TaxSetupForm } from '../components/SetupForms'
import { useHouseholdMembers } from '../hooks/useHouseholdMembers'

// ── Fetcher ───────────────────────────────────────────────────────────────────

function useTaxAnalysis(conversion?: number) {
  return useSWR<TaxAnalysis>(
    conversion != null ? `/taxes?conversion=${conversion}` : '/taxes',
    () => taxApi.analyze(conversion),
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

const fmt$ = (n: number) =>
  n.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })

const fmtPct = (n: number, decimals = 1) => `${(n * 100).toFixed(decimals)}%`

type Tab = 'bracket' | 'roth' | 'withdrawal' | 'aca'

const TABS: { id: Tab; label: string }[] = [
  { id: 'bracket', label: 'Tax Bracket' },
  { id: 'roth', label: 'Roth Conversion' },
  { id: 'withdrawal', label: 'Withdrawal & RMDs' },
  { id: 'aca', label: 'ACA Subsidy' },
]

const FILING_STATUS_LABELS: Record<string, string> = {
  single: 'Single',
  married_filing_jointly: 'Married Filing Jointly',
}

// ── Sub-components ────────────────────────────────────────────────────────────

function InfoCard({ children }: { children: React.ReactNode }) {
  return (
    <div className="bg-accent-tint-bg border border-accent/20 rounded-lg p-4 text-sm text-text-secondary">
      {children}
    </div>
  )
}

function StatBox({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="bg-surface border border-border-default rounded-lg p-4">
      <div className="text-xs text-text-tertiary mb-1">{label}</div>
      <div className="text-xl font-semibold text-text-primary">{value}</div>
      {sub && <div className="text-xs text-text-muted mt-0.5">{sub}</div>}
    </div>
  )
}

// ── Tab: Tax Bracket ──────────────────────────────────────────────────────────

function BracketTab({ data }: { data: TaxAnalysis }) {
  const { bracketAnalysis: b } = data
  const marginalPct = Math.round(b.marginalRate * 100)
  const effectivePct = (b.effectiveRate * 100).toFixed(1)

  // Contribution strategy recommendation
  let strategyTitle = ''
  let strategyBody = ''
  if (b.marginalRate >= 0.24) {
    strategyTitle = 'Prioritize traditional (pre-tax) contributions'
    strategyBody = `You're in the ${marginalPct}% federal bracket. Contributions to a traditional 401(k) or IRA reduce your taxable income now at that rate. Convert to Roth during early retirement when your income is lower.`
  } else if (b.marginalRate >= 0.12) {
    strategyBody = `You're in the ${marginalPct}% bracket — a good time for Roth contributions if you expect higher income later, or traditional contributions to preserve flexibility. Roth conversions this year cost relatively little tax.`
    strategyTitle = 'Consider splitting between traditional and Roth'
  } else {
    strategyTitle = 'Favor Roth contributions'
    strategyBody = `You're in the ${marginalPct}% bracket — one of the lowest rates you may ever pay. Roth contributions and conversions are highly tax-efficient right now. Future withdrawals will be tax-free.`
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatBox label="Filing Status" value={FILING_STATUS_LABELS[b.filingStatus] ?? b.filingStatus} />
        <StatBox label="Gross Income" value={fmt$(b.grossIncome)} sub="annual" />
        <StatBox label="Estimated AGI" value={fmt$(b.estimatedAgi)} sub={`after ${fmt$(b.standardDeduction)} std deduction`} />
        <StatBox label="Total Tax" value={fmt$(b.totalTaxOwed)} sub={`${effectivePct}% effective`} />
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatBox label="Federal Tax" value={fmt$(b.federalTaxOwed)} />
        <StatBox label="State Tax" value={fmt$(b.stateTaxOwed)} />
        <StatBox label="Marginal Rate" value={`${marginalPct}%`} />
        <StatBox label="Room in Bracket" value={fmt$(b.bracketHeadroom)} sub="until next bracket" />
      </div>

      {/* Bracket breakdown */}
      <div>
        <h3 className="text-sm font-medium text-text-secondary mb-3">Federal Tax Brackets</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border-default text-text-tertiary text-left">
                <th className="pb-2 font-medium">Rate</th>
                <th className="pb-2 font-medium">Income Range</th>
                <th className="pb-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border-default">
              {b.allBrackets.map((br) => {
                const inBracket = b.estimatedAgi >= br.min && (br.max === 0 || b.estimatedAgi < br.max)
                return (
                  <tr
                    key={br.rate}
                    className={inBracket ? 'bg-accent-tint-bg' : ''}
                  >
                    <td className="py-2 font-medium text-text-primary">{Math.round(br.rate * 100)}%</td>
                    <td className="py-2 text-text-secondary">
                      {fmt$(br.min)} – {br.max === 0 ? '∞' : fmt$(br.max)}
                    </td>
                    <td className="py-2">
                      {inBracket && (
                        <span className="text-xs bg-accent text-white px-2 py-0.5 rounded-full">
                          You are here
                        </span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* Contribution strategy */}
      <InfoCard>
        <p className="font-medium text-text-primary mb-1">{strategyTitle}</p>
        <p>{strategyBody}</p>
      </InfoCard>

      {b.grossIncome === 0 && (
        <p className="text-sm text-text-muted">
          Set your monthly income in Settings to get personalized bracket analysis.
        </p>
      )}
    </div>
  )
}

// ── Conversion Window Timeline ────────────────────────────────────────────────

interface WindowInfo {
  currentAge: number
  retirementAge: number
  windowEnd: number
  ssStartAge: number
  rmdAge: number
}

function ConversionWindowTimeline({ w }: { w: WindowInfo }) {
  const alreadyRetired = w.currentAge >= w.retirementAge
  const windowYears = Math.max(0, w.windowEnd - w.retirementAge)

  const phases = alreadyRetired
    ? [
        {
          label: 'Optimal window',
          ages: `Ages ${w.retirementAge}–${w.windowEnd}`,
          note: windowYears > 0 ? `${windowYears}-year window` : 'Narrow window',
          highlight: true,
        },
        {
          label: 'SS & RMDs',
          ages: `Age ${w.windowEnd}+`,
          note: 'Income rises',
          highlight: false,
        },
      ]
    : [
        {
          label: 'Working years',
          ages: `Ages ${w.currentAge}–${w.retirementAge}`,
          note: 'Defer conversions',
          highlight: false,
        },
        {
          label: 'Optimal window',
          ages: `Ages ${w.retirementAge}–${w.windowEnd}`,
          note: windowYears > 0 ? `${windowYears}-year window` : 'Narrow window',
          highlight: true,
        },
        {
          label: 'SS & RMDs',
          ages: `Age ${w.windowEnd}+`,
          note: 'Income rises',
          highlight: false,
        },
      ]

  return (
    <div>
      <div className="text-xs font-medium text-text-secondary mb-3 uppercase tracking-wider">
        Optimal conversion timing
      </div>
      <div className="flex gap-2">
        {phases.map((phase) => (
          <div
            key={phase.label}
            className={`flex-1 rounded-lg p-3 border ${
              phase.highlight
                ? 'bg-positive/10 border-positive/30'
                : 'bg-subtle border-border-default'
            }`}
          >
            <div
              className={`text-xs font-semibold ${
                phase.highlight ? 'text-positive' : 'text-text-secondary'
              }`}
            >
              {phase.label}
            </div>
            <div className="text-xs text-text-muted mt-0.5">{phase.ages}</div>
            <div
              className={`text-xs mt-1.5 ${
                phase.highlight ? 'text-positive' : 'text-text-muted'
              }`}
            >
              {phase.note}
            </div>
          </div>
        ))}
      </div>
      <p className="text-xs text-text-muted mt-2">
        The optimal window is when earned income stops but Social Security
        {w.ssStartAge < w.rmdAge ? ` (age ${w.ssStartAge})` : ''} and RMDs
        {` (age ${w.rmdAge})`} haven't begun — your lowest-income years.
      </p>
    </div>
  )
}

// ── Tab: Roth Conversion Ladder ───────────────────────────────────────────────

function RothTab({ data, onConversionChange }: { data: TaxAnalysis; onConversionChange: (v: number | undefined) => void }) {
  const { rothLadder: ladder, bracketAnalysis: b } = data
  const { data: profile } = useProfile()
  const [inputVal, setInputVal] = useState('')

  const handleApply = () => {
    const v = parseFloat(inputVal)
    onConversionChange(isNaN(v) || inputVal === '' ? undefined : v)
  }
  const handleReset = () => {
    setInputVal('')
    onConversionChange(undefined)
  }

  // Projected retirement bracket — single source of truth is the backend
  // (RothLadder.retirementRate/retirementGrossIncome), which also gates
  // recommendedAnnual and ACA MAGI off the same figures. Don't recompute
  // this independently here; a client/server mismatch is exactly what let
  // the "recommended" conversion amount and ACA MAGI silently disagree with
  // this bracket comparison before.
  const retirementBracket =
    ladder.retirementRate != null
      ? { grossIncome: ladder.retirementGrossIncome, rate: ladder.retirementRate }
      : null

  // Conversion window timing
  const windowInfo = useMemo((): WindowInfo | null => {
    if (!profile?.birthday) return null
    const today = new Date()
    const birth = parseISO(profile.birthday)
    const currentAge = Math.floor(
      (today.getTime() - birth.getTime()) / (1000 * 60 * 60 * 24 * 365.25),
    )
    const retirementAge = profile.retirementAge ?? 65
    const ssStartAge = profile.socialSecurityStartAge ?? 67
    const rmdAge = 73
    const windowEnd = Math.min(ssStartAge, rmdAge)
    return { currentAge, retirementAge, windowEnd, ssStartAge, rmdAge }
  }, [profile])

  const currentRate = b.marginalRate
  const retirementRate = retirementBracket?.rate ?? null
  const convertingNowIsCheaper =
    retirementRate !== null && currentRate <= retirementRate
  const bracketDiff =
    retirementRate !== null
      ? Math.round((currentRate - retirementRate) * 100)
      : null

  return (
    <div className="space-y-6">
      {/* Stat row */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatBox
          label="Tax-Deferred Balance"
          value={fmt$(ladder.taxDeferredBalance)}
          sub="traditional IRA / 401(k)"
        />
        <StatBox
          label="Roth Balance"
          value={fmt$(ladder.rothBalance)}
          sub="Roth IRA / 401(k) / HSA"
        />
        <StatBox
          label="Current Bracket"
          value={`${Math.round(currentRate * 100)}%`}
          sub="marginal rate today"
        />
        {retirementBracket ? (
          <StatBox
            label="Projected Retirement Bracket"
            value={`${Math.round(retirementRate! * 100)}%`}
            sub={`on ${fmt$(retirementBracket.grossIncome)}/yr income`}
          />
        ) : (
          <StatBox
            label="Bracket Capacity"
            value={fmt$(ladder.recommendedAnnual)}
            sub={`fills ${Math.round(currentRate * 100)}% bracket`}
          />
        )}
      </div>

      {/* Override control */}
      <div className="flex items-end gap-3 flex-wrap">
        <div>
          <label className="block text-xs text-text-tertiary mb-1">
            Override conversion amount ($) <InfoTip term="rothConversion" />
          </label>
          <input
            type="number"
            min={0}
            step={1000}
            placeholder={String(Math.round(ladder.recommendedAnnual))}
            value={inputVal}
            onChange={(e) => setInputVal(e.target.value)}
            className="w-44 bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent"
          />
        </div>
        <button
          onClick={handleApply}
          className="px-4 py-2 text-sm rounded-lg bg-accent text-white hover:bg-accent/90 transition-colors"
        >
          Apply
        </button>
        {ladder.conversionAmount !== ladder.recommendedAnnual && (
          <button
            onClick={handleReset}
            className="px-4 py-2 text-sm rounded-lg bg-subtle text-text-secondary hover:bg-muted transition-colors"
          >
            Reset to recommended
          </button>
        )}
      </div>

      {ladder.taxDeferredBalance === 0 ? (
        <InfoCard>
          No tax-deferred accounts found. Add a traditional IRA or 401(k) to see
          conversion projections.
        </InfoCard>
      ) : (
        <>
          {/* Bracket comparison recommendation */}
          {retirementBracket && bracketDiff !== null ? (
            convertingNowIsCheaper ? (
              <InfoCard>
                <p className="font-medium text-text-primary mb-1">
                  Converting now is tax-efficient
                </p>
                <p>
                  Your current bracket ({Math.round(currentRate * 100)}%) is at or below your
                  projected retirement bracket ({Math.round(retirementRate! * 100)}%). Converting{' '}
                  {fmt$(ladder.conversionAmount)} now and paying tax at {Math.round(currentRate * 100)}%
                  is equal to or cheaper than withdrawing it in retirement. The bracket capacity shown
                  above is the maximum you can convert without crossing into a higher bracket this year.
                </p>
              </InfoCard>
            ) : (
              <div className="bg-warning/10 border border-warning/30 rounded-lg p-4 text-sm text-text-secondary">
                <p className="font-medium text-text-primary mb-1">
                  Target your early retirement window instead
                </p>
                <p>
                  You're currently in the {Math.round(currentRate * 100)}% bracket, but your projected
                  retirement income puts you in the {Math.round(retirementRate! * 100)}% bracket —{' '}
                  {bracketDiff} percentage point{bracketDiff !== 1 ? 's' : ''} lower. Converting today
                  costs more in tax than waiting. Your most efficient window is{' '}
                  {windowInfo
                    ? `ages ${windowInfo.retirementAge}–${windowInfo.windowEnd}`
                    : 'early retirement'}{' '}
                  — after income from work stops but before Social Security and RMDs begin.
                </p>
                {windowInfo && windowInfo.retirementAge > windowInfo.currentAge && (
                  <p className="mt-2 text-xs text-text-muted">
                    In the meantime, consider maximizing traditional (pre-tax) contributions to reduce
                    your current taxable income at {Math.round(currentRate * 100)}%.
                  </p>
                )}
              </div>
            )
          ) : (
            <InfoCard>
              <p className="font-medium text-text-primary mb-1">How the ladder works</p>
              <p>
                Convert {fmt$(ladder.conversionAmount)} this year, pay tax now at your current
                bracket. The converted funds become accessible{' '}
                <strong>penalty-free in 5 years</strong> (the "5-year rule"). Set your retirement
                income in Settings to see a bracket comparison.
              </p>
            </InfoCard>
          )}

          {/* Conversion window timeline */}
          {windowInfo && <ConversionWindowTimeline w={windowInfo} />}

          {/* 10-year projection table */}
          <div>
            <h3 className="text-sm font-medium text-text-secondary mb-3">
              10-Year Projection ({(ladder.returnRateUsed * 100).toFixed(0)}% growth)
            </h3>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border-default text-text-tertiary text-left">
                    <th className="pb-2 font-medium">Year</th>
                    <th className="pb-2 font-medium">Age</th>
                    <th className="pb-2 font-medium">Convert</th>
                    <th className="pb-2 font-medium">Tax Cost</th>
                    <th className="pb-2 font-medium">Access Year</th>
                    <th className="pb-2 font-medium">Trad Balance</th>
                    <th className="pb-2 font-medium">Roth Balance</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border-default">
                  {ladder.rungs.map((rung: LadderRung) => (
                    <tr key={rung.year} className="hover:bg-subtle">
                      <td className="py-2 text-text-primary">{rung.year}</td>
                      <td className="py-2 text-text-secondary">
                        {rung.age > 0 ? rung.age : '—'}
                      </td>
                      <td className="py-2 font-medium">{fmt$(rung.conversionAmount)}</td>
                      <td className="py-2 text-negative">{fmt$(rung.taxCost)}</td>
                      <td className="py-2 text-positive">
                        {rung.accessYear}{' '}
                        {rung.accessAge > 0 ? `(age ${rung.accessAge})` : ''}
                      </td>
                      <td className="py-2 text-text-secondary">{fmt$(rung.tradBalance)}</td>
                      <td className="py-2 text-positive">{fmt$(rung.rothBalance)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

// ── Tab: Withdrawal Order & RMDs ──────────────────────────────────────────────

function WithdrawalTab({ data }: { data: TaxAnalysis }) {
  const { withdrawalOrder: wo, rmdProjection } = data

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
        <StatBox label="Total Portfolio" value={fmt$(wo.totalPortfolio)} />
        <StatBox label="Monthly Spending" value={fmt$(wo.monthlySpending)} sub="retirement target" />
        <StatBox
          label="Annual Spending"
          value={fmt$(wo.monthlySpending * 12)}
        />
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-medium text-text-secondary">Optimal Withdrawal Order</h3>
        {wo.buckets.map((bucket: WithdrawalBucket) => (
          <div key={bucket.taxTreatment} className="bg-surface border border-border-default rounded-lg p-4">
            <div className="flex items-start justify-between gap-4 mb-2">
              <div className="flex items-center gap-2">
                <span className="w-6 h-6 rounded-full bg-accent text-white text-xs flex items-center justify-center font-bold flex-shrink-0">
                  {bucket.priority}
                </span>
                <div>
                  <span className="font-medium text-text-primary">{bucket.label}</span>
                  <span className="ml-2 text-sm text-text-tertiary">{bucket.portfolioPct.toFixed(1)}% of portfolio</span>
                </div>
              </div>
              <div className="text-right flex-shrink-0">
                <div className="font-semibold text-text-primary">{fmt$(bucket.balance)}</div>
                {bucket.yearsOfIncome > 0 && (
                  <div className="text-xs text-text-muted">{bucket.yearsOfIncome.toFixed(1)} yrs of spending</div>
                )}
              </div>
            </div>
            <p className="text-xs text-text-muted">{bucket.taxNote}</p>
            {bucket.accounts.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {bucket.accounts.map((acc) => (
                  <span key={acc.id} className="text-xs bg-subtle text-text-secondary px-2 py-0.5 rounded">
                    {acc.name}
                  </span>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* RMD Projection */}
      {rmdProjection.length > 0 ? (
        <div>
          <h3 className="text-sm font-medium text-text-secondary mb-3">Required Minimum Distribution Projection <InfoTip term="rmd" /></h3>
          <InfoCard>
            RMDs begin at age 73 (SECURE Act 2.0). Every dollar withdrawn is ordinary income — coordinate with
            Social Security and Roth conversions to manage your bracket.
          </InfoCard>
          <div className="overflow-x-auto mt-3">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-default text-text-tertiary text-left">
                  <th className="pb-2 font-medium">Age</th>
                  <th className="pb-2 font-medium">Year</th>
                  <th className="pb-2 font-medium">Start Balance</th>
                  <th className="pb-2 font-medium">RMD Amount</th>
                  <th className="pb-2 font-medium">Est. Tax</th>
                  <th className="pb-2 font-medium">End Balance</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-default">
                {rmdProjection.map((row: RMDYear) => (
                  <tr key={row.age} className="hover:bg-subtle">
                    <td className="py-2 text-text-primary font-medium">{row.age}</td>
                    <td className="py-2 text-text-secondary">{row.year}</td>
                    <td className="py-2 text-text-secondary">{fmt$(row.startBalance)}</td>
                    <td className="py-2 font-medium text-warning">{fmt$(row.rmdAmount)}</td>
                    <td className="py-2 text-negative">{fmt$(row.taxEstimate)}</td>
                    <td className="py-2 text-text-secondary">{fmt$(row.endBalance)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <InfoCard>
          RMD projections require a date of birth and tax-deferred account balances. Set your birthday in Settings.
        </InfoCard>
      )}
    </div>
  )
}

// ── Tab: ACA Subsidy ──────────────────────────────────────────────────────────

const ACA_STATUS_CONFIG = {
  full_subsidy: { label: 'Full Subsidy Eligible', color: 'text-positive', bg: 'bg-positive/10' },
  partial_subsidy: { label: 'Partial Subsidy', color: 'text-accent-text', bg: 'bg-accent-tint-bg' },
  at_risk: { label: 'At Risk — Near Cliff', color: 'text-warning', bg: 'bg-warning/10' },
  over_cliff: { label: 'Over Subsidy Cliff', color: 'text-negative', bg: 'bg-negative/10' },
}

function ACATab({ data }: { data: TaxAnalysis }) {
  const { acaAnalysis: aca } = data
  const status = ACA_STATUS_CONFIG[aca.subsidyStatus] ?? ACA_STATUS_CONFIG.partial_subsidy

  // Width for progress bar (capped at 120% of cliff for display)
  const barPct = Math.min((aca.projectedMagi / aca.cliffAmount) * 100, 120)

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatBox label="Household Size" value={String(aca.householdSize)} />
        <StatBox label="100% FPL" value={fmt$(aca.fpl)} sub="federal poverty level" />
        <StatBox label="Projected MAGI" value={fmt$(aca.projectedMagi)} sub="income + conversions" />
        <StatBox label="Subsidy Cliff (400% FPL)" value={fmt$(aca.cliffAmount)} />
      </div>

      {/* Status badge */}
      <div className={`rounded-lg p-4 ${status.bg}`}>
        <div className={`text-sm font-semibold ${status.color} mb-1`}>{status.label}</div>
        {aca.headroomToCliff > 0 ? (
          <p className="text-sm text-text-secondary">
            You have <strong>{fmt$(aca.headroomToCliff)}</strong> of headroom before losing ACA subsidies.
            {aca.conversionImpact > 0 && ` This includes ${fmt$(aca.conversionImpact)} from Roth conversions, which count as MAGI.`}
          </p>
        ) : (
          <p className="text-sm text-text-secondary">
            Your projected MAGI exceeds the subsidy cliff by <strong>{fmt$(Math.abs(aca.headroomToCliff))}</strong>.
            Consider reducing Roth conversions or income to restore eligibility.
          </p>
        )}
      </div>

      {/* MAGI bar */}
      <div>
        <div className="flex justify-between text-xs text-text-muted mb-1">
          <span>$0</span>
          <span>Cliff: {fmt$(aca.cliffAmount)}</span>
        </div>
        <div className="relative h-3 bg-subtle rounded-full overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${
              aca.subsidyStatus === 'over_cliff' ? 'bg-negative' :
              aca.subsidyStatus === 'at_risk' ? 'bg-warning' :
              aca.subsidyStatus === 'partial_subsidy' ? 'bg-accent' : 'bg-positive'
            }`}
            style={{ width: `${Math.min(barPct, 100)}%` }}
          />
        </div>
        <div className="text-xs text-text-muted mt-1 text-right">
          {aca.fplPercent.toFixed(0)}% FPL — {fmt$(aca.projectedMagi)}
        </div>
      </div>

      {/* Threshold table */}
      <div>
        <h3 className="text-sm font-medium text-text-secondary mb-3">ACA Income Thresholds <InfoTip term="aca" /></h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border-default text-text-tertiary text-left">
                <th className="pb-2 font-medium">% FPL</th>
                <th className="pb-2 font-medium">Annual MAGI</th>
                <th className="pb-2 font-medium">Benefit</th>
                <th className="pb-2 font-medium">Your Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border-default">
              {aca.thresholds.map((t: ACAThreshold) => {
                const below = aca.projectedMagi <= t.amount
                return (
                  <tr key={t.pct} className="hover:bg-subtle">
                    <td className="py-2 font-medium text-text-primary">{t.pct}%</td>
                    <td className="py-2 text-text-secondary">{fmt$(t.amount)}</td>
                    <td className="py-2 text-text-secondary">{t.label}</td>
                    <td className="py-2">
                      {below ? (
                        <span className="text-xs text-positive">Below</span>
                      ) : (
                        <span className="text-xs text-negative">Above</span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>

      <InfoCard>
        <p className="font-medium text-text-primary mb-1">ACA & early retirement</p>
        <p>
          Roth conversions, capital gains, and traditional withdrawals all count toward your MAGI for ACA purposes.
          Managing conversions to stay under 400% FPL ({fmt$(aca.cliffAmount)}) can save thousands in annual premiums.
          MAGI below 150% FPL ({fmt$(aca.fpl * 1.5)}) may qualify for maximum cost-sharing reductions.
        </p>
      </InfoCard>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function TaxesPage() {
  const [activeTab, setActiveTab] = useState<Tab>('bracket')
  const [conversionOverride, setConversionOverride] = useState<number | undefined>(undefined)

  const { data, error, isLoading } = useTaxAnalysis(conversionOverride)
  const { data: profile } = useProfile()
  // /household/members returns null (not []) when the user has none.
  const { data: members, isLoading: membersLoading } = useHouseholdMembers()
  const needsSetup =
    !!profile && !membersLoading &&
    ((!profile.stateTaxCode && profile.stateTaxRate == null) ||
      ((members ?? []).length > 0 && (members ?? []).every((m) => !m.monthlyIncome)))

  const tabCls = (id: Tab) =>
    `px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
      activeTab === id
        ? 'bg-accent text-white'
        : 'text-text-secondary hover:text-text-primary hover:bg-subtle'
    }`

  if (error) {
    return (
      <div className="space-y-6 max-w-6xl">
        <p className="text-negative text-sm">Failed to load tax analysis: {error.message}</p>
      </div>
    )
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div>
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold text-text-primary">Tax Planning</h1>
          <PageGuide id="taxes" />
        </div>
        <p className="text-sm text-text-secondary mt-1">
          Bracket analysis, Roth conversion ladder, withdrawal strategy, and ACA subsidy tracking.
        </p>
      </div>

      {needsSetup && (
        <SetupPrompt
          message="Add your filing status, state, and income to get tax numbers based on your situation."
          buttonLabel="Set up taxes"
          title="Tax Profile"
        >
          {(done) => <TaxSetupForm onDone={done} />}
        </SetupPrompt>
      )}

      <div className="flex gap-1 flex-wrap">
        {TABS.map(({ id, label }) => (
          <button key={id} onClick={() => setActiveTab(id)} className={tabCls(id)}>
            {label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin" />
        </div>
      ) : data ? (
        <div>
          {activeTab === 'bracket' && <BracketTab data={data} />}
          {activeTab === 'roth' && (
            <RothTab data={data} onConversionChange={setConversionOverride} />
          )}
          {activeTab === 'withdrawal' && <WithdrawalTab data={data} />}
          {activeTab === 'aca' && <ACATab data={data} />}
        </div>
      ) : null}
    </div>
  )
}
