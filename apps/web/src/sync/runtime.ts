import { SyncEngine } from './engine'
import { SyncRepository } from './repository'
import { useSyncStore } from '../store/sync'

let engine: SyncEngine | null = null
let opening: Promise<SyncEngine> | null = null

export async function startSync(): Promise<SyncEngine> {
  if (engine) return engine
  opening ??= SyncRepository.open().then((repo) => {
    engine = new SyncEngine(repo)
    useSyncStore.getState().setEnabled(true)
    engine.onStatus((status) => useSyncStore.getState().setStatus(status))
    engine.onConflict((conflict) => useSyncStore.getState().setConflict(conflict))
    void engine.sync()
    return engine
  })
  return opening
}

export function syncEngine(): SyncEngine | null { return engine }

export function wakeSync(): void { engine?.wake() }
export function cachedSyncNote(path: string) { return engine?.note(path) }
export function cacheSyncNote(note: Parameters<SyncEngine['cacheNote']>[0]) { return engine?.cacheNote(note) }
