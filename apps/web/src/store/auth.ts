import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Permission identifiers mirror the backend (packages/auth).
export type Permission =
  | 'notes:read'
  | 'notes:edit'
  | 'notes:delete'
  | 'history:read'
  | 'files:upload'
  | 'settings:write'
  | 'trash:read'
  | 'trash:purge'

interface AuthState {
  token: string | null
  username: string | null
  role: string | null
  permissions: Permission[] | null
  unauthorized: boolean
  setSession: (
    token: string,
    username: string,
    role: string,
    permissions: Permission[],
  ) => void
  setUnauthorized: () => void
  logout: () => void
  /** True when the action is allowed: either auth is off (no session
   *  needed) or the JWT the user logged in with grants the permission. */
  can: (perm: Permission) => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      username: null,
      role: null,
      permissions: null,
      unauthorized: false,
      setSession: (token, username, role, permissions) =>
        set({ token, username, role, permissions, unauthorized: false }),
      setUnauthorized: () =>
        set({ token: null, role: null, permissions: null, unauthorized: true }),
      logout: () =>
        set({
          token: null,
          username: null,
          role: null,
          permissions: null,
          unauthorized: false,
        }),
      can: (perm) => {
        const { token, permissions } = get()
        if (!token) return true // auth disabled: server accepts everything
        return permissions?.includes(perm) ?? false
      },
    }),
    { name: 'obsidianweb-auth' },
  ),
)
