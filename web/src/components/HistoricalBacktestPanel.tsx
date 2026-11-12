import { useState } from 'react'
import { Card } from './ui/Card'
import { useBacktest } from '../hooks/useBacktest'

// Distinct from the "statistical" Monte Carlo success rate shown in
// FIMetricsPanel: this replays the plan against every real historical
// starting year (1928-present) instead of drawing returns from an assumed
// distribution.
export function HistoricalBacktestPanel() {
  const [expanded, setExpanded] = useState(false)
  const { data, error, isLoading } = useBacktest(expanded)

  return (
    <Card
      title="Historical Backtest"
      action={
        <button
          onClick={() => setExpanded((e) => !e)}
          className="shrink-0 text-xs text-text-tertiary hover:text-text-primary border border-border-strong rounded px-3 py-1.5 transition-colors"
        >
          {expanded ? 'Hide' : 'Show'}
        </button>
      }
    >
      {!expanded && (
        <p className="text-sm text-text-tertiary">
          See how your plan would have fared starting from every real historical year since 1928 &mdash;
          distinct from the statistical Monte Carlo simulation above.
        </p>
      )}

      {expanded && isLoading && (
        <div className="flex items-center gap-3 py-4">
          <div className="w-5 h-5 border-2 border-accent border-t-transparent rounded-full animate-spin" />
          <span className="text-sm text-text-tertiary">Running every historical starting year&hellip;</span>
        </div>
      )}

      {expanded && error && (
        <div className="text-sm text-negative">Couldn&rsquo;t run the historical backtest: {error.message}</div>
      )}

      {expanded && data && data.runs.length > 0 && (
        <div className="space-y-4">
          <div>
            <div
              className={`text-2xl font-bold ${
                data.survivalPct >= 80 ? 'text-positive' : data.survivalPct >= 60 ? 'text-warning' : 'text-negative'
              }`}
            >
              {data.survivalPct.toFixed(0)}%
            </div>
            <p className="text-sm text-text-secondary mt-1">
              of historical starting years, {data.earliestDataYear}&ndash;{data.latestDataYear}, sustained your plan.
            </p>
          </div>

          <div className="flex flex-wrap gap-[3px]">
            {data.runs.map((run) => (
              <div
                key={run.startYear}
                title={`${run.startYear}: ${run.survived ? 'Survived' : 'Depleted'}`}
                className={`w-3 h-3 rounded-sm ${run.survived ? 'bg-positive' : 'bg-negative'}`}
              />
            ))}
          </div>

          {data.dataCaveats.length > 0 && (
            <ul className="text-xs text-text-muted space-y-1 pt-2 border-t border-border-default/50">
              {data.dataCaveats.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      {expanded && data && data.runs.length === 0 && (
        <p className="text-sm text-text-tertiary">
          Your projection horizon is longer than the embedded historical dataset, so no historical starting
          year has enough data to run to completion.
        </p>
      )}
    </Card>
  )
}
