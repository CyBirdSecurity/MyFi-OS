import { create } from 'zustand'
import { mutate } from 'swr'
import type { User } from '../api/client'

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  setUser: (user: User | null) => void
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  isAuthenticated: false,
  setUser: (user) => {
    // Signing out, or switching to someone else in the same tab: drop every
    // cached response so the next person never sees (or saves against) the
    // previous user's data. The first sign-in after page load keeps the cache,
    // which only holds public data like /config at that point.
    const prev = get().user
    if (prev && prev.id !== user?.id) {
      mutate(() => true, undefined, { revalidate: false })
    }
    set({ user, isAuthenticated: user !== null })
  },
}))
