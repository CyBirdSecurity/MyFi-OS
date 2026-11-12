import { Link } from 'react-router-dom'
import { CheckCircle } from '@phosphor-icons/react'
import { Card } from '../ui/Card'
import { useConcentration } from '../../hooks/useConcentration'
import { SEVERITY_LABELS, SEVERITY_STYLES } from '../holdings/ConcentrationPanel'
import { flagSummary, sortFlags } from '../../utils/portfolioCheck'

/** How many flags to list before pointing to the Holdings page for the rest. */
const SHOWN = 3

export function PortfolioCheckupCard() {
  const { data, isLoading } = useConcentration()

  if (isLoading || !data) {
    return (
      <Card title="Portfolio Check-up">
        <div className="text-text-tertiary text-sm">Checking your holdings…</div>
      </Card>
    )
  }

  const flags = sortFlags(data.flags)

  return (
    <Card title="Portfolio Check-up">
      {flags.length === 0 ? (
        <div className="flex items-start gap-2 text-sm text-text-secondary">
          <CheckCircle size={18} weight="fill" className="text-positive shrink-0 mt-0.5" />
          <span>No concentration or allocation issues found across your holdings.</span>
        </div>
      ) : (
        <>
          <p className="text-xs text-text-tertiary mb-2">{flagSummary(flags)}</p>
          <div>
            {flags.slice(0, SHOWN).map((flag, i) => (
              <div
                key={`${flag.type}-${flag.ticker ?? flag.assetClass ?? i}-${i}`}
                className="flex items-start gap-3 py-2 border-b border-border-default/50 last:border-0"
              >
                <span
                  className={`shrink-0 mt-0.5 inline-block px-2 py-0.5 rounded-full text-xs font-medium ${SEVERITY_STYLES[flag.severity]}`}
                >
                  {SEVERITY_LABELS[flag.severity]}
                </span>
                <p className="text-sm text-text-secondary line-clamp-3">{flag.message}</p>
              </div>
            ))}
          </div>
          <Link to="/holdings" className="block mt-2 text-xs text-accent-text hover:text-accent">
            {flags.length > SHOWN ? `See all ${flags.length} on Holdings →` : 'Review on Holdings →'}
          </Link>
        </>
      )}
    </Card>
  )
}
