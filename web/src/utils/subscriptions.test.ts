import { describe, expect, it } from 'vitest'
import { detectSubscriptions, displayMerchant, merchantKey, subscriptionTotals, type TxnLike } from './subscriptions'

// Mid-October 2026.
const TODAY = new Date(2026, 9, 15)

const t = (transDate: string, merchant: string, amount: number, categoryName?: string): TxnLike => ({
  transDate,
  merchant,
  amount,
  categoryName,
})

/** The same charge on the same day of each month, for the given months of 2026. */
const monthly = (merchant: string, amount: number, day: number, months: number[]) =>
  months.map((m) => t(`2026-${String(m).padStart(2, '0')}-${String(day).padStart(2, '0')}`, merchant, amount))

describe('merchantKey', () => {
  it('groups the ways one merchant is written on statements', () => {
    expect(merchantKey('NETFLIX.COM 866-579-7172')).toBe(merchantKey('Netflix.com 800-555-0100'))
    expect(merchantKey('SPOTIFY USA #1234')).toBe('spotify')
    expect(merchantKey('Netflix.com')).toBe(merchantKey('NETFLIX'))
  })

  it('drops reference codes that change every charge', () => {
    expect(merchantKey('SPOTIFY P2B4E8C1F2')).toBe('spotify')
    expect(merchantKey('Spotify P3A91C0D4E')).toBe('spotify')
    expect(merchantKey('SPOTIFY P1234ABC STOCKHOLM')).toBe('spotify stockholm')
  })

  it('keeps a merchant name that starts with digits', () => {
    expect(merchantKey('1PASSWORD*BILLING')).toBe('password billing')
    expect(merchantKey('7-Eleven 1234')).toBe('eleven')
  })

  it('keeps different merchants apart', () => {
    expect(merchantKey('Amazon Prime')).not.toBe(merchantKey('Amazon Marketplace'))
  })

  it('is empty when nothing but digits and symbols remain', () => {
    expect(merchantKey('1234-5678')).toBe('')
  })
})

describe('displayMerchant', () => {
  it('drops trailing reference numbers from the statement text', () => {
    expect(displayMerchant('NETFLIX.COM 866-579')).toBe('NETFLIX.COM')
    expect(displayMerchant('SPOTIFY USA #1234')).toBe('SPOTIFY USA')
    expect(displayMerchant('  Hulu 800 555 0100 ')).toBe('Hulu')
  })

  it('leaves a clean name alone, and keeps a name that is only digits', () => {
    expect(displayMerchant('Amazon Prime')).toBe('Amazon Prime')
    expect(displayMerchant('7-Eleven')).toBe('7-Eleven')
  })
})

describe('detectSubscriptions', () => {
  it('finds a monthly subscription with its cost, next charge and yearly total', () => {
    const [sub] = detectSubscriptions(monthly('Netflix', 15.49, 2, [6, 7, 8, 9, 10]), TODAY)
    expect(sub).toMatchObject({
      merchant: 'Netflix',
      cadence: 'monthly',
      amount: 15.49,
      lastDate: '2026-10-02',
      nextDate: '2026-11-02',
      charges: 5,
      monthlyCost: 15.49,
    })
    expect(sub.yearlyCost).toBeCloseTo(185.88, 2)
  })

  it('groups differently written merchant names and uses the latest amount', () => {
    const [sub] = detectSubscriptions(
      [
        t('2026-08-02', 'NETFLIX.COM 866-579', 15.49),
        t('2026-09-02', 'Netflix.com 800-555', 15.49),
        t('2026-10-02', 'NETFLIX', 15.49),
      ],
      TODAY,
    )
    expect(sub.charges).toBe(3)
  })

  it('tolerates a modest price increase and reports the new price', () => {
    const [sub] = detectSubscriptions(
      [t('2026-06-05', 'Spotify', 10.99), t('2026-07-05', 'Spotify', 10.99), t('2026-08-05', 'Spotify', 10.99),
       t('2026-09-05', 'Spotify', 11.99), t('2026-10-05', 'Spotify', 11.99)],
      TODAY,
    )
    expect(sub.amount).toBe(11.99)
  })

  it('tolerates a charge that lands a few days early or late', () => {
    const subs = detectSubscriptions(
      [t('2026-07-01', 'Gym', 40), t('2026-07-31', 'Gym', 40), t('2026-09-02', 'Gym', 40), t('2026-10-01', 'Gym', 40)],
      TODAY,
    )
    expect(subs).toHaveLength(1)
  })

  it('costs a yearly charge per month', () => {
    const [sub] = detectSubscriptions([t('2024-12-01', 'Domain Renewal', 120), t('2025-12-01', 'Domain Renewal', 120)], new Date(2026, 5, 1))
    expect(sub).toMatchObject({ cadence: 'yearly', yearlyCost: 120, monthlyCost: 10, nextDate: '2026-12-01' })
  })

  it('does not treat irregular shopping as a subscription', () => {
    const groceries = ['2026-06-03', '2026-06-19', '2026-07-02', '2026-07-30', '2026-08-04', '2026-09-12', '2026-10-01']
    expect(detectSubscriptions(groceries.map((d) => t(d, 'Aldi', 60)), TODAY)).toEqual([])
  })

  it('does not treat a regular but wildly varying charge as a subscription', () => {
    const subs = detectSubscriptions(
      [t('2026-07-01', 'Restaurant', 20), t('2026-08-01', 'Restaurant', 95), t('2026-09-01', 'Restaurant', 30), t('2026-10-01', 'Restaurant', 140)],
      TODAY,
    )
    expect(subs).toEqual([])
  })

  it('needs at least three monthly charges', () => {
    expect(detectSubscriptions(monthly('Netflix', 15.49, 2, [9, 10]), TODAY)).toEqual([])
  })

  it('leaves out a subscription that has stopped', () => {
    // Last charged in May: well over 1.5 months ago.
    expect(detectSubscriptions(monthly('Old Gym', 30, 10, [2, 3, 4, 5]), TODAY)).toEqual([])
  })

  it('ignores transfers, income and refunds', () => {
    const rows = [
      ...monthly('Savings Transfer', 500, 1, [8, 9, 10]).map((x) => ({ ...x, categoryName: 'Transfer' })),
      ...monthly('Paycheck', 2000, 1, [8, 9, 10]).map((x) => ({ ...x, categoryName: 'Income' })),
      ...monthly('Refund Co', -15, 1, [8, 9, 10]),
    ]
    expect(detectSubscriptions(rows, TODAY)).toEqual([])
  })

  it('finds Spotify when every charge carries a different reference code', () => {
    const codes = ['P2B4E8C1F2', 'P3A91C0D4E', 'P7Q2W9E4R1', 'P0Z8X6C4V2']
    const rows = [7, 8, 9, 10].map((m, i) => t(`2026-${String(m).padStart(2, '0')}-12`, `SPOTIFY ${codes[i]}`, 11.99))
    const [sub] = detectSubscriptions(rows, TODAY)
    expect(sub).toMatchObject({ merchant: 'SPOTIFY', charges: 4, duplicates: [] })
  })

  it('counts a duplicated same-day charge once and flags it', () => {
    const rows = [...monthly('Netflix', 15.49, 2, [8, 9, 10]), t('2026-10-02', 'Netflix', 15.49)]
    const [sub] = detectSubscriptions(rows, TODAY)
    expect(sub.charges).toBe(3)
    expect(sub.duplicates).toEqual([{ date: '2026-10-02', charges: 2, total: 30.98 }])
  })

  it('still finds a subscription charged twice in one month, and flags that month', () => {
    const rows = [...monthly('Hulu', 17.99, 3, [7, 8, 9, 10]), t('2026-09-14', 'Hulu', 17.99)]
    const [sub] = detectSubscriptions(rows, TODAY)
    expect(sub).toMatchObject({ cadence: 'monthly', charges: 4, nextDate: '2026-11-03' })
    expect(sub.duplicates).toEqual([{ date: '2026-09-03', charges: 2, total: 35.98 }])
  })

  it('flags every month when two plans with one merchant bill on different days', () => {
    const rows = [...monthly('Spotify', 11.99, 5, [7, 8, 9, 10]), ...monthly('Spotify', 11.99, 19, [7, 8, 9, 10])]
    const [sub] = detectSubscriptions(rows, TODAY)
    expect(sub.duplicates.map((d) => d.date)).toEqual(['2026-10-05', '2026-09-05', '2026-08-05', '2026-07-05'])
  })

  it('does not flag charges at the start and end of the same month as duplicates', () => {
    const [sub] = detectSubscriptions(
      [t('2026-07-01', 'Gym', 40), t('2026-07-31', 'Gym', 40), t('2026-09-02', 'Gym', 40), t('2026-10-01', 'Gym', 40)],
      TODAY,
    )
    expect(sub.duplicates).toEqual([])
  })

  it('accepts ISO timestamps for the date', () => {
    const rows = monthly('Netflix', 15.49, 2, [8, 9, 10]).map((x) => ({ ...x, transDate: `${x.transDate}T00:00:00Z` }))
    expect(detectSubscriptions(rows, TODAY)[0].lastDate).toBe('2026-10-02')
  })

  it('clamps the next charge to the end of a shorter month', () => {
    const [sub] = detectSubscriptions(
      [t('2026-11-30', 'Cloud', 5), t('2026-12-30', 'Cloud', 5), t('2027-01-30', 'Cloud', 5)].map((x) => x),
      new Date(2027, 1, 5),
    )
    expect(sub.nextDate).toBe('2027-02-28')
  })

  it('lists the most expensive subscription first', () => {
    const subs = detectSubscriptions(
      [...monthly('Streaming', 9.99, 3, [8, 9, 10]), ...monthly('Gym', 45, 4, [8, 9, 10])],
      TODAY,
    )
    expect(subs.map((s) => s.merchant)).toEqual(['Gym', 'Streaming'])
  })
})

describe('subscriptionTotals', () => {
  it('adds up monthly and yearly cost', () => {
    expect(subscriptionTotals([{ monthlyCost: 10, yearlyCost: 120 }, { monthlyCost: 5, yearlyCost: 60 }])).toEqual({
      monthly: 15,
      yearly: 180,
    })
    expect(subscriptionTotals([])).toEqual({ monthly: 0, yearly: 0 })
  })
})
