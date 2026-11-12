// Finds recurring charges (subscriptions) in a user's transactions: a merchant
// that charges a similar amount at a regular interval.

export interface TxnLike {
  transDate: string // "YYYY-MM-DD", or an ISO timestamp beginning with that date
  merchant: string
  amount: number // positive = money spent
  categoryName?: string
}

export type Cadence = 'monthly' | 'yearly'

export interface Subscription {
  /** The merchant grouping key (merchantKey); what a dismissal is saved under. */
  key: string
  merchant: string
  cadence: Cadence
  /** The most recent charge. */
  amount: number
  lastDate: string // YYYY-MM-DD
  nextDate: string // YYYY-MM-DD, the expected next charge
  charges: number
  monthlyCost: number
  yearlyCost: number
  categoryName?: string
  /** Billing cycles with more than one charge, most recent first. */
  duplicates: DuplicateCharge[]
}

/** A billing cycle in which the merchant charged more than once. */
export interface DuplicateCharge {
  date: string // YYYY-MM-DD, the cycle's first charge
  charges: number
  total: number
}

const DAY = 24 * 60 * 60 * 1000

// Days between charges that still count as one interval.
const MONTHLY_GAP = { min: 26, max: 35 }
const YEARLY_GAP = { min: 350, max: 380 }
/** Share of intervals that must fit the cadence, so one skipped or doubled month doesn't hide a subscription. */
const MIN_REGULAR = 0.7
/** Charges within this fraction of the typical amount count as "the same price", so a modest increase is fine. */
const AMOUNT_TOLERANCE = 0.25
const MIN_STABLE = 0.7
/** A subscription is considered ended once this many intervals pass without a charge. */
const ACTIVE_FOR = 1.5
/** A charge this soon after a cycle's first charge belongs to that cycle (a double charge or a second plan). */
const SAME_CYCLE_DAYS = 20

const NOT_SPENDING = new Set(['transfer', 'income'])

/** Words that statements add to a merchant's name but that don't identify it. */
const FILLER_WORDS = new Set(['com', 'net', 'org', 'www', 'inc', 'llc', 'ltd', 'co', 'usa', 'us'])

function toDay(s: string): number {
  const [y, m, d] = s.slice(0, 10).split('-').map(Number)
  return Date.UTC(y, m - 1, d)
}

function fromDay(t: number): string {
  return new Date(t).toISOString().slice(0, 10)
}

function addMonths(day: string, months: number): string {
  const [y, m, d] = day.split('-').map(Number)
  const target = new Date(Date.UTC(y, m - 1 + months, 1))
  // Clamp to the target month's last day (Jan 31 + 1 month = Feb 28/29).
  const last = new Date(Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0)).getUTCDate()
  target.setUTCDate(Math.min(d, last))
  return target.toISOString().slice(0, 10)
}

/**
 * A key that groups the different ways one merchant is written on statements:
 * "NETFLIX.COM 866-579", "Netflix.com 123" and "NETFLIX" all become "netflix".
 * After the first word, any word containing a digit is a reference code and is
 * dropped whole ("SPOTIFY P2B4E8C1F2" -> "spotify"; stripping only the digits
 * would leave letters that change every month). Punctuation and filler words
 * (com, inc, usa...) are dropped too, and the first three remaining words kept.
 */
export function merchantKey(merchant: string): string {
  return merchant
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter((word, i) => word && (i === 0 || !/\d/.test(word)))
    .map((word) => word.replace(/\d/g, ''))
    .filter((word) => word && !FILLER_WORDS.has(word))
    .slice(0, 3)
    .join(' ')
}

/**
 * A readable name for a merchant: the statement text without the trailing
 * reference numbers ("NETFLIX.COM 866-579" -> "NETFLIX.COM", "SPOTIFY #1234" -> "SPOTIFY").
 * Falls back to the original text if that would leave nothing.
 */
export function displayMerchant(merchant: string): string {
  const words = merchant.trim().split(/\s+/)
  while (words.length > 1 && /\d/.test(words[words.length - 1])) words.pop()
  const cleaned = words.join(' ')
  return /\d/.test(cleaned) && words.length === 1 ? merchant.trim() : cleaned
}

function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

function cadenceOf(gaps: number[], charges: number): Cadence | null {
  const share = (range: { min: number; max: number }) =>
    gaps.filter((g) => g >= range.min && g <= range.max).length / gaps.length
  if (charges >= 3 && share(MONTHLY_GAP) >= MIN_REGULAR) return 'monthly'
  // A yearly charge can only be seen twice, so require both gaps to fit.
  if (charges >= 2 && share(YEARLY_GAP) === 1) return 'yearly'
  return null
}

/**
 * Recurring charges still going on as of `today`, largest yearly cost first.
 *
 * A merchant qualifies with at least three charges about a month apart (or two
 * about a year apart) at a roughly steady amount, and a charge recent enough that
 * it hasn't plausibly been cancelled. Transfers and income are ignored.
 */
export function detectSubscriptions(txns: TxnLike[], today: Date): Subscription[] {
  const groups = new Map<string, TxnLike[]>()
  for (const t of txns) {
    if (!(t.amount > 0) || NOT_SPENDING.has((t.categoryName ?? '').toLowerCase())) continue
    const key = merchantKey(t.merchant)
    if (!key) continue
    groups.set(key, [...(groups.get(key) ?? []), t])
  }

  const now = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate())
  const found: Subscription[] = []

  for (const [key, group] of groups) {
    // Split charges into billing cycles; a charge soon after a cycle's first one
    // is an extra charge in that cycle, not a new interval.
    const sorted = [...group].sort((x, y) => toDay(x.transDate) - toDay(y.transDate))
    const cycles: TxnLike[][] = []
    for (const t of sorted) {
      const cycle = cycles[cycles.length - 1]
      if (cycle && (toDay(t.transDate) - toDay(cycle[0].transDate)) / DAY < SAME_CYCLE_DAYS) cycle.push(t)
      else cycles.push([t])
    }
    if (cycles.length < 2) continue

    const days = cycles.map((c) => toDay(c[0].transDate))
    const gaps = days.slice(1).map((d, i) => (d - days[i]) / DAY)
    const cadence = cadenceOf(gaps, days.length)
    if (!cadence) continue

    const charges = cycles.map((c) => c[0])
    const typical = median(charges.map((c) => c.amount))
    const steady = charges.filter((c) => Math.abs(c.amount - typical) <= typical * AMOUNT_TOLERANCE).length
    if (steady / charges.length < MIN_STABLE) continue

    const lastDay = Math.max(...cycles[cycles.length - 1].map((c) => toDay(c.transDate)))
    const intervalDays = cadence === 'monthly' ? 30 : 365
    if ((now - lastDay) / DAY > intervalDays * ACTIVE_FOR) continue

    const latest = charges[charges.length - 1]
    const lastDate = fromDay(lastDay)
    const duplicates = cycles
      .filter((c) => c.length > 1)
      .map((c) => ({ date: fromDay(toDay(c[0].transDate)), charges: c.length, total: c.reduce((sum, x) => sum + x.amount, 0) }))
      .reverse()
    found.push({
      key,
      merchant: displayMerchant(latest.merchant),
      cadence,
      amount: latest.amount,
      lastDate,
      nextDate: addMonths(fromDay(days[days.length - 1]), cadence === 'monthly' ? 1 : 12),
      charges: charges.length,
      monthlyCost: cadence === 'monthly' ? latest.amount : latest.amount / 12,
      yearlyCost: cadence === 'monthly' ? latest.amount * 12 : latest.amount,
      categoryName: latest.categoryName,
      duplicates,
    })
  }

  return found.sort((a, b) => b.yearlyCost - a.yearlyCost)
}

/** Total monthly and yearly cost of a set of subscriptions. */
export function subscriptionTotals(subs: Pick<Subscription, 'monthlyCost' | 'yearlyCost'>[]) {
  return {
    monthly: subs.reduce((s, x) => s + x.monthlyCost, 0),
    yearly: subs.reduce((s, x) => s + x.yearlyCost, 0),
  }
}
