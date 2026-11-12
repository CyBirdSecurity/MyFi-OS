/** Small monochrome mark used to indicate Plaid-connected data. Renders in `currentColor`. */
export function PlaidLogo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
      role="img"
      aria-label="Plaid"
    >
      <rect x="9.5" y="1"  width="5" height="9" rx="2.5" />
      <rect x="9.5" y="14" width="5" height="9" rx="2.5" />
      <rect x="1"  y="9.5" width="9" height="5" rx="2.5" />
      <rect x="14" y="9.5" width="9" height="5" rx="2.5" />
    </svg>
  )
}
