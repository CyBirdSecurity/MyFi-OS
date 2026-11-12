/**
 * Whether a ticker was made up rather than being a real trading symbol.
 * Securities with no symbol (common for 401(k) collective trusts) are stored
 * under their CUSIP or a "PLAID-XXXXXXXX" id, which means nothing to the user.
 * Kept in step with isPlaceholderTicker in internal/services/concentration.go.
 */
export function isPlaceholderTicker(ticker: string): boolean {
  return ticker.startsWith('PLAID-') || (/^[0-9A-Z]{9}$/.test(ticker) && /\d/.test(ticker))
}

/** How to name a holding: its ticker, or its saved name when the ticker is a stand-in. */
export function holdingLabel(ticker: string, name?: string | null): string {
  return name?.trim() && isPlaceholderTicker(ticker) ? name.trim() : ticker
}
