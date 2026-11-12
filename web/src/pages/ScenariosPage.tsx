import { useState } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts'
import { useScenarios } from '../hooks/useScenarios'
import { useSolver } from '../hooks/useSolver'
import { Card } from '../components/ui/Card'
import { formatCurrency, formatPercent } from '../utils/currency'
import type { CurrentPlanSummary, ScenarioParams, ScenarioResult, SolveFor, SolveParams } from '../api/client'
import { useChartPalette } from '../hooks/useChartPalette'
import { PageGuide } from '../components/PageGuide'

// Scenario line colors come from palette.forecast[i] at each usage site
// (index 0 = baseline, 1-3 = user scenarios) rather than a fixed array,
// since the palette itself is theme-dependent.

// ─── Types ────────────────────────────────────────────────────────────────────

// Fields hold the absolute target value the user wants (e.g. "retire at 62"),
// not a delta — ScenarioEditor shows the (+/-) vs. the current plan as a hint.
interface ScenarioForm {
  name: string
  retirementAge: string
  monthlyContribution: string
  returnRate: string // percent, e.g. "6.5"
  monthlyExpense: string
}

const EMPTY_FORM: ScenarioForm = {
  name: '',
  retirementAge: '',
  monthlyContribution: '',
  returnRate: '',
  monthlyExpense: '',
}

function formToParams(f: ScenarioForm, index: number): ScenarioParams {
  return {
    name: f.name.trim() || `Scenario ${index + 1}`,
    retirementAge: f.retirementAge ? parseInt(f.retirementAge, 10) : undefined,
    monthlyContribution: f.monthlyContribution ? parseFloat(f.monthlyContribution) : undefined,
    returnRate: f.returnRate ? parseFloat(f.returnRate) / 100 : undefined,
    monthlyExpense: f.monthlyExpense ? parseFloat(f.monthlyExpense) : undefined,
  }
}

// ─── Delta hints ──────────────────────────────────────────────────────────────

// Describes how a field's typed absolute value compares to the current plan,
// e.g. "+3 yrs vs your current 62" — falls back to a generic placeholder hint
// while empty or before the current plan has loaded. formatDelta owns its own
// sign (each formatter below already prefixes +/-).
function deltaHint(
  rawValue: string,
  baseline: number | undefined,
  parse: (v: string) => number,
  formatDelta: (delta: number) => string,
  baselineLabel: (baseline: number) => string,
): string {
  if (baseline == null) return 'Leave blank to keep your current plan'
  if (!rawValue.trim()) return `Leave blank to keep your current ${baselineLabel(baseline)}`
  const parsed = parse(rawValue)
  if (Number.isNaN(parsed)) return ''
  const delta = parsed - baseline
  if (Math.abs(delta) < 1e-9) return `Same as your current ${baselineLabel(baseline)}`
  return `${formatDelta(delta)} vs your current ${baselineLabel(baseline)}`
}

const yearsDelta = (d: number) => {
  const rounded = Math.round(d)
  return `${rounded > 0 ? '+' : ''}${rounded} yr${Math.abs(rounded) !== 1 ? 's' : ''}`
}
const currencyDelta = (d: number) => `${formatCurrency(d, { showSign: true })}/mo`
// d is already in percentage-point units here (baseline and parsed value are
// both passed to deltaHint as percentages, e.g. 6.5 for 6.5%), so this must
// NOT be multiplied by 100 again.
const ratePointsDelta = (d: number) => formatPercent(d, 1)

// ─── Sub-components ───────────────────────────────────────────────────────────

function FieldRow({
  label,
  hint,
  prefix,
  suffix,
  value,
  onChange,
  placeholder,
}: {
  label: string
  hint?: string
  prefix?: string
  suffix?: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-text-secondary mb-1">{label}</label>
      <div className="flex items-center gap-1">
        {prefix && <span className="text-xs text-text-tertiary">{prefix}</span>}
        <input
          type="number"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder ?? '0'}
          className="w-full bg-subtle border border-border-strong rounded-md px-2.5 py-1.5 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
        />
        {suffix && <span className="text-xs text-text-tertiary">{suffix}</span>}
      </div>
      {hint && <p className="text-xs text-text-muted mt-0.5">{hint}</p>}
    </div>
  )
}

function ScenarioEditor({
  index,
  form,
  color,
  currentPlan,
  onChange,
  onRemove,
}: {
  index: number
  form: ScenarioForm
  color: string
  currentPlan?: CurrentPlanSummary
  onChange: (f: ScenarioForm) => void
  onRemove: () => void
}) {
  const set = (key: keyof ScenarioForm) => (val: string) =>
    onChange({ ...form, [key]: val })

  return (
    <div
      className="bg-surface border rounded-lg p-4 space-y-3"
      style={{ borderColor: color + '55' }}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className="inline-block w-2.5 h-2.5 rounded-full"
            style={{ backgroundColor: color }}
          />
          <input
            type="text"
            value={form.name}
            onChange={(e) => set('name')(e.target.value)}
            placeholder={`Scenario ${index + 1}`}
            className="bg-transparent border-none text-sm font-semibold text-text-primary placeholder-text-muted focus:outline-none focus:ring-0 w-full"
          />
        </div>
        <button
          onClick={onRemove}
          className="text-xs text-text-muted hover:text-negative transition-colors ml-2 shrink-0"
        >
          Remove
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <FieldRow
          label="Retirement age"
          hint={deltaHint(form.retirementAge, currentPlan?.retirementAge, (v) => parseInt(v, 10), yearsDelta, (b) => `age (${b})`)}
          value={form.retirementAge}
          onChange={set('retirementAge')}
          placeholder={currentPlan ? String(currentPlan.retirementAge) : 'e.g. 62'}
        />
        <FieldRow
          label="Return rate"
          hint={deltaHint(
            form.returnRate,
            currentPlan ? currentPlan.returnRate * 100 : undefined,
            parseFloat,
            ratePointsDelta,
            (b) => `rate (${b.toFixed(1)}%)`,
          )}
          suffix="%"
          value={form.returnRate}
          onChange={set('returnRate')}
          placeholder={currentPlan ? (currentPlan.returnRate * 100).toFixed(1) : '0'}
        />
        <FieldRow
          label="Monthly contribution"
          hint={deltaHint(form.monthlyContribution, currentPlan?.monthlyContribution, parseFloat, currencyDelta, (b) => `contribution (${formatCurrency(b)}/mo)`)}
          prefix="$"
          value={form.monthlyContribution}
          onChange={set('monthlyContribution')}
          placeholder={currentPlan ? String(Math.round(currentPlan.monthlyContribution)) : '0'}
        />
        <FieldRow
          label="Monthly expense"
          hint={deltaHint(form.monthlyExpense, currentPlan?.monthlyExpense, parseFloat, currencyDelta, (b) => `expense (${formatCurrency(b)}/mo)`)}
          prefix="$"
          value={form.monthlyExpense}
          onChange={set('monthlyExpense')}
          placeholder={currentPlan ? String(Math.round(currentPlan.monthlyExpense)) : '0'}
        />
      </div>
    </div>
  )
}

// ─── Chart helpers ────────────────────────────────────────────────────────────

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function ChartTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-surface border border-border-strong rounded-lg px-3 py-2 text-xs shadow-float">
      <div className="text-text-secondary mb-1">Year {label}</div>
      {payload.map((p: { name: string; value: number; color: string }) => (
        <div key={p.name} style={{ color: p.color }} className="flex gap-2 justify-between">
          <span className="truncate max-w-[120px]">{p.name}</span>
          <span className="font-semibold">{formatCurrency(p.value)}</span>
        </div>
      ))}
    </div>
  )
}

function buildChartData(
  baseline: ScenarioResult,
  scenarios: ScenarioResult[],
): Record<string, number | string>[] {
  const allYears = new Set<number>()
  ;[baseline, ...scenarios].forEach((s) => s.years.forEach((y) => allYears.add(y.year)))

  const sorted = Array.from(allYears).sort((a, b) => a - b)

  const totals: Record<number, Record<string, number>> = {}
  for (const yr of sorted) totals[yr] = {}

  const all = [{ ...baseline, name: 'Current Plan' }, ...scenarios]
  for (const sc of all) {
    for (const y of sc.years) {
      if (totals[y.year]) totals[y.year][sc.name] = y.total
    }
  }

  return sorted.map((yr) => ({ year: yr, ...totals[yr] }))
}

// ─── Current plan summary ─────────────────────────────────────────────────────

function CurrentPlanSummaryCard({ plan }: { plan: CurrentPlanSummary }) {
  const stats = [
    { label: 'Retirement Age', value: `${plan.retirementAge}` },
    { label: 'Return Rate', value: `${(plan.returnRate * 100).toFixed(1)}%` },
    { label: 'Monthly Contribution', value: `${formatCurrency(plan.monthlyContribution)}/mo` },
    { label: 'Monthly Expense', value: `${formatCurrency(plan.monthlyExpense)}/mo` },
  ]

  return (
    <div className="bg-surface border border-border-default rounded-lg px-4 py-3">
      <div className="flex flex-wrap gap-x-8 gap-y-3">
        {stats.map((s) => (
          <div key={s.label}>
            <div className="text-xs text-text-tertiary">{s.label}</div>
            <div className="text-sm font-semibold text-text-primary">{s.value}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

// ─── Comparison table ─────────────────────────────────────────────────────────

function ComparisonTable({
  baseline,
  scenarios,
  colors,
}: {
  baseline: ScenarioResult
  scenarios: ScenarioResult[]
  colors: string[]
}) {
  const rows = [{ ...baseline, name: 'Current Plan' }, ...scenarios]
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm min-w-[560px]">
        <thead>
          <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
            <th className="pb-2 font-medium">Scenario</th>
            <th className="pb-2 font-medium text-right">FI Year</th>
            <th className="pb-2 font-medium text-right">FI Age</th>
            <th className="pb-2 font-medium text-right">Portfolio at Retirement</th>
            <th className="pb-2 font-medium text-right">Sustainable Monthly (today's $)</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.name} className="border-b border-border-default/50">
              <td className="py-3">
                <div className="flex items-center gap-2">
                  <span
                    className="inline-block w-2.5 h-2.5 rounded-full shrink-0"
                    style={{ backgroundColor: colors[i] }}
                  />
                  <span className="text-text-primary font-medium">{r.name}</span>
                </div>
              </td>
              <td className="py-3 text-right text-text-secondary">
                {r.fiYear ?? <span className="text-text-muted">—</span>}
              </td>
              <td className="py-3 text-right text-text-secondary">
                {r.fiAge ?? <span className="text-text-muted">—</span>}
              </td>
              <td className="py-3 text-right text-text-primary font-semibold">
                {formatCurrency(r.portfolioAtRetirement)}
              </td>
              <td className="py-3 text-right text-positive font-semibold">
                {formatCurrency(r.sustainableMonthly)}/mo
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── Solve-for form ───────────────────────────────────────────────────────────

interface SolverFormState {
  solveFor: SolveFor
  targetRetirementAge: string
  monthlyContribution: string
  returnRate: string // percent, e.g. "6.5"
  monthlyExpense: string
}

const EMPTY_SOLVER_FORM: SolverFormState = {
  solveFor: 'contribution',
  targetRetirementAge: '',
  monthlyContribution: '',
  returnRate: '',
  monthlyExpense: '',
}

const SOLVE_FOR_LABELS: Record<SolveFor, string> = {
  contribution: 'Monthly contribution',
  returnRate: 'Return rate',
  retirementAge: 'Retirement age',
}

function solverFormToParams(f: SolverFormState): SolveParams | null {
  if (f.solveFor !== 'retirementAge' && !f.targetRetirementAge.trim()) return null
  return {
    solveFor: f.solveFor,
    targetRetirementAge: f.targetRetirementAge ? parseInt(f.targetRetirementAge, 10) : undefined,
    monthlyContribution: f.monthlyContribution ? parseFloat(f.monthlyContribution) : undefined,
    returnRate: f.returnRate ? parseFloat(f.returnRate) / 100 : undefined,
    monthlyExpense: f.monthlyExpense ? parseFloat(f.monthlyExpense) : undefined,
  }
}

function SolverForm({
  form,
  currentPlan,
  onChange,
}: {
  form: SolverFormState
  currentPlan?: CurrentPlanSummary
  onChange: (f: SolverFormState) => void
}) {
  const set = (key: keyof SolverFormState) => (val: string) => onChange({ ...form, [key]: val })

  return (
    <div className="bg-surface border border-border-default rounded-lg p-4 space-y-3">
      <div>
        <label className="block text-xs font-medium text-text-secondary mb-1">Solve for</label>
        <select
          value={form.solveFor}
          onChange={(e) => set('solveFor')(e.target.value)}
          className="w-full bg-subtle border border-border-strong rounded-md px-2.5 py-1.5 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent"
        >
          <option value="contribution">Monthly contribution</option>
          <option value="returnRate">Return rate</option>
          <option value="retirementAge">Retirement age</option>
        </select>
      </div>

      {form.solveFor !== 'retirementAge' && (
        <FieldRow
          label="Target retirement age"
          hint="The age you want to be able to retire by"
          value={form.targetRetirementAge}
          onChange={set('targetRetirementAge')}
          placeholder="e.g. 50"
        />
      )}

      {form.solveFor !== 'contribution' && (
        <FieldRow
          label="Monthly contribution (fixed)"
          hint={deltaHint(form.monthlyContribution, currentPlan?.monthlyContribution, parseFloat, currencyDelta, (b) => `contribution (${formatCurrency(b)}/mo)`)}
          prefix="$"
          value={form.monthlyContribution}
          onChange={set('monthlyContribution')}
          placeholder={currentPlan ? String(Math.round(currentPlan.monthlyContribution)) : '0'}
        />
      )}

      {form.solveFor !== 'returnRate' && (
        <FieldRow
          label="Return rate (fixed)"
          hint={deltaHint(
            form.returnRate,
            currentPlan ? currentPlan.returnRate * 100 : undefined,
            parseFloat,
            ratePointsDelta,
            (b) => `rate (${b.toFixed(1)}%)`,
          )}
          suffix="%"
          value={form.returnRate}
          onChange={set('returnRate')}
          placeholder={currentPlan ? (currentPlan.returnRate * 100).toFixed(1) : '0'}
        />
      )}

      <FieldRow
        label="Monthly expense"
        hint={deltaHint(form.monthlyExpense, currentPlan?.monthlyExpense, parseFloat, currencyDelta, (b) => `expense (${formatCurrency(b)}/mo)`)}
        prefix="$"
        value={form.monthlyExpense}
        onChange={set('monthlyExpense')}
        placeholder={currentPlan ? String(Math.round(currentPlan.monthlyExpense)) : '0'}
      />
    </div>
  )
}

function solvedValueLabel(solveFor: SolveFor, value: number): string {
  switch (solveFor) {
    case 'contribution':
      return `${formatCurrency(value)}/mo`
    case 'returnRate':
      return `${(value * 100).toFixed(1)}%`
    case 'retirementAge':
      return `Age ${Math.round(value)}`
  }
}

// Describes the solved absolute value's (+/-) vs. the current plan, e.g.
// "+$200/mo vs your current $500/mo" — mirrors the ScenarioEditor hints.
function solvedValueDelta(solveFor: SolveFor, value: number, currentPlan?: CurrentPlanSummary): string | null {
  if (!currentPlan) return null
  switch (solveFor) {
    case 'contribution':
      return `${currencyDelta(value - currentPlan.monthlyContribution)} vs your current ${formatCurrency(currentPlan.monthlyContribution)}/mo`
    case 'returnRate':
      return `${ratePointsDelta(value * 100 - currentPlan.returnRate * 100)} vs your current ${(currentPlan.returnRate * 100).toFixed(1)}%`
    case 'retirementAge':
      return `${yearsDelta(value - currentPlan.retirementAge)} vs your current age ${currentPlan.retirementAge}`
  }
}

function SolverResultPanel({
  solveFor,
  targetAge,
  currentPlan,
}: {
  solveFor: SolveFor
  targetAge: string
  currentPlan?: CurrentPlanSummary
}) {
  const palette = useChartPalette()
  const [form, setForm] = useState<SolverFormState>({ ...EMPTY_SOLVER_FORM, solveFor, targetRetirementAge: targetAge })
  const params = solverFormToParams(form)
  const { data: result, isLoading } = useSolver(params)

  const chartData = (result?.scenario.years ?? []).map((y) => ({ year: y.year, Total: y.total }))

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div className="lg:col-span-1">
        <SolverForm form={form} currentPlan={currentPlan} onChange={setForm} />
      </div>
      <div className="lg:col-span-2 space-y-6">
        {!params && (
          <div className="h-64 flex items-center justify-center text-sm text-text-muted bg-surface border border-border-default rounded-lg">
            Enter a target retirement age to solve.
          </div>
        )}

        {isLoading && (
          <div className="flex items-center gap-2 text-xs text-text-tertiary">
            <div className="w-3.5 h-3.5 border border-accent border-t-transparent rounded-full animate-spin" />
            Solving…
          </div>
        )}

        {result && (
          <>
            {!result.achievable && (
              <div className="bg-warning-tint-bg border border-warning/30 rounded-lg px-5 py-4">
                <div className="text-warning font-medium mb-1">Not achievable within search bounds</div>
                <p className="text-sm text-text-secondary">
                  Even at the search limit ({solvedValueLabel(result.solveFor, result.solvedValue)}), this target
                  isn't reachable. Try a later target age or a smaller change.
                </p>
              </div>
            )}

            <Card title={`Required ${SOLVE_FOR_LABELS[result.solveFor].toLowerCase()}`}>
              <div className="text-3xl font-bold text-text-primary">
                {solvedValueLabel(result.solveFor, result.solvedValue)}
              </div>
              {solvedValueDelta(result.solveFor, result.solvedValue, currentPlan) && (
                <p className="text-xs text-text-muted mt-1">
                  {solvedValueDelta(result.solveFor, result.solvedValue, currentPlan)}
                </p>
              )}
              <p className="text-sm text-text-tertiary mt-2">
                Portfolio at retirement: {formatCurrency(result.scenario.portfolioAtRetirement)} ·
                Sustainable income: {formatCurrency(result.scenario.sustainableMonthly)}/mo in today's dollars
              </p>
            </Card>

            <Card title="Projected Portfolio">
              <ResponsiveContainer width="100%" height={280}>
                <LineChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} />
                  <XAxis dataKey="year" tick={{ fill: palette.tick, fontSize: 11 }} tickLine={false} />
                  <YAxis
                    tickFormatter={(v) => formatCurrency(v, { compact: true })}
                    tick={{ fill: palette.tick, fontSize: 11 }}
                    tickLine={false}
                    axisLine={false}
                    width={70}
                  />
                  <Tooltip content={<ChartTooltip />} />
                  <ReferenceLine
                    x={result.scenario.retirementYear}
                    stroke={palette.axis}
                    strokeDasharray="4 4"
                    label={{ value: 'Retirement', fill: palette.tick, fontSize: 10, position: 'insideTopRight' }}
                  />
                  <Line type="monotone" dataKey="Total" stroke={palette.forecast[0]} strokeWidth={2} dot={false} activeDot={{ r: 4 }} />
                </LineChart>
              </ResponsiveContainer>
            </Card>
          </>
        )}
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function ScenariosPage() {
  const palette = useChartPalette()
  const [mode, setMode] = useState<'whatif' | 'solve'>('whatif')
  const [forms, setForms] = useState<ScenarioForm[]>([{ ...EMPTY_FORM, name: 'Scenario 1' }])

  const params = forms.map((f, i) => formToParams(f, i))
  const { data: result, isLoading } = useScenarios(params)

  function addScenario() {
    if (forms.length >= 3) return
    setForms((prev) => [...prev, { ...EMPTY_FORM, name: `Scenario ${prev.length + 1}` }])
  }

  function removeScenario(i: number) {
    setForms((prev) => prev.filter((_, idx) => idx !== i))
  }

  function updateForm(i: number, f: ScenarioForm) {
    setForms((prev) => prev.map((old, idx) => (idx === i ? f : old)))
  }

  const allResults = result
    ? [{ ...result.baseline, name: 'Current Plan' }, ...result.scenarios]
    : []

  const chartData = result
    ? buildChartData(result.baseline, result.scenarios)
    : []

  const scenarioColors = palette.forecast.slice(0, 1 + (result?.scenarios.length ?? 0))

  return (
    <div className="space-y-6 max-w-6xl">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold text-text-primary">Scenarios</h1>
          <PageGuide id="scenarios" />
        </div>
        <p className="text-sm text-text-tertiary mt-1">
          {mode === 'whatif'
            ? 'Compare up to 3 what-if scenarios against your current plan side by side.'
            : "Solve for what it would take to hit a retirement target — pick one variable, and we'll find the value."}
        </p>
      </div>

      {/* Current plan summary — the baseline every scenario/solve is measured against */}
      {result && <CurrentPlanSummaryCard plan={result.currentPlan} />}

      {/* Mode toggle */}
      <div className="inline-flex rounded-lg border border-border-default p-1 bg-surface">
        {(['whatif', 'solve'] as const).map((m) => (
          <button
            key={m}
            onClick={() => setMode(m)}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              mode === m ? 'bg-accent-tint-bg text-accent-text' : 'text-text-tertiary hover:text-text-primary'
            }`}
          >
            {m === 'whatif' ? 'What-If' : 'Solve For…'}
          </button>
        ))}
      </div>

      {mode === 'solve' && (
        <SolverResultPanel solveFor="contribution" targetAge="" currentPlan={result?.currentPlan} />
      )}

      {mode === 'whatif' && (
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left: scenario builder sidebar */}
        <div className="lg:col-span-1 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-text-secondary">What-If Scenarios</h2>
            {forms.length < 3 && (
              <button
                onClick={addScenario}
                className="text-xs text-accent-text hover:text-accent-text border border-accent/30 rounded px-2.5 py-1 transition-colors"
              >
                + Add scenario
              </button>
            )}
          </div>

          {forms.map((form, i) => (
            <ScenarioEditor
              key={i}
              index={i}
              form={form}
              color={palette.forecast[i + 1]}
              currentPlan={result?.currentPlan}
              onChange={(f) => updateForm(i, f)}
              onRemove={() => removeScenario(i)}
            />
          ))}

          {forms.length === 0 && (
            <div className="text-sm text-text-muted bg-surface border border-border-default rounded-lg p-4">
              Add a scenario above to compare against your current plan.
            </div>
          )}

          {isLoading && (
            <div className="flex items-center gap-2 text-xs text-text-tertiary">
              <div className="w-3.5 h-3.5 border border-accent border-t-transparent rounded-full animate-spin" />
              Calculating…
            </div>
          )}
        </div>

        {/* Right: chart + table */}
        <div className="lg:col-span-2 space-y-6">
          {result && (
            <>
              {/* Multi-line chart */}
              <Card title="Portfolio Projection Comparison">
                <ResponsiveContainer width="100%" height={320}>
                  <LineChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} />
                    <XAxis
                      dataKey="year"
                      tick={{ fill: palette.tick, fontSize: 11 }}
                      tickLine={false}
                    />
                    <YAxis
                      tickFormatter={(v) => formatCurrency(v, { compact: true })}
                      tick={{ fill: palette.tick, fontSize: 11 }}
                      tickLine={false}
                      axisLine={false}
                      width={70}
                    />
                    <Tooltip content={<ChartTooltip />} />
                    <Legend
                      iconType="circle"
                      iconSize={8}
                      wrapperStyle={{ fontSize: 11, color: palette.tickMuted }}
                    />
                    {/* Retirement reference line */}
                    <ReferenceLine
                      x={result.baseline.retirementYear}
                      stroke={palette.axis}
                      strokeDasharray="4 4"
                      label={{ value: 'Retirement', fill: palette.tick, fontSize: 10, position: 'insideTopRight' }}
                    />
                    {/* Baseline */}
                    <Line
                      type="monotone"
                      dataKey="Current Plan"
                      stroke={palette.forecast[0]}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    {/* User scenarios */}
                    {result.scenarios.map((s, i) => (
                      <Line
                        key={s.name}
                        type="monotone"
                        dataKey={s.name}
                        stroke={palette.forecast[i + 1]}
                        strokeWidth={2}
                        strokeDasharray="5 3"
                        dot={false}
                        activeDot={{ r: 4 }}
                      />
                    ))}
                  </LineChart>
                </ResponsiveContainer>
              </Card>

              {/* Comparison table */}
              <Card title="Side-by-Side Comparison">
                <ComparisonTable
                  baseline={result.baseline}
                  scenarios={result.scenarios}
                  colors={scenarioColors}
                />
              </Card>
            </>
          )}

          {!result && !isLoading && (
            <div className="h-64 flex items-center justify-center text-sm text-text-muted bg-surface border border-border-default rounded-lg">
              Configure a scenario on the left to see results here.
            </div>
          )}
        </div>
      </div>
      )}
    </div>
  )
}
