import React from 'react'

interface CardProps {
  children: React.ReactNode
  className?: string
  title?: React.ReactNode
  action?: React.ReactNode
}

export function Card({ children, className = '', title, action }: CardProps) {
  return (
    <div className={`bg-surface rounded-lg border border-border-default p-5 ${className}`}>
      {(title || action) && (
        <div className="flex items-center justify-between mb-4">
          {title && (
            <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wider">
              {title}
            </h2>
          )}
          {action}
        </div>
      )}
      {children}
    </div>
  )
}
