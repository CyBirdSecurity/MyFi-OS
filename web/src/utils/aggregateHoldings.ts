import type { AccountBalance, Holding } from '../api/client'

/** One row of the Holdings page: a ticker combined across every account that holds it. */
export type AggRow = {
  ticker: string
  name?: string
  assetClass: string
  shares: number
  totalValue: number
  avgCostBasis?: number
  sharesWithCost: number
  pricePerShare?: number
  priceFetchedAt?: string
  lots: Holding[]
}

const CASH_TICKER = 'CASH'
const CASH_CLASS = 'cash_equivalent'

/**
 * Combines holdings into one row per ticker across accounts, with the average
 * cost per share weighted by shares.
 *
 * All cash is a single row, however it is held: cash holdings under any ticker
 * (a brokerage's cash line, a money-market sweep) and positive checking/savings
 * balances (`cashAccountTypes`) are added together. Cash has no price or cost
 * basis, so none is kept for it.
 */
export function aggregateHoldings(
  holdings: Holding[],
  accounts: Pick<AccountBalance, 'accountType' | 'balance'>[],
  cashAccountTypes: Set<string>,
): AggRow[] {
  const rows: Record<string, AggRow> = {}

  const addCash = (shares: number, value: number, lot?: Holding) => {
    const row = (rows[CASH_TICKER] ??= {
      ticker: CASH_TICKER,
      assetClass: CASH_CLASS,
      shares: 0,
      totalValue: 0,
      sharesWithCost: 0,
      lots: [],
    })
    row.shares += shares
    row.totalValue += value
    if (lot) row.lots.push(lot)
  }

  for (const h of holdings) {
    if (h.assetClass === CASH_CLASS) {
      addCash(h.shares, h.totalValue, h)
      continue
    }
    const row = rows[h.ticker]
    if (!row) {
      rows[h.ticker] = {
        ticker: h.ticker,
        name: h.name,
        assetClass: h.assetClass,
        shares: h.shares,
        totalValue: h.totalValue,
        avgCostBasis: h.costBasis ?? undefined,
        sharesWithCost: h.costBasis != null ? h.shares : 0,
        pricePerShare: h.pricePerShare,
        priceFetchedAt: h.priceFetchedAt,
        lots: [h],
      }
      continue
    }
    row.shares += h.shares
    row.totalValue += h.totalValue
    row.lots.push(h)
    if (h.costBasis != null) {
      row.avgCostBasis =
        row.avgCostBasis != null
          ? (row.avgCostBasis * row.sharesWithCost + h.costBasis * h.shares) / (row.sharesWithCost + h.shares)
          : h.costBasis
      row.sharesWithCost += h.shares
    }
  }

  for (const a of accounts) {
    if (cashAccountTypes.has(a.accountType) && a.balance > 0) addCash(a.balance, a.balance)
  }

  return Object.values(rows)
}
