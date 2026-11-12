import { describe, expect, it } from 'vitest'
import type { ConcentrationFlag } from '../api/client'
import { flagSummary, sortFlags } from './portfolioCheck'

const flag = (severity: ConcentrationFlag['severity'], pct: number, type: ConcentrationFlag['type'] = 'single_ticker'): ConcentrationFlag => ({
  type,
  severity,
  pct,
  message: `${severity} ${pct}`,
})

describe('sortFlags', () => {
  it('orders by severity, then by larger share first', () => {
    const sorted = sortFlags([flag('info', 5), flag('warning', 12), flag('critical', 30), flag('warning', 18)])
    expect(sorted.map((f) => f.message)).toEqual(['critical 30', 'warning 18', 'warning 12', 'info 5'])
  })

  it('accepts the null the API sends when there are no holdings', () => {
    expect(sortFlags(null)).toEqual([])
    expect(sortFlags(undefined)).toEqual([])
  })

  it('does not modify its input', () => {
    const input = [flag('info', 1), flag('critical', 2)]
    sortFlags(input)
    expect(input.map((f) => f.severity)).toEqual(['info', 'critical'])
  })
})

describe('flagSummary', () => {
  it('counts each severity with correct pluralization', () => {
    expect(flagSummary([flag('critical', 1), flag('warning', 1), flag('warning', 2), flag('info', 1)])).toBe(
      '1 critical · 2 warnings · 1 note',
    )
    expect(flagSummary([flag('warning', 1)])).toBe('1 warning')
    expect(flagSummary([flag('info', 1), flag('info', 2)])).toBe('2 notes')
  })

  it('is empty when there is nothing to report', () => {
    expect(flagSummary([])).toBe('')
  })
})
