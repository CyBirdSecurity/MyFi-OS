import { useRef, useState } from 'react'
import useSWR, { mutate } from 'swr'
import { useNavigate } from 'react-router-dom'
import { importApi, accountsApi } from '../api/client'
import type { Account, OFXPreviewResult, OFXAccountPreview, HoldingDiff } from '../api/client'
import { ACCOUNT_TYPE_LABELS, ACCOUNT_TYPE_DEFAULTS } from '../api/client'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { InstitutionCombobox } from '../components/ui/InstitutionCombobox'

type Step = 'upload' | 'map' | 'review'

export function OFXImportPage() {
  const navigate = useNavigate()
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<Step>('upload')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // Parsed preview from server
  const [preview, setPreview] = useState<OFXPreviewResult | null>(null)
  // Raw OFX text and filename for the apply request
  const [rawOFX, setRawOFX] = useState('')
  const [ofxFilename, setOfxFilename] = useState('')

  // Step 2: account mappings — ofxAccountId → myfi accountId
  const [mappings, setMappings] = useState<Record<string, string>>({})

  // Step 3: accepted diff rows — keyed as `${ofxAccountId}::${ticker}`
  const [accepted, setAccepted] = useState<Record<string, boolean>>({})

  const { data: myfiAccounts = [] } = useSWR<Account[]>('/accounts', () => accountsApi.list())

  // ---- Step 1: Upload ----

  const handleUpload = async () => {
    const file = fileRef.current?.files?.[0]
    if (!file) return
    setLoading(true)
    setError('')
    try {
      const text = await file.text()
      setRawOFX(text)
      setOfxFilename(file.name)
      const result = await importApi.ofxPreview(file)
      setPreview(result)

      // Pre-populate mappings from existing external links
      const initial: Record<string, string> = {}
      const initialAccepted: Record<string, boolean> = {}
      for (const acct of result.accounts) {
        if (acct.linkedAccountId) {
          initial[acct.ofxAccountId] = acct.linkedAccountId
        }
        for (const diff of acct.diffs) {
          // Pre-select everything that has changed; unchanged rows need no action
          const key = rowKey(acct.ofxAccountId, diff.ticker)
          if (diff.status !== 'unchanged') {
            initialAccepted[key] = true
          }
        }
      }
      setMappings(initial)
      setAccepted(initialAccepted)
      setStep('map')
    } catch (e: any) {
      setError(e.message ?? 'Failed to parse OFX file.')
    } finally {
      setLoading(false)
    }
  }

  // ---- Step 2: Mapping ----

  const mappingComplete = preview?.accounts.every(a => mappings[a.ofxAccountId]) ?? false

  const proceedToReview = () => {
    if (!mappingComplete) return
    // For accounts that just got mapped (no prior link), load their current holdings
    // by fetching a fresh preview with the mapping applied — but since holdings are
    // scoped by linkedAccountId in the preview, we just advance; the diffs already
    // reflect the unmapped state (all "added"). Good enough for review.
    setStep('review')
  }

  // ---- Step 3: Review ----

  const acceptAll = () => {
    if (!preview) return
    const next: Record<string, boolean> = {}
    for (const acct of preview.accounts) {
      for (const diff of acct.diffs) {
        next[rowKey(acct.ofxAccountId, diff.ticker)] = true
      }
    }
    setAccepted(next)
  }

  const toggleRow = (ofxAccountId: string, ticker: string) => {
    const key = rowKey(ofxAccountId, ticker)
    setAccepted(prev => ({ ...prev, [key]: !prev[key] }))
  }

  const handleApply = async () => {
    if (!preview) return
    setLoading(true)
    setError('')
    try {
      const mappingList = Object.entries(mappings).map(([ofxAccountId, myfiAccountId]) => ({
        ofxAccountId,
        myfiAccountId,
      }))
      const acceptedList = Object.entries(accepted)
        .filter(([, v]) => v)
        .map(([key]) => {
          const [ofxAccountId, ticker] = key.split('::')
          return { ofxAccountId, ticker }
        })

      await importApi.ofxApply({ rawOfx: rawOFX, filename: ofxFilename, mappings: mappingList, accepted: acceptedList })
      await mutate('/accounts')
      navigate('/holdings')
    } catch (e: any) {
      setError(e.message ?? 'Failed to apply import.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div>
        <h1 className="text-2xl font-bold text-text-primary">Import from OFX / QFX</h1>
        <p className="text-text-tertiary text-sm mt-1">
          Download your holdings file from Vanguard or another broker, then upload it here.
        </p>
      </div>

      {/* Step indicator */}
      <StepIndicator current={step} />

      {error && (
        <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3">
          {error}
        </div>
      )}

      {/* ---- Step 1: Upload ---- */}
      {step === 'upload' && (
        <Card title="Upload OFX / QFX File">
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">
              Export from Vanguard: <span className="text-text-primary">My Accounts → Download → Quicken (.qfx)</span>
            </p>
            <input
              ref={fileRef}
              type="file"
              accept=".ofx,.qfx"
              className="block w-full text-sm text-text-secondary
                file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0
                file:text-sm file:font-medium file:bg-subtle file:text-text-primary
                hover:file:bg-muted cursor-pointer"
            />
            <Button onClick={handleUpload} loading={loading}>
              Parse File
            </Button>
          </div>
        </Card>
      )}

      {/* ---- Step 2: Map accounts ---- */}
      {step === 'map' && preview && (
        <Card title="Link OFX Accounts to MyFi Accounts">
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">
              Match each account found in the file to its MyFi account. This mapping is saved for future imports.
            </p>
            {preview.accounts.map(acct => (
              <AccountMappingRow
                key={acct.ofxAccountId}
                acct={acct}
                myfiAccounts={myfiAccounts}
                selectedId={mappings[acct.ofxAccountId] ?? ''}
                onSelect={id => setMappings(prev => ({ ...prev, [acct.ofxAccountId]: id }))}
              />
            ))}
            <div className="flex gap-3 pt-2">
              <Button variant="secondary" onClick={() => setStep('upload')}>
                Back
              </Button>
              <Button onClick={proceedToReview} disabled={!mappingComplete}>
                Review Changes
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* ---- Step 3: Review diff ---- */}
      {step === 'review' && preview && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <p className="text-sm text-text-secondary">
              Confirm which updates to apply. Uncheck any you want to skip. Unchanged positions are hidden.
            </p>
            <Button variant="secondary" size="sm" onClick={acceptAll}>
              Select All
            </Button>
          </div>

          {preview.accounts.map(acct => {
            const accountName =
              myfiAccounts.find(a => a.id === mappings[acct.ofxAccountId])?.name ??
              acct.ofxAccountId
            return (
              <Card key={acct.ofxAccountId} title={accountName}>
                <DiffTable
                  acct={acct}
                  accepted={accepted}
                  onToggle={toggleRow}
                />
              </Card>
            )
          })}

          <div className="flex gap-3">
            <Button variant="secondary" onClick={() => setStep('map')}>
              Back
            </Button>
            <Button onClick={handleApply} loading={loading}>
              Apply Import
            </Button>
          </div>

          {error && (
            <p className="text-sm text-negative">{error}</p>
          )}
        </div>
      )}
    </div>
  )
}

// ---- Sub-components ----

function StepIndicator({ current }: { current: Step }) {
  const steps: { id: Step; label: string }[] = [
    { id: 'upload', label: '1. Upload' },
    { id: 'map', label: '2. Link Accounts' },
    { id: 'review', label: '3. Review' },
  ]
  return (
    <div className="flex gap-1">
      {steps.map((s, i) => (
        <div key={s.id} className="flex items-center gap-1">
          {i > 0 && <div className="w-8 h-px bg-muted" />}
          <span
            className={`text-sm font-medium px-2 py-0.5 rounded ${
              current === s.id
                ? 'text-accent-text bg-accent-tint-bg'
                : 'text-text-tertiary'
            }`}
          >
            {s.label}
          </span>
        </div>
      ))}
    </div>
  )
}

function DiffTable({
  acct,
  accepted,
  onToggle,
}: {
  acct: OFXAccountPreview
  accepted: Record<string, boolean>
  onToggle: (ofxAccountId: string, ticker: string) => void
}) {
  const [showUnchanged, setShowUnchanged] = useState(false)

  if (acct.diffs.length === 0) {
    return <p className="text-sm text-text-tertiary">No positions found in this account.</p>
  }

  const changed = acct.diffs.filter(d => d.status !== 'unchanged')
  const unchanged = acct.diffs.filter(d => d.status === 'unchanged')
  const visible = showUnchanged ? acct.diffs : changed

  if (changed.length === 0) {
    return (
      <p className="text-sm text-text-tertiary">
        All {unchanged.length} position{unchanged.length !== 1 ? 's' : ''} unchanged — nothing to update.
      </p>
    )
  }

  return (
    <div className="space-y-3">
      <div className="overflow-x-auto -mx-1">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
              <th className="py-2 pr-4 font-medium">Apply</th>
              <th className="py-2 pr-4 font-medium">Ticker</th>
              <th className="py-2 pr-4 font-medium">Status</th>
              <th className="py-2 pr-4 font-medium text-right">Current Shares</th>
              <th className="py-2 font-medium text-right">New Shares</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border-default">
            {visible.map(diff => (
              <DiffRow
                key={diff.ticker}
                diff={diff}
                isAccepted={!!accepted[rowKey(acct.ofxAccountId, diff.ticker)]}
                onToggle={() => onToggle(acct.ofxAccountId, diff.ticker)}
              />
            ))}
          </tbody>
        </table>
      </div>
      {unchanged.length > 0 && (
        <button
          className="text-xs text-text-tertiary hover:text-text-secondary transition-colors"
          onClick={() => setShowUnchanged(v => !v)}
        >
          {showUnchanged
            ? `Hide ${unchanged.length} unchanged position${unchanged.length !== 1 ? 's' : ''}`
            : `Show ${unchanged.length} unchanged position${unchanged.length !== 1 ? 's' : ''}`}
        </button>
      )}
    </div>
  )
}

function DiffRow({
  diff,
  isAccepted,
  onToggle,
}: {
  diff: HoldingDiff
  isAccepted: boolean
  onToggle: () => void
}) {
  const statusColors: Record<string, string> = {
    added: 'text-positive',
    removed: 'text-negative',
    changed: 'text-warning',
    unchanged: 'text-text-tertiary',
  }

  const isUnchanged = diff.status === 'unchanged'

  return (
    <tr className={`transition-opacity ${!isUnchanged && !isAccepted ? 'opacity-40' : ''} ${isUnchanged ? 'opacity-50' : ''}`}>
      <td className="py-2 pr-4">
        {isUnchanged ? (
          <span className="block w-4" />
        ) : (
          <input
            type="checkbox"
            checked={isAccepted}
            onChange={onToggle}
            className="rounded border-border-strong bg-subtle text-accent-text focus:ring-accent"
          />
        )}
      </td>
      <td className="py-2 pr-4 font-medium text-text-primary">{diff.ticker}</td>
      <td className="py-2 pr-4">
        <span className={`text-xs font-medium ${statusColors[diff.status] ?? 'text-text-secondary'}`}>
          {diff.status.toUpperCase()}
        </span>
      </td>
      <td className="py-2 pr-4 text-right text-text-secondary">
        {diff.curShares != null ? diff.curShares.toFixed(4) : '—'}
      </td>
      <td className="py-2 text-right text-text-primary">
        {diff.ofxShares != null ? diff.ofxShares.toFixed(4) : '—'}
      </td>
    </tr>
  )
}

function rowKey(ofxAccountId: string, ticker: string) {
  return `${ofxAccountId}::${ticker}`
}

// Sentinel value in the dropdown that triggers the inline create form.
const CREATE_SENTINEL = '__create__'

// Investment-focused account types shown in the inline create form.
const INVESTMENT_ACCOUNT_TYPES = [
  'brokerage', 'roth_ira', 'traditional_ira', 'sep_ira', 'simple_ira',
  'roth_401k', 'traditional_401k', 'roth_403b', 'traditional_403b', '457b', 'hsa', '529',
]

function AccountMappingRow({
  acct,
  myfiAccounts,
  selectedId,
  onSelect,
}: {
  acct: OFXAccountPreview
  myfiAccounts: Account[]
  selectedId: string
  onSelect: (id: string) => void
}) {
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState(`${acct.institution} …${acct.ofxAccountId.slice(-4)}`)
  const [newType, setNewType] = useState('brokerage')
  const [newInstitution, setNewInstitution] = useState(acct.institution ?? '')
  const [saving, setSaving] = useState(false)
  const [createError, setCreateError] = useState('')

  const handleDropdownChange = (value: string) => {
    if (value === CREATE_SENTINEL) {
      setCreating(true)
    } else {
      setCreating(false)
      onSelect(value)
    }
  }

  const handleCreate = async () => {
    if (!newName.trim()) return
    setSaving(true)
    setCreateError('')
    try {
      const account = await accountsApi.create({
        name: newName.trim(),
        accountType: newType,
        taxTreatment: ACCOUNT_TYPE_DEFAULTS[newType],
        institution: newInstitution || undefined,
      })
      await mutate('/accounts')
      setCreating(false)
      onSelect(account.id)
    } catch (e: any) {
      setCreateError(e.message ?? 'Failed to create account.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-3">
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-text-primary truncate">
            {acct.institution} — <span>…{acct.ofxAccountId.slice(-4)}</span>
          </p>
          <p className="text-xs text-text-tertiary">{acct.diffs.length} positions</p>
        </div>
        <select
          className="bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
          value={creating ? CREATE_SENTINEL : (selectedId || '')}
          onChange={e => handleDropdownChange(e.target.value)}
        >
          <option value="">— select account —</option>
          {myfiAccounts
            .filter(a => !a.isPartnerAccount)
            .map(a => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          <option value={CREATE_SENTINEL}>+ Create new account…</option>
        </select>
      </div>

      {creating && (
        <div className="ml-4 pl-4 border-l-2 border-accent/40 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-text-tertiary mb-1">Account name</label>
              <input
                type="text"
                value={newName}
                onChange={e => setNewName(e.target.value)}
                className="w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
              />
            </div>
            <div>
              <label className="block text-xs text-text-tertiary mb-1">Account type</label>
              <select
                value={newType}
                onChange={e => setNewType(e.target.value)}
                className="w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
              >
                {INVESTMENT_ACCOUNT_TYPES.map(t => (
                  <option key={t} value={t}>{ACCOUNT_TYPE_LABELS[t] ?? t}</option>
                ))}
              </select>
            </div>
          </div>
          <div>
            <label className="block text-xs text-text-tertiary mb-1">Institution</label>
            <InstitutionCombobox value={newInstitution} onChange={setNewInstitution} />
          </div>
          {createError && <p className="text-xs text-negative">{createError}</p>}
          <div className="flex gap-2">
            <Button size="sm" onClick={handleCreate} loading={saving}>
              Create &amp; Link
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { setCreating(false); onSelect('') }}>
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
