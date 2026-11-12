import { describe, expect, it } from 'vitest'
import type { ForecastYear } from '../api/client'
import { findFIYear, yearAtRetirement } from './fi'

const years = [900_000, 1_020_000, 1_070_000].map((total, i): ForecastYear => ({ year: 2026 + i, age: 40 + i, accounts: {}, total }))

describe('findFIYear', () => {
  it('grows the FI number with inflation in a nominal projection', () => {
    expect(findFIYear(years, { fiNumber: 1_000_000, fiNumberGrowth: 0.03 })?.year).toBe(2028)
  })

  it('compares directly when the projection is in today’s dollars', () => {
    expect(findFIYear(years, { fiNumber: 1_000_000, fiNumberGrowth: 0 })?.year).toBe(2027)
  })

  it('is never reached with no FI number', () => {
    expect(findFIYear(years, { fiNumber: 0, fiNumberGrowth: 0 })).toBeUndefined()
  })
})

describe('yearAtRetirement', () => {
  it('is the last working year, before any retirement withdrawals', () => {
    expect(yearAtRetirement(years, 2028)?.year).toBe(2027)
  })

  it('is the retirement year when retiring this year', () => {
    expect(yearAtRetirement(years, 2026)?.year).toBe(2026)
  })
})
