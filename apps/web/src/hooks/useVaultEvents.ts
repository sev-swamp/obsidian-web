import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type { VaultEvent } from '../api/types'

// Keeps the UI live: subscribes to vault events over WebSocket and
// invalidates affected queries so views refresh without a page reload.
export function useVaultEvents() {
  const queryClient = useQueryClient()

  useEffect(() => {
    let socket: WebSocket | null = null
    let closed = false
    let retry = 1000

    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      socket = new WebSocket(`${proto}://${location.host}/ws`)
      socket.onopen = () => {
        retry = 1000
      }
      socket.onmessage = (msg) => {
        let event: VaultEvent
        try {
          event = JSON.parse(msg.data as string) as VaultEvent
        } catch {
          return
        }
        switch (event.type) {
          case 'file.created':
          case 'file.deleted':
          case 'tree.changed':
            void queryClient.invalidateQueries({ queryKey: ['tree'] })
            void queryClient.invalidateQueries({ queryKey: ['recent'] })
            if (event.path) {
              void queryClient.invalidateQueries({ queryKey: ['note', event.path] })
            }
            break
          case 'file.changed':
            if (event.path) {
              void queryClient.invalidateQueries({ queryKey: ['note', event.path] })
            }
            void queryClient.invalidateQueries({ queryKey: ['recent'] })
            break
          case 'index.updated':
            void queryClient.invalidateQueries({ queryKey: ['search'] })
            break
        }
      }
      socket.onclose = () => {
        if (!closed) {
          setTimeout(connect, retry)
          retry = Math.min(retry * 2, 15000)
        }
      }
    }

    connect()
    return () => {
      closed = true
      socket?.close()
    }
  }, [queryClient])
}
