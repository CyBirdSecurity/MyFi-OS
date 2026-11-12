import useSWR from 'swr'
import { householdMembersApi } from '../api/client'
import type { HouseholdMember } from '../api/client'

export function useHouseholdMembers() {
  return useSWR<HouseholdMember[]>('/household/members', () => householdMembersApi.list())
}
