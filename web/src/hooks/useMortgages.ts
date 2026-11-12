import useSWR from 'swr'
import { homeApi } from '../api/client'
import { useAccounts } from './useAccounts'
import { mortgageFromDetails, type Mortgage } from '../utils/debt'

/** The mortgage on each home account that has loan details. */
export function useMortgages() {
  const { data: accounts } = useAccounts()
  const homes = (accounts ?? []).filter((a) => a.accountType === 'home')

  return useSWR<Mortgage[]>(
    homes.length > 0 ? ['mortgages', ...homes.map((h) => h.id)] : null,
    async () => {
      // A home with no details yet answers 404; that just means no mortgage.
      const all = await Promise.all(homes.map((h) => homeApi.getDetails(h.id).catch(() => null)))
      return homes.flatMap((h, i) => mortgageFromDetails(h.id, h.name, all[i]) ?? [])
    },
  )
}
