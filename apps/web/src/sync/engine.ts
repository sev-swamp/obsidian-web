import { ApiError, api } from '../api/client'
import { SyncRepository } from './repository'
import type { LocalNote, SyncOperation, SyncStatus } from './types'

export type SyncConflict = { opId: string; path: string; currentHash: string; currentContent: string }
type SyncConflictResponse = { opId?: string; path?: string; contentHash?: string; content?: string }

export class SyncEngine {
  private status: SyncStatus = 'idle'
  private listeners = new Set<(status: SyncStatus) => void>()
  private conflictListeners = new Set<(conflict: SyncConflict | null) => void>()
  private running = false
  private retryTimer: number | undefined

  constructor(private readonly repo: SyncRepository) {}

  onStatus(listener: (status: SyncStatus) => void): () => void {
    this.listeners.add(listener)
    listener(this.status)
    return () => this.listeners.delete(listener)
  }

  onConflict(listener: (conflict: SyncConflict | null) => void): () => void {
    this.conflictListeners.add(listener)
    return () => this.conflictListeners.delete(listener)
  }

  async save(path: string, content: string, baseHash?: string): Promise<void> {
    const deviceId = await this.repo.deviceId()
    const operation: SyncOperation = { opId: crypto.randomUUID(), deviceId, kind: 'upsert', path, content, baseHash, createdAt: Date.now() }
    await this.repo.saveLocal({ path, content, contentHash: baseHash, updatedAt: Date.now() } satisfies LocalNote, operation)
    this.setStatus('local')
    void this.sync()
  }

  async remove(path: string, baseHash?: string): Promise<void> {
    const deviceId = await this.repo.deviceId()
    const operation: SyncOperation = { opId: crypto.randomUUID(), deviceId, kind: 'delete', path, baseHash, createdAt: Date.now() }
    await this.repo.saveLocal({ path, content: '', deleted: true, updatedAt: Date.now() }, operation)
    this.setStatus('local')
    void this.sync()
  }

  async resolveConflict(opId: string): Promise<void> {
    await this.repo.discard(opId)
    this.publishConflict(null)
    void this.sync()
  }

  async sync(): Promise<void> {
    if (this.running) return
    if (!navigator.onLine) { this.setStatus('offline'); return }
    this.running = true
    this.setStatus('syncing')
    try {
      const deviceId = await this.repo.deviceId()
      const pending = await this.repo.pending()
      if (pending.length) {
        const result = await api.syncPush(deviceId, pending.map(({ opId, kind, path, content, baseHash }) => ({ opId, kind, path, content, baseHash })))
        await this.repo.acknowledge(result.acknowledged)
      }
      let more = true
      while (more) {
        const result = await api.syncPull(await this.repo.cursor(), 100, await this.repo.needsBootstrap())
        if (result.snapshot) await this.repo.applySnapshot(result.snapshot.notes, result.snapshot.baseCursor)
        await this.repo.applyRemote(result.changes.map((c) => ({ ...c, deviceId: c.deviceId ?? '', createdAt: Date.now() })), result.nextCursor)
        more = result.hasMore
      }
      this.setStatus('synced')
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        const body = error.body as SyncConflictResponse | undefined
        if (body?.opId && body.path && body.contentHash !== undefined && body.content !== undefined) {
          this.publishConflict({ opId: body.opId, path: body.path, currentHash: body.contentHash, currentContent: body.content })
        }
        this.setStatus('conflict')
      }
      else this.setStatus(navigator.onLine ? 'error' : 'offline')
      this.scheduleRetry()
    } finally { this.running = false }
  }

  wake(): void { void this.sync() }
  note(path: string) { return this.repo.note(path) }
  cacheNote(note: LocalNote) { return this.repo.cacheNote(note) }
  stop(): void { if (this.retryTimer !== undefined) window.clearTimeout(this.retryTimer) }

  private scheduleRetry(): void {
    if (this.retryTimer !== undefined) return
    this.retryTimer = window.setTimeout(() => { this.retryTimer = undefined; void this.sync() }, 2000 + Math.random() * 1000)
  }
  private publishConflict(conflict: SyncConflict | null): void { this.conflictListeners.forEach((listener) => listener(conflict)) }
  private setStatus(status: SyncStatus): void { this.status = status; this.listeners.forEach((listener) => listener(status)) }
}
