// Module-level handle to the live WebSocket so any component can send
// presence updates without prop drilling. The socket itself is managed
// by useVaultEvents.

let socket: WebSocket | null = null

export function setSocket(s: WebSocket | null) {
  socket = s
}

export type PresenceState = 'viewing' | 'editing' | 'left'

export function sendPresence(path: string, state: PresenceState) {
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'presence', path, state }))
  }
}
