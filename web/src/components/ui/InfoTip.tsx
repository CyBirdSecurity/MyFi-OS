import { useEffect, useState } from 'react'
import { Info } from '@phosphor-icons/react'

// Plain-language definitions for financial terms, shared across pages.
export const GLOSSARY = {
  fiNumber:
    "The portfolio size that can pay for your retirement spending indefinitely: your annual retirement expenses divided by your safe withdrawal rate, in today's dollars.",
  coastFi:
    "The amount you'd need invested today to reach your FI Number by retirement age with no further contributions, just growth.",
  withdrawalRate:
    'The share of your portfolio you withdraw each year in retirement. 4% is a common rule of thumb for a portfolio meant to last 30+ years.',
  readiness:
    'A 0–100 score of how well your current savings, contributions, and timeline cover your retirement spending.',
  taxTreatment:
    'Taxable accounts are taxed on gains and dividends. Tax-deferred accounts (401(k), traditional IRA) are taxed when withdrawn. Tax-free accounts (Roth) are never taxed on qualified withdrawals.',
  drift:
    "How far an asset class's actual share of your portfolio is from your target. Large drift suggests rebalancing.",
  rmd:
    'Required Minimum Distributions: the amount the IRS requires you to withdraw from tax-deferred accounts each year starting at age 73.',
  rothConversion:
    'Moving money from a tax-deferred account to a Roth account. You pay income tax now so future growth and withdrawals are tax-free.',
  fundSize:
    'Company size: large-cap funds hold the biggest companies (like the S&P 500), small-cap funds hold smaller ones, which tend to swing more. Total-market funds hold all sizes, mostly large.',
  aca:
    'Affordable Care Act marketplace health insurance. Subsidies depend on your MAGI (modified adjusted gross income), which matters if you retire before Medicare at 65.',
} as const

export type GlossaryTerm = keyof typeof GLOSSARY

const TIP_WIDTH = 256

/** An ⓘ icon that shows a term's definition on hover, focus, or tap. */
export function InfoTip({ term }: { term: GlossaryTerm }) {
  // Fixed position, clamped to the viewport so tips near a screen edge stay visible.
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)
  const show = (el: HTMLElement) => {
    const r = el.getBoundingClientRect()
    const left = Math.min(Math.max(8, r.left + r.width / 2 - TIP_WIDTH / 2), window.innerWidth - TIP_WIDTH - 8)
    setPos({ left, top: r.bottom + 6 })
  }
  const hide = () => setPos(null)

  // A fixed tip would drift away from its icon on scroll, so close it instead.
  useEffect(() => {
    if (!pos) return
    window.addEventListener('scroll', hide, true)
    return () => window.removeEventListener('scroll', hide, true)
  }, [pos])

  return (
    <span className="inline-flex align-middle normal-case tracking-normal font-normal">
      <button
        type="button"
        aria-label="What does this mean?"
        // Taps fire mouseenter then click, so click must open rather than toggle.
        onClick={(e) => { e.preventDefault(); e.stopPropagation(); show(e.currentTarget) }}
        onMouseEnter={(e) => show(e.currentTarget)}
        onMouseLeave={hide}
        onFocus={(e) => show(e.currentTarget)}
        onBlur={hide}
        className="text-text-muted hover:text-accent-text transition-colors"
      >
        <Info size={14} weight="bold" />
      </button>
      {pos && (
        <span
          role="tooltip"
          style={{ left: pos.left, top: pos.top, width: TIP_WIDTH }}
          className="fixed z-50 rounded-lg border border-border-default bg-surface px-3 py-2 text-left text-xs leading-relaxed text-text-secondary shadow-float"
        >
          {GLOSSARY[term]}
        </span>
      )}
    </span>
  )
}
