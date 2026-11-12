import type { ConcentrationFlag } from '../api/client'

const RANK: Record<ConcentrationFlag['severity'], number> = { critical: 0, warning: 1, info: 2 }

/**
 * The portfolio flags, most serious first (larger positions first within a
 * severity). The API sends null rather than an empty list when there are no
 * holdings, so that is accepted too. The input is not modified.
 */
export function sortFlags(flags: ConcentrationFlag[] | null | undefined): ConcentrationFlag[] {
  return [...(flags ?? [])].sort((a, b) => RANK[a.severity] - RANK[b.severity] || b.pct - a.pct)
}

/** "1 critical · 2 warnings · 1 note", or "" when there is nothing to report. */
export function flagSummary(flags: ConcentrationFlag[]): string {
  const count = (s: ConcentrationFlag['severity']) => flags.filter((f) => f.severity === s).length
  const parts: string[] = []
  const critical = count('critical')
  const warnings = count('warning')
  const notes = count('info')
  if (critical) parts.push(`${critical} critical`)
  if (warnings) parts.push(`${warnings} ${warnings === 1 ? 'warning' : 'warnings'}`)
  if (notes) parts.push(`${notes} ${notes === 1 ? 'note' : 'notes'}`)
  return parts.join(' · ')
}
