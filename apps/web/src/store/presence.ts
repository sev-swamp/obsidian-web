import { create } from 'zustand'

export interface NotePresence {
  viewers: string[]
  editors: string[]
}

interface PresenceStore {
  byPath: Record<string, NotePresence>
  update: (path: string, presence: NotePresence) => void
}

export const usePresenceStore = create<PresenceStore>()((set) => ({
  byPath: {},
  update: (path, presence) =>
    set((s) => ({ byPath: { ...s.byPath, [path]: presence } })),
}))
