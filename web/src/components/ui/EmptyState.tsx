import { Link } from 'react-router-dom'

/**
 * What a card shows before there's anything in it: a line on what will appear
 * here, and a link to the step that fills it.
 */
export function EmptyState({
  children,
  action,
  className = '',
}: {
  children: React.ReactNode
  action?: { label: string; to: string }
  className?: string
}) {
  return (
    <div className={`text-sm ${className}`}>
      <p className="text-text-tertiary">{children}</p>
      {action && (
        <Link to={action.to} className="mt-2 inline-block font-medium text-accent-text hover:text-accent">
          {action.label} →
        </Link>
      )}
    </div>
  )
}
