import { describe, expect, it } from 'vitest'
import { fundTypeBreakdown, fundTypeOf, sizeBreakdown, sizeOf } from './fundProfile'
import type { Holding } from '../api/client'

const h = (ticker: string, instrumentType: string | undefined, quoteName: string, totalValue = 100, assetClass = 'equity') =>
  ({ ticker, instrumentType, quoteName, totalValue, assetClass }) as Holding

describe('fundTypeOf', () => {
  it('reads the quote instrument type', () => {
    expect(fundTypeOf(h('VTI', 'ETF', ''))).toBe('etf')
    expect(fundTypeOf(h('FXAIX', 'MUTUALFUND', ''))).toBe('mutual_fund')
    expect(fundTypeOf(h('AAPL', 'EQUITY', ''))).toBe('stock')
    expect(fundTypeOf(h('BTC-USD', 'CRYPTOCURRENCY', ''))).toBe('crypto')
  })

  it('treats cash as cash and a missing quote as unknown', () => {
    expect(fundTypeOf(h('CASH', undefined, '', 100, 'cash_equivalent'))).toBe('cash')
    expect(fundTypeOf(h('MYFUND', undefined, ''))).toBe('unknown')
  })
})

describe('sizeOf', () => {
  it.each([
    ['Avantis US Small Cap Value ETF', 'small'],
    ['iShares Russell 2000 ETF', 'small'],
    ['Vanguard Small-Cap Index Fund', 'small'],
    ['Vanguard Mid-Cap ETF', 'mid'],
    ['iShares Core S&P Small-Mid Cap ETF', 'mid'],
    ['Vanguard Extended Market Index Fund', 'mid'],
    ['Fidelity 500 Index', 'large'],
    ['Vanguard S&P 500 ETF', 'large'],
    ['Invesco QQQ Trust, Series 1 Nasdaq-100', 'large'],
    ['Vanguard Mega Cap Growth ETF', 'large'],
    ['Vanguard Total Stock Market ETF', 'total'],
    ['Vanguard Total International Stock ETF', 'total'],
    ['Schwab U.S. Dividend Equity ETF', 'unknown'],
  ])('%s -> %s', (name, size) => {
    expect(sizeOf(h('X', 'ETF', name))).toBe(size)
  })

  it('does not size individual stocks or non-stock holdings', () => {
    expect(sizeOf(h('AAPL', 'EQUITY', 'Apple Inc.'))).toBe('stocks')
    expect(sizeOf(h('BND', 'ETF', 'Vanguard Total Bond Market', 100, 'bond'))).toBeNull()
  })

  it('falls back to the saved name when there is no quote name', () => {
    expect(sizeOf({ ticker: 'X', assetClass: 'equity', instrumentType: 'MUTUALFUND', name: 'Small Cap Index' } as Holding)).toBe('small')
  })
})

describe('breakdowns', () => {
  const holdings = [
    h('VTI', 'ETF', 'Vanguard Total Stock Market ETF', 500),
    h('AVUV', 'ETF', 'Avantis US Small Cap Value ETF', 200),
    h('FXAIX', 'MUTUALFUND', 'Fidelity 500 Index', 200),
    h('AAPL', 'EQUITY', 'Apple Inc.', 100),
    h('BND', 'ETF', 'Vanguard Total Bond Market', 300, 'bond'),
    h('CASH', undefined, '', 0, 'cash_equivalent'),
  ]

  it('splits all holdings by fund type with percentages', () => {
    expect(fundTypeBreakdown(holdings).map((b) => [b.key, b.value, Math.round(b.pct)])).toEqual([
      ['etf', 1000, 77],
      ['mutual_fund', 200, 15],
      ['stock', 100, 8],
    ])
  })

  it('splits only stock holdings by size', () => {
    expect(sizeBreakdown(holdings).map((b) => [b.key, b.value])).toEqual([
      ['total', 500],
      ['small', 200],
      ['large', 200],
      ['stocks', 100],
    ])
  })
})
