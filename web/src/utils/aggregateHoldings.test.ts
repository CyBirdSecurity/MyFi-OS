import { describe, expect, it } from 'vitest'
import type { Holding } from '../api/client'
import { aggregateHoldings } from './aggregateHoldings'

const CASH_TYPES = new Set(['checking', 'savings'])

const holding = (over: Partial<Holding>): Holding => ({
  id: crypto.randomUUID(),
  accountId: 'acc',
  ticker: 'VTI',
  assetClass: 'equity',
  shares: 10,
  manualPrice: false,
  dripEnabled: false,
  totalValue: 2500,
  createdAt: '',
  ...over,
})

const cash = (ticker: string, amount: number, accountId = 'acc') =>
  holding({ ticker, accountId, assetClass: 'cash_equivalent', shares: amount, totalValue: amount })

const cashRows = (rows: ReturnType<typeof aggregateHoldings>) => rows.filter((r) => r.assetClass === 'cash_equivalent')

describe('aggregateHoldings: cash', () => {
  it('shows one Cash row that sums every cash holding, whatever its ticker', () => {
    const rows = aggregateHoldings([cash('CASH', 1000, 'a'), cash('SPAXX', 2500, 'b'), cash('USD', 300, 'c')], [], CASH_TYPES)
    const [row, ...rest] = cashRows(rows)
    expect(rest).toEqual([])
    expect(row).toMatchObject({ ticker: 'CASH', shares: 3800, totalValue: 3800 })
  })

  it('adds checking and savings balances into the same row', () => {
    const rows = aggregateHoldings(
      [cash('CASH', 1000)],
      [
        { accountType: 'checking', balance: 400 },
        { accountType: 'savings', balance: 600 },
      ],
      CASH_TYPES,
    )
    expect(cashRows(rows)).toHaveLength(1)
    expect(cashRows(rows)[0].totalValue).toBe(2000)
  })

  it('makes a Cash row from account balances alone', () => {
    const rows = aggregateHoldings([], [{ accountType: 'checking', balance: 750 }], CASH_TYPES)
    expect(rows).toEqual([
      { ticker: 'CASH', assetClass: 'cash_equivalent', shares: 750, totalValue: 750, sharesWithCost: 0, lots: [] },
    ])
  })

  it('ignores overdrawn and non-cash accounts', () => {
    const rows = aggregateHoldings(
      [],
      [
        { accountType: 'checking', balance: -50 },
        { accountType: 'credit_card', balance: 900 },
        { accountType: 'brokerage', balance: 5000 },
      ],
      CASH_TYPES,
    )
    expect(rows).toEqual([])
  })

  it('has no Cash row when there is no cash', () => {
    expect(cashRows(aggregateHoldings([holding({})], [], CASH_TYPES))).toEqual([])
  })

  it('keeps no price or cost basis on cash', () => {
    const row = cashRows(aggregateHoldings([holding({ ticker: 'SPAXX', assetClass: 'cash_equivalent', shares: 100, totalValue: 100, costBasis: 100, pricePerShare: 1 })], [], CASH_TYPES))[0]
    expect(row.avgCostBasis).toBeUndefined()
    expect(row.pricePerShare).toBeUndefined()
  })
})

describe('aggregateHoldings: securities', () => {
  it('combines the same ticker across accounts', () => {
    const rows = aggregateHoldings(
      [holding({ ticker: 'VTI', shares: 10, totalValue: 2500, accountId: 'a' }), holding({ ticker: 'VTI', shares: 5, totalValue: 1250, accountId: 'b' })],
      [],
      CASH_TYPES,
    )
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ ticker: 'VTI', shares: 15, totalValue: 3750 })
    expect(rows[0].lots).toHaveLength(2)
  })

  it('weights the average cost per share by shares', () => {
    // 10 shares at $100 and 30 at $200 -> (1000 + 6000) / 40 = $175.
    const [row] = aggregateHoldings([holding({ shares: 10, costBasis: 100 }), holding({ shares: 30, costBasis: 200 })], [], CASH_TYPES)
    expect(row.avgCostBasis).toBe(175)
    expect(row.sharesWithCost).toBe(40)
  })

  it('leaves shares with no cost basis out of the average', () => {
    const [row] = aggregateHoldings([holding({ shares: 10, costBasis: 100 }), holding({ shares: 30 })], [], CASH_TYPES)
    expect(row.avgCostBasis).toBe(100)
    expect(row.sharesWithCost).toBe(10)
  })

  it('keeps different tickers apart', () => {
    expect(aggregateHoldings([holding({ ticker: 'VTI' }), holding({ ticker: 'BND', assetClass: 'bond' })], [], CASH_TYPES)).toHaveLength(2)
  })
})
