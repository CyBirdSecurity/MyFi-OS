import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  SquaresFour,
  Bank,
  Wallet,
  DotsThreeCircle,
} from '@phosphor-icons/react'
import { MoreSheet } from './MoreSheet'
import { useReviewCount } from '../../hooks/useReviewCount'
import { QuickAddButton } from '../QuickAdd/QuickAddButton'

// Add sits in the middle; Goals lives in More.
const leftLinks = [
  { to: '/dashboard', label: 'Dashboard', icon: SquaresFour },
  { to: '/accounts', label: 'Accounts', icon: Bank },
]
const rightLinks = [{ to: '/budget', label: 'Budget', icon: Wallet }]

const tabCls = (isActive: boolean) =>
  `flex flex-col items-center justify-center gap-0.5 flex-1 h-full text-[11px] font-medium transition-colors ${
    isActive ? 'text-accent-text' : 'text-text-tertiary'
  }`

export function BottomNav() {
  const [moreOpen, setMoreOpen] = useState(false)
  const reviewCount = useReviewCount()

  return (
    <>
      <nav
        className="lg:hidden fixed bottom-0 inset-x-0 z-30 bg-surface border-t border-border-default flex items-stretch"
        // 4rem of tabs plus the home-indicator area; the padding sits outside the tabs.
        style={{ height: 'calc(4rem + env(safe-area-inset-bottom))', paddingBottom: 'env(safe-area-inset-bottom)' }}
      >
        {leftLinks.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} className={({ isActive }) => tabCls(isActive)}>
            <Icon size={24} weight="bold" className="shrink-0" />
            {label}
          </NavLink>
        ))}
        <QuickAddButton variant="tab" />
        {rightLinks.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} className={({ isActive }) => tabCls(isActive)}>
            <Icon size={24} weight="bold" className="shrink-0" />
            {label}
          </NavLink>
        ))}
        <button onClick={() => setMoreOpen(true)} className={tabCls(false)}>
          <span className="relative">
            <DotsThreeCircle size={24} weight="bold" className="block" />
            {reviewCount > 0 && (
              <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-warning" />
            )}
          </span>
          More
        </button>
      </nav>
      {moreOpen && (
        <MoreSheet
          onClose={() => setMoreOpen(false)}
          reviewCount={reviewCount}
        />
      )}
    </>
  )
}
