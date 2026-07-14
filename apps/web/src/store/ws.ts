import { create } from 'zustand'

interface WsStore {
  connectionId: number
  bump: () => void
}

// Incremented on every successful WS connect so components can re-announce
// state (e.g. presence) that the server loses when the socket drops.
export const useWsStore = create<WsStore>()((set) => ({
  connectionId: 0,
  bump: () => set((s) => ({ connectionId: s.connectionId + 1 })),
}))
