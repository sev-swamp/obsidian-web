import { create } from 'zustand'

interface SearchState {
  open: boolean
  /** Query the dialog starts with when opened (e.g. a property filter). */
  initialQuery: string
  openSearch: (query?: string) => void
  toggle: () => void
  close: () => void
}

export const useSearchStore = create<SearchState>()((set) => ({
  open: false,
  initialQuery: '',
  openSearch: (query = '') => set({ open: true, initialQuery: query }),
  toggle: () => set((s) => ({ open: !s.open, initialQuery: '' })),
  close: () => set({ open: false }),
}))
