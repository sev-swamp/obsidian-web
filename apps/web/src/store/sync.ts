import { create } from 'zustand'
import type { SyncStatus } from '../sync/types'
import type { SyncConflict } from '../sync/engine'

type SyncState = { enabled: boolean; status: SyncStatus; conflict: SyncConflict | null; setEnabled: (enabled: boolean) => void; setStatus: (status: SyncStatus) => void; setConflict: (conflict: SyncConflict | null) => void }

export const useSyncStore = create<SyncState>()((set) => ({
  enabled: false,
  status: 'idle',
  conflict: null,
  setEnabled: (enabled) => set({ enabled }),
  setStatus: (status) => set({ status }),
  setConflict: (conflict) => set({ conflict }),
}))
