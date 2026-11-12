import React, { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { X } from '@phosphor-icons/react'

interface ModalProps {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  size?: 'md' | 'lg'
}

export function Modal({ open, onClose, title, children, size = 'md' }: ModalProps) {
  useEffect(() => {
    if (!open) return
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [open, onClose])

  if (!open) return null

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 pt-[max(1rem,env(safe-area-inset-top))]"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-overlay backdrop-blur-sm"
        onClick={onClose}
      />
      {/* Panel */}
      {/* Capped to the viewport so tall forms (or a phone keyboard / landscape
          screen) scroll inside the panel instead of running off-screen. */}
      <div className={`relative z-10 w-full ${size === 'lg' ? 'max-w-2xl' : 'max-w-md'} max-h-[calc(100dvh-2rem-env(safe-area-inset-top))] flex flex-col bg-surface border border-border-default rounded-lg shadow-float`}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-border-default shrink-0">
          <h2 id="modal-title" className="text-base font-semibold text-text-primary">
            {title}
          </h2>
          <button
            onClick={onClose}
            className="text-text-tertiary hover:text-text-primary transition-colors"
            aria-label="Close"
          >
            <X size={20} weight="bold" />
          </button>
        </div>
        <div className="px-6 py-5 overflow-y-auto">{children}</div>
      </div>
    </div>,
    document.body,
  )
}
