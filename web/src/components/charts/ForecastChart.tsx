import { useEffect, useState } from 'react'
import { parseISO } from 'date-fns'
import {
  ComposedChart,
  Bar,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts'
import type { ForecastResult, AccountMeta, Goal, FIMetrics } from '../../api/client'
import { ACCOUNT_TYPE_LABELS, LIABILITY_ACCOUNT_TYPES } from '../../api/client'
import { formatCurrency } from '../../utils/currency'
import { findFIYear } from '../../utils/fi'
import { findGoalHitYear } from '../../utils/goalProjection'
import { useChartPalette } from '../../hooks/useChartPalette'

type RangeKey = '10Y' | '20Y' | '30Y' | 'retirement' | 'full'

const MILESTONE_GAP: Record<RangeKey, number> = {
  '10Y':        0,
  '20Y':        2,
  '30Y':        3,
  'retirement': 4,
  'full':       4,
}

interface ForecastChartProps {
  data: ForecastResult
  scenario: 'base' | 'optimistic' | 'pessimistic'
  showBands?: boolean
  endYear?: number
  rangeKey?: RangeKey
  goals?: Goal[]
  mortgagePayoffYears?: Record<string, number>
  fiMetrics?: FIMetrics
}

// ─── Milestone label with collision-aware stagger ─────────────────────────────

interface Milestone {
  year: number
  label: string
  color: string
  yOffset: number
  goal?: Goal
  hitAge?: number
}

function assignOffsets(
  milestones: Omit<Milestone, 'yOffset'>[],
  gap = 4,
): Milestone[] {
  const sorted = [...milestones].sort((a, b) => a.year - b.year)
  let offset = 0
  let prevYear = -Infinity
  return sorted.map((m) => {
    offset = m.year - prevYear <= gap ? offset + 1 : 0
    prevYear = m.year
    return { ...m, yOffset: offset }
  })
}

interface MilestoneLabelProps {
  viewBox?: { x: number; y: number; width: number; height: number }
  milestone: Milestone
  palette: ReturnType<typeof useChartPalette>
  /** Full chart width in px. A vertical line's viewBox has width 0, so it's passed in. */
  chartWidth: number
}

const COMPACT_PLOT_WIDTH = 420
const SHORT_SCREEN = '(max-height: 500px)'

function MilestoneLabel({ viewBox, milestone, palette, chartWidth }: MilestoneLabelProps) {
  const [hovered, setHovered] = useState(false)
  if (!viewBox) return null

  const { x, y, height } = viewBox
  const width = chartWidth
  const labelY = y + 14 + milestone.yOffset * 13

  const isGoal = !!milestone.goal
  const maxChars = 24
  const truncated =
    milestone.label.length > maxChars
      ? milestone.label.slice(0, maxChars) + '…'
      : milestone.label

  // Near the right edge, anchor the label to its end so long text runs leftward
  // into the plot instead of overflowing (and being clipped by) the chart edge.
  const nearRightEdge = x > width * 0.8
  const textX = nearRightEdge ? x - 4 : x + 4
  const textAnchor = nearRightEdge ? 'end' : 'start'

  const openLeft = x > width * 0.55
  const tooltipW = 158
  const tooltipX = openLeft ? x - tooltipW - 6 : x + 8

  return (
    <g>
      <rect
        x={x - 6}
        y={y}
        width={12}
        height={height}
        fill="transparent"
        style={{ cursor: isGoal ? 'pointer' : 'default' }}
        onMouseEnter={() => isGoal && setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      />
      {/* On phone-width charts the labels can't be staggered apart, so only
          the dashed lines show; the Goal Projections card lists them. */}
      {!hovered && width >= COMPACT_PLOT_WIDTH && (
        <text
          x={textX}
          y={labelY}
          textAnchor={textAnchor}
          fill={milestone.color}
          fontSize={9}
          fontWeight={600}
          stroke={palette.tooltipBg}
          strokeWidth={3}
          style={{ paintOrder: 'stroke', userSelect: 'none', pointerEvents: 'none' }}
        >
          {truncated}
        </text>
      )}
      {hovered && isGoal && milestone.goal && (
        <g
          onMouseEnter={() => setHovered(true)}
          onMouseLeave={() => setHovered(false)}
        >
          <rect
            x={tooltipX}
            y={y + 2}
            width={tooltipW}
            height={62}
            rx={4}
            fill={palette.tooltipBg}
            stroke={palette.tooltipBorder}
            strokeWidth={1}
          />
          <text x={tooltipX + 10} y={y + 20} fill={milestone.color} fontSize={10} fontWeight="600" style={{ userSelect: 'none' }}>
            {milestone.goal.name}
          </text>
          <text x={tooltipX + 10} y={y + 36} fill={palette.tooltipMuted} fontSize={9} style={{ userSelect: 'none' }}>
            {milestone.goal.goalType === 'spend' ? 'Purchase' : 'Target'}: {formatCurrency(milestone.goal.targetAmount)}
          </text>
          <text x={tooltipX + 10} y={y + 52} fill={palette.tick} fontSize={9} style={{ userSelect: 'none' }}>
            {milestone.year} · age {milestone.hitAge}
          </text>
        </g>
      )}
    </g>
  )
}

// ─── Main chart ───────────────────────────────────────────────────────────────

export function ForecastChart({ data, scenario, showBands = false, endYear, rangeKey = 'full', goals = [], mortgagePayoffYears, fiMetrics }: ForecastChartProps) {
  const palette = useChartPalette()
  const [chartWidth, setChartWidth] = useState(0)
  // A phone in landscape is ~400px tall; keep the whole chart on screen there.
  const [shortScreen, setShortScreen] = useState(() => window.matchMedia(SHORT_SCREEN).matches)
  useEffect(() => {
    const mq = window.matchMedia(SHORT_SCREEN)
    const onChange = () => setShortScreen(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const allYears = data[scenario] ?? data.base
  const years = endYear ? allYears.filter((y) => y.year <= endYear) : allYears

  const chartData = years.map((y) => {
    return {
      label: `${y.year}`,
      age: y.age,
      year: y.year,
      ...y.accounts,
      __total: y.p50 ?? y.total,
      __outerBase:  y.p10 ?? 0,
      __outerRange: Math.max(0, (y.p90 ?? 0) - (y.p10 ?? 0)),
      __innerBase:  y.p25 ?? 0,
      __innerRange: Math.max(0, (y.p75 ?? 0) - (y.p25 ?? 0)),
      __p10: y.p10 ?? 0,
      __p25: y.p25 ?? 0,
      __p50: y.p50 ?? 0,
      __p75: y.p75 ?? 0,
      __p90: y.p90 ?? 0,
    }
  })

  const accountIds = data.accountMeta
    .filter((m) => m.countsTowardPortfolio)
    .map((m) => m.id)
  // Debts don't count toward the portfolio but are drawn below the axis so
  // their payoff shows. One already paid off would only add a legend entry.
  const debtIds = data.accountMeta
    .filter((m) => LIABILITY_ACCOUNT_TYPES.has(m.accountType) && years.some((y) => (y.accounts[m.id] ?? 0) < 0))
    .map((m) => m.id)

  if (accountIds.length === 0 && debtIds.length === 0) {
    return (
      <div className="h-[420px] flex items-center justify-center text-sm text-text-tertiary">
        No account data to display.
      </div>
    )
  }

  const formatY = (val: number) => {
    const sign = val < 0 ? '−' : ''
    const abs = Math.abs(val)
    if (abs >= 1_000_000) return `${sign}$${(abs / 1_000_000).toFixed(1)}M`
    if (abs >= 1_000) return `${sign}$${(abs / 1_000).toFixed(0)}K`
    return `${sign}$${abs.toFixed(0)}`
  }

  const CustomTooltip = ({
    active,
    payload,
    label,
  }: {
    active?: boolean
    payload?: any[]
    label?: string
  }) => {
    if (!active || !payload?.length) return null
    const yr = years.find((y) => `${y.year}` === label)
    const row = chartData.find((d) => d.label === label)

    if (showBands) {
      return (
        <div className="rounded-lg px-3 py-2 text-xs shadow-float min-w-[160px]" style={{ background: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}` }}>
          <div className="font-semibold mb-2" style={{ color: palette.tooltipText }}>
            {label} &mdash; Age {yr?.age}
            {yr?.spouseAge != null ? ` / ${yr.spouseAge}` : ''}
          </div>
          {[
            { label: 'P90 (optimistic)', val: row?.__p90, color: palette.percentileP90 },
            { label: 'P75',              val: row?.__p75, color: palette.percentileP75 },
            { label: 'P50 (median)',     val: row?.__p50, color: palette.percentileP50 },
            { label: 'P25',              val: row?.__p25, color: palette.percentileP25 },
            { label: 'P10 (cautious)',   val: row?.__p10, color: palette.percentileP10 },
          ].map(({ label: l, val, color }) => (
            <div key={l} className="flex justify-between gap-4 py-0.5">
              <span style={{ color }} className="font-medium">{l}</span>
              <span style={{ color: palette.tooltipText }}>{formatY(val ?? 0)}</span>
            </div>
          ))}
        </div>
      )
    }

    const total = payload
      .filter((p: any) => accountIds.includes(p.dataKey))
      .reduce((sum: number, p: any) => sum + (p.value || 0), 0)
    return (
      <div className="rounded-lg px-3 py-2 text-xs shadow-float" style={{ background: palette.tooltipBg, border: `1px solid ${palette.tooltipBorder}` }}>
        <div className="font-semibold mb-1" style={{ color: palette.tooltipText }}>
          {label} &mdash; Age {yr?.age}
          {yr?.spouseAge != null ? ` / ${yr.spouseAge}` : ''}
        </div>
        {[...payload].reverse()
          .filter((p: any) => !p.dataKey.startsWith('__'))
          .map((p: any) => {
            const meta = data.accountMeta.find((m) => m.id === p.dataKey)
            return (
              <div key={p.dataKey} className="flex items-center gap-2 py-0.5">
                <span
                  className="w-2.5 h-2.5 rounded-sm flex-shrink-0"
                  style={{ backgroundColor: p.fill }}
                />
                <span className="truncate max-w-[140px]" style={{ color: palette.tooltipMuted }}>
                  {meta?.name ?? p.dataKey}
                </span>
                <span className="ml-auto font-medium" style={{ color: palette.tooltipText }}>
                  {formatY(p.value || 0)}
                </span>
              </div>
            )
          })}
        <div className="border-t mt-1 pt-1 flex justify-between font-semibold" style={{ borderColor: palette.tooltipBorder, color: palette.tooltipText }}>
          <span>{debtIds.length ? 'Portfolio' : 'Total'}</span>
          <span>{formatY(total)}</span>
        </div>
      </div>
    )
  }

  const CustomTick = ({ x, y, payload }: any) => {
    const yr = years.find((y) => `${y.year}` === payload.value)
    if (!yr) return null
    const showTick = yr.age % 5 === 0
    if (!showTick) return null
    return (
      <g transform={`translate(${x},${y})`}>
        <text x={0} y={0} dy={12} textAnchor="middle" fill={palette.tick} fontSize={11}>
          {yr.year}
        </text>
        <text x={0} y={0} dy={24} textAnchor="middle" fill={palette.tickMuted} fontSize={10}>
          age {yr.age}
        </text>
      </g>
    )
  }

  const accountLabel = (meta: AccountMeta) =>
    `${meta.name} (${ACCOUNT_TYPE_LABELS[meta.accountType] ?? meta.accountType})`

  const activeGoals = (goals ?? []).filter((g) => !g.isAchieved && g.targetAmount > 0)
  const savingsGoals = activeGoals.filter((g) => g.goalType !== 'spend')
  const spendGoals = activeGoals.filter((g) => g.goalType === 'spend')

  const goalAchievements = savingsGoals
    .map((g) => {
      const hit = findGoalHitYear(g, allYears, data.accountMeta)
      return { goal: g, hitYear: hit?.year ?? null, hitAge: hit?.age ?? null }
    })
    .filter(
      (item): item is { goal: Goal; hitYear: number; hitAge: number } =>
        item.hitYear !== null && item.hitAge !== null,
    )
    .filter(({ hitYear }) => years.some((y) => y.year === hitYear))

  const spendMilestones = spendGoals
    .filter((g) => g.targetDate != null)
    .map((g) => {
      const spendYear = parseISO(g.targetDate!).getFullYear()
      const yr = allYears.find((y) => y.year === spendYear)
      if (!yr || !years.some((y) => y.year === spendYear)) return null
      return { goal: g, hitYear: spendYear, hitAge: yr.age }
    })
    .filter((item): item is { goal: Goal; hitYear: number; hitAge: number } => item !== null)

  const rawMilestones: Omit<Milestone, 'yOffset'>[] = [
    ...(data.retirementYear
      ? [{ year: data.retirementYear, label: 'Retirement Goal', color: palette.milestoneRetirement }]
      : []),
    ...(data.spouseRetirementYear
      ? [{ year: data.spouseRetirementYear, label: 'Spouse Retirement', color: palette.milestoneSpouseRetirement }]
      : []),
    ...(() => {
      if (!fiMetrics) return []
      const hit = findFIYear(allYears, fiMetrics)
      if (!hit || !years.some((y) => y.year === hit.year)) return []
      return [{ year: hit.year, label: 'FI Number', color: palette.milestoneFiNumber }]
    })(),
    ...goalAchievements.map(({ goal, hitYear, hitAge }) => ({
      year: hitYear,
      label: goal.name,
      color: palette.milestoneGoalSaved,
      goal,
      hitAge,
    })),
    ...spendMilestones.map(({ goal, hitYear, hitAge }) => ({
      year: hitYear,
      label: goal.name,
      color: palette.milestoneGoalSpend,
      goal,
      hitAge,
    })),
    ...(mortgagePayoffYears
      ? Object.entries(mortgagePayoffYears).map(([accId, payoffYear]) => {
          const meta = data.accountMeta.find((m) => m.id === accId)
          return {
            year: payoffYear,
            label: meta ? `${meta.name} Paid Off` : 'Mortgage Paid Off',
            color: palette.milestoneMortgagePayoff,
          }
        })
      : []),
  ]

  const milestones = assignOffsets(rawMilestones, MILESTONE_GAP[rangeKey])

  return (
    <ResponsiveContainer width="100%" height={shortScreen ? 260 : 420} onResize={(w) => setChartWidth(w)}>
      <ComposedChart
        data={chartData}
        margin={{ top: 10, right: 10, left: 10, bottom: 30 }}
        barCategoryGap="2%"
        stackOffset="sign"
      >
        <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
        <XAxis
          dataKey="label"
          tick={<CustomTick />}
          tickLine={false}
          axisLine={{ stroke: palette.axis }}
          height={40}
          interval={0}
        />
        <YAxis
          tickFormatter={formatY}
          tick={{ fill: palette.tick, fontSize: 11 }}
          tickLine={false}
          axisLine={false}
          width={60}
        />
        <Tooltip content={<CustomTooltip />} cursor={{ fill: 'rgba(127,127,127,0.06)' }} />
        <Legend
          formatter={(value) => {
            if (value === '__total')      return <span className="text-xs text-accent-text">Portfolio (P50)</span>
            if (value === '__outerRange') return <span className="text-xs" style={{ color: palette.bandFill, opacity: 0.7 }}>P10–P90 range</span>
            if (value === '__innerRange') return <span className="text-xs" style={{ color: palette.bandFill }}>P25–P75 range</span>
            const meta = data.accountMeta.find((m) => m.id === value)
            return <span className="text-xs text-text-secondary">{meta ? accountLabel(meta) : value}</span>
          }}
          wrapperStyle={{ paddingTop: 8 }}
        />
        {showBands ? (
          <>
            <Area dataKey="__outerBase" stackId="band" stroke="none" fill="transparent"
              legendType="none" isAnimationActive={false} dot={false} activeDot={false} />
            <Area dataKey="__outerRange" stackId="band" name="__outerRange"
              stroke="none" fill={palette.bandFill} fillOpacity={palette.bandOuterAlpha} legendType="square"
              isAnimationActive={false} dot={false} activeDot={false} />
            <Area dataKey="__innerBase" stackId="band2" stroke="none" fill="transparent"
              legendType="none" isAnimationActive={false} dot={false} activeDot={false} />
            <Area dataKey="__innerRange" stackId="band2" name="__innerRange"
              stroke="none" fill={palette.bandFill} fillOpacity={palette.bandInnerAlpha} legendType="square"
              isAnimationActive={false} dot={false} activeDot={false} />
            <Area dataKey="__total" name="__total" stackId={undefined}
              stroke={palette.medianLine} strokeWidth={2} fill="transparent" legendType="line"
              isAnimationActive={false} dot={false} activeDot={{ r: 4, fill: palette.medianLine }} />
          </>
        ) : (
          <>
            {accountIds.map((id, i) => (
              <Bar key={id} dataKey={id} stackId="portfolio"
                fill={palette.forecast[i % palette.forecast.length]} isAnimationActive={false} />
            ))}
            {debtIds.map((id, i) => (
              <Bar key={id} dataKey={id} stackId="portfolio"
                fill={palette.goalLow} fillOpacity={0.8 - (i % 3) * 0.2} isAnimationActive={false} />
            ))}
          </>
        )}
        {/* Rendered after the bars/areas so the goal lines and their labels
            paint on top instead of being hidden behind tall bars. */}
        {milestones.map((m) => (
          <ReferenceLine
            key={`${m.year}-${m.label}`}
            x={String(m.year)}
            stroke={m.color}
            strokeDasharray="4 2"
            label={<MilestoneLabel milestone={m} palette={palette} chartWidth={chartWidth} />}
          />
        ))}
      </ComposedChart>
    </ResponsiveContainer>
  )
}
