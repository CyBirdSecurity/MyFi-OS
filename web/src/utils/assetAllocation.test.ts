import { describe, expect, it } from 'vitest'
import { allocationByAssetClass } from './assetAllocation'
import type { AccountBalance, Holding } from '../api/client'

const acc = (accountId: string, accountType: string, balance: number): AccountBalance => ({
  accountId,
  name: accountId,
  accountType,
  taxTreatment: 'taxable',
  balance,
})
const hold = (accountId: string, assetClass: string, totalValue: number) => ({ accountId, assetClass, totalValue }) as Holding

const asMap = (slices: { key: string; value: number }[]) => Object.fromEntries(slices.map((s) => [s.key, Math.round(s.value)]))

describe('allocationByAssetClass', () => {
  it('splits holdings by asset class and adds them up across accounts', () => {
    const slices = allocationByAssetClass(
      [acc('brk', 'brokerage', 1000), acc('ira', 'roth_ira', 500)],
      [hold('brk', 'equity', 600), hold('brk', 'bond', 400), hold('ira', 'equity', 500)],
    )
    expect(asMap(slices)).toEqual({ equity: 1100, bond: 400 })
    expect(slices[0]).toMatchObject({ key: 'equity', name: 'Equity' })
  })

  it('puts balances without holdings by account kind', () => {
    const slices = allocationByAssetClass(
      [acc('chk', 'checking', 200), acc('home', 'home', 300000), acc('car', 'automobile', 15000), acc('k', 'traditional_401k', 50000)],
      [],
    )
    expect(asMap(slices)).toEqual({ cash_equivalent: 200, real_estate: 300000, vehicles: 15000, unitemized: 50000 })
  })

  it('counts uninvested money in an account as unitemized', () => {
    expect(asMap(allocationByAssetClass([acc('brk', 'brokerage', 1000)], [hold('brk', 'equity', 900)]))).toEqual({
      equity: 900,
      unitemized: 100,
    })
  })

  it('scales holdings down to the account balance so totals match', () => {
    const slices = allocationByAssetClass([acc('brk', 'brokerage', 900)], [hold('brk', 'equity', 600), hold('brk', 'bond', 600)])
    expect(asMap(slices)).toEqual({ equity: 450, bond: 450 })
  })

  it('leaves out liabilities and empty accounts', () => {
    expect(allocationByAssetClass([acc('cc', 'credit_card', 500), acc('sv', 'savings', 0)], [])).toEqual([])
  })
})
