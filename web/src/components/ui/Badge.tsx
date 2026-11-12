import React from 'react'

type Color = 'brand' | 'positive' | 'negative' | 'warning' | 'neutral'

const colorClasses: Record<Color, string> = {
  brand:    'bg-accent-tint-bg text-accent-text ring-accent-tint-border',
  positive: 'bg-positive-tint-bg text-positive ring-positive/20',
  negative: 'bg-negative-tint-bg text-negative ring-negative/20',
  warning:  'bg-warning-tint-bg text-warning ring-warning/20',
  neutral:  'bg-subtle text-text-secondary ring-border-default',
}

interface BadgeProps {
  children: React.ReactNode
  color?: Color
}

export function Badge({ children, color = 'neutral' }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${colorClasses[color]}`}
    >
      {children}
    </span>
  )
}

export function TaxBadge({ treatment }: { treatment: string }) {
  const config: Record<string, { label: string; color: Color }> = {
    taxable:      { label: 'Taxable',  color: 'neutral' },
    tax_deferred: { label: 'Tax-Deferred', color: 'neutral' },
    tax_free:     { label: 'Tax-Free', color: 'positive' },
  }
  const { label, color } = config[treatment] ?? { label: treatment, color: 'neutral' }
  return <Badge color={color}>{label}</Badge>
}
