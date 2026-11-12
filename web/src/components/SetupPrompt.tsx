import { useState } from 'react'
import { Sparkle } from '@phosphor-icons/react'
import { Modal } from './ui/Modal'

/**
 * An inline banner telling the user a page needs a bit more information,
 * with a button that opens a small form in a popup.
 */
export function SetupPrompt({
  message,
  buttonLabel,
  title,
  children,
}: {
  message: string
  buttonLabel: string
  title: string
  children: (done: () => void) => React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <div className="flex flex-col sm:flex-row sm:items-center gap-3 bg-accent-tint-bg border border-accent-tint-border rounded-lg px-4 py-3">
        <Sparkle size={18} weight="bold" className="text-accent-text shrink-0 hidden sm:block" />
        <p className="text-sm text-text-primary flex-1">{message}</p>
        <button
          onClick={() => setOpen(true)}
          className="text-sm font-medium text-accent-text hover:text-accent whitespace-nowrap self-start sm:self-auto"
        >
          {buttonLabel} →
        </button>
      </div>
      <Modal open={open} onClose={() => setOpen(false)} title={title}>
        {children(() => setOpen(false))}
      </Modal>
    </>
  )
}
