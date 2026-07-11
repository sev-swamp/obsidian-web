import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type Theme = 'light' | 'dark'

interface ThemeState {
  theme: Theme
  toggle: () => void
}

function apply(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

const initial: Theme = window.matchMedia('(prefers-color-scheme: dark)').matches
  ? 'dark'
  : 'light'

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      theme: initial,
      toggle: () => {
        const next: Theme = get().theme === 'dark' ? 'light' : 'dark'
        apply(next)
        set({ theme: next })
      },
    }),
    {
      name: 'obsidianweb-theme',
      onRehydrateStorage: () => (state) => {
        apply(state?.theme ?? initial)
      },
    },
  ),
)

apply(useThemeStore.getState().theme)
