// Typed API client — all fetches include the httpOnly cookie automatically.

const BASE = '/api/v1'

// iOS can silently drop in-flight fetches when a home-screen app is suspended,
// leaving the promise pending forever (and SWR deduping onto it). Time out reads
// so they reject and get retried; mutations are left alone.
const GET_TIMEOUT_MS = 30_000

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    credentials: 'include',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    signal: method === 'GET' ? AbortSignal.timeout(GET_TIMEOUT_MS) : undefined,
  })
  if (res.status === 204) return undefined as T
  // Non-JSON bodies (e.g. a proxy's 502 page) must not surface as a parse error.
  const json = await res.json().catch(() => null)
  if (!res.ok) {
    throw new Error(json?.error?.message ?? 'request failed')
  }
  return json.data as T
}

// --- Config ---
export const configApi = {
  get: () =>
    request<{ env: string; plaidEnabled: boolean; setupRequired: boolean }>('GET', '/config'),
}

// --- Auth ---
export const authApi = {
  /** Creates the instance's only account during first-run setup. */
  register: (email: string, password: string, displayName: string) =>
    request<User>('POST', '/auth/register', { email, password, displayName }),
  login: (email: string, password: string) =>
    request<User>('POST', '/auth/login', { email, password }),
  logout: () => fetch(`${BASE}/auth/logout`, { method: 'POST', credentials: 'include' }),
  me: () => request<User>('GET', '/auth/me'),
  /** Records that the user finished or skipped the welcome screen. */
  markOnboarded: () => request<void>('POST', '/auth/onboarded'),
  /** Stops a page's first-visit guide from opening automatically. */
  dismissGuide: (guide: string) => request<void>('POST', `/auth/guides/${guide}/dismiss`),
  /** Sets a new password after checking the current one; other sessions are signed out. */
  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>('POST', '/auth/password/change', { currentPassword, newPassword }),
}

// --- Accounts ---
export const accountsApi = {
  list: () => request<Account[]>('GET', '/accounts'),
  get: (id: string) => request<Account>('GET', `/accounts/${id}`),
  create: (data: CreateAccountInput) =>
    request<Account>('POST', '/accounts', data),
  update: (id: string, data: UpdateAccountInput) =>
    request<Account>('PUT', `/accounts/${id}`, data),
  delete: (id: string) =>
    fetch(`${BASE}/accounts/${id}`, { method: 'DELETE', credentials: 'include' }),
}

// --- Snapshots ---
export const snapshotsApi = {
  list: (accountId: string) =>
    request<BalanceSnapshot[]>('GET', `/accounts/${accountId}/snapshots`),
  create: (accountId: string, data: CreateSnapshotInput) =>
    request<BalanceSnapshot>('POST', `/accounts/${accountId}/snapshots`, data),
  update: (accountId: string, snapId: string, data: CreateSnapshotInput) =>
    request<BalanceSnapshot>('PUT', `/accounts/${accountId}/snapshots/${snapId}`, data),
  delete: (accountId: string, snapId: string) =>
    fetch(`${BASE}/accounts/${accountId}/snapshots/${snapId}`, {
      method: 'DELETE',
      credentials: 'include',
    }),
  /** Manually snapshots all of the current user's holdings-based accounts now. */
  triggerNow: () =>
    request<{ accountsUpdated: number }>('POST', '/snapshots/trigger'),
}

// --- Holdings ---
export const holdingsApi = {
  listByAccount: (accountId: string) =>
    request<Holding[]>('GET', `/accounts/${accountId}/holdings`),
  listAll: () => request<Holding[]>('GET', '/holdings'),
  create: (accountId: string, data: CreateHoldingInput) =>
    request<Holding>('POST', `/accounts/${accountId}/holdings`, data),
  update: (accountId: string, holdingId: string, data: UpdateHoldingInput) =>
    request<Holding>('PUT', `/accounts/${accountId}/holdings/${holdingId}`, data),
  delete: (accountId: string, holdingId: string) =>
    fetch(`${BASE}/accounts/${accountId}/holdings/${holdingId}`, {
      method: 'DELETE',
      credentials: 'include',
    }),
  refreshPrices: () =>
    request<{ updated: number }>('POST', '/holdings/refresh-prices'),
  bulkUpdateCostBasis: (updates: Array<{ id: string; costBasis: number | null }>) =>
    fetch(`${BASE}/holdings/cost-basis`, {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates),
    }),
  concentration: () => request<ConcentrationResult>('GET', '/holdings/concentration'),
}

export interface ConcentrationFlag {
  type: 'single_ticker' | 'asset_class_drift' | 'redundant_ticker' | 'sector_overlap'
  severity: 'info' | 'warning' | 'critical'
  ticker?: string
  tickers?: string[]
  assetClass?: string
  pct: number
  message: string
  accounts?: string[]
}

export interface ConcentrationResult {
  flags: ConcentrationFlag[]
  totalValue: number
  hasFundCompositionData: boolean
}

export interface CostBasisMatchedHolding {
  id: string
  accountName: string
  shares: number
  currentBasis: number | null
}

export interface CostBasisPreviewRow {
  ticker: string
  costBasis: number | null
  sharesInCsv: number | null
  account: string
  autoSelectedMatchId: string
  matches: CostBasisMatchedHolding[]
}

export interface CostBasisPreviewResult {
  columns: { ticker: string; costBasis: string; shares: string; account: string }
  rows: CostBasisPreviewRow[]
}

export const costBasisImportApi = {
  preview: (file: File): Promise<CostBasisPreviewResult> => {
    const form = new FormData()
    form.append('file', file)
    return fetch(`${BASE}/import/cost-basis/preview`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    }).then((r) => {
      if (!r.ok) return r.json().then((e) => Promise.reject(new Error(e.message ?? 'Preview failed')))
      return r.json()
    })
  },
  apply: (rows: Array<{ holdingId: string; costBasis: number }>) =>
    request<{ updated: number }>('POST', '/import/cost-basis/apply', { rows }),
}

// --- Net Worth ---
export const networthApi = {
  current: () => request<NetWorthCurrent>('GET', '/networth/current'),
  /** Net worth over time broken down by account (balances sum to net worth). */
  accountHistory: (from: string, to: string, interval = 'month') =>
    request<AccountHistory>(
      'GET',
      `/networth/history/accounts?from=${from}&to=${to}&interval=${interval}`,
    ),
  history: (from: string, to: string, interval = 'month') =>
    request<HistoryPoint[]>(
      'GET',
      `/networth/history?from=${from}&to=${to}&interval=${interval}`,
    ),
}

// --- Goals ---
export const goalsApi = {
  list: () => request<Goal[]>('GET', '/goals'),
  create: (data: CreateGoalInput) => request<Goal>('POST', '/goals', data),
  update: (id: string, data: UpdateGoalInput) =>
    request<Goal>('PUT', `/goals/${id}`, data),
  delete: (id: string) =>
    fetch(`${BASE}/goals/${id}`, { method: 'DELETE', credentials: 'include' }),
}

// --- Profile ---
export const profileApi = {
  get: () => request<UserProfile>('GET', '/profile'),
  upsert: (data: UpsertProfileInput) => request<UserProfile>('PUT', '/profile', data),
}

// --- Allocation Targets ---
export const allocationTargetsApi = {
  get: () => request<AllocationTarget>('GET', '/allocation-targets'),
  upsert: (data: AllocationTarget) => request<AllocationTarget>('PUT', '/allocation-targets', data),
}

// --- Contributions ---
export const contributionsApi = {
  listAll: () => request<Contribution[]>('GET', '/contributions'),
  listByAccount: (accountId: string) =>
    request<Contribution[]>('GET', `/accounts/${accountId}/contributions`),
  create: (accountId: string, data: CreateContributionInput) =>
    request<Contribution>('POST', `/accounts/${accountId}/contributions`, data),
  update: (accountId: string, cId: string, data: UpdateContributionInput) =>
    request<Contribution>('PUT', `/accounts/${accountId}/contributions/${cId}`, data),
  delete: (accountId: string, cId: string) =>
    fetch(`${BASE}/accounts/${accountId}/contributions/${cId}`, {
      method: 'DELETE',
      credentials: 'include',
    }),
}

// --- Actual contributions this year ---
export const contributionEntriesApi = {
  progress: () => request<ContributionProgress[]>('GET', '/contribution-progress'),
  list: (accountId: string) =>
    request<ContributionEntry[]>('GET', `/accounts/${accountId}/contribution-entries`),
  create: (accountId: string, data: ContributionEntryInput) =>
    request<ContributionEntry>('POST', `/accounts/${accountId}/contribution-entries`, data),
  update: (accountId: string, entryId: string, data: ContributionEntryInput) =>
    request<ContributionEntry>('PUT', `/accounts/${accountId}/contribution-entries/${entryId}`, data),
  delete: (accountId: string, entryId: string) =>
    fetch(`${BASE}/accounts/${accountId}/contribution-entries/${entryId}`, {
      method: 'DELETE',
      credentials: 'include',
    }),
}

// --- Return Rates ---
export const returnRatesApi = {
  list: () => request<ReturnRateEntry[]>('GET', '/return-rates'),
  upsert: (assetClass: string, returnRate: number) =>
    request<UserReturnRate>('PUT', `/return-rates/${assetClass}`, { returnRate }),
  delete: (assetClass: string) =>
    fetch(`${BASE}/return-rates/${assetClass}`, { method: 'DELETE', credentials: 'include' }),
  setAccountOverride: (accountId: string, returnRate: number | null) =>
    request<Account>('PUT', `/accounts/${accountId}/return-rate`, { returnRate }),
  effectiveRate: (accountId: string) =>
    request<AccountEffectiveRate>('GET', `/accounts/${accountId}/effective-rate`),
}

// --- Home ---
export const homeApi = {
  getDetails: (accountId: string) =>
    request<HomeDetails>('GET', `/accounts/${accountId}/home-details`),
  upsertDetails: (accountId: string, data: Partial<HomeDetails>) =>
    request<HomeDetails>('PUT', `/accounts/${accountId}/home-details`, data),
  getAmortization: (accountId: string, extra?: number) =>
    request<AmortizationResult>(
      'GET',
      `/accounts/${accountId}/amortization${extra != null ? `?extra=${extra}` : ''}`,
    ),
}

// --- Vehicle ---
export const vehicleApi = {
  getDetails: (accountId: string) =>
    request<VehicleDetails>('GET', `/accounts/${accountId}/vehicle-details`),
  /** value, when given, records the owner's estimate as the account's balance today. */
  upsertDetails: (accountId: string, data: Partial<VehicleDetails> & { value?: number }) =>
    request<VehicleDetails>('PUT', `/accounts/${accountId}/vehicle-details`, data),
}

// --- Credit Card ---
export const creditCardApi = {
  getDetails: (accountId: string) =>
    request<CreditCardDetails>('GET', `/accounts/${accountId}/credit-card-details`),
  upsertDetails: (accountId: string, data: Partial<CreditCardDetails>) =>
    request<CreditCardDetails>('PUT', `/accounts/${accountId}/credit-card-details`, data),
}

// --- Loan details ---
export const loanApi = {
  getDetails: (accountId: string) =>
    request<LoanDetails>('GET', `/accounts/${accountId}/loan-details`),
  upsertDetails: (accountId: string, data: Partial<LoanDetails>) =>
    request<LoanDetails>('PUT', `/accounts/${accountId}/loan-details`, data),
}

// --- Ticker detail ---
export const tickerApi = {
  detail: (ticker: string) => request<TickerDetail>('GET', `/holdings/ticker/${ticker}`),
  history: (ticker: string, period = '1y') =>
    request<TickerHistory>('GET', `/holdings/ticker/${ticker}/history?period=${period}`),
}

// --- Household Members ---
export const householdMembersApi = {
  list: () => request<HouseholdMember[]>('GET', '/household/members'),
  upsert: (data: UpsertMemberInput) =>
    request<HouseholdMember>('POST', '/household/members', data),
  update: (id: string, data: Partial<UpsertMemberInput>) =>
    request<HouseholdMember>('PUT', `/household/members/${id}`, data),
  delete: (id: string) =>
    fetch(`${BASE}/household/members/${id}`, { method: 'DELETE', credentials: 'include' }),
}

// --- Forecast ---
export const forecastApi = {
  get: () => request<ForecastResult>('GET', '/forecast'),
  scenarios: (params: ScenarioParams[]) =>
    request<ScenariosResult>('POST', '/forecast/scenarios', { scenarios: params }),
  solve: (params: SolveParams) => request<SolveResult>('POST', '/forecast/solve', params),
  backtest: () => request<BacktestResult>('GET', '/forecast/backtest'),
}

// --- Retirement ---
export const retirementApi = {
  get: (monthlySpending?: number) =>
    request<RetirementPlan>(
      'GET',
      `/retirement${monthlySpending != null ? `?monthlySpending=${monthlySpending}` : ''}`,
    ),
}

// --- Plaid ---
export interface PlaidAccountPreview {
  plaidAccountId: string
  name: string
  mask: string
  type: string
  subtype: string
  balance: number | null
  currency: string
  suggestedAccountType: string
  suggestedTaxTreatment: string
  /** Set when this Plaid account was previously linked to a MyFi account. */
  linkedAccountId: string | null
}

export interface PlaidItemPreview {
  itemId: string
  institutionName: string
  accounts: PlaidAccountPreview[]
}

export interface PlaidLinkedAccount {
  myfiAccountId: string
  name: string
  plaidAccountId: string
}

export interface PlaidItem {
  id: string
  itemId: string
  institutionName?: string
  status: 'active' | 'error' | 'login_required'
  errorCode?: string
  lastSyncedAt?: string
  lastSyncError?: string
  backfillRequested: boolean
  backfilledAt?: string
  createdAt: string
  accounts: PlaidLinkedAccount[]
}

export interface PlaidSyncResult {
  accountsSynced: number
  holdingsReplaced: number
  snapshotsWritten: number
  errors: string[] | null
}

export interface PlaidMappingInput {
  plaidAccountId: string
  myfiAccountId?: string
  createNew?: {
    name: string
    accountType: string
    taxTreatment?: string
    institution?: string
    currency?: string
  }
}

export interface PlaidApplyResult {
  linked: number
  created: number
  synced: PlaidSyncResult | null
  backfillStarted: boolean
}

// Sync diagnostics for one Plaid-linked account: the Item's persisted status
// (reflects the last sync of any kind, scheduled or manual — unlike the
// ephemeral result of a single "sync now" click) plus how far back the
// actually-synced transaction data goes for this account.
export interface PlaidSyncInfo {
  status: 'active' | 'error' | 'login_required'
  lastSyncedAt?: string
  lastSyncError?: string
  earliestTransactionDate?: string
  transactionCount: number
}

export const plaidApi = {
  createLinkToken: (mode: 'investments' | 'banking', itemId?: string) =>
    request<{ linkToken: string }>('POST', '/plaid/link-token', { mode, itemId }),
  exchange: (publicToken: string, institutionId?: string, institutionName?: string) =>
    request<PlaidItemPreview>('POST', '/plaid/exchange', { publicToken, institutionId, institutionName }),
  listItems: () => request<PlaidItem[]>('GET', '/plaid/items'),
  applyMappings: (itemId: string, mappings: PlaidMappingInput[], backfill: boolean) =>
    request<PlaidApplyResult>('POST', `/plaid/items/${itemId}/mappings`, { mappings, backfill }),
  syncItem: (itemId: string) =>
    request<PlaidSyncResult>('POST', `/plaid/items/${itemId}/sync`),
  syncAll: () => request<PlaidSyncResult>('POST', '/plaid/sync'),
  removeItem: (itemId: string) =>
    request<void>('DELETE', `/plaid/items/${itemId}`),
  syncInfo: (accountId: string) =>
    request<PlaidSyncInfo>('GET', `/accounts/${accountId}/plaid-sync-info`),
}

// --- Import ---
export const importApi = {
  ofxPreview: (file: File): Promise<OFXPreviewResult> => {
    const form = new FormData()
    form.append('file', file)
    return fetch(`${BASE}/import/ofx/preview`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
      .then(r => r.json())
      .then(j => {
        if (!j.data) throw new Error(j.error?.message ?? 'preview failed')
        return j.data as OFXPreviewResult
      })
  },
  ofxApply: (body: OFXApplyRequest): Promise<OFXApplyResult> =>
    request<OFXApplyResult>('POST', '/import/ofx/apply', body),
  history: () => request<ImportLog[]>('GET', '/import/history'),
  csv: (file: File): Promise<ImportResult> => {
    const form = new FormData()
    form.append('file', file)
    return fetch(`${BASE}/import/csv`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
      .then((r) => r.json())
      .then((j) => {
        if (!j.data) throw new Error(j.error?.message ?? 'import failed')
        return j.data as ImportResult
      })
  },
  fullBackup: (file: File): Promise<FullImportSummary> => {
    const form = new FormData()
    form.append('file', file)
    return fetch(`${BASE}/import/full`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
      .then((r) => r.json())
      .then((j) => {
        if (!j.data) throw new Error(j.error?.message ?? 'import failed')
        return j.data as FullImportSummary
      })
  },
}

// --- Export ---
export const exportApi = {
  download: async (): Promise<void> => {
    const res = await fetch(`${BASE}/export`, { credentials: 'include' })
    if (!res.ok) {
      const j = await res.json()
      throw new Error(j.error?.message ?? 'export failed')
    }
    const bundle = await res.json()
    const date = new Date().toISOString().slice(0, 10)
    const blob = new Blob([JSON.stringify(bundle.data ?? bundle, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `myfi-backup-${date}.json`
    a.click()
    URL.revokeObjectURL(url)
  },
}

// ---- Types ----
export interface User {
  id: string
  email: string
  displayName: string
  /** Null until the user finishes or skips the welcome screen. */
  onboardedAt: string | null
  /** Page guide IDs that should no longer open automatically. */
  dismissedGuides: string[]
  createdAt: string
}

export interface Account {
  id: string
  userId: string
  name: string
  accountType: string
  taxTreatment: 'taxable' | 'tax_deferred' | 'tax_free'
  institution?: string
  currency: string
  isManual: boolean
  /** True when this account is attributed to the household partner member. */
  isPartnerAccount: boolean
  /** True when this account is owned jointly by both household members. */
  isJoint: boolean
  /** Display name of the account owner ("A & B" for a joint account). */
  ownerName: string
  /** UUID of the household member this account is attributed to, if any. */
  memberId?: string
  returnRateOverride?: number | null
  externalProvider?: string
  externalAccountId?: string
  externalItemId?: string
  lastSyncedAt?: string
  createdAt: string
  updatedAt: string
}

export interface BalanceSnapshot {
  id: string
  accountId: string
  snapshotDate: string
  balance: number
  miles?: number
  notes?: string
  source: string
  createdAt: string
}

export interface Holding {
  id: string
  accountId: string
  ticker: string
  name?: string
  assetClass: string
  shares: number
  costBasis?: number
  manualPrice: boolean
  pricePerShare?: number
  dripEnabled: boolean
  totalValue: number
  priceFetchedAt?: string
  createdAt: string
  /** From the live quote: EQUITY, ETF, MUTUALFUND, CRYPTOCURRENCY... Absent without a quote. */
  instrumentType?: string
  /** The ticker's full name from the live quote. */
  quoteName?: string
}

export interface Goal {
  id: string
  name: string
  goalType: string
  accountId?: string
  accountTypeFilter?: string
  assetClassFilter?: string
  targetAmount: number
  targetDate?: string
  notes?: string
  isAchieved: boolean
  createdAt: string
}

/** One stacked series of the per-account history. */
export interface AccountSeries {
  id: string
  name: string
  accountType: string
}

export interface AccountHistoryPoint {
  date: string
  /** Balance of each series (by id) at this date; liabilities are negative. */
  balances: Record<string, number>
}

export interface AccountHistory {
  series: AccountSeries[]
  points: AccountHistoryPoint[]
}

export interface HistoryPoint {
  date: string
  netWorth: number
  assets: number
  liabilities: number
}

export interface TaxTreatmentBreakdown {
  taxTreatment: string
  total: number
  percentOfAssets: number
}

export interface AccountTypeBreakdown {
  accountType: string
  taxTreatment: string
  total: number
  percentOfAssets: number
}

export interface AccountBalance {
  accountId: string
  name: string
  accountType: string
  taxTreatment: string
  balance: number
  displayBalance?: number
}

export interface NetWorthCurrent {
  totalNetWorth: number
  totalAssets: number
  totalLiabilities: number
  byTaxTreatment: TaxTreatmentBreakdown[]
  byAccountType: AccountTypeBreakdown[]
  accounts: AccountBalance[]
  asOf: string
}

export interface ImportResult {
  imported: number
  skipped: number
  errors: string[]
}

export type HoldingDiffStatus = 'added' | 'removed' | 'changed' | 'unchanged'

export interface HoldingDiff {
  ticker: string
  name: string
  assetClass: string
  status: HoldingDiffStatus
  ofxShares?: number
  ofxPrice?: number
  curShares?: number
  curPrice?: number
}

export interface OFXAccountPreview {
  ofxAccountId: string
  brokerId: string
  institution: string
  linkedAccountId: string | null
  diffs: HoldingDiff[]
}

export interface OFXPreviewResult {
  accounts: OFXAccountPreview[]
}

export interface OFXApplyMapping {
  ofxAccountId: string
  myfiAccountId: string
}

export interface OFXApplyAccepted {
  ofxAccountId: string
  ticker: string
}

export interface OFXApplyRequest {
  rawOfx: string
  filename: string
  mappings: OFXApplyMapping[]
  accepted: OFXApplyAccepted[]
}

export interface ImportLog {
  id: string
  userId: string
  importType: 'csv_snapshot' | 'ofx_holdings' | 'csv_transactions' | 'full_backup'
  filename: string
  status: 'success' | 'partial' | 'failed'
  rowCount: number
  skipped: number
  errorCount: number
  importedAt: string
}

export interface OFXApplyResult {
  replaced: number
  skipped: number
}

export interface FullImportSummary {
  accountsCreated: number
  accountsMatched: number
  snapshotsUpserted: number
  holdingsRestored: number
  contributionsRestored: number
  goalsRestored: number
  warnings?: string[]
}

// Input types
export interface CreateAccountInput {
  name: string
  accountType: string
  taxTreatment?: string
  institution?: string
  currency?: string
  memberId?: string
  isJoint?: boolean
}

export interface UpdateAccountInput {
  name: string
  accountType: string
  taxTreatment?: string
  institution?: string
  memberId?: string
  clearMember?: boolean
  isJoint?: boolean
}

export interface CreateSnapshotInput {
  snapshotDate: string
  balance: number
  miles?: number
  notes?: string
}

export interface CreateHoldingInput {
  ticker: string
  name?: string
  assetClass?: string
  shares: number
  costBasis?: number
  manualPrice?: boolean
  pricePerShare?: number
}

export interface UpdateHoldingInput {
  ticker: string
  name?: string
  assetClass?: string
  shares: number
  costBasis?: number
  manualPrice?: boolean
  pricePerShare?: number
  dripEnabled?: boolean
}

export interface CreateGoalInput {
  name: string
  goalType: string
  accountId?: string
  accountTypeFilter?: string
  assetClassFilter?: string
  targetAmount: number
  targetDate?: string
  notes?: string
}

export interface UpdateGoalInput {
  name: string
  goalType: string
  accountId?: string
  accountTypeFilter?: string
  assetClassFilter?: string
  targetAmount: number
  targetDate?: string
  notes?: string
  isAchieved?: boolean
}

export interface UserProfile {
  id: string
  userId: string
  // Shared household fields
  useInflationAdjustment: boolean
  inflationRate: number
  monthlyExpenses: number
  currentMonthlyExpenses: number
  useBudgetForExpenses: boolean
  withdrawalRate?: number | null
  filingStatus?: string
  stateTaxRate?: number | null
  stateTaxCode?: string | null
  dependentsCount: number
  // Overlaid from 'self' household member
  birthday?: string
  retirementAge: number
  useSocialSecurity: boolean
  socialSecurityMonthly: number
  socialSecurityStartAge: number
  monthlyIncome: number
  householdSize: number
  updatedAt: string
}

export interface Contribution {
  id: string
  accountId: string
  userId: string
  label?: string
  amount: number
  frequency: 'monthly' | 'annually' | 'biweekly'
  annualIncreasePct: number
  amountType: ContributionAmountType
  incomePct: number   // fraction of the owner's salary, for 'percent'
  matchTiers: MatchTier[]
  isActive: boolean
  nextPayDate?: string
  startDate?: string  // YYYY-MM-DD; null = from projection start
  endDate?: string    // YYYY-MM-DD; null = until retirement
  createdAt: string
}

export type ContributionSource = 'employee' | 'employer'

/** Money actually put into an account, synced from Plaid or logged by hand. */
export interface ContributionEntry {
  id: string
  accountId: string
  entryDate: string
  amount: number
  source: ContributionSource
  description?: string
  sourceEdited: boolean
  /** From Plaid: only `source` can be changed. */
  isSynced: boolean
}

export interface ContributionEntryInput {
  entryDate: string
  amount: number
  source: ContributionSource
  description?: string
}

/** This year's actual contributions vs. what the plan expected by `asOf`. */
export interface ContributionProgress {
  accountId: string
  year: number
  asOf: string
  employeeActual: number
  employerActual: number
  employeeExpected: number
  employerExpected: number
  employeeOffPlan: boolean
  employerOffPlan: boolean
  /** A plan matching this year's pace; no contributionId means add a new plan. */
  suggestion?: {
    contributionId?: string
    annualPace: number
    amountType: ContributionAmountType
    amount: number
    frequency: 'monthly' | 'annually' | 'biweekly'
    incomePct: number
  }
}

export type ContributionAmountType = 'fixed' | 'percent'

/** Employer matches matchPct of the contribution on the next upToPct of salary; null = everything left. */
export interface MatchTier {
  matchPct: number
  upToPct: number | null
}

export interface UserReturnRate {
  id: string
  userId: string
  assetClass: string
  returnRate: number
}

export interface ReturnRateEntry {
  assetClass: string
  defaultRate: number
  customRate?: number
  effectiveRate: number
}

export interface HouseholdMember {
  id: string
  userId: string
  relationship: 'self' | 'partner'
  name: string
  birthday?: string
  isEmployed: boolean
  monthlyIncome: number
  salaryGrowthPct: number
  retirementAge: number
  useSocialSecurity: boolean
  socialSecurityMonthly: number
  socialSecurityStartAge: number
  createdAt: string
  updatedAt: string
}

export interface UpsertMemberInput {
  relationship: 'self' | 'partner'
  name: string
  birthday?: string
  isEmployed?: boolean
  monthlyIncome?: number
  annualIncome?: number
  salaryGrowthPct?: number
  retirementAge?: number
  useSocialSecurity?: boolean
  socialSecurityMonthly?: number
  socialSecurityStartAge?: number
}

export interface ForecastYear {
  year: number
  age: number
  spouseAge?: number
  accounts: Record<string, number>
  total: number
  p10?: number
  p25?: number
  p50?: number
  p75?: number
  p90?: number
}

export interface AccountMeta {
  id: string
  name: string
  accountType: string
  ownerUserId: string
  countsTowardPortfolio: boolean
}

export interface FIMetrics {
  fiNumber: number
  currentPortfolio: number
  fiProgressPct: number
  timeToFIYear?: number
  timeToFIAge?: number
  coastFIAmount: number
  isCoastFI: boolean
  savingsRate?: number
  withdrawalRate?: number
  withdrawalRateWarn: boolean
  monteCarloSuccessPct: number
  /** Yearly growth of the FI number (today's dollars) in the projection's dollars: inflation when nominal, else 0. */
  fiNumberGrowth: number
}

export interface ScoreComponent {
  name: string
  score: number
  weight: number
  grade: string
  insight: string
}

export interface ReadinessScore {
  overall: number
  grade: string
  components: ScoreComponent[]
  headline: string
}

export interface ForecastResult {
  base: ForecastYear[]
  optimistic: ForecastYear[]
  pessimistic: ForecastYear[]
  accountMeta: AccountMeta[]
  retirementYear: number
  spouseRetirementYear?: number
  fiMetrics?: FIMetrics
  readinessScore?: ReadinessScore
  mortgagePayoffYears?: Record<string, number>
}

export interface DrawdownYear {
  year: number
  age: number
  balance: number
}

export interface RetirementScenario {
  label: string
  rate: number
  monthlyIncome: number
  depletionAge?: number
}

export interface RetirementPlan {
  retirementAge: number
  retirementYear: number
  retirementPortfolio: number
  sustainableMonthly: number
  monthlySpending: number
  annualSocialSecurity: number
  depletionAge?: number
  survivesToHundred: boolean
  drawdownSchedule: DrawdownYear[]
  rateScenarios: RetirementScenario[]
}

// Absolute target values, not deltas — omit a field to keep the current plan's
// value for that lever. The backend computes the delta against CurrentPlanSummary
// internally.
export interface ScenarioParams {
  name: string
  retirementAge?: number
  monthlyContribution?: number
  returnRate?: number
  monthlyExpense?: number
}

export interface ScenarioResult {
  name: string
  years: ForecastYear[]
  retirementYear: number
  fiYear?: number
  fiAge?: number
  portfolioAtRetirement: number
  sustainableMonthly: number
}

export interface CurrentPlanSummary {
  retirementAge: number
  returnRate: number
  monthlyContribution: number
  monthlyExpense: number
}

export interface ScenariosResult {
  baseline: ScenarioResult
  scenarios: ScenarioResult[]
  accountMeta: AccountMeta[]
  currentPlan: CurrentPlanSummary
}

export type SolveFor = 'contribution' | 'returnRate' | 'retirementAge'

// Absolute target values held fixed while solving for the other variable
// (omit a field to keep the current plan's value). Mirrors ScenarioParams.
export interface SolveParams {
  solveFor: SolveFor
  targetRetirementAge?: number
  monthlyContribution?: number
  returnRate?: number
  monthlyExpense?: number
}

export interface SolveResult {
  solveFor: SolveFor
  solvedValue: number
  achievable: boolean
  scenario: ScenarioResult
}

export interface HistoricalRunResult {
  startYear: number
  years: ForecastYear[]
  survived: boolean
}

export interface BacktestResult {
  runs: HistoricalRunResult[]
  survivalPct: number
  dataCaveats: string[]
  earliestDataYear: number
  latestDataYear: number
}

// Input types for new features
export interface UpsertProfileInput {
  useInflationAdjustment?: boolean
  inflationRate?: number
  monthlyExpenses?: number
  currentMonthlyExpenses?: number
  useBudgetForExpenses?: boolean
  withdrawalRate?: number | null
  filingStatus?: string
  stateTaxRate?: number | null
  stateTaxCode?: string | null
  dependentsCount?: number
}

export interface AllocationTarget {
  equityPct: number
  bondPct: number
  cryptoPct: number
  cashPct: number
  otherPct: number
}

export interface TickerAccountPosition {
  accountId: string
  accountName: string
  accountType: string
  assetClass: string
  shares: number
  pricePerShare?: number
  totalValue: number
}

export interface TickerDetail {
  ticker: string
  name: string
  positions: TickerAccountPosition[]
  totalShares: number
  totalValue: number
  pricePerShare?: number
}

export interface PricePoint {
  date: string
  close: number
}

export interface TickerHistory {
  available: boolean
  points: PricePoint[]
}

export interface CreateContributionInput {
  label?: string
  amount: number
  frequency: 'monthly' | 'annually' | 'biweekly'
  annualIncreasePct: number
  amountType: ContributionAmountType
  incomePct: number
  matchTiers: MatchTier[]
  nextPayDate?: string
  startDate?: string
  endDate?: string
}

export interface UpdateContributionInput extends CreateContributionInput {
  isActive: boolean
}

// Account type metadata
export const ACCOUNT_TYPE_DEFAULTS: Record<string, 'taxable' | 'tax_deferred' | 'tax_free'> = {
  checking: 'taxable',
  savings: 'taxable',
  money_market: 'taxable',
  brokerage: 'taxable',
  crypto: 'taxable',
  home: 'taxable',
  other_asset: 'taxable',
  automobile: 'taxable',
  credit_card: 'taxable',
  loan: 'taxable',
  other_liability: 'taxable',
  traditional_ira: 'tax_deferred',
  sep_ira: 'tax_deferred',
  simple_ira: 'tax_deferred',
  traditional_401k: 'tax_deferred',
  traditional_403b: 'tax_deferred',
  '457b': 'tax_deferred',
  roth_ira: 'tax_free',
  roth_401k: 'tax_free',
  roth_403b: 'tax_free',
  hsa: 'tax_free',
  '529': 'tax_free',
}

export const ACCOUNT_TYPE_LABELS: Record<string, string> = {
  checking: 'Checking',
  savings: 'Savings',
  brokerage: 'Brokerage',
  crypto: 'Crypto',
  home: 'Home',
  other_asset: 'Other Asset',
  automobile: 'Automobile',
  traditional_ira: 'Traditional IRA',
  sep_ira: 'SEP-IRA',
  simple_ira: 'SIMPLE IRA',
  traditional_401k: 'Traditional 401(k)',
  traditional_403b: 'Traditional 403(b)',
  '457b': '457(b)',
  roth_ira: 'Roth IRA',
  roth_401k: 'Roth 401(k)',
  roth_403b: 'Roth 403(b)',
  hsa: 'HSA',
  '529': '529',
  credit_card: 'Credit Card',
  loan: 'Loan',
  other_liability: 'Other Liability',
}

export const ASSET_CLASS_LABELS: Record<string, string> = {
  equity:          'Equity',
  bond:            'Bond / Fixed Income',
  crypto:          'Cryptocurrency',
  cash_equivalent: 'Cash',
  other:           'Other',
}

export const TAX_TREATMENT_LABELS: Record<string, string> = {
  taxable: 'Taxable',
  tax_deferred: 'Tax-Deferred',
  tax_free: 'Tax-Free',
}

// 401(k)/403(b)/457(b) and IRA accounts, whose actual contributions are tracked against the plan
export const RETIREMENT_ACCOUNT_TYPES = new Set([
  'traditional_401k',
  'roth_401k',
  'traditional_403b',
  'roth_403b',
  '457b',
  'traditional_ira',
  'roth_ira',
  'sep_ira',
  'simple_ira',
])

// Account types that support employer match contributions
export const EMPLOYER_MATCH_ACCOUNT_TYPES = new Set([
  'traditional_401k',
  'traditional_403b',
  'roth_401k',
  'roth_403b',
])

// Liability account types — no tax treatment displayed
export const LIABILITY_ACCOUNT_TYPES = new Set([
  'credit_card',
  'loan',
  'other_liability',
])

// Account types where the user manually picks tax treatment
export const FREE_CHOICE_TAX_ACCOUNT_TYPES = new Set(['other_asset'])

// Account types that hide the Holdings tab and ticker/shares UI
export const NO_HOLDINGS_ACCOUNT_TYPES = new Set(['home', 'automobile', 'credit_card'])

// Account types whose balance is displayed as a synthetic Cash holding
// instead of allowing manual holdings entry
export const CASH_BALANCE_ACCOUNT_TYPES = new Set(['checking', 'savings'])

/**
 * Accounts you save into on a schedule (retirement and investment accounts),
 * where a monthly contribution is what makes the forecast grow.
 */
export function acceptsContributions(accountType: string) {
  return !CASH_BALANCE_ACCOUNT_TYPES.has(accountType) && !LIABILITY_ACCOUNT_TYPES.has(accountType)
    && accountType !== 'home' && accountType !== 'automobile'
}

export interface AccountEffectiveRate {
  rate: number
  source: 'account_override' | 'holdings_weighted' | 'user_custom' | 'asset_class_default'
  assetClass?: string
}

export interface HomeDetails {
  id?: string
  accountId: string
  address: string
  lastZestimate?: number
  lastZestimateAt?: string
  interestRate: number
  loanTermYears: number
  originalLoanValue: number
  currentLoanBalance: number
  mortgageStartDate?: string  // YYYY-MM-DD — used for amortization scheduling in the forecast
  extraMonthlyPayment?: number
  trackMortgageAsLiability?: boolean
  includeInRetirementCalcs?: boolean
  /** When currentLoanBalance was last entered or synced. */
  loanBalanceAsOf?: string
  /** currentLoanBalance was rolled forward from loanBalanceAsOf by the payments due since. */
  loanBalanceEstimated?: boolean
  propertyTaxAnnual?: number
  homeInsuranceAnnual?: number
  hoaMonthly?: number
  pmiMonthly?: number
  /** PMI ends when the loan reaches 78% of this. */
  purchasePrice?: number
  equity?: number
  /** Write-only field: sets property value (last_zestimate) manually. Not returned by the server. */
  homeValue?: number
}

export interface AmortizationMonth {
  month: number
  date: string
  payment: number
  principal: number
  interest: number
  extraPrincipal: number
  balance: number
  cumulativeInterest: number
}

export interface AmortizationSummary {
  monthlyPayment: number
  totalInterest: number
  totalMonths: number
  payoffDate: string
  propertyTaxMonthly: number
  insuranceMonthly: number
  hoaMonthly: number
  /** 0 once PMI has dropped off. */
  pmiMonthly: number
  pmiEndDate?: string
  totalMonthlyCost: number
}

export interface AmortizationWithExtra {
  totalInterest: number
  totalMonths: number
  payoffDate: string
  interestSaved: number
  monthsSaved: number
}

export interface AmortizationResult {
  schedule: AmortizationMonth[]
  summary: AmortizationSummary
  withExtra?: AmortizationWithExtra
}

export interface VehicleDetails {
  id?: string
  accountId: string
  vehicleYear: number
  vehicleMake: string
  vehicleModel: string
  vehicleTrim?: string
  lastValue?: number
  lastValueAt?: string
}

export interface CreditCardDetails {
  id?: string
  accountId: string
  aprPercentage?: number
  creditLimit?: number
  lastStatementBalance?: number
  lastStatementIssueDate?: string
  minimumPaymentAmount?: number
  nextPaymentDueDate?: string
  lastPaymentAmount?: number
  lastPaymentDate?: string
  isOverdue?: boolean
}

export interface LoanDetails {
  id?: string
  accountId: string
  aprPercentage?: number
  monthlyPayment?: number
}

// --- Budget ---

export interface BudgetCategory {
  id: string
  userId?: string
  name: string
  isSystem: boolean
  sortOrder: number
  createdAt: string
}

/** A user's monthly spending limit for one category. */
export interface BudgetLimit {
  categoryId: string
  monthlyLimit: number
}

export interface Transaction {
  id: string
  userId: string
  transDate: string
  merchant: string
  amount: number
  categoryId?: string
  categoryName?: string
  accountId?: string
  accountLabel?: string
  sourceFile?: string
  rawDescription: string
  needsReview: boolean
  notes?: string
  createdAt: string
  updatedAt: string
}

export type RecurringFrequency = 'monthly' | 'biweekly'

export interface RecurringTransaction {
  id: string
  merchant: string
  amount: number
  categoryId?: string
  categoryName?: string
  accountId?: string
  accountLabel?: string
  notes?: string
  frequency: RecurringFrequency
  startDate: string
  nextDate: string
  createdAt: string
}

export interface BudgetSummaryRow {
  categoryId: string
  categoryName: string
  total: number
  monthAvg: number
  count: number
}

export interface BudgetSummary {
  categories: BudgetSummaryRow[]
  reviewCount: number
  monthlyIncome: number
}

export interface PreviewTransaction {
  transDate: string
  merchant: string
  rawDescription: string
  amount: number
  sourceCategory: string
  categoryId?: string
  categoryName: string
  needsReview: boolean
}

export interface ImportPreview {
  transactions: PreviewTransaction[]
  filename: string
}

export interface ImportResult {
  inserted: number
  skipped: number
  failed: number
}

export interface MonthlyBreakdownRow {
  month: number
  categoryName: string
  total: number
}

export interface MonthlyIncomeRow {
  month: number
  total: number
}

export const budgetApi = {
  listCategories: () =>
    request<BudgetCategory[]>('GET', '/budget/categories'),

  listLimits: () => request<BudgetLimit[]>('GET', '/budget/limits'),

  setLimit: (categoryId: string, monthlyLimit: number) =>
    request<BudgetLimit>('PUT', `/budget/limits/${categoryId}`, { monthlyLimit }),

  deleteLimit: (categoryId: string) => request<void>('DELETE', `/budget/limits/${categoryId}`),

  createCategory: (name: string) =>
    request<BudgetCategory>('POST', '/budget/categories', { name }),

  importPreview: (file: File): Promise<ImportPreview> => {
    const form = new FormData()
    form.append('file', file)
    return fetch(`${BASE}/budget/import/preview`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
      .then(r => r.json())
      .then(j => {
        if (!j.data) throw new Error(j.error?.message ?? 'preview failed')
        return j.data as ImportPreview
      })
  },

  importConfirm: (filename: string, transactions: PreviewTransaction[], accountId?: string): Promise<ImportResult> =>
    request<ImportResult>('POST', '/budget/import/confirm', { filename, transactions, accountId: accountId || undefined }),

  importQuick: (file: File, accountId?: string): Promise<ImportResult & { reviewCount: number }> => {
    const form = new FormData()
    form.append('file', file)
    if (accountId) form.append('accountId', accountId)
    return fetch(`${BASE}/budget/import/quick`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
      .then(r => r.json())
      .then(j => {
        if (!j.data) throw new Error(j.error?.message ?? 'quick import failed')
        return j.data as ImportResult & { reviewCount: number }
      })
  },

  listDismissedSubscriptions: (): Promise<string[]> =>
    request<string[]>('GET', '/budget/subscriptions/dismissed'),

  dismissSubscription: (key: string): Promise<void> =>
    request<void>('POST', '/budget/subscriptions/dismissed', { key }),

  restoreSubscription: (key: string): Promise<void> =>
    request<void>('POST', '/budget/subscriptions/restore', { key }),

  getReviewCount: (): Promise<{ count: number }> =>
    request<{ count: number }>('GET', '/budget/review-count'),

  monthlyBreakdown: (year: number): Promise<MonthlyBreakdownRow[]> =>
    request<MonthlyBreakdownRow[]>('GET', `/budget/monthly-breakdown?year=${year}`),

  incomeBreakdown: (year: number): Promise<MonthlyIncomeRow[]> =>
    request<MonthlyIncomeRow[]>('GET', `/budget/income-breakdown?year=${year}`),

  summary: (year: number, month: number, annual = false): Promise<BudgetSummary> => {
    const q = annual
      ? `year=${year}&annual=true`
      : `year=${year}&month=${month}`
    return request<BudgetSummary>('GET', `/budget/summary?${q}`)
  },

  listTransactions: (params: {
    accountId?: string
    from?: string
    to?: string
    categoryIds?: string[]
    search?: string
    needsReview?: boolean
    limit?: number
  } = {}): Promise<Transaction[]> => {
    const q = new URLSearchParams()
    if (params.accountId) q.set('accountId', params.accountId)
    if (params.from) q.set('from', params.from)
    if (params.to) q.set('to', params.to)
    for (const id of params.categoryIds ?? []) q.append('categoryId', id)
    if (params.search) q.set('search', params.search)
    if (params.needsReview) q.set('needsReview', 'true')
    if (params.limit) q.set('limit', String(params.limit))
    const qs = q.toString()
    return request<Transaction[]>('GET', `/budget/transactions${qs ? `?${qs}` : ''}`)
  },

  createTransaction: (data: {
    transDate: string
    merchant: string
    amount: number
    categoryId?: string
    accountId?: string
    notes?: string
  }) => request<Transaction>('POST', '/budget/transactions', data),

  updateTransaction: (id: string, data: { categoryId?: string; notes?: string; accountId?: string; applyToVendor?: boolean }) =>
    request<Transaction>('PUT', `/budget/transactions/${id}`, data),

  deleteTransaction: (id: string) =>
    fetch(`${BASE}/budget/transactions/${id}`, { method: 'DELETE', credentials: 'include' }),

  listRecurring: () => request<RecurringTransaction[]>('GET', '/budget/recurring'),

  /** Saves a recurring transaction; `added` is how many occurrences were already due. */
  createRecurring: (data: {
    startDate: string
    frequency: RecurringFrequency
    merchant: string
    amount: number
    categoryId?: string
    accountId?: string
    notes?: string
  }) => request<{ id: string; added: number }>('POST', '/budget/recurring', data),

  deleteRecurring: (id: string) => request<void>('DELETE', `/budget/recurring/${id}`),

  // Manual/CSV transactions that a Plaid sync has since duplicated (preview only).
  listDuplicates: () => request<Transaction[]>('GET', '/budget/duplicates'),

  bulkDeleteTransactions: (ids: string[]) =>
    request<{ deleted: number }>('POST', '/budget/transactions/bulk-delete', { ids }),
}

// --- Tax Analysis ---

export const taxApi = {
  analyze: (conversionOverride?: number) =>
    request<TaxAnalysis>(
      'GET',
      `/taxes${conversionOverride != null ? `?conversion=${conversionOverride}` : ''}`,
    ),
}

export interface TaxBracket {
  rate: number
  min: number
  max: number
}

export interface BracketAnalysis {
  filingStatus: string
  grossIncome: number
  standardDeduction: number
  estimatedAgi: number
  federalTaxOwed: number
  stateTaxOwed: number
  totalTaxOwed: number
  effectiveRate: number
  marginalRate: number
  bracketHeadroom: number
  allBrackets: TaxBracket[]
}

export interface LadderRung {
  year: number
  age: number
  conversionAmount: number
  taxCost: number
  accessYear: number
  accessAge: number
  tradBalance: number
  rothBalance: number
}

export interface RothLadder {
  taxDeferredBalance: number
  rothBalance: number
  recommendedAnnual: number
  conversionAmount: number
  annualTaxCost: number
  returnRateUsed: number
  rungs: LadderRung[]
  retirementRate?: number | null
  retirementGrossIncome: number
}

export interface TaxAccount {
  id: string
  name: string
  accountType: string
  balance: number
}

export interface WithdrawalBucket {
  taxTreatment: string
  label: string
  balance: number
  portfolioPct: number
  priority: number
  yearsOfIncome: number
  taxNote: string
  accounts: TaxAccount[]
}

export interface WithdrawalOrder {
  buckets: WithdrawalBucket[]
  totalPortfolio: number
  monthlySpending: number
}

export interface RMDYear {
  age: number
  year: number
  startBalance: number
  rmdAmount: number
  taxEstimate: number
  endBalance: number
}

export interface ACAThreshold {
  pct: number
  amount: number
  label: string
}

export interface ACAAnalysis {
  householdSize: number
  fpl: number
  projectedMagi: number
  cliffAmount: number
  headroomToCliff: number
  fplPercent: number
  subsidyStatus: 'full_subsidy' | 'partial_subsidy' | 'at_risk' | 'over_cliff'
  conversionImpact: number
  thresholds: ACAThreshold[]
}

export interface TaxAnalysis {
  bracketAnalysis: BracketAnalysis
  rothLadder: RothLadder
  withdrawalOrder: WithdrawalOrder
  rmdProjection: RMDYear[]
  acaAnalysis: ACAAnalysis
}
