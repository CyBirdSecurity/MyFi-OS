import { useThemeStore } from '../store/themeStore'

// Warm-neutral + single-hue "Ink" accent family. Every color
// below is desaturated (<80% saturation) and pulled toward the ink/pastel
// family instead of a fully-saturated rainbow. Structural values (grid,
// axis, tick, tooltip*) match the border/text CSS variables in index.css;
// goalLow/Mid/High map directly onto the negative/warning/positive tokens.

const LIGHT = {
  forecast: [
    '#1F6C9F', '#4C8FB0', '#6FA8D9', '#5B8A76', '#8A9F6E',
    '#A98A55', '#9F6E5B', '#7A8A9A', '#5B7A8A', '#8A7A5B',
    '#6B7FA0', '#7A9F8A', '#B08947', '#6E5B8A', '#8A5B6E',
    '#5B8A9F', '#9F8A5B', '#7A5B8A', '#5B9F8A', '#9F5B6E',
  ],
  // Liabilities (cards, loans) in net worth charts: shades of the negative
  // red, alternating light and dark so neighboring segments stay distinct.
  liability: ['#9F2F2D', '#C9625E', '#7A1F1E', '#DB8E8A', '#B5413E'],
  pieTax:  ['#1F6C9F', '#A98A55', '#5B8A76'],
  // Ordered so neighboring slices differ in hue, not just lightness.
  pieType: [
    '#1F6C9F', '#A98A55', '#5B8A76', '#6FA8D9', '#9F6E5B',
    '#8A9F6E', '#4C8FB0', '#7A8A9A', '#8A7A5B', '#5B7A8A',
  ],
  grid:          '#EAEAEA',
  axis:          '#D8D6D0',
  tick:          '#5B564D',
  tickMuted:     '#8A8579',
  tooltipBg:     '#FFFFFF',
  tooltipBorder: '#EAEAEA',
  tooltipText:   '#17140F',
  tooltipMuted:  '#5B564D',
  bandFill:      '#1F6C9F',
  bandOuterAlpha: 0.10,
  bandInnerAlpha: 0.18,
  medianLine:    '#17516F',
  netWorthLine:  '#1F6C9F',
  netWorthGradientFrom: 'rgba(31, 108, 159, 0.30)',
  netWorthGradientTo:   'rgba(31, 108, 159, 0)',
  gaugeTrack:    '#ECEBE7',
  goalLow:  '#9F2F2D',
  goalMid:  '#956400',
  goalHigh: '#346538',

  // Forecast percentile band, P10 (conservative) -> P90 (optimistic) —
  // a sequential ramp within the ink family instead of violet.
  percentileP10: '#0F3A4D',
  percentileP25: '#17516F',
  percentileP50: '#1F6C9F',
  percentileP75: '#4C8FB0',
  percentileP90: '#6FA8D9',

  // ForecastChart milestone reference lines — one distinct, desaturated
  // tone per milestone type, reusing hues already present in `forecast`.
  milestoneRetirement:       '#1F6C9F',
  milestoneSpouseRetirement: '#5B7A8A',
  milestoneFiNumber:         '#346538',
  milestoneGoalSaved:        '#A98A55',
  milestoneGoalSpend:        '#9F6E5B',
  milestoneMortgagePayoff:   '#8A9F6E',

  // ReadinessScoreCard grade gauge, A (best) -> F (worst).
  gradeA: '#346538',
  gradeB: '#5B8A76',
  gradeC: '#956400',
  gradeD: '#9F6E5B',
  gradeF: '#9F2F2D',
} as const

const DARK = {
  forecast: [
    '#6FA8D9', '#8DBEE3', '#A8D4F0', '#8FBBA3', '#B3C495',
    '#D4B37E', '#C99A85', '#A3B3C2', '#85A8B8', '#C2AB85',
    '#96A8C9', '#A3C9B3', '#D4AF6B', '#9B85B3', '#B385A0',
    '#85B3C9', '#C9B385', '#A585B3', '#85C9AF', '#C98596',
  ],
  liability: ['#F0817E', '#C25A56', '#F7AFAC', '#A8403D', '#E0706C'],
  pieTax:  ['#6FA8D9', '#D4B37E', '#8FBBA3'],
  pieType: [
    '#6FA8D9', '#D4B37E', '#8FBBA3', '#A8D4F0', '#C99A85',
    '#B3C495', '#8DBEE3', '#A3B3C2', '#C2AB85', '#85A8B8',
  ],
  grid:          '#332F26',
  axis:          '#4A4432',
  tick:          '#A39C8E',
  tickMuted:     '#7A7466',
  tooltipBg:     '#1C1A16',
  tooltipBorder: '#332F26',
  tooltipText:   '#F5F3EE',
  tooltipMuted:  '#A39C8E',
  bandFill:      '#6FA8D9',
  bandOuterAlpha: 0.18,
  bandInnerAlpha: 0.28,
  medianLine:    '#8DBEE3',
  netWorthLine:  '#6FA8D9',
  netWorthGradientFrom: 'rgba(111, 168, 217, 0.30)',
  netWorthGradientTo:   'rgba(111, 168, 217, 0)',
  gaugeTrack:    '#38342A',
  goalLow:  '#F0817E',
  goalMid:  '#F0B23D',
  goalHigh: '#6FCB86',

  percentileP10: '#2F6E94',
  percentileP25: '#4C8FB0',
  percentileP50: '#6FA8D9',
  percentileP75: '#8DBEE3',
  percentileP90: '#A8D4F0',

  milestoneRetirement:       '#6FA8D9',
  milestoneSpouseRetirement: '#85A8B8',
  milestoneFiNumber:         '#6FCB86',
  milestoneGoalSaved:        '#D4B37E',
  milestoneGoalSpend:        '#C99A85',
  milestoneMortgagePayoff:   '#B3C495',

  gradeA: '#6FCB86',
  gradeB: '#8FBBA3',
  gradeC: '#F0B23D',
  gradeD: '#C99A85',
  gradeF: '#F0817E',
} as const

export type ChartPalette = {
  forecast: readonly string[]
  liability: readonly string[]
  pieTax: readonly string[]
  pieType: readonly string[]
  grid: string
  axis: string
  tick: string
  tickMuted: string
  tooltipBg: string
  tooltipBorder: string
  tooltipText: string
  tooltipMuted: string
  bandFill: string
  bandOuterAlpha: number
  bandInnerAlpha: number
  medianLine: string
  netWorthLine: string
  netWorthGradientFrom: string
  netWorthGradientTo: string
  gaugeTrack: string
  goalLow: string
  goalMid: string
  goalHigh: string
  percentileP10: string
  percentileP25: string
  percentileP50: string
  percentileP75: string
  percentileP90: string
  milestoneRetirement: string
  milestoneSpouseRetirement: string
  milestoneFiNumber: string
  milestoneGoalSaved: string
  milestoneGoalSpend: string
  milestoneMortgagePayoff: string
  gradeA: string
  gradeB: string
  gradeC: string
  gradeD: string
  gradeF: string
}

export function useChartPalette(): ChartPalette {
  const theme = useThemeStore((s) => s.theme)
  return theme === 'dark' ? DARK : LIGHT
}
