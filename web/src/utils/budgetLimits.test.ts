import { describe, expect, it } from 'vitest'
import { limitProgress, roundLimit, suggestLimits } from './budgetLimits'

describe('limitProgress', () => {
  it('is ok well under the limit', () => {
    expect(limitProgress(100, 500)).toEqual({ pct: 20, state: 'ok', remaining: 400 })
  })

  it('warns from 80% of the limit up to the limit itself', () => {
    expect(limitProgress(400, 500).state).toBe('warn')
    expect(limitProgress(399, 500).state).toBe('ok')
    expect(limitProgress(500, 500)).toEqual({ pct: 100, state: 'warn', remaining: 0 })
  })

  it('is over once the limit is passed, with the bar capped and the overage negative', () => {
    expect(limitProgress(620, 500)).toEqual({ pct: 100, state: 'over', remaining: -120 })
  })

  it('handles no spending, and a missing limit, without dividing by zero', () => {
    expect(limitProgress(0, 500)).toEqual({ pct: 0, state: 'ok', remaining: 500 })
    expect(limitProgress(50, 0).pct).toBe(0)
  })
})

describe('roundLimit', () => {
  it('rounds up to a tidy figure that grows with the amount', () => {
    expect(roundLimit(42.1)).toBe(45)
    expect(roundLimit(100)).toBe(100)
    expect(roundLimit(412.5)).toBe(420)
    expect(roundLimit(1203)).toBe(1250)
  })

  it('never rounds down below what the user actually spends', () => {
    for (const v of [1.2, 37, 99.99, 101, 999, 1001, 5400]) expect(roundLimit(v)).toBeGreaterThanOrEqual(v)
  })
})

describe('suggestLimits', () => {
  // Mid-October 2026: the window is Jul, Aug, Sep.
  const NOW = new Date(2026, 9, 15)
  const row = (month: number, categoryName: string, total: number) => ({ month, categoryName, total })

  it('averages each category over the three completed months', () => {
    const out = suggestLimits(
      {
        2026: [
          row(7, 'Groceries', 300),
          row(8, 'Groceries', 450),
          row(9, 'Groceries', 390), // 1140 / 3 = 380
          row(9, 'Dining', 60), // only in one of three months: 60 / 3 = 20
          row(10, 'Groceries', 9999), // the current month is ignored
        ],
      },
      NOW,
    )
    expect(out.get('Groceries')).toBe(380)
    expect(out.get('Dining')).toBe(20)
  })

  it('averages over the months that have data, not always three', () => {
    // Only August and September have any spending: 400 / 2 = 200, not 400 / 3.
    const out = suggestLimits({ 2026: [row(8, 'Groceries', 150), row(9, 'Groceries', 250)] }, NOW)
    expect(out.get('Groceries')).toBe(200)
  })

  it('reads the window across a year boundary', () => {
    // Mid-January 2026: the window is Oct, Nov, Dec of 2025.
    const out = suggestLimits(
      { 2025: [row(10, 'Fuel', 90), row(11, 'Fuel', 90), row(12, 'Fuel', 90)], 2026: [row(1, 'Fuel', 500)] },
      new Date(2026, 0, 15),
    )
    expect(out.get('Fuel')).toBe(90)
  })

  it('suggests nothing without history, and skips trivial averages', () => {
    expect(suggestLimits({}, NOW).size).toBe(0)
    expect(suggestLimits({ 2026: [row(10, 'Groceries', 500)] }, NOW).size).toBe(0) // current month only
    expect(suggestLimits({ 2026: [row(9, 'Pennies', 0.5), row(9, 'Groceries', 300)] }, NOW).has('Pennies')).toBe(false)
  })
})
