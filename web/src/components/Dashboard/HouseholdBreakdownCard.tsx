import { useAccounts } from '../../hooks/useAccounts'
import { useNetWorthCurrent } from '../../hooks/useNetWorth'
import { useHousehold } from '../../hooks/useHousehold'
import { useAuthStore } from '../../store/authStore'
import { Card } from '../ui/Card'
import { formatCurrency } from '../../utils/currency'

export function HouseholdBreakdownCard() {
  const { data: accounts } = useAccounts()
  const { data: nw } = useNetWorthCurrent()
  const { data: members } = useHousehold()
  const user = useAuthStore((s) => s.user)

  const partnerMember = (members ?? []).find((m) => m.relationship === 'partner')
  if (!partnerMember || !accounts || !nw) return null

  const selfMember = (members ?? []).find((m) => m.relationship === 'self')
  const selfName = selfMember?.name || user?.displayName || 'You'
  const partnerName = partnerMember.name || 'Partner'

  const partnerAccountIds = new Set(accounts.filter((a) => a.isPartnerAccount).map((a) => a.id))
  const jointAccountIds = new Set(accounts.filter((a) => a.isJoint).map((a) => a.id))

  let selfTotal = 0
  let partnerTotal = 0
  let jointTotal = 0
  for (const a of nw.accounts) {
    if (jointAccountIds.has(a.accountId)) {
      jointTotal += a.balance
    } else if (partnerAccountIds.has(a.accountId)) {
      partnerTotal += a.balance
    } else {
      selfTotal += a.balance
    }
  }

  const hasJoint = jointAccountIds.size > 0
  const total = selfTotal + partnerTotal + jointTotal
  const selfPct = total !== 0 ? Math.min(Math.max((selfTotal / total) * 100, 0), 100) : 50
  const jointPct = hasJoint && total !== 0 ? Math.min(Math.max((jointTotal / total) * 100, 0), 100 - selfPct) : 0
  const partnerPct = 100 - selfPct - jointPct

  return (
    <Card title="Household Breakdown">
      <div className={`grid ${hasJoint ? 'grid-cols-3' : 'grid-cols-2'} gap-4 mb-3`}>
        <div>
          <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">{selfName}</div>
          <div className="text-lg font-semibold text-text-primary">{formatCurrency(selfTotal)}</div>
        </div>
        {hasJoint && (
          <div>
            <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">Joint</div>
            <div className="text-lg font-semibold text-text-primary">{formatCurrency(jointTotal)}</div>
          </div>
        )}
        <div>
          <div className="text-xs text-text-tertiary uppercase tracking-wider mb-1">{partnerName}</div>
          <div className="text-lg font-semibold text-text-primary">{formatCurrency(partnerTotal)}</div>
        </div>
      </div>
      <div className="h-2 rounded-full overflow-hidden flex bg-muted">
        <div className="h-full bg-accent" style={{ width: `${selfPct}%` }} />
        {hasJoint && <div className="h-full bg-text-tertiary" style={{ width: `${jointPct}%` }} />}
        <div className="h-full bg-warning" style={{ width: `${partnerPct}%` }} />
      </div>
    </Card>
  )
}
