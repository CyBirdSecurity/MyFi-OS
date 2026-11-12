import useSWR from 'swr'
import { forecastApi } from '../api/client'
import type { ScenarioParams, ScenariosResult } from '../api/client'

/**
 * Runs up to 3 what-if scenarios against the user's actual forecast data.
 * The SWR key is the JSON-serialised params array so results auto-refresh
 * whenever any scenario parameter changes.
 */
export function useScenarios(params: ScenarioParams[]) {
  const key = params.length > 0 ? `/forecast/scenarios:${JSON.stringify(params)}` : null

  return useSWR<ScenariosResult>(
    key,
    () => forecastApi.scenarios(params),
    { revalidateOnFocus: false },
  )
}
