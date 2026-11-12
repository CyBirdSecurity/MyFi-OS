import useSWR from 'swr'
import { forecastApi } from '../api/client'
import type { ForecastResult } from '../api/client'
import { useProfile } from './useProfile'

export function useForecast() {
  // Projections need a date of birth; skip the request until one is set.
  const { data: profile } = useProfile()
  return useSWR<ForecastResult>(profile?.birthday ? '/forecast' : null, () => forecastApi.get(), {
    // Forecasts are expensive — don't revalidate on focus.
    revalidateOnFocus: false,
  })
}
