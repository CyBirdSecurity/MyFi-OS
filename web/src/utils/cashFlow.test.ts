import { describe, expect, it } from 'vitest'
import { averageSavingsRate, buildMonths, savingsRate, saved, summarize } from './cashFlow'

// Mid-October 2026.
const NOW = new Date(2026, 9, 15)

describe('buildMonths', () => {
  it('returns the last six months ending this month, oldest first, across a year boundary', () => {
    const months = buildMonths({}, {}, new Date(2026, 1, 10), 6) // Feb 2026
    expect(months.map((m) => `${m.year}-${m.month}`)).toEqual(['2025-9', '2025-10', '2025-11', '2025-12', '2026-1', '2026-2'])
    expect(months.map((m) => m.label)).toEqual(['Sep', 'Oct', 'Nov', 'Dec', 'Jan', 'Feb'])
  })

  it('sums category rows into one spending figure per month and picks the right year', () => {
    const months = buildMonths(
      {
        2026: [
          { month: 10, categoryName: 'Groceries', total: 100 },
          { month: 10, categoryName: 'Dining', total: 50 },
          { month: 9, categoryName: 'Groceries', total: 80 },
        ],
        2025: [{ month: 12, categoryName: 'Groceries', total: 999 }],
      },
      { 2026: [{ month: 10, total: 4000 }, { month: 9, total: 3500 }] },
      NOW,
      2,
    )
    expect(months).toEqual([
      { year: 2026, month: 9, label: 'Sep', income: 3500, spending: 80 },
      { year: 2026, month: 10, label: 'Oct', income: 4000, spending: 150 },
    ])
  })

  it('treats months with no data as zeros', () => {
    expect(buildMonths({}, {}, NOW, 1)).toEqual([{ year: 2026, month: 10, label: 'Oct', income: 0, spending: 0 }])
  })
})

describe('savingsRate', () => {
  it('is the share of income not spent', () => {
    expect(savingsRate(4000, 3000)).toBe(25)
    expect(savingsRate(4000, 0)).toBe(100)
  })
  it('is negative when spending exceeds income', () => {
    expect(savingsRate(1000, 1500)).toBe(-50)
  })
  it('is null with no income, so there is no divide-by-zero or misleading figure', () => {
    expect(savingsRate(0, 500)).toBeNull()
  })
})

describe('averageSavingsRate', () => {
  it('weights by income: total saved over total income', () => {
    // Saved 1000 of 2000 and 0 of 8000 -> 1000 of 10000 = 10%, not the 25% mean of 50% and 0%.
    expect(averageSavingsRate([{ income: 2000, spending: 1000 }, { income: 8000, spending: 8000 }])).toBe(10)
  })
  it('skips months with no income', () => {
    expect(averageSavingsRate([{ income: 0, spending: 300 }, { income: 1000, spending: 500 }])).toBe(50)
  })
  it('is null when no month has income', () => {
    expect(averageSavingsRate([{ income: 0, spending: 300 }])).toBeNull()
    expect(averageSavingsRate([])).toBeNull()
  })
})

describe('summarize', () => {
  const m = (month: number, income: number, spending: number) => ({ year: 2026, month, label: String(month), income, spending })

  it('reports this month and averages the three completed months before it', () => {
    const s = summarize([m(5, 9999, 0), m(6, 1000, 500), m(7, 1000, 500), m(8, 1000, 500), m(9, 0, 0), m(10, 100, 400)])
    expect(s.current.month).toBe(10)
    expect(s.currentRate).toBe(-300)
    // Only the three months before this one count (Jul, Aug, Sep). May is too old,
    // and Sep has no income so it is skipped: Jul and Aug each saved 500 of 1000.
    expect(s.averageRate).toBe(50)
  })

  it('does not let the partial current month distort the average', () => {
    const s = summarize([m(8, 1000, 500), m(9, 1000, 500), m(10, 0, 3000)])
    expect(s.averageRate).toBe(50)
  })

  it('has data when any month has income or spending, and none otherwise', () => {
    expect(summarize([m(9, 0, 0), m(10, 0, 0)]).hasData).toBe(false)
    expect(summarize([m(9, 0, 0), m(10, 0, 20)]).hasData).toBe(true)
  })
})

describe('saved', () => {
  it('is income minus spending', () => {
    expect(saved({ income: 4000, spending: 3100 })).toBe(900)
    expect(saved({ income: 0, spending: 50 })).toBe(-50)
  })
})
