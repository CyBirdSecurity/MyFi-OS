import useSWR from 'swr'
import { profileApi } from '../api/client'
import type { UserProfile } from '../api/client'

export function useProfile() {
  return useSWR<UserProfile>('/profile', () => profileApi.get())
}
