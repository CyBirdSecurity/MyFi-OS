import { Link } from 'react-router-dom'
import { useHoldings } from '../../hooks/useHoldings'
import { Card } from '../ui/Card'
import { formatPercent } from '../../utils/currency'

export function TopMoversCard() {
  const { data: holdings } = useHoldings()

  const movers = (holdings ?? [])
    .filter((h) => h.assetClass !== 'cash_equivalent' && h.costBasis && h.costBasis > 0 && h.shares > 0)
    .map((h) => {
      const costTotal = h.costBasis! * h.shares
      const gainPct = ((h.totalValue - costTotal) / costTotal) * 100
      return { holding: h, gainPct }
    })
    .sort((a, b) => Math.abs(b.gainPct) - Math.abs(a.gainPct))
    .slice(0, 3)

  return (
    <Card title="Top Movers" action={<Link to="/holdings" className="text-xs text-accent-text hover:text-accent">View Holdings →</Link>}>
      {movers.length === 0 ? (
        <div className="text-text-tertiary text-sm">No priced holdings yet.</div>
      ) : (
        <div className="space-y-2.5">
          {movers.map(({ holding, gainPct }) => (
            <div key={holding.id} className="flex items-center justify-between gap-3 text-sm">
              <div className="min-w-0 flex items-baseline gap-1.5">
                <span className="font-medium text-text-primary shrink-0">{holding.ticker}</span>
                {holding.name && <span className="text-text-tertiary truncate">{holding.name}</span>}
              </div>
              <span className={`font-medium shrink-0 ${gainPct >= 0 ? 'text-positive' : 'text-negative'}`}>
                {formatPercent(gainPct)}
              </span>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
