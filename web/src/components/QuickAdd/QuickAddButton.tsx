import { useState } from 'react'
import { Plus } from '@phosphor-icons/react'
import { QuickAddTransactionModal } from './QuickAddTransactionModal'

/**
 * "Add transaction", built into the navigation rather than floating over page
 * content: a sidebar button on desktop, the center tab of the bottom bar on phones.
 */
export function QuickAddButton({ variant }: { variant: 'sidebar' | 'tab' }) {
  const [open, setOpen] = useState(false)

  return (
    <>
      {variant === 'sidebar' ? (
        <button
          onClick={() => setOpen(true)}
          className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg border border-border-strong text-sm font-medium text-text-primary hover:bg-subtle transition-colors"
        >
          <Plus size={16} weight="bold" />
          Add transaction
        </button>
      ) : (
        <button
          onClick={() => setOpen(true)}
          aria-label="Add transaction"
          className="flex flex-col items-center justify-center gap-0.5 flex-1 h-full text-[11px] font-medium text-text-tertiary"
        >
          <span className="flex items-center justify-center w-9 h-9 -my-1.5 rounded-full bg-text-primary text-surface">
            <Plus size={20} weight="bold" />
          </span>
          Add
        </button>
      )}
      {open && <QuickAddTransactionModal onClose={() => setOpen(false)} />}
    </>
  )
}
