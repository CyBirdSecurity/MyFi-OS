import useSWR from 'swr'
import { contributionsApi, contributionEntriesApi } from '../api/client'
import type { Contribution, ContributionEntry, ContributionProgress } from '../api/client'

export function useContributions(accountId: string) {
  return useSWR<Contribution[]>(
    accountId ? `/accounts/${accountId}/contributions` : null,
    () => contributionsApi.listByAccount(accountId),
  )
}

export function useContributionProgress() {
  return useSWR<ContributionProgress[]>('/contribution-progress', () => contributionEntriesApi.progress())
}

export function useContributionEntries(accountId: string) {
  return useSWR<ContributionEntry[] | null>(
    accountId ? `/accounts/${accountId}/contribution-entries` : null,
    () => contributionEntriesApi.list(accountId),
  )
}
