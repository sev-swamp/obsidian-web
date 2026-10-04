export type SyncKind = 'upsert' | 'delete'

export type SyncOperation = {
  opId: string
  deviceId: string
  kind: SyncKind
  path: string
  content?: string
  baseHash?: string
  createdAt: number
}

export type LocalNote = {
  path: string
  content: string
  contentHash?: string
  deleted?: boolean
  updatedAt: number
}

export type SyncStatus = 'idle' | 'local' | 'syncing' | 'synced' | 'offline' | 'conflict' | 'error'
