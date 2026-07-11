import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  token: string | null
  username: string | null
  unauthorized: boolean
  setSession: (token: string, username: string) => void
  setUnauthorized: () => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      username: null,
      unauthorized: false,
      setSession: (token, username) => set({ token, username, unauthorized: false }),
      setUnauthorized: () => set({ token: null, unauthorized: true }),
      logout: () => set({ token: null, username: null, unauthorized: false }),
    }),
    { name: 'obsidianweb-auth' },
  ),
)
