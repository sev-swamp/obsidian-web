import { complete, openSyncDB, request } from './db'
import type { LocalNote, SyncOperation } from './types'

const stateKey = 'state'

export class SyncRepository {
  private constructor(private readonly db: IDBDatabase) {}

  static async open(): Promise<SyncRepository> { return new SyncRepository(await openSyncDB()) }

  async deviceId(): Promise<string> {
    const state = await this.state()
    if (typeof state.deviceId === 'string' && state.deviceId) return state.deviceId
    const deviceId = crypto.randomUUID()
    await this.putState({ ...state, deviceId })
    return deviceId
  }

  async saveLocal(note: LocalNote, operation: SyncOperation): Promise<void> {
    const tx = this.db.transaction(['notes', 'outbox'], 'readwrite')
    tx.objectStore('notes').put(note)
    tx.objectStore('outbox').put(operation)
    await complete(tx)
  }

  async pending(limit = 100): Promise<SyncOperation[]> {
    const tx = this.db.transaction('outbox', 'readonly')
    const all = await request(tx.objectStore('outbox').getAll()) as SyncOperation[]
    await complete(tx)
    return all.sort((a, b) => a.createdAt - b.createdAt).slice(0, limit)
  }

  async acknowledge(opIds: string[]): Promise<void> {
    const tx = this.db.transaction('outbox', 'readwrite')
    const store = tx.objectStore('outbox')
    opIds.forEach((id) => store.delete(id))
    await complete(tx)
  }

  async discard(opId: string): Promise<void> {
    const tx = this.db.transaction('outbox', 'readwrite')
    tx.objectStore('outbox').delete(opId)
    await complete(tx)
  }

  async cursor(): Promise<number> { return Number((await this.state()).cursor ?? 0) }
  async needsBootstrap(): Promise<boolean> { return (await this.state()).bootstrapped !== true }

  async note(path: string): Promise<LocalNote | undefined> {
    const tx = this.db.transaction('notes', 'readonly')
    const value = await request(tx.objectStore('notes').get(path)) as LocalNote | undefined
    await complete(tx)
    return value?.deleted ? undefined : value
  }

  async cacheNote(note: LocalNote): Promise<void> {
    const tx = this.db.transaction('notes', 'readwrite')
    tx.objectStore('notes').put(note)
    await complete(tx)
  }

  async applyRemote(changes: Array<SyncOperation & { accountSeq: number }>, cursor: number): Promise<void> {
    const seenTx = this.db.transaction('applied_operations', 'readonly')
    const seen = new Set(await request(seenTx.objectStore('applied_operations').getAllKeys()) as string[])
    await complete(seenTx)
    const state = await this.state()
    const tx = this.db.transaction(['notes', 'sync_state', 'applied_operations'], 'readwrite')
    const notes = tx.objectStore('notes')
    const applied = tx.objectStore('applied_operations')
    for (const change of changes) {
      if (!seen.has(change.opId)) {
        if (change.kind === 'delete') notes.put({ path: change.path, content: '', deleted: true, updatedAt: Date.now() } satisfies LocalNote)
        else notes.put({ path: change.path, content: change.content ?? '', updatedAt: Date.now() } satisfies LocalNote)
        applied.put({ opId: change.opId, accountSeq: change.accountSeq })
      }
    }
    tx.objectStore('sync_state').put({ key: stateKey, ...state, cursor, bootstrapped: true })
    await complete(tx)
  }

  async applySnapshot(notes: Record<string, string>, cursor: number): Promise<void> {
    const state = await this.state()
    const tx = this.db.transaction(['notes', 'sync_state', 'applied_operations'], 'readwrite')
    tx.objectStore('notes').clear()
    for (const [path, content] of Object.entries(notes)) tx.objectStore('notes').put({ path, content, updatedAt: Date.now() } satisfies LocalNote)
    tx.objectStore('applied_operations').clear()
    tx.objectStore('sync_state').put({ key: stateKey, ...state, cursor, bootstrapped: true })
    await complete(tx)
  }

  private async state(): Promise<Record<string, unknown>> {
    const tx = this.db.transaction('sync_state', 'readonly')
    const value = await request(tx.objectStore('sync_state').get(stateKey)) as Record<string, unknown> | undefined
    await complete(tx)
    return value ?? { key: stateKey, cursor: 0 }
  }

  private async putState(state: Record<string, unknown>): Promise<void> {
    const tx = this.db.transaction('sync_state', 'readwrite')
    tx.objectStore('sync_state').put({ key: stateKey, ...state })
    await complete(tx)
  }
}
