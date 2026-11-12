import { describe, expect, it } from 'vitest'
import { holdingLabel, isPlaceholderTicker } from './holdingLabel'

describe('holdingLabel', () => {
  it('uses the saved name for a made-up ticker', () => {
    expect(holdingLabel('PLAID-RJ6M41MY', 'Target Date 2050 Trust')).toBe('Target Date 2050 Trust')
    expect(holdingLabel('31617E471', 'Stable Value Fund')).toBe('Stable Value Fund')
  })

  it('keeps real tickers, and a stand-in with no name', () => {
    expect(holdingLabel('VTI', 'Vanguard Total Stock Market ETF')).toBe('VTI')
    expect(holdingLabel('BRK.B', 'Berkshire')).toBe('BRK.B')
    expect(holdingLabel('PLAID-RJ6M41MY', undefined)).toBe('PLAID-RJ6M41MY')
  })

  it('does not mistake real symbols for stand-ins', () => {
    expect(isPlaceholderTicker('GOOGL')).toBe(false)
    expect(isPlaceholderTicker('BTC-USD')).toBe(false)
    expect(isPlaceholderTicker('ABCDEFGHI')).toBe(false)
  })
})
