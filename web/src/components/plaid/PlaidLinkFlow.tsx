import { useEffect, useRef, useState } from 'react'
import { usePlaidLink } from 'react-plaid-link'
import { plaidApi, ACCOUNT_TYPE_LABELS } from '../../api/client'
import type {
  Account,
  PlaidAccountPreview,
  PlaidApplyResult,
  PlaidItemPreview,
  PlaidMappingInput,
  PlaidSyncInfo,
} from '../../api/client'
import { Button } from '../ui/Button'
import { Modal } from '../ui/Modal'
import { mutateAfterPlaidSync } from '../../hooks/usePlaid'

const inputCls =
  'w-full bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent'
const selectCls =
  'bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'

type LinkMode = 'investments' | 'banking'
type Step = 'mode' | 'map' | 'done'

const SKIP = '__skip__'
const CREATE = '__create__'

/** One Plaid account's chosen mapping in the UI. */
interface RowState {
  choice: string // MyFi account id, SKIP, or CREATE
  newName: string
  newType: string
}

/**
 * Full "connect an institution" flow: pick mode → Plaid Link popup →
 * map each Plaid account onto an existing MyFi account (retaining its
 * contributions/settings/history) or create a new one → apply + first sync.
 *
 * When `autoMode` is set, the mode-picker step is skipped entirely and the
 * Plaid popup opens immediately in that mode — used when the caller already
 * knows which mode applies (e.g. re-linking a bank account after a reset, see
 * PlaidGetFullHistoryButton) and forcing the user through a redundant choice
 * would just be friction.
 */
export function PlaidLinkFlow({
  open,
  onClose,
  accounts,
  autoMode,
  title = 'Connect an Institution',
}: {
  open: boolean
  onClose: () => void
  accounts: Account[]
  autoMode?: LinkMode
  title?: string
}) {
  const [step, setStep] = useState<Step>('mode')
  const [mode, setMode] = useState<LinkMode>(autoMode ?? 'investments')
  const [linkToken, setLinkToken] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<PlaidItemPreview | null>(null)
  const [rows, setRows] = useState<Record<string, RowState>>({})
  const [backfill, setBackfill] = useState(true)
  const [result, setResult] = useState<PlaidApplyResult | null>(null)

  const reset = () => {
    setStep('mode')
    setLinkToken(null)
    setError('')
    setPreview(null)
    setRows({})
    setBackfill(true)
    setResult(null)
  }

  const handleClose = () => {
    reset()
    onClose()
  }

  const { open: openLink, ready } = usePlaidLink({
    token: linkToken,
    onSuccess: async (publicToken, metadata) => {
      setLoading(true)
      setError('')
      try {
        const p = await plaidApi.exchange(
          publicToken,
          metadata.institution?.institution_id,
          metadata.institution?.name,
        )
        setPreview(p)
        // Pre-select: previously linked account on re-link, otherwise Skip.
        const initial: Record<string, RowState> = {}
        for (const acct of p.accounts) {
          initial[acct.plaidAccountId] = {
            choice: acct.linkedAccountId ?? SKIP,
            newName: defaultName(acct, p.institutionName),
            newType: acct.suggestedAccountType,
          }
        }
        setRows(initial)
        setStep('map')
      } catch (e: any) {
        setError(e.message ?? 'Failed to link institution.')
      } finally {
        setLoading(false)
      }
    },
    onExit: (err, metadata) => {
      setLinkToken(null)
      // Surface Plaid's actual failure instead of leaving the generic
      // "Something went wrong" with no explanation. A plain user cancel
      // (err == null) stays silent. request_id is included because errors
      // like INTERNAL_SERVER_ERROR are institution/Plaid-side — Plaid
      // support can only investigate a specific failed attempt if given
      // this id, so it needs to survive past this one screen.
      if (err) {
        const inst = metadata?.institution?.name ? ` at ${metadata.institution.name}` : ''
        const detail = err.display_message || err.error_message || 'Link could not be completed.'
        const reqId = metadata?.request_id ? ` — request ID: ${metadata.request_id}` : ''
        setError(`${detail}${inst}${err.error_code ? ` (${err.error_code})` : ''}${reqId}`)
      }
    },
  })

  // Open the Plaid popup as soon as the token is minted and Link is ready.
  useEffect(() => {
    if (linkToken && ready) openLink()
  }, [linkToken, ready, openLink])

  const startLink = async () => {
    setLoading(true)
    setError('')
    try {
      const { linkToken: token } = await plaidApi.createLinkToken(mode)
      setLinkToken(token)
    } catch (e: any) {
      setError(e.message ?? 'Failed to create link token.')
    } finally {
      setLoading(false)
    }
  }

  // When autoMode is set, skip the mode-picker step entirely and go straight
  // to minting a link token — guarded so it only fires once per open cycle,
  // not on every re-render while the modal is open.
  const autoStarted = useRef(false)
  useEffect(() => {
    if (open && autoMode && !autoStarted.current) {
      autoStarted.current = true
      startLink()
    }
    if (!open) autoStarted.current = false
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, autoMode])

  const anyMapped = Object.values(rows).some((r) => r.choice !== SKIP)
  const hasInvestment = preview?.accounts.some((a) => a.type === 'investment' || a.type === 'brokerage') ?? false

  const handleApply = async () => {
    if (!preview) return
    setLoading(true)
    setError('')
    try {
      const mappings: PlaidMappingInput[] = []
      for (const acct of preview.accounts) {
        const row = rows[acct.plaidAccountId]
        if (!row || row.choice === SKIP) continue
        if (row.choice === CREATE) {
          mappings.push({
            plaidAccountId: acct.plaidAccountId,
            createNew: {
              name: row.newName.trim() || defaultName(acct, preview.institutionName),
              accountType: row.newType,
              institution: preview.institutionName || undefined,
              currency: acct.currency || undefined,
            },
          })
        } else {
          mappings.push({ plaidAccountId: acct.plaidAccountId, myfiAccountId: row.choice })
        }
      }
      const res = await plaidApi.applyMappings(preview.itemId, mappings, backfill && hasInvestment)
      setResult(res)
      await mutateAfterPlaidSync()
      setStep('done')
    } catch (e: any) {
      setError(e.message ?? 'Failed to link accounts.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal open={open} onClose={handleClose} title={title} size="lg">
      <div className="space-y-4">
        {error && (
          <div className="text-negative text-sm bg-negative-tint-bg rounded px-3 py-2">{error}</div>
        )}

        {step === 'mode' && autoMode && (
          <div className="py-6 text-center text-sm text-text-tertiary">
            Opening Plaid…
          </div>
        )}

        {step === 'mode' && !autoMode && (
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">
              Link a real account through Plaid to sync it automatically. Each institution login
              uses one of your Plaid connections.
            </p>
            <div className="space-y-2">
              <ModeOption
                selected={mode === 'investments'}
                onSelect={() => setMode('investments')}
                title="Investment accounts"
                description="Brokerage, IRA, 401(k)… Syncs holdings, cost basis, and balances daily, and can backfill up to 2 years of weekly history."
              />
              <ModeOption
                selected={mode === 'banking'}
                onSelect={() => setMode('banking')}
                title="Bank & credit accounts"
                description="Checking, savings, credit cards, loans — including a card with no linked checking/savings at the same institution. Syncs balances and transactions daily, plus APR/statement/limit details for credit cards."
              />
            </div>
            <div className="flex gap-3">
              <Button onClick={startLink} loading={loading || (!!linkToken && !ready)}>
                Continue with Plaid
              </Button>
              <Button variant="ghost" onClick={handleClose}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        {step === 'map' && preview && (
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">
              Match each account at{' '}
              <span className="text-text-primary">{preview.institutionName || 'this institution'}</span>{' '}
              to a MyFi account. Linking to an existing account keeps its contributions, settings,
              and snapshot history.
            </p>
            <div className="space-y-3">
              {preview.accounts.map((acct) => (
                <MappingRow
                  key={acct.plaidAccountId}
                  acct={acct}
                  accounts={accounts}
                  row={rows[acct.plaidAccountId]}
                  onChange={(row) => setRows((prev) => ({ ...prev, [acct.plaidAccountId]: row }))}
                />
              ))}
            </div>
            {hasInvestment && (
              <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer">
                <input
                  type="checkbox"
                  checked={backfill}
                  onChange={(e) => setBackfill(e.target.checked)}
                  className="rounded border-border-strong bg-subtle text-accent-text focus:ring-accent"
                />
                Backfill up to 24 months of weekly history from past transactions
              </label>
            )}
            <div className="flex gap-3 pt-1">
              <Button onClick={handleApply} loading={loading} disabled={!anyMapped}>
                Link Accounts
              </Button>
              <Button variant="ghost" onClick={handleClose}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        {step === 'done' && result && (
          <div className="space-y-4">
            <p className="text-sm text-text-primary">
              Linked {result.linked + result.created} account
              {result.linked + result.created !== 1 ? 's' : ''}
              {result.created > 0 ? ` (${result.created} newly created)` : ''}.
            </p>
            {result.synced && (
              <ul className="text-sm text-text-secondary space-y-1">
                <li>• {result.synced.accountsSynced} account balance(s) synced</li>
                <li>• {result.synced.holdingsReplaced} holding(s) updated</li>
                {result.backfillStarted && (
                  <li>• Historical backfill running in the background — weekly snapshots will appear shortly.</li>
                )}
              </ul>
            )}
            {result.synced?.errors && result.synced.errors.length > 0 && (
              <div className="text-warning text-sm bg-warning-tint-bg rounded px-3 py-2">
                {result.synced.errors.join('; ')}
              </div>
            )}
            <Button onClick={handleClose}>Done</Button>
          </div>
        )}
      </div>
    </Modal>
  )
}

/**
 * Update-mode Link button: re-authenticates an existing Item, then re-syncs it.
 * Used both to recover a login_required Item and — via the `label` override — to
 * upgrade an Item linked before transaction sync existed, since update mode now
 * also grants transactions consent (see Service.CreateLinkToken).
 */
export function PlaidReconnectButton({
  itemId,
  onDone,
  label = 'Reconnect',
}: {
  itemId: string
  onDone?: () => void
  label?: string
}) {
  const [linkToken, setLinkToken] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const { open, ready } = usePlaidLink({
    token: linkToken,
    onSuccess: async () => {
      try {
        await plaidApi.syncItem(itemId)
        await mutateAfterPlaidSync()
      } finally {
        setLinkToken(null)
        onDone?.()
      }
    },
    onExit: (err, metadata) => {
      setLinkToken(null)
      if (err) {
        const detail = err.display_message || err.error_message || 'Reconnect could not be completed.'
        const reqId = metadata?.request_id ? ` — request ID: ${metadata.request_id}` : ''
        setError(`${detail}${err.error_code ? ` (${err.error_code})` : ''}${reqId}`)
      }
    },
  })

  useEffect(() => {
    if (linkToken && ready) open()
  }, [linkToken, ready, open])

  const start = async () => {
    setLoading(true)
    setError('')
    try {
      const { linkToken: token } = await plaidApi.createLinkToken('investments', itemId)
      setLinkToken(token)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex flex-col items-start gap-1">
      <Button size="sm" variant="secondary" onClick={start} loading={loading || (!!linkToken && !ready)}>
        {label}
      </Button>
      {error && <span className="text-negative text-xs">{error}</span>}
    </div>
  )
}

/**
 * "Get full transaction history" action for one already-linked Plaid bank
 * account. Plaid locks in transactions.days_requested the first time an Item
 * ever gets the Transactions product — reconnecting an Item that's already
 * syncing transactions (PlaidReconnectButton) cannot retroactively widen that
 * window, since Plaid ignores a changed value on an existing grant. The only
 * way to get more history is a genuinely new Item: this disconnects the
 * account (its existing balances/transactions are preserved, not deleted)
 * and immediately reopens Plaid Link so the user re-maps it onto the same
 * MyFi account — this time as a first-time consent grant, which does honor
 * the requested window.
 *
 * Plaid exposes no way to check in advance how much history an institution
 * will actually provide — it's only knowable after a sync completes, and
 * varies bank to bank. So instead of promising a result, this reports exactly
 * what was achieved the moment the relink finishes (via the same
 * earliest-date/count diagnostic used by PlaidSyncBadge), rather than leaving
 * the user to go check elsewhere or wonder if it silently came up short.
 */
export function PlaidGetFullHistoryButton({
  itemId,
  accountId,
  accounts,
  onDone,
}: {
  itemId: string
  accountId: string
  accounts: Account[]
  onDone?: () => void
}) {
  const [showConfirm, setShowConfirm] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [error, setError] = useState('')
  const [relinkOpen, setRelinkOpen] = useState(false)
  const [checkingResult, setCheckingResult] = useState(false)
  const [historyResult, setHistoryResult] = useState<PlaidSyncInfo | null>(null)

  const handleConfirm = async () => {
    setResetting(true)
    setError('')
    try {
      await plaidApi.removeItem(itemId)
      await mutateAfterPlaidSync()
      setShowConfirm(false)
      setHistoryResult(null)
      setRelinkOpen(true)
    } catch (e: any) {
      setError(e.message ?? 'Failed to disconnect.')
    } finally {
      setResetting(false)
    }
  }

  const handleRelinkClose = async () => {
    setRelinkOpen(false)
    await mutateAfterPlaidSync()
    // The mapping step's "Apply" already ran a synchronous first sync
    // (including transactions), so this reflects the real, just-achieved
    // result rather than a stale pre-relink state.
    setCheckingResult(true)
    try {
      const info = await plaidApi.syncInfo(accountId)
      setHistoryResult(info)
    } catch {
      setHistoryResult(null)
    } finally {
      setCheckingResult(false)
    }
    onDone?.()
  }

  return (
    <>
      <div className="flex flex-col items-start gap-1 max-w-sm">
        <Button size="sm" variant="secondary" onClick={() => setShowConfirm(true)}>
          Get full 24-month history
        </Button>
        {error && <span className="text-negative text-xs">{error}</span>}
        {checkingResult && (
          <span className="text-text-tertiary text-xs">Checking what synced…</span>
        )}
        {!checkingResult && historyResult && (
          <span className="text-text-tertiary text-xs">
            {historyResult.transactionCount > 0 && historyResult.earliestTransactionDate ? (
              <>
                Result: {historyResult.transactionCount} transaction
                {historyResult.transactionCount !== 1 ? 's' : ''} synced, earliest dated{' '}
                {new Date(historyResult.earliestTransactionDate).toLocaleDateString()}.
                {' '}Shorter than expected? Plaid may still be preparing more — check back after
                the next sync.
              </>
            ) : (
              'No transactions synced yet — try Sync Now on this account in a bit.'
            )}
          </span>
        )}
      </div>

      <Modal open={showConfirm} onClose={() => setShowConfirm(false)} title="Get full transaction history" size="md">
        <div className="space-y-4 text-sm text-text-secondary">
          <p>
            Plaid locks in how much history it fetches the first time it starts syncing an account —
            reconnecting alone can&apos;t extend a connection that&apos;s already syncing. Getting the
            full 24 months means disconnecting this bank login and reconnecting it fresh.
          </p>
          <ul className="list-disc pl-5 space-y-1">
            <li>Your existing balances and transactions for this account aren&apos;t deleted — you&apos;ll remap the new connection onto this same account.</li>
            <li>If other accounts (like a linked savings account) share this same bank login, they&apos;ll need remapping too — you&apos;ll see them in the next step.</li>
            <li>
              Plaid doesn&apos;t guarantee 24 months — some institutions provide less, and it can take
              time after reconnecting to prepare the deeper history. You&apos;ll see exactly what came
              through as soon as this finishes, not just a generic "success."
            </li>
          </ul>
          <div className="flex gap-3 pt-1">
            <Button onClick={handleConfirm} loading={resetting}>
              Disconnect &amp; Reconnect
            </Button>
            <Button variant="ghost" onClick={() => setShowConfirm(false)}>
              Cancel
            </Button>
          </div>
        </div>
      </Modal>

      <PlaidLinkFlow
        open={relinkOpen}
        onClose={handleRelinkClose}
        accounts={accounts}
        autoMode="banking"
        title="Reconnect for Full History"
      />
    </>
  )
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function ModeOption({
  selected,
  onSelect,
  title,
  description,
}: {
  selected: boolean
  onSelect: () => void
  title: string
  description: string
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`w-full text-left rounded-lg border px-4 py-3 transition-colors ${
        selected
          ? 'border-accent bg-accent-tint-bg'
          : 'border-border-default bg-surface hover:border-border-strong'
      }`}
    >
      <p className="text-sm font-medium text-text-primary">{title}</p>
      <p className="text-xs text-text-tertiary mt-0.5">{description}</p>
    </button>
  )
}

function MappingRow({
  acct,
  accounts,
  row,
  onChange,
}: {
  acct: PlaidAccountPreview
  accounts: Account[]
  row: RowState
  onChange: (row: RowState) => void
}) {
  const balance =
    acct.balance != null
      ? acct.balance.toLocaleString(undefined, { style: 'currency', currency: acct.currency || 'USD' })
      : null

  // A Plaid mortgage account has no address, so it can never create a new
  // MyFi account — it must attach to an existing home account (see
  // ApplyMappings's server-side guard on the "home" account type).
  const isMortgage = acct.suggestedAccountType === 'home'
  const homeAccounts = accounts.filter((a) => a.accountType === 'home')
  const selectableAccounts = isMortgage ? homeAccounts : accounts

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-3">
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-text-primary truncate">
            {acct.name}
            {acct.mask && <span className="text-text-tertiary"> …{acct.mask}</span>}
          </p>
          <p className="text-xs text-text-tertiary">
            {ACCOUNT_TYPE_LABELS[acct.suggestedAccountType] ?? acct.subtype}
            {balance ? ` · ${balance}` : ''}
            {acct.linkedAccountId ? ' · previously linked' : ''}
          </p>
        </div>
        {isMortgage && homeAccounts.length === 0 ? (
          <select className={selectCls} value={SKIP} disabled>
            <option value={SKIP}>— skip —</option>
          </select>
        ) : (
          <select
            className={selectCls}
            value={row?.choice ?? SKIP}
            onChange={(e) => onChange({ ...row, choice: e.target.value })}
          >
            <option value={SKIP}>— skip —</option>
            {selectableAccounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.ownerName ? `${a.name} (${a.ownerName})` : a.name}
              </option>
            ))}
            {!isMortgage && <option value={CREATE}>+ Create new account…</option>}
          </select>
        )}
      </div>

      {isMortgage && homeAccounts.length === 0 && (
        <p className="text-xs text-warning ml-1">
          This is a mortgage — Plaid can&apos;t create a home account (it has no address). Add a home
          account manually first (with its address), then relink this institution to attach the
          mortgage to it.
        </p>
      )}

      {row?.choice === CREATE && (
        <div className="ml-4 pl-4 border-l-2 border-accent/40 grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs text-text-tertiary mb-1">Account name</label>
            <input
              type="text"
              value={row.newName}
              onChange={(e) => onChange({ ...row, newName: e.target.value })}
              className={inputCls}
            />
          </div>
          <div>
            <label className="block text-xs text-text-tertiary mb-1">Account type</label>
            <select
              value={row.newType}
              onChange={(e) => onChange({ ...row, newType: e.target.value })}
              className={`${selectCls} w-full`}
            >
              {Object.entries(ACCOUNT_TYPE_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </div>
        </div>
      )}
    </div>
  )
}

function defaultName(acct: PlaidAccountPreview, institution: string) {
  const inst = institution ? `${institution} ` : ''
  return acct.mask ? `${inst}${acct.name} …${acct.mask}` : `${inst}${acct.name}`
}
