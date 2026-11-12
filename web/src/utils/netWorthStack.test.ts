import { describe, expect, it } from 'vitest'
import type { AccountHistory } from '../api/client'
import { orderSeries, seriesColors, stackBounds, toRows, withLiveBalances } from './netWorthStack'

const history: AccountHistory = {
  series: [
    { id: 'brokerage', name: 'Brokerage', accountType: 'brokerage' },
    { id: 'card', name: 'Visa', accountType: 'credit_card' },
    { id: 'savings', name: 'Savings', accountType: 'savings' },
  ],
  points: [
    { date: '2026-01-01', balances: { brokerage: 5000, card: -400, savings: 1000 } },
    { date: '2026-02-01', balances: { brokerage: 5200, card: -900, savings: 800 } },
  ],
}

describe('stackBounds', () => {
  it('is the largest positive stack and the deepest negative one', () => {
    // Jan: up 6000, down -400. Feb: up 6000, down -900.
    expect(stackBounds(history.points)).toEqual({ min: -900, max: 6000 })
  })

  it('always includes zero, even when everything is on one side', () => {
    expect(stackBounds([{ date: 'd', balances: { a: 100, b: 50 } }])).toEqual({ min: 0, max: 150 })
    expect(stackBounds([{ date: 'd', balances: { a: -100 } }])).toEqual({ min: -100, max: 0 })
    expect(stackBounds([])).toEqual({ min: 0, max: 0 })
  })

  it('takes the maximum across dates, not the last one', () => {
    const pts = [
      { date: '1', balances: { a: 900 } },
      { date: '2', balances: { a: 100 } },
    ]
    expect(stackBounds(pts).max).toBe(900)
  })
})

describe('orderSeries', () => {
  it('puts the largest latest balance first, by size whether asset or liability', () => {
    // Latest: brokerage 5200, card -900, savings 800.
    expect(orderSeries(history).map((s) => s.id)).toEqual(['brokerage', 'card', 'savings'])
  })

  it('keeps the original order when nothing separates the series', () => {
    const flat: AccountHistory = { series: history.series, points: [] }
    expect(orderSeries(flat).map((s) => s.id)).toEqual(['brokerage', 'card', 'savings'])
  })

  it('does not modify the history', () => {
    const before = history.series.map((s) => s.id)
    orderSeries(history)
    expect(history.series.map((s) => s.id)).toEqual(before)
  })
})

describe('toRows', () => {
  it('makes a row per date with a key per series', () => {
    expect(toRows(history)).toEqual([
      { date: '2026-01-01', brokerage: 5000, card: -400, savings: 1000 },
      { date: '2026-02-01', brokerage: 5200, card: -900, savings: 800 },
    ])
  })

  it('fills a series that is missing at a date with zero, e.g. an account opened later', () => {
    const later: AccountHistory = {
      series: [{ id: 'new', name: 'New', accountType: 'savings' }, { id: 'old', name: 'Old', accountType: 'savings' }],
      points: [{ date: '1', balances: { old: 10 } }, { date: '2', balances: { old: 10, new: 5 } }],
    }
    expect(toRows(later)[0]).toEqual({ date: '1', new: 0, old: 10 })
  })
})

describe('withLiveBalances', () => {
  it('replaces the last date with today and the live balances, leaving earlier dates alone', () => {
    const out = withLiveBalances(history, { brokerage: 5400, card: -850, savings: 790 }, '2026-02-19')
    expect(out.points).toHaveLength(2)
    expect(out.points[0]).toEqual(history.points[0])
    expect(out.points[1]).toEqual({ date: '2026-02-19', balances: { brokerage: 5400, card: -850, savings: 790 } })
  })

  it('keeps the snapshot balance for a series with no live figure, such as closed accounts', () => {
    const out = withLiveBalances(history, { brokerage: 5400 }, '2026-02-19')
    expect(out.points[1].balances).toEqual({ brokerage: 5400, card: -900, savings: 800 })
  })

  it('ignores live balances for accounts that are not in the history', () => {
    const out = withLiveBalances(history, { stranger: 999 }, '2026-02-19')
    expect(out.points[1].balances).not.toHaveProperty('stranger')
  })

  it('does not modify the input, and returns an empty history as is', () => {
    const before = JSON.stringify(history)
    withLiveBalances(history, { brokerage: 1 }, '2026-02-19')
    expect(JSON.stringify(history)).toBe(before)
    const empty: AccountHistory = { series: [], points: [] }
    expect(withLiveBalances(empty, {}, '2026-02-19')).toBe(empty)
  })
})

describe('seriesColors', () => {
  const h: AccountHistory = {
    series: [
      { id: 'brk', name: 'Brokerage', accountType: 'brokerage' },
      { id: 'card', name: 'Card', accountType: 'credit_card' },
      { id: 'home', name: 'Home', accountType: 'home' },
      { id: 'car', name: 'Car loan', accountType: 'loan' },
      { id: 'paid', name: 'Paid-off card', accountType: 'credit_card' },
      { id: 'cash', name: 'Savings', accountType: 'savings' },
    ],
    points: [{ date: '2026-10-01', balances: { brk: 5000, card: -800, home: -2000, car: -9000, paid: 0, cash: 300 } }],
  }

  it('gives accounts below zero and liability types the liability shades, in order', () => {
    const colors = seriesColors(h, h.series, ['a1', 'a2'], ['l1', 'l2', 'l3'])
    expect(colors).toEqual({ brk: 'a1', card: 'l1', home: 'l2', car: 'l3', paid: 'l1', cash: 'a2' })
  })
})
