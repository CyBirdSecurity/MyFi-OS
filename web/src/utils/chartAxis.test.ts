import { describe, expect, it } from 'vitest'
import { formatAxis, niceTicks } from './chartAxis'

const diffs = (ticks: number[]) => ticks.slice(1).map((t, i) => Number((t - ticks[i]).toPrecision(10)))

describe('niceTicks', () => {
  it('replaces the odd labels from the chart ($307.9K ... $1.1M) with round, even ones', () => {
    const ticks = niceTicks(307_900, 1_096_000)
    expect(ticks).toEqual([200_000, 400_000, 600_000, 800_000, 1_000_000, 1_200_000])
  })

  it('is evenly spaced for any range', () => {
    for (const [lo, hi] of [[0, 1], [3, 97], [-48_000, 213_000], [1_203, 1_347], [40_000_000, 91_500_000], [0.002, 0.0071]]) {
      const steps = diffs(niceTicks(lo, hi))
      expect(new Set(steps).size, `${lo}..${hi}: ${steps}`).toBe(1)
    }
  })

  it('covers the data: the first tick is at or below the minimum and the last at or above the maximum', () => {
    for (const [lo, hi] of [[307_900, 1_096_000], [-48_000, 213_000], [5, 6], [99_999, 100_001]]) {
      const t = niceTicks(lo, hi)
      expect(t[0]).toBeLessThanOrEqual(lo)
      expect(t[t.length - 1]).toBeGreaterThanOrEqual(hi)
    }
  })

  it('uses round steps: 1, 2, 2.5 or 5 times a power of ten', () => {
    for (const [lo, hi] of [[0, 130], [0, 4_800], [250_000, 900_000], [0, 12_000_000], [17, 18]]) {
      const [step] = diffs(niceTicks(lo, hi))
      const mantissa = step / 10 ** Math.floor(Math.log10(step))
      expect([1, 2, 2.5, 5], `${lo}..${hi} step ${step}`).toContain(Number(mantissa.toPrecision(6)))
    }
  })

  it('includes zero when the range crosses it, as for liabilities below the axis', () => {
    const ticks = niceTicks(-200_000, 1_200_000)
    expect(ticks).toContain(0)
    expect(ticks[0]).toBeLessThanOrEqual(-200_000)
  })

  it('gives a flat series, and an all-zero one, a usable axis', () => {
    const flat = niceTicks(500_000, 500_000)
    expect(flat.length).toBeGreaterThan(1)
    expect(flat[0]).toBeLessThan(500_000)
    expect(flat[flat.length - 1]).toBeGreaterThan(500_000)
    expect(niceTicks(0, 0).length).toBeGreaterThan(1)
  })

  it('stays within a reasonable number of ticks', () => {
    for (const [lo, hi] of [[0, 1], [307_900, 1_096_000], [-1e9, 3e9], [12, 13]]) {
      const n = niceTicks(lo, hi).length
      expect(n).toBeGreaterThanOrEqual(2)
      expect(n).toBeLessThanOrEqual(8)
    }
  })

  it('tolerates reversed or non-finite input', () => {
    expect(niceTicks(10, 0)).toEqual(niceTicks(0, 10))
    expect(niceTicks(NaN, 5)).toEqual([0])
  })
})

describe('formatAxis', () => {
  it('writes compact labels without a stray .0', () => {
    expect(formatAxis(1_200_000)).toBe('$1.2M')
    expect(formatAxis(1_000_000)).toBe('$1M')
    expect(formatAxis(250_000)).toBe('$250K')
    expect(formatAxis(2_500)).toBe('$2.5K')
    expect(formatAxis(1_250_000)).toBe('$1.25M')
    expect(formatAxis(0)).toBe('$0')
    expect(formatAxis(750)).toBe('$750')
  })

  it('puts the minus sign before the dollar sign', () => {
    expect(formatAxis(-500_000)).toBe('-$500K')
    expect(formatAxis(-2_000_000)).toBe('-$2M')
  })
})
