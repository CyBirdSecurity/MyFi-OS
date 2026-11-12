import useSWR from 'swr'
import { forecastApi } from '../api/client'
import type { BacktestResult } from '../api/client'

// enabled gates the fetch behind a null SWR key so the heavier historical run
// only fires once the panel is actually opened, not on every dashboard load.
export function useBacktest(enabled: boolean) {
  return useSWR<BacktestResult>(enabled ? 'forecast/backtest' : null, () => forecastApi.backtest(), {
    revalidateOnFocus: false,
  })
}
