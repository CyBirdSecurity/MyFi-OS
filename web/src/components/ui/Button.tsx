import React from 'react'

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'
type Size = 'sm' | 'md'

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  loading?: boolean
}

const variantClasses: Record<Variant, string> = {
  // Ink-stamp: monochrome fill, not accent-colored — color stays reserved
  // for links/nav/tags.
  primary:   'bg-text-primary hover:bg-ink-hover text-canvas',
  secondary: 'bg-transparent hover:bg-subtle text-text-primary border border-border-strong',
  danger:    'bg-negative-tint-bg hover:brightness-95 text-negative',
  ghost:     'text-text-secondary hover:text-text-primary hover:bg-subtle',
}

const sizeClasses: Record<Size, string> = {
  sm: 'px-3 py-1.5 text-sm',
  md: 'px-4 py-2 text-sm',
}

export function Button({
  variant = 'primary',
  size = 'md',
  loading,
  children,
  disabled,
  className = '',
  ...rest
}: ButtonProps) {
  return (
    <button
      disabled={disabled || loading}
      className={`
        inline-flex items-center gap-2 rounded-md font-medium
        transition-colors duration-150 active:scale-[0.98] focus:outline-none focus:ring-2
        focus:ring-accent focus:ring-offset-2 focus:ring-offset-canvas
        disabled:opacity-50 disabled:cursor-not-allowed
        ${variantClasses[variant]} ${sizeClasses[size]} ${className}
      `}
      {...rest}
    >
      {loading && (
        <span className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
      )}
      {children}
    </button>
  )
}
