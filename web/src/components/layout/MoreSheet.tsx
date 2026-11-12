import { NavLink } from 'react-router-dom'
import {
  ChartBar,
  TrendUp,
  Umbrella,
  GitDiff,
  Receipt,
  FileArrowDown,
  Gear,
  SignOut,
  Target,
} from '@phosphor-icons/react'
import { useAuthStore } from '../../store/authStore'
import { authApi } from '../../api/client'
import { Modal } from '../ui/Modal'

const links = [
  { to: '/goals', label: 'Goals', icon: Target, showBadge: false },
  { to: '/holdings', label: 'Holdings', icon: ChartBar, showBadge: false },
  { to: '/forecast', label: 'Forecast', icon: TrendUp, showBadge: false },
  { to: '/retirement', label: 'Retirement', icon: Umbrella, showBadge: false },
  { to: '/scenarios', label: 'Scenarios', icon: GitDiff, showBadge: false },
  { to: '/taxes', label: 'Taxes', icon: Receipt, showBadge: false },
  { to: '/import', label: 'Import', icon: FileArrowDown, showBadge: true },
]

export function MoreSheet({
  onClose,
  reviewCount,
}: {
  onClose: () => void
  reviewCount: number
}) {
  const { setUser } = useAuthStore()

  const handleLogout = async () => {
    await authApi.logout()
    setUser(null)
    onClose()
  }

  const itemCls =
    'flex items-center gap-3 px-6 py-3.5 text-sm font-medium text-text-primary hover:bg-subtle transition-colors'

  return (
    <Modal open title="More" onClose={onClose}>
      <div className="-mx-6 -mb-5 divide-y divide-border-default">
        {links.map(({ to, label, icon: Icon, showBadge }) => (
          <NavLink key={to} to={to} onClick={onClose} className={itemCls}>
            <Icon size={20} weight="bold" className="text-text-tertiary" />
            <span className="flex-1">{label}</span>
            {showBadge && reviewCount > 0 && (
              <span className="flex items-center justify-center min-w-[20px] h-5 px-1 rounded-full bg-warning text-black text-xs font-bold">
                {reviewCount > 99 ? '99+' : reviewCount}
              </span>
            )}
          </NavLink>
        ))}
        <NavLink to="/settings" onClick={onClose} className={itemCls}>
          <Gear size={20} weight="bold" className="text-text-tertiary" />
          Settings
        </NavLink>
        <button onClick={handleLogout} className={`${itemCls} w-full text-left text-text-tertiary hover:text-negative`}>
          <SignOut size={20} weight="bold" />
          Sign out
        </button>
      </div>
    </Modal>
  )
}
