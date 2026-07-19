import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Personal editor preferences. They belong to the person, not the
// server, so they persist in this browser only (like theme and language).
interface PrefsState {
  lineNumbers: boolean
  openInEdit: boolean
  showProperties: boolean
  setLineNumbers: (v: boolean) => void
  setOpenInEdit: (v: boolean) => void
  setShowProperties: (v: boolean) => void
}

export const usePrefsStore = create<PrefsState>()(
  persist(
    (set) => ({
      lineNumbers: true,
      openInEdit: false,
      showProperties: true,
      setLineNumbers: (lineNumbers) => set({ lineNumbers }),
      setOpenInEdit: (openInEdit) => set({ openInEdit }),
      setShowProperties: (showProperties) => set({ showProperties }),
    }),
    { name: 'obsidianweb-prefs' },
  ),
)
