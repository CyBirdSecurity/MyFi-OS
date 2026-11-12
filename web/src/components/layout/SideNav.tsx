import { NavLink } from 'react-router-dom'
import {
  SquaresFour,
  Bank,
  ChartBar,
  TrendUp,
  Umbrella,
  GitDiff,
  Target,
  Wallet,
  Receipt,
  FileArrowDown,
  Gear,
  SignOut,
} from '@phosphor-icons/react'
import { useAuthStore } from '../../store/authStore'
import { authApi } from '../../api/client'
import { useReviewCount } from '../../hooks/useReviewCount'
import { QuickAddButton } from '../QuickAdd/QuickAddButton'

const links = [
  { to: '/dashboard',  label: 'Dashboard',  icon: SquaresFour },
  { to: '/accounts',   label: 'Accounts',   icon: Bank },
  { to: '/holdings',   label: 'Holdings',   icon: ChartBar },
  { to: '/forecast',   label: 'Forecast',   icon: TrendUp },
  { to: '/retirement', label: 'Retirement', icon: Umbrella },
  { to: '/scenarios',  label: 'Scenarios',  icon: GitDiff },
  { to: '/goals',      label: 'Goals',      icon: Target },
  { to: '/budget',     label: 'Budget',     icon: Wallet },
  { to: '/taxes',      label: 'Taxes',      icon: Receipt },
  { to: '/import',     label: 'Import',     icon: FileArrowDown },
]

const navCls = (isActive: boolean) =>
  `relative flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
    isActive
      ? 'bg-accent-tint-bg text-accent-text'
      : 'text-text-secondary hover:text-text-primary hover:bg-subtle'
  }`

export function SideNav() {
  const { user, setUser } = useAuthStore()
  const reviewCount = useReviewCount()

  const handleLogout = async () => {
    await authApi.logout()
    setUser(null)
  }

  return (
    <nav className="hidden lg:flex fixed top-0 left-0 h-screen w-56 bg-surface border-r border-border-default flex-col transition-all">
      {/* Logo */}
      <div className="px-5 py-4 border-b border-border-default flex items-center gap-2.5">
        <img src="/logo-icon.png" alt="" className="h-7 w-7 object-contain" />
        <span className="text-xl font-bold text-accent-text tracking-tight">MyFi</span>
      </div>

      <div className="px-3 pt-4">
        <QuickAddButton variant="sidebar" />
      </div>

      {/* Nav links */}
      <div className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {links.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            title={label}
            className={({ isActive }) => navCls(isActive)}
          >
            <Icon size={20} weight="bold" className="flex-shrink-0" />
            <span>{label}</span>
            {to === '/import' && reviewCount > 0 && (
              <span className="ml-auto flex items-center justify-center min-w-[20px] h-5 px-1 rounded-full bg-warning text-black text-xs font-bold">
                {reviewCount > 99 ? '99+' : reviewCount}
              </span>
            )}
          </NavLink>
        ))}
      </div>

      {/* Bottom section */}
      <div className="px-3 pb-2 space-y-1">
        <NavLink
          to="/settings"
          title="Settings"
          className={({ isActive }) => navCls(isActive)}
        >
          <Gear size={20} weight="bold" className="flex-shrink-0" />
          <span>Settings</span>
        </NavLink>
      </div>

      {/* User + logout */}
      <div className="px-4 py-4 border-t border-border-default">
        <div className="text-xs text-text-tertiary truncate mb-2">{user?.email}</div>
        <button
          onClick={handleLogout}
          title="Sign out"
          className="w-full flex items-center gap-3 text-xs text-text-tertiary hover:text-negative transition-colors"
        >
          <SignOut size={18} weight="bold" className="flex-shrink-0" />
          <span>Sign out</span>
        </button>
      </div>
    </nav>
  )
}
