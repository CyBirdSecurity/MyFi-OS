import type { Goal } from '../../api/client'
import { formatCurrency } from '../../utils/currency'
import { formatDate } from '../../utils/dates'

interface GoalProgressBarProps {
  goal: Goal
  currentValue: number
}

export function GoalProgressBar({ goal, currentValue }: GoalProgressBarProps) {
  const rawPct = goal.targetAmount > 0 ? (currentValue / goal.targetAmount) * 100 : 100
  const pct = Math.min(Math.max(rawPct, 0), 100)
  const remaining = goal.targetAmount - currentValue

  return (
    <div>
      <div className="flex items-center justify-between mb-1.5">
        <span className="text-sm font-medium text-text-primary">{goal.name}</span>
        <span className="text-xs text-text-secondary">
          {formatCurrency(currentValue)} /{' '}
          {formatCurrency(goal.targetAmount)}
        </span>
      </div>
      <div className="h-2 bg-muted rounded-full overflow-hidden">
        <div
          // Progress is neutral: a goal partway done isn't a warning.
          className="h-full rounded-full bg-accent transition-all duration-500"
          style={{ width: `${pct}%` }}
        />
      </div>
      <div className="flex items-center justify-between mt-1">
        <span className="text-xs text-text-tertiary">{pct.toFixed(0)}% complete</span>
        <div className="flex items-center gap-3">
          {remaining > 0 && (
            <span className="text-xs text-text-tertiary">
              {formatCurrency(remaining)} to go
            </span>
          )}
          {goal.targetDate && (
            <span className="text-xs text-text-tertiary">
              By {formatDate(goal.targetDate, 'MMM yyyy')}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
