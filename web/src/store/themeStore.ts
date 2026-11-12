import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type Theme = 'light' | 'dark'

interface ThemeState {
  theme: Theme
  setTheme: (t: Theme) => void
  toggle: () => void
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      theme: 'light',
      setTheme: (theme) => {
        set({ theme })
        applyThemeToDOM(theme)
      },
      toggle: () => {
        const next: Theme = get().theme === 'light' ? 'dark' : 'light'
        set({ theme: next })
        applyThemeToDOM(next)
      },
    }),
    {
      name: 'myfi-theme',
      onRehydrateStorage: () => (state) => {
        if (state) applyThemeToDOM(state.theme)
      },
    }
  )
)

function applyThemeToDOM(theme: Theme) {
  if (theme === 'dark') document.documentElement.classList.add('dark')
  else document.documentElement.classList.remove('dark')
}
