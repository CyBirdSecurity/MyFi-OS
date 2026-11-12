import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import useSWR, { mutate } from 'swr'
import {
  importApi, exportApi, accountsApi, budgetApi, costBasisImportApi,
  ACCOUNT_TYPE_LABELS, ACCOUNT_TYPE_DEFAULTS,
} from '../api/client'
import type {
  ImportResult, ImportLog, FullImportSummary,
  Account, OFXPreviewResult, OFXAccountPreview, HoldingDiff,
  CostBasisPreviewResult, BudgetCategory,
} from '../api/client'
import { formatCurrency } from '../utils/currency'
import { Card } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { InstitutionCombobox } from '../components/ui/InstitutionCombobox'
import { PageGuide } from '../components/PageGuide'
import { useReviewCount } from '../hooks/useReviewCount'

// ── Shared helpers ────────────────────────────────────────────────────────────

function downloadTemplate() {
  const csv = [
    'account_name,snapshot_date,balance,notes',
    'Chase Checking,2024-01-01,5000.00,',
    'Fidelity 401k,2024-01-01,120000.00,',
    'Chase Credit Card,2024-01-01,-2500.00,Monthly statement',
  ].join('\n')
  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'myfi-import-template.csv'
  a.click()
  URL.revokeObjectURL(url)
}

const IMPORT_TYPE_LABELS: Record<string, string> = {
  csv_snapshot: 'Snapshot CSV',
  ofx_holdings: 'OFX Holdings',
  csv_transactions: 'Transactions CSV',
  full_backup: 'Full Backup',
}

const STATUS_STYLES: Record<string, string> = {
  success: 'text-positive',
  partial: 'text-warning',
  failed: 'text-negative',
}

const STATUS_LABELS: Record<string, string> = {
  success: 'Success',
  partial: 'Partial',
  failed: 'Failed',
}

function DropZone({
  id,
  accept,
  file,
  onChange,
  hint,
}: {
  id: string
  accept: string
  file: File | null
  onChange: (f: File | null) => void
  hint?: string
}) {
  return (
    <div
      className="border-2 border-dashed border-border-strong rounded-lg p-8 text-center hover:border-accent transition-colors cursor-pointer"
      onClick={() => document.getElementById(id)?.click()}
    >
      <input
        id={id}
        type="file"
        accept={accept}
        onChange={(e) => onChange(e.target.files?.[0] ?? null)}
        className="hidden"
      />
      <div className="text-4xl mb-3">⤓</div>
      {file ? (
        <div>
          <div className="text-text-primary font-medium">{file.name}</div>
          <div className="text-text-tertiary text-sm mt-1">{(file.size / 1024).toFixed(1)} KB</div>
        </div>
      ) : (
        <div>
          <div className="text-text-secondary">Click to select a file</div>
          {hint && <div className="text-text-muted text-sm mt-1">{hint}</div>}
        </div>
      )}
    </div>
  )
}

// ── Tab: Upload History ───────────────────────────────────────────────────────

function UploadHistoryTab() {
  const { data: logs, isLoading } = useSWR<ImportLog[]>('/import/history', () => importApi.history())

  if (isLoading) return <div className="text-text-tertiary text-sm py-8 text-center">Loading history…</div>

  if (!logs?.length) {
    return (
      <div className="text-center py-12 text-text-tertiary text-sm">
        No imports yet. Upload a file to see your history here.
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm min-w-[560px]">
        <thead>
          <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
            <th className="pb-3 font-medium">File</th>
            <th className="pb-3 font-medium">Type</th>
            <th className="pb-3 font-medium">Status</th>
            <th className="pb-3 font-medium text-right">Imported</th>
            <th className="pb-3 font-medium text-right">Skipped</th>
            <th className="pb-3 font-medium text-right">Date</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border-default">
          {logs.map((log) => (
            <tr key={log.id} className="hover:bg-subtle/30 transition-colors">
              <td className="py-3 pr-4 text-text-primary text-xs max-w-[200px] truncate" title={log.filename}>
                {log.filename || '—'}
              </td>
              <td className="py-3 pr-4 text-text-secondary">{IMPORT_TYPE_LABELS[log.importType] ?? log.importType}</td>
              <td className={`py-3 pr-4 font-medium ${STATUS_STYLES[log.status] ?? 'text-text-secondary'}`}>
                {STATUS_LABELS[log.status] ?? log.status}
              </td>
              <td className="py-3 pr-4 text-right text-text-primary">{log.rowCount}</td>
              <td className="py-3 pr-4 text-right text-text-tertiary">{log.skipped}</td>
              <td className="py-3 text-right text-text-tertiary whitespace-nowrap">
                {new Date(log.importedAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── Tab: Snapshot CSV ─────────────────────────────────────────────────────────

function SnapshotImportTab() {
  const [file, setFile] = useState<File | null>(null)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState('')

  const handleImport = async () => {
    if (!file) return
    setLoading(true)
    setError('')
    setResult(null)
    try {
      const res = await importApi.csv(file)
      setResult(res)
      await Promise.all([mutate('/accounts'), mutate('/networth/current'), mutate('/import/history')])
    } catch (err: any) {
      setError(err.message ?? 'Import failed. Please check your file and try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <Card title="CSV Format">
        <p className="text-sm text-text-secondary mb-3">Required columns (case-insensitive header row):</p>
        <div className="bg-canvas rounded-lg p-4 text-sm text-text-primary space-y-1">
          <div className="text-text-tertiary">account_name, snapshot_date, balance [, notes]</div>
          <div>Chase Checking, 2023-01-01, 5000.00</div>
          <div>Fidelity 401k, 2023-01-01, 120000.00</div>
          <div>Chase Credit Card, 2023-01-01, -2500.00, Monthly statement</div>
        </div>
        <ul className="mt-3 space-y-1 text-xs text-text-tertiary">
          <li>• <strong className="text-text-secondary">account_name</strong> must match an existing account name exactly (case-insensitive)</li>
          <li>• <strong className="text-text-secondary">snapshot_date</strong> format: YYYY-MM-DD</li>
          <li>• <strong className="text-text-secondary">balance</strong> is numeric; use negative values for liabilities</li>
          <li>• Existing snapshots for the same account + date will be overwritten</li>
        </ul>
        <div className="mt-4">
          <Button variant="secondary" onClick={downloadTemplate}>Download Template</Button>
        </div>
      </Card>

      <Card title="Upload File">
        <div className="space-y-4">
          <DropZone id="csv-input" accept=".csv,text/csv" file={file} onChange={setFile} hint="account_name, snapshot_date, balance" />

          {error && <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3">{error}</div>}

          {result && (
            <div className="rounded-lg border text-sm overflow-hidden">
              <div className={`px-4 py-3 font-medium ${result.imported > 0 ? 'bg-positive-tint-bg border-positive/30 text-positive' : 'bg-warning-tint-bg border-warning/30 text-warning'}`}>
                {result.imported > 0
                  ? `Successfully imported ${result.imported} snapshot${result.imported !== 1 ? 's' : ''}`
                  : 'No snapshots were imported'}
              </div>
              {(result.skipped > 0 || (result.errors ?? []).length > 0) && (
                <div className="bg-subtle/60 px-4 py-3 space-y-2 border-t border-border-strong">
                  {result.skipped > 0 && <div className="text-warning">{result.skipped} row{result.skipped !== 1 ? 's' : ''} skipped</div>}
                  {(result.errors ?? []).map((e, i) => (
                    <div key={i} className="text-negative text-xs bg-negative/5 rounded px-2 py-1">{e}</div>
                  ))}
                </div>
              )}
            </div>
          )}

          <Button onClick={handleImport} disabled={!file} loading={loading} className="w-full justify-center">
            Import CSV
          </Button>
        </div>
      </Card>
    </div>
  )
}

// ── Tab: OFX / QFX Holdings ───────────────────────────────────────────────────

type OFXStep = 'upload' | 'map' | 'review'

function rowKey(ofxAccountId: string, ticker: string) {
  return `${ofxAccountId}::${ticker}`
}

const INVESTMENT_ACCOUNT_TYPES = [
  'brokerage', 'roth_ira', 'traditional_ira', 'sep_ira', 'simple_ira',
  'roth_401k', 'traditional_401k', 'roth_403b', 'traditional_403b', '457b', 'hsa', '529',
]

const CREATE_SENTINEL = '__create__'

function OFXStepIndicator({ current }: { current: OFXStep }) {
  const steps: { id: OFXStep; label: string }[] = [
    { id: 'upload', label: '1. Upload' },
    { id: 'map', label: '2. Link Accounts' },
    { id: 'review', label: '3. Review' },
  ]
  return (
    <div className="flex gap-1">
      {steps.map((s, i) => (
        <div key={s.id} className="flex items-center gap-1">
          {i > 0 && <div className="w-8 h-px bg-muted" />}
          <span className={`text-sm font-medium px-2 py-0.5 rounded ${current === s.id ? 'text-accent-text bg-accent-tint-bg' : 'text-text-tertiary'}`}>
            {s.label}
          </span>
        </div>
      ))}
    </div>
  )
}

function DiffRow({ diff, isAccepted, onToggle }: { diff: HoldingDiff; isAccepted: boolean; onToggle: () => void }) {
  const statusColors: Record<string, string> = {
    added: 'text-positive', removed: 'text-negative', changed: 'text-warning', unchanged: 'text-text-tertiary',
  }
  return (
    <tr className={`transition-opacity ${isAccepted ? '' : 'opacity-40'}`}>
      <td className="py-2 pr-4">
        <input type="checkbox" checked={isAccepted} onChange={onToggle} className="rounded border-border-strong bg-subtle text-accent-text focus:ring-accent" />
      </td>
      <td className="py-2 pr-4 font-medium text-text-primary">{diff.ticker}</td>
      <td className="py-2 pr-4">
        <span className={`text-xs font-medium ${statusColors[diff.status] ?? 'text-text-secondary'}`}>{diff.status.toUpperCase()}</span>
      </td>
      <td className="py-2 pr-4 text-right text-text-secondary">{diff.curShares != null ? diff.curShares.toFixed(4) : '—'}</td>
      <td className="py-2 text-right text-text-primary">{diff.ofxShares != null ? diff.ofxShares.toFixed(4) : '—'}</td>
    </tr>
  )
}

function DiffTable({ acct, accepted, onToggle }: { acct: OFXAccountPreview; accepted: Record<string, boolean>; onToggle: (ofxAccountId: string, ticker: string) => void }) {
  if (acct.diffs.length === 0) return <p className="text-sm text-text-tertiary">No positions found in this account.</p>
  return (
    <div className="overflow-x-auto -mx-1">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
            <th className="py-2 pr-4 font-medium">Accept</th>
            <th className="py-2 pr-4 font-medium">Ticker</th>
            <th className="py-2 pr-4 font-medium">Status</th>
            <th className="py-2 pr-4 font-medium text-right">Current Shares</th>
            <th className="py-2 font-medium text-right">OFX Shares</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border-default">
          {acct.diffs.map((diff) => (
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
  )
}

function AccountMappingRow({
  acct, myfiAccounts, selectedId, onSelect,
}: {
  acct: OFXAccountPreview; myfiAccounts: Account[]; selectedId: string; onSelect: (id: string) => void
}) {
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState(`${acct.institution} …${acct.ofxAccountId.slice(-4)}`)
  const [newType, setNewType] = useState('brokerage')
  const [newInstitution, setNewInstitution] = useState(acct.institution ?? '')
  const [saving, setSaving] = useState(false)
  const [createError, setCreateError] = useState('')

  const handleDropdownChange = (value: string) => {
    if (value === CREATE_SENTINEL) { setCreating(true) } else { setCreating(false); onSelect(value) }
  }

  const handleCreate = async () => {
    if (!newName.trim()) return
    setSaving(true)
    setCreateError('')
    try {
      const account = await accountsApi.create({ name: newName.trim(), accountType: newType, taxTreatment: ACCOUNT_TYPE_DEFAULTS[newType], institution: newInstitution || undefined })
      await mutate('/accounts')
      setCreating(false)
      onSelect(account.id)
    } catch (e: any) {
      setCreateError(e.message ?? 'Failed to create account.')
    } finally {
      setSaving(false)
    }
  }

  const fieldCls = 'w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent'

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-3">
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-text-primary truncate">{acct.institution} — <span>…{acct.ofxAccountId.slice(-4)}</span></p>
          <p className="text-xs text-text-tertiary">{acct.diffs.length} positions</p>
        </div>
        <select className={fieldCls} style={{ width: 'auto' }} value={creating ? CREATE_SENTINEL : (selectedId || '')} onChange={(e) => handleDropdownChange(e.target.value)}>
          <option value="">— select account —</option>
          {myfiAccounts.filter((a) => !a.isPartnerAccount).map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          <option value={CREATE_SENTINEL}>+ Create new account…</option>
        </select>
      </div>
      {creating && (
        <div className="ml-4 pl-4 border-l-2 border-accent/40 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-text-tertiary mb-1">Account name</label>
              <input type="text" value={newName} onChange={(e) => setNewName(e.target.value)} className={fieldCls} />
            </div>
            <div>
              <label className="block text-xs text-text-tertiary mb-1">Account type</label>
              <select value={newType} onChange={(e) => setNewType(e.target.value)} className={fieldCls}>
                {INVESTMENT_ACCOUNT_TYPES.map((t) => <option key={t} value={t}>{ACCOUNT_TYPE_LABELS[t] ?? t}</option>)}
              </select>
            </div>
          </div>
          <div>
            <label className="block text-xs text-text-tertiary mb-1">Institution</label>
            <InstitutionCombobox value={newInstitution} onChange={setNewInstitution} />
          </div>
          {createError && <p className="text-xs text-negative">{createError}</p>}
          <div className="flex gap-2">
            <Button size="sm" onClick={handleCreate} loading={saving}>Create &amp; Link</Button>
            <Button size="sm" variant="ghost" onClick={() => { setCreating(false); onSelect('') }}>Cancel</Button>
          </div>
        </div>
      )}
    </div>
  )
}

function OFXImportTab() {
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<OFXStep>('upload')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)

  const [preview, setPreview] = useState<OFXPreviewResult | null>(null)
  const [rawOFX, setRawOFX] = useState('')
  const [ofxFilename, setOfxFilename] = useState('')
  const [mappings, setMappings] = useState<Record<string, string>>({})
  const [accepted, setAccepted] = useState<Record<string, boolean>>({})

  const { data: myfiAccounts = [] } = useSWR<Account[]>('/accounts', () => accountsApi.list())

  const handleUpload = async () => {
    const file = fileRef.current?.files?.[0]
    if (!file) return
    setLoading(true)
    setError('')
    setSuccess(false)
    try {
      const text = await file.text()
      setRawOFX(text)
      setOfxFilename(file.name)
      const result = await importApi.ofxPreview(file)
      setPreview(result)
      const initial: Record<string, string> = {}
      const initialAccepted: Record<string, boolean> = {}
      for (const acct of result.accounts) {
        if (acct.linkedAccountId) initial[acct.ofxAccountId] = acct.linkedAccountId
        for (const diff of acct.diffs) {
          if (diff.status === 'added' || diff.status === 'unchanged') {
            initialAccepted[rowKey(acct.ofxAccountId, diff.ticker)] = true
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

  const mappingComplete = preview?.accounts.every((a) => mappings[a.ofxAccountId]) ?? false

  const acceptAll = () => {
    if (!preview) return
    const next: Record<string, boolean> = {}
    for (const acct of preview.accounts) {
      for (const diff of acct.diffs) next[rowKey(acct.ofxAccountId, diff.ticker)] = true
    }
    setAccepted(next)
  }

  const toggleRow = (ofxAccountId: string, ticker: string) => {
    const key = rowKey(ofxAccountId, ticker)
    setAccepted((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const handleApply = async () => {
    if (!preview) return
    setLoading(true)
    setError('')
    try {
      const mappingList = Object.entries(mappings).map(([ofxAccountId, myfiAccountId]) => ({ ofxAccountId, myfiAccountId }))
      const acceptedList = Object.entries(accepted).filter(([, v]) => v).map(([key]) => {
        const [ofxAccountId, ticker] = key.split('::')
        return { ofxAccountId, ticker }
      })
      await importApi.ofxApply({ rawOfx: rawOFX, filename: ofxFilename, mappings: mappingList, accepted: acceptedList })
      await Promise.all([mutate('/accounts'), mutate('/holdings'), mutate('/import/history')])
      setSuccess(true)
      setStep('upload')
      setPreview(null)
      if (fileRef.current) fileRef.current.value = ''
    } catch (e: any) {
      setError(e.message ?? 'Failed to apply import.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <p className="text-sm text-text-secondary">
        Download a holdings export from Vanguard (<span className="text-text-primary">My Accounts → Download → Quicken (.qfx)</span>) or any broker that supports OFX/QFX format.
      </p>

      <OFXStepIndicator current={step} />

      {error && <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3">{error}</div>}

      {success && (
        <div className="rounded-lg bg-positive-tint-bg border border-positive/20 px-4 py-3 text-sm text-positive">
          Holdings imported successfully.{' '}
          <Link to="/holdings" className="underline">View Holdings →</Link>
        </div>
      )}

      {step === 'upload' && (
        <Card title="Upload OFX / QFX File">
          <div className="space-y-4">
            <input ref={fileRef} type="file" accept=".ofx,.qfx"
              className="block w-full text-sm text-text-secondary file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-sm file:font-medium file:bg-subtle file:text-text-primary hover:file:bg-muted cursor-pointer" />
            <Button onClick={handleUpload} loading={loading}>Parse File</Button>
          </div>
        </Card>
      )}

      {step === 'map' && preview && (
        <Card title="Link OFX Accounts to MyFi Accounts">
          <div className="space-y-4">
            <p className="text-sm text-text-secondary">Match each account in the file to its MyFi account. This mapping is saved for future imports.</p>
            {preview.accounts.map((acct) => (
              <AccountMappingRow key={acct.ofxAccountId} acct={acct} myfiAccounts={myfiAccounts} selectedId={mappings[acct.ofxAccountId] ?? ''} onSelect={(id) => setMappings((prev) => ({ ...prev, [acct.ofxAccountId]: id }))} />
            ))}
            <div className="flex gap-3 pt-2">
              <Button variant="secondary" onClick={() => setStep('upload')}>Back</Button>
              <Button onClick={() => setStep('review')} disabled={!mappingComplete}>Review Changes</Button>
            </div>
          </div>
        </Card>
      )}

      {step === 'review' && preview && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <p className="text-sm text-text-secondary">Review each change. Deselect rows you want to keep unchanged.</p>
            <Button variant="secondary" size="sm" onClick={acceptAll}>Accept All</Button>
          </div>
          {preview.accounts.map((acct) => {
            const accountName = myfiAccounts.find((a) => a.id === mappings[acct.ofxAccountId])?.name ?? acct.ofxAccountId
            return (
              <Card key={acct.ofxAccountId} title={accountName}>
                <DiffTable acct={acct} accepted={accepted} onToggle={toggleRow} />
              </Card>
            )
          })}
          <div className="flex gap-3">
            <Button variant="secondary" onClick={() => setStep('map')}>Back</Button>
            <Button onClick={handleApply} loading={loading}>Apply Import</Button>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Tab: Cost Basis CSV ───────────────────────────────────────────────────────

function CostBasisTab() {
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<'upload' | 'preview'>('upload')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [preview, setPreview] = useState<CostBasisPreviewResult | null>(null)
  const [accepted, setAccepted] = useState<Record<string, number>>({})

  const inputCls = 'w-24 bg-muted border border-border-strong rounded px-2 py-1 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent text-right'

  const handleUpload = async () => {
    const file = fileRef.current?.files?.[0]
    if (!file) return
    setLoading(true)
    setError('')
    setSuccess(false)
    try {
      const result = await costBasisImportApi.preview(file)
      setPreview(result)
      const initial: Record<string, number> = {}
      for (const row of result.rows) {
        if (row.costBasis == null) continue
        const targets = row.autoSelectedMatchId ? row.matches.filter((m) => m.id === row.autoSelectedMatchId) : row.matches
        for (const m of targets) initial[m.id] = row.costBasis
      }
      setAccepted(initial)
      setStep('preview')
    } catch (e: any) {
      setError(e.message ?? 'Failed to parse file.')
    } finally {
      setLoading(false)
    }
  }

  const handleApply = async () => {
    const rows = Object.entries(accepted).filter(([, v]) => v > 0).map(([holdingId, costBasis]) => ({ holdingId, costBasis }))
    if (!rows.length) { setStep('upload'); return }
    setLoading(true)
    setError('')
    try {
      await costBasisImportApi.apply(rows)
      await mutate('/holdings')
      setSuccess(true)
      setStep('upload')
      setPreview(null)
      if (fileRef.current) fileRef.current.value = ''
    } catch (e: any) {
      setError(e.message ?? 'Apply failed.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <p className="text-sm text-text-secondary">
        Upload a portfolio positions CSV from your broker (Vanguard, Fidelity, Schwab, etc.).
        The file should include a column for ticker/symbol and average cost basis per share.
      </p>

      {error && <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3">{error}</div>}

      {success && (
        <div className="rounded-lg bg-positive-tint-bg border border-positive/20 px-4 py-3 text-sm text-positive">
          Cost basis updated successfully.{' '}
          <Link to="/holdings" className="underline">View Holdings →</Link>
        </div>
      )}

      {step === 'upload' && (
        <Card title="Upload Positions CSV">
          <div className="space-y-4">
            <input ref={fileRef} type="file" accept=".csv,.txt"
              className="block w-full text-sm text-text-secondary file:mr-3 file:py-1.5 file:px-3 file:rounded file:border-0 file:text-sm file:bg-muted file:text-text-primary hover:file:bg-muted" />
            <Button onClick={handleUpload} loading={loading}>Parse File</Button>
          </div>
        </Card>
      )}

      {step === 'preview' && preview && (
        <Card title="Review Cost Basis Updates">
          <div className="space-y-3">
            <p className="text-xs text-text-tertiary">
              Detected — Ticker: <span className="text-text-primary">{preview.columns.ticker}</span>
              {' · '}Cost Basis: <span className="text-text-primary">{preview.columns.costBasis}</span>
              {preview.columns.account && <>{' · '}Account: <span className="text-text-primary">{preview.columns.account}</span></>}
            </p>
            <div className="overflow-x-auto overflow-y-auto max-h-96">
              <table className="w-full text-sm min-w-[420px]">
                <thead>
                  <tr className="text-left text-xs text-text-tertiary border-b border-border-default">
                    <th className="pb-2 font-medium">Ticker</th>
                    <th className="pb-2 font-medium">Account</th>
                    <th className="pb-2 font-medium text-right">Current Basis</th>
                    <th className="pb-2 font-medium text-right">New Basis / Share</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.rows.map((row) =>
                    row.matches.length === 0 ? (
                      <tr key={row.ticker} className="border-b border-border-default/40 opacity-40">
                        <td className="py-2 text-text-tertiary">{row.ticker}</td>
                        <td className="py-2 text-xs text-text-muted" colSpan={3}>no matching holding found</td>
                      </tr>
                    ) : (
                      row.matches.map((m) => {
                        const isAutoSelected = row.autoSelectedMatchId === m.id
                        const sharesMismatch = row.sharesInCsv != null && Math.abs(row.sharesInCsv - m.shares) / Math.max(m.shares, 0.0001) > 0.01
                        return (
                          <tr key={m.id} className={`border-b border-border-default/40 ${!isAutoSelected && row.autoSelectedMatchId ? 'opacity-50' : ''}`}>
                            <td className="py-2 text-text-primary">{row.ticker}</td>
                            <td className="py-2 text-xs text-text-tertiary">
                              {m.accountName}
                              {sharesMismatch && <span className="ml-2 text-warning" title={`CSV: ${row.sharesInCsv?.toFixed(3)} shares, holding: ${m.shares.toFixed(3)}`}>⚠ shares mismatch</span>}
                            </td>
                            <td className="py-2 text-right text-text-tertiary text-xs">{m.currentBasis != null ? formatCurrency(m.currentBasis) : '—'}</td>
                            <td className="py-2 text-right">
                              <input type="number" step="0.01" min="0" className={inputCls}
                                value={accepted[m.id] ?? row.costBasis ?? ''}
                                onChange={(e) => { const v = parseFloat(e.target.value); setAccepted((prev) => ({ ...prev, [m.id]: isNaN(v) ? 0 : v })) }} />
                            </td>
                          </tr>
                        )
                      })
                    )
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex justify-between items-center pt-1">
              <button className="text-sm text-text-tertiary hover:text-text-primary" onClick={() => setStep('upload')}>← Re-upload</button>
              <Button onClick={handleApply} loading={loading}>
                Apply {Object.values(accepted).filter((v) => v > 0).length} Updates
              </Button>
            </div>
          </div>
        </Card>
      )}
    </div>
  )
}

// ── Tab: Transactions CSV ─────────────────────────────────────────────────────

function TransactionsTab() {
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [accountId, setAccountId] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [savingId, setSavingId] = useState<string | null>(null)

  const { data: categories = [] } = useSWR<BudgetCategory[]>('/budget/categories', () => budgetApi.listCategories())
  const { data: accounts = [] } = useSWR<Account[]>('/accounts', () => accountsApi.list())

  const REVIEW_KEY = '/budget/transactions?needsReview=true'
  const { data: reviewTxns = [], mutate: mutateReview } = useSWR<import('../api/client').Transaction[]>(
    REVIEW_KEY,
    () => budgetApi.listTransactions({ needsReview: true }),
    { refreshInterval: 15_000 },
  )

  const handleStartImport = async () => {
    if (!file) return
    setLoading(true)
    setError('')
    setSuccess('')
    try {
      const result = await budgetApi.importQuick(file, accountId)
      const parts = [`${result.inserted} transaction${result.inserted !== 1 ? 's' : ''} imported.`]
      if (result.skipped > 0) parts.push(`${result.skipped} duplicates skipped.`)
      if (result.reviewCount > 0) parts.push(`${result.reviewCount} need category review below; suggested categories will appear over the next few minutes.`)
      setSuccess(parts.join(' '))
      setFile(null)
      setAccountId('')
      if (fileRef.current) fileRef.current.value = ''
      await Promise.all([mutate('/import/history'), mutateReview(), mutate('budget/review-count')])
    } catch (e: any) {
      setError(e.message ?? 'Import failed.')
    } finally {
      setLoading(false)
    }
  }

  const handleCategoryChange = async (txnId: string, categoryId: string) => {
    setSavingId(txnId)
    try {
      await budgetApi.updateTransaction(txnId, { categoryId })
      await Promise.all([mutateReview(), mutate('budget/review-count')])
    } finally {
      setSavingId(null)
    }
  }

  return (
    <div className="space-y-6">
      <Card title="Import Transactions CSV">
        <div className="space-y-4">
          <p className="text-sm text-text-secondary">
            Upload a CSV export from your credit card, checking account, or savings account.
            Transactions are saved immediately — any that can't be auto-classified appear in the review queue below.
          </p>

          {error && <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3">{error}</div>}
          {success && (
            <div className="rounded-lg bg-positive-tint-bg border border-positive/20 px-4 py-3 text-sm text-positive">
              {success}
            </div>
          )}

          <input
            ref={fileRef}
            type="file"
            accept=".csv"
            onChange={(e) => { setFile(e.target.files?.[0] ?? null); setSuccess('') }}
            className="block w-full text-sm text-text-secondary file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:bg-muted file:text-text-primary hover:file:bg-muted"
          />
          {file && (
            <div className="flex items-center gap-2 px-3 py-2 bg-subtle rounded-lg text-sm text-text-secondary">
              <span className="text-positive">✓</span>
              <span>{file.name} — ready to import</span>
            </div>
          )}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Account <span className="text-text-tertiary">(optional)</span>
            </label>
            <select
              value={accountId}
              onChange={(e) => setAccountId(e.target.value)}
              className="w-full bg-subtle border border-border-strong rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent"
            >
              <option value="">— none —</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
          </div>
          <Button onClick={handleStartImport} loading={loading} disabled={!file}>
            Start Import
          </Button>
        </div>
      </Card>

      {reviewTxns.length > 0 && (
        <Card title={`Needs Review — ${reviewTxns.length} transaction${reviewTxns.length !== 1 ? 's' : ''}`}>
          <div className="space-y-1">
            <p className="text-sm text-text-secondary mb-3">
              These transactions couldn't be auto-classified. Select a category for each one.
            </p>
            <div className="grid grid-cols-[auto_1fr_auto_auto] gap-x-3 gap-y-0 px-2 py-1 text-xs text-text-tertiary border-b border-border-default mb-1">
              <span>Date</span>
              <span>Merchant</span>
              <span className="text-right">Amount</span>
              <span>Category</span>
            </div>
            {reviewTxns.map((txn) => (
              <div key={txn.id} className="grid grid-cols-[auto_1fr_auto_auto] gap-x-3 items-center px-2 py-2 rounded hover:bg-subtle/50">
                <span className="text-text-tertiary whitespace-nowrap text-xs">
                  {txn.transDate.slice(0, 10)}
                </span>
                <div className="min-w-0">
                  <span className="text-text-primary text-sm truncate block" title={txn.rawDescription}>
                    {txn.merchant}
                  </span>
                  {txn.accountLabel && (
                    <span className="text-text-tertiary text-xs">{txn.accountLabel}</span>
                  )}
                </div>
                <span className="text-text-primary text-sm whitespace-nowrap text-right">
                  {formatCurrency(txn.amount)}
                </span>
                <select
                  value=""
                  disabled={savingId === txn.id}
                  onChange={(e) => handleCategoryChange(txn.id, e.target.value)}
                  className="bg-subtle border border-border-strong rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:ring-1 focus:ring-accent disabled:opacity-50"
                >
                  <option value="" disabled>— pick —</option>
                  {categories.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  )
}

// ── Tab: Full Backup ──────────────────────────────────────────────────────────

function FullBackupTab() {
  const [exportLoading, setExportLoading] = useState(false)
  const [exportError, setExportError] = useState('')
  const [importFile, setImportFile] = useState<File | null>(null)
  const [importLoading, setImportLoading] = useState(false)
  const [importError, setImportError] = useState('')
  const [importResult, setImportResult] = useState<FullImportSummary | null>(null)

  const handleExport = async () => {
    setExportLoading(true)
    setExportError('')
    try {
      await exportApi.download()
    } catch (err: any) {
      setExportError(err.message ?? 'Export failed.')
    } finally {
      setExportLoading(false)
    }
  }

  const handleImport = async () => {
    if (!importFile) return
    setImportLoading(true)
    setImportError('')
    setImportResult(null)
    try {
      const res = await importApi.fullBackup(importFile)
      setImportResult(res)
      await Promise.all([
        mutate('/accounts'), mutate('/networth/current'), mutate('/goals'),
        mutate('/profile'), mutate('/return-rates'), mutate('/allocation-targets'), mutate('/import/history'),
      ])
    } catch (err: any) {
      setImportError(err.message ?? 'Import failed. Make sure this is a valid MyFi backup file.')
    } finally {
      setImportLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <Card title="Export Backup">
        <p className="text-sm text-text-secondary mb-4">
          Download a complete backup of all your account data — snapshots, holdings, contributions, goals, and settings — as a single JSON file.
        </p>
        {exportError && <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3 mb-4">{exportError}</div>}
        <Button onClick={handleExport} loading={exportLoading}>Download Backup</Button>
      </Card>

      <Card title="Restore from Backup">
        <p className="text-sm text-text-secondary mb-4">
          Upload a previously downloaded MyFi backup file to restore your data. Existing accounts are matched by name and type — snapshots and holdings are updated, and missing accounts are created.
        </p>
        <div className="space-y-4">
          <DropZone id="backup-input" accept=".json,application/json" file={importFile} onChange={(f) => { setImportFile(f); setImportResult(null) }} hint="myfi-backup-YYYY-MM-DD.json" />

          {importError && <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-4 py-3">{importError}</div>}

          {importResult && (
            <div className="rounded-lg border overflow-hidden text-sm">
              <div className="px-4 py-3 font-medium bg-positive-tint-bg text-positive border-b border-positive/20">Restore complete</div>
              <div className="bg-subtle/60 px-4 py-3 space-y-1 text-text-secondary">
                <div>{importResult.accountsCreated} account{importResult.accountsCreated !== 1 ? 's' : ''} created, {importResult.accountsMatched} matched</div>
                <div>{importResult.snapshotsUpserted} snapshot{importResult.snapshotsUpserted !== 1 ? 's' : ''} restored</div>
                <div>{importResult.holdingsRestored} holding{importResult.holdingsRestored !== 1 ? 's' : ''} restored</div>
                <div>{importResult.contributionsRestored} contribution{importResult.contributionsRestored !== 1 ? 's' : ''} restored</div>
                <div>{importResult.goalsRestored} goal{importResult.goalsRestored !== 1 ? 's' : ''} restored</div>
                {(importResult.warnings ?? []).map((w, i) => (
                  <div key={i} className="text-warning text-xs bg-warning/5 rounded px-2 py-1">{w}</div>
                ))}
              </div>
            </div>
          )}

          <Button onClick={handleImport} disabled={!importFile} loading={importLoading} className="w-full justify-center">
            Restore Backup
          </Button>
        </div>
      </Card>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'snapshot' | 'ofx' | 'costbasis' | 'transactions' | 'backup' | 'history'

const TABS: { id: Tab; label: string }[] = [
  { id: 'snapshot',     label: 'Snapshot CSV' },
  { id: 'ofx',         label: 'OFX / QFX Holdings' },
  { id: 'costbasis',   label: 'Cost Basis' },
  { id: 'transactions', label: 'Transactions' },
  { id: 'backup',      label: 'Full Backup' },
  { id: 'history',     label: 'Upload History' },
]

export function ImportPage() {
  const [tab, setTab] = useState<Tab>('snapshot')
  const reviewCount = useReviewCount()

  const tabCls = (id: Tab) =>
    `px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ${
      tab === id ? 'border-accent text-accent-text' : 'border-transparent text-text-tertiary hover:text-text-primary'
    }`

  return (
    <div className="space-y-6 max-w-6xl">
      <div>
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold text-text-primary">Import &amp; Export</h1>
          <PageGuide id="import" />
        </div>
        <p className="text-text-tertiary text-sm mt-1">
          Upload data from your broker, import transactions, or export a full backup of your account.
        </p>
      </div>

      <div className="flex border-b border-border-default overflow-x-auto">
        {TABS.map(({ id, label }) => (
          <button key={id} onClick={() => setTab(id)} className={tabCls(id)}>
            <span className="flex items-center gap-1.5">
              {label}
              {id === 'transactions' && reviewCount > 0 && (
                <span className="flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-warning text-black text-xs font-bold leading-none">
                  {reviewCount > 99 ? '99+' : reviewCount}
                </span>
              )}
            </span>
          </button>
        ))}
      </div>

      {tab === 'snapshot'     && <SnapshotImportTab />}
      {tab === 'ofx'         && <OFXImportTab />}
      {tab === 'costbasis'   && <CostBasisTab />}
      {tab === 'transactions' && <TransactionsTab />}
      {tab === 'backup'      && <FullBackupTab />}
      {tab === 'history'     && <Card title="Upload History"><UploadHistoryTab /></Card>}
    </div>
  )
}
