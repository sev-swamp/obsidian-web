import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type { VaultEvent } from '../api/types'
import { useAuthStore } from '../store/auth'
import { usePresenceStore } from '../store/presence'
import { useWsStore } from '../store/ws'
import { setSocket } from '../ws'

interface PresenceEvent {
  type: 'presence.changed'
  path: string
  viewers: string[]
  editors: string[]
}

// Keeps the UI live: subscribes to vault events over WebSocket and
// invalidates affected queries so views refresh without a page reload.
// Also feeds the presence store (who is viewing/editing which note).
export function useVaultEvents() {
  const queryClient = useQueryClient()
  const token = useAuthStore((s) => s.token)
  const updatePresence = usePresenceStore((s) => s.update)
  const bumpConnection = useWsStore((s) => s.bump)

  useEffect(() => {
    let socket: WebSocket | null = null
    let closed = false
    let retry = 1000

    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const tokenQuery = token ? `?token=${encodeURIComponent(token)}` : ''
      socket = new WebSocket(`${proto}://${location.host}/ws${tokenQuery}`)
      socket.onopen = () => {
        retry = 1000
        setSocket(socket)
        bumpConnection()
      }
      socket.onmessage = (msg) => {
        let event: VaultEvent | PresenceEvent
        try {
          event = JSON.parse(msg.data as string) as VaultEvent | PresenceEvent
        } catch {
          return
        }
        switch (event.type) {
          case 'presence.changed':
            updatePresence(event.path, { viewers: event.viewers, editors: event.editors })
            break
          case 'file.created':
          case 'file.deleted':
          case 'tree.changed':
            void queryClient.invalidateQueries({ queryKey: ['tree'] })
            void queryClient.invalidateQueries({ queryKey: ['recent'] })
            void queryClient.invalidateQueries({ queryKey: ['trash'] })
            if (event.path) {
              void queryClient.invalidateQueries({ queryKey: ['note', event.path] })
            }
            break
          case 'file.changed':
            if (event.path) {
              void queryClient.invalidateQueries({ queryKey: ['note', event.path] })
              void queryClient.invalidateQueries({ queryKey: ['history', event.path] })
            }
            void queryClient.invalidateQueries({ queryKey: ['recent'] })
            break
          case 'index.updated':
            void queryClient.invalidateQueries({ queryKey: ['search'] })
            break
          case 'plugin.changed':
            void queryClient.invalidateQueries({ queryKey: ['plugins'] })
            break
        }
      }
      socket.onclose = () => {
        setSocket(null)
        if (!closed) {
          setTimeout(connect, retry)
          retry = Math.min(retry * 2, 15000)
        }
      }
    }

    connect()
    return () => {
      closed = true
      setSocket(null)
      socket?.close()
    }
  }, [queryClient, token, updatePresence, bumpConnection])
}
