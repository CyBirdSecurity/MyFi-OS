import { Card } from '../ui/Card'
import { useConcentration } from '../../hooks/useConcentration'
import type { ConcentrationFlag } from '../../api/client'

export const SEVERITY_STYLES: Record<ConcentrationFlag['severity'], string> = {
  critical: 'bg-negative-tint-bg text-negative',
  warning: 'bg-warning-tint-bg text-warning',
  info: 'bg-subtle text-text-tertiary',
}

export const SEVERITY_LABELS: Record<ConcentrationFlag['severity'], string> = {
  critical: 'Critical',
  warning: 'Warning',
  info: 'Info',
}

// Asset-class drift is already surfaced by the Asset Allocation panel above
// this one, so it's excluded here to avoid showing the same thing twice.
const RELEVANT_TYPES = new Set<ConcentrationFlag['type']>(['single_ticker', 'redundant_ticker'])

export function ConcentrationPanel() {
  const { data, isLoading } = useConcentration()

  if (isLoading || !data) return null

  // The API sends flags: null (not []) when there are no holdings.
  const allFlags = data.flags ?? []
  const flags = allFlags.filter((f) => RELEVANT_TYPES.has(f.type))
  const overlapFlags = allFlags.filter((f) => f.type === 'sector_overlap')
  if (flags.length === 0 && !data.hasFundCompositionData) return null

  return (
    <Card title="Concentration">
      {flags.length > 0 && (
        <div className="space-y-2">
          {flags.map((flag, i) => (
            <div
              key={`${flag.type}-${flag.ticker}-${i}`}
              className="flex items-start gap-3 py-2 border-b border-border-default/50 last:border-0"
            >
              <span
                className={`shrink-0 mt-0.5 inline-block px-2 py-0.5 rounded-full text-xs font-medium ${SEVERITY_STYLES[flag.severity]}`}
              >
                {SEVERITY_LABELS[flag.severity]}
              </span>
              <p className="text-sm text-text-secondary">{flag.message}</p>
            </div>
          ))}
        </div>
      )}

      {data.hasFundCompositionData && (
        <div className={flags.length > 0 ? 'mt-4 pt-4 border-t border-border-default/50' : ''}>
          <div className="flex items-center gap-2 mb-2">
            <h3 className="text-xs font-semibold text-text-tertiary uppercase tracking-wider">
              Sector Overlap
            </h3>
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-subtle text-text-tertiary">
              Experimental
            </span>
          </div>
          {overlapFlags.length > 0 ? (
            <div className="space-y-2">
              {overlapFlags.map((flag, i) => (
                <div
                  key={`overlap-${i}`}
                  className="flex items-start gap-3 py-2 border-b border-border-default/50 last:border-0"
                >
                  <span
                    className={`shrink-0 mt-0.5 inline-block px-2 py-0.5 rounded-full text-xs font-medium ${SEVERITY_STYLES[flag.severity]}`}
                  >
                    {SEVERITY_LABELS[flag.severity]}
                  </span>
                  <p className="text-sm text-text-secondary">{flag.message}</p>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-text-muted">
              No significant sector overlap detected among funds with known compositions.
            </p>
          )}
        </div>
      )}
    </Card>
  )
}
