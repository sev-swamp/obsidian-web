const dbName = 'obsidianweb-sync'
const dbVersion = 1

export type SyncDB = IDBDatabase

export function openSyncDB(): Promise<SyncDB> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(dbName, dbVersion)
    request.onerror = () => reject(request.error)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains('notes')) db.createObjectStore('notes', { keyPath: 'path' })
      if (!db.objectStoreNames.contains('outbox')) db.createObjectStore('outbox', { keyPath: 'opId' })
      if (!db.objectStoreNames.contains('sync_state')) db.createObjectStore('sync_state', { keyPath: 'key' })
      if (!db.objectStoreNames.contains('applied_operations')) db.createObjectStore('applied_operations', { keyPath: 'opId' })
    }
    request.onsuccess = () => resolve(request.result)
  })
}

export function request<T>(value: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    value.onsuccess = () => resolve(value.result)
    value.onerror = () => reject(value.error)
  })
}

export function complete(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
    tx.onabort = () => reject(tx.error)
  })
}
