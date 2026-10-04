# ADR-0006: SQLite sync journal beside the vault with controlled-write recovery

Status: proposed (2026-10)

## Context

Plan 08 adds offline-first replication while preserving the existing
vault-first model: Markdown files remain the materialised note state and the
filesystem watcher must still report changes made outside Obsidian Web.
The server therefore needs a durable, ordered operation journal independent
of the vault and of git history.

The implementation was examined before adding production code:

- `filesystem.Vault.Write` uses `os.WriteFile`, and `Delete` uses
  `os.Remove` (`packages/filesystem/vault.go`). Neither operation records its
  origin.
- `filesystem.Watcher` maps every `fsnotify` create/write/remove/rename event
  directly to its callback (`packages/filesystem/watcher.go`). It cannot tell a
  platform write from an external editor write.
- `NoteService.SaveNote`/`DeleteNote` emit an in-process event, but their file
  writes also reach `NoteService.HandleFSEvent`, which labels them
  `ActorExternal` (`packages/core/service.go`).
- `NoteService` has a per-path lock, optimistic content hashes and ACL checks
  in API handlers. There is no move operation today. Git history is optional
  and must not be the sync journal.

Consequently the current watcher cannot reliably distinguish a sync write from
an external filesystem change. Adding a journal or sync endpoint before an
origin mechanism would create a second `source: filesystem` change for one
sync operation and violate the no-duplicates invariant.

## Decision

This ADR is deliberately limited to the design prerequisite (stage 0 of Plan
08). No runtime configuration, database, API, or watcher behaviour is changed
by this ADR. Stages 1--3 may start only after the controlled-write mechanism
below is reviewed and implemented first.

### Journal location and configuration

Add a `sync` configuration section in stage 1:

```yaml
sync:
  enabled: false
  dataFile: "sync.db"
```

`dataFile` is an absolute path or a path resolved relative to the configuration
file. Startup must reject a path equal to, or contained by, `vault.path` after
both paths are made absolute and cleaned. It must also create the parent
directory with the least permissive practical mode. The Docker example will
set `sync.dataFile: /data/sync.db`; `/data` is already the writable persistent
mount that holds `users.yaml` and `runtime.yaml`. The vault and its `.git`
directory never contain sync metadata, intents, or SQLite side files
(`-wal`/`-shm`).

Use `modernc.org/sqlite`, a pure-Go SQLite driver compatible with the existing
`CGO_ENABLED=0` production build. The sync adapter owns migrations and opens
the database with one write connection (`SetMaxOpenConns(1)`) plus a process
writer mutex. Every mutation described below is one SQLite transaction; there
is no network database in this MVP.

The initial schema is:

```text
sync_operations(op_id TEXT PRIMARY KEY, actor TEXT NOT NULL,
  device_id TEXT NOT NULL, path TEXT NOT NULL, kind TEXT NOT NULL,
  payload JSON NOT NULL, base_hash TEXT, account_seq INTEGER UNIQUE NOT NULL,
  created_at TIMESTAMP NOT NULL)
sync_outbox(account_seq INTEGER PRIMARY KEY, event JSON NOT NULL,
  published_at TIMESTAMP NULL)
sync_devices(actor TEXT NOT NULL, device_id TEXT NOT NULL,
  last_cursor INTEGER NOT NULL, last_seen_at TIMESTAMP NOT NULL,
  PRIMARY KEY(actor, device_id))
sync_snapshots(base_cursor INTEGER PRIMARY KEY, state BLOB NOT NULL,
  checksum TEXT NOT NULL, note_count INTEGER NOT NULL, created_at TIMESTAMP NOT NULL)
sync_intents(op_id TEXT PRIMARY KEY, actor TEXT NOT NULL,
  device_id TEXT NOT NULL, path TEXT NOT NULL, kind TEXT NOT NULL,
  operation JSON NOT NULL, before_state JSON NOT NULL, after_state JSON NOT NULL,
  state TEXT NOT NULL, created_at TIMESTAMP NOT NULL, failure TEXT NULL)
```

`sync_operations` is the authoritative append-only history used by `pull`.
For an `upsert` its `payload` necessarily contains the Markdown content (or an
equivalent immutable content blob): a device that was offline must be able to
rebuild the changed note. It is not a second working copy of the whole vault;
it is a sequence of accepted changes. `sync_outbox` is normally tiny and can
be removed after publication because `sync_operations` remains the pull
source. `sync_devices` stores only a cursor and timestamp per device. Intents
exist only around an in-flight filesystem mutation and are removed when it is
finalised or explicitly marked as failed.

This makes journal growth proportional to the total bytes saved, not to the
current vault size. For example, 100 accepted full-note saves per day with an
average 10 KiB payload add roughly 1 MB/day (about 365 MB/year), before SQLite
indexes and WAL overhead; at 100 KiB per save it is roughly 3.65 GB/year.
Editor debounce and the 1 MiB push limit bound individual requests but do not
bound total growth. The SQLite WAL is transient and is checkpointed; it is not
an additional permanent copy of that size.

Acknowledged operations are never deleted merely because they are old. An old
device can have an earlier cursor, so deleting its missing operations would
make a normal `pull` incomplete. A 30-day tombstone only limits when a deleted
note may be physically purged; it does not make its historical operation safe
to remove. Instead, this plan includes snapshot/bootstrap and compaction before
sync is enabled by default.

A **snapshot** is a durable, versioned image of the materialised sync state at
one `base_cursor`: every live note's canonical path, content hash and content,
plus unexpired delete tombstones. It is stored as a checksummed (and, when
beneficial, compressed) `sync_snapshots.state` blob. It is not a filesystem
backup and it is not a client cache. Enabling sync first creates a cursor-zero
snapshot from the current vault under a startup write barrier. Later snapshots
are built from the preceding verified snapshot plus the ordered operations up
to their high-water `base_cursor`; concurrent later operations remain in the
journal. A device whose cursor predates `base_cursor` receives a paged
bootstrap response containing that snapshot, applies it atomically to its
local store, sets its cursor to `base_cursor`, then pulls normal operations
after that cursor. Thus an offline device remains recoverable even after old
operation rows have gone.

**Compaction** creates and verifies such a snapshot first, records its checksum
and `base_cursor` transactionally, and only then deletes operation/outbox rows
at or before that cursor. It never removes active operations or unexpired
tombstones. A failed or interrupted compaction leaves the old journal intact;
the new snapshot is used only after its checksum and row counts verify. The
initial implementation may keep one previous verified snapshot as rollback
until the next compaction succeeds. This turns storage from unbounded edit
history into approximately: one current vault-sized snapshot, recent
operations since its cursor, and the configured tombstone window.

For example, a 500 MB vault with 30 days of 1 MB/day edits normally needs about
530 MB plus SQLite indexes and a temporary second snapshot during compaction,
rather than retaining every historical edit forever. The exact size still
depends on Markdown compressibility and the retention interval. Before enabling
sync by default, the server must expose configured warning and hard thresholds.
At the hard threshold it stops accepting new sync writes with a clear service
error, while reads, recovery and downloading a backup remain available; it
must never silently drop acknowledged history. `/data/sync.db` (including WAL
while active) must be included in normal backups.

`account_seq` is one strictly increasing, server-assigned sequence for the
current single-vault deployment. It is never derived from a client clock. Paths
are canonical vault-relative slash paths: cleaned, without a leading slash or
`..`, and Markdown note paths use the existing `core.NormalizeNotePath` rule.
The adapter must centralise this validation rather than accept arbitrary paths
that merely happen to pass `Vault.AbsPath`.

### Controlled writes and watcher origin

Before writing a synced note, the sync use case must acquire the same path lock
as `NoteService`, then write a durable intent containing the complete operation,
the previous state (existence and SHA-256) and the desired state (existence and
SHA-256). A new filesystem controlled-writer API must:

1. register an in-memory suppression token with the watcher for the canonical
   path and the expected post-write hash (or expected absence for a delete);
2. atomically write a temporary file in the target directory, `fsync` it,
   rename it into place, and `fsync` the directory; deletes must likewise be
   represented by an expected absence;
3. let the watcher suppress only notifications whose observed on-disk state
   matches that exact token, and consume/expire the token after a bounded
   interval.

The watcher holds suppression entries as `{path, expected state, expiresAt}`.
On every fsnotify event it reads the observable state for that path (present
with SHA-256, or absent) before deciding whether to invoke the external-change
callback:

1. no matching entry means an ordinary external event;
2. matching path **and** expected state consumes or retains the entry until the
   platform's duplicate event window has elapsed, then suppresses the callback;
3. matching path but a different state removes the entry and emits an external
   event, because another writer won the race;
4. an expired entry is removed and cannot hide a later external change.

The implementation must coalesce the create/write/rename burst from one
atomic replacement into one suppression decision. It must never suppress by
path and time alone: an external write at the same path can otherwise be
hidden. Events not matching the expected state stay external and generate one
filesystem journal change. The controlled writer invokes the normal
`NoteService` indexing and domain-event path exactly once after its durable
file operation; the watcher is not used as the acknowledgement path. If a
process dies, the in-memory entries disappear; startup recovery runs before
the watcher begins and finalises the intent, so no stale token can conceal an
external event. Unit tests need to cover the platform events emitted as create,
write, rename, or more than once, as well as an external overwrite racing a
token.

This is the minimum viable origin marker: it is in-process, state-verified and
does not add sentinel files to user vaults. It cannot identify writes made by
another process with identical bytes; those are semantically indistinguishable
from the already-materialised state and must not create a second change.

### Crash recovery and acknowledgement

SQLite and a Markdown filesystem cannot share one atomic transaction. The
server's sequence is therefore:

1. validate ACL and `baseHash`, acquire the path lock, and persist the intent;
2. perform the durable controlled write/delete;
3. in one SQLite transaction insert `sync_operations`, allocate
   `account_seq`, insert `sync_outbox`, and remove the intent;
4. only then return the operation in `acknowledged`.

On startup, before the watcher starts, recovery reads all non-final intents.
If the actual state equals the intent's desired state, it completes step 3
idempotently. If it equals the recorded previous state, it removes the intent;
the client has no acknowledgement and safely retries its `op_id`. Any other
state is retained as a failed intent, logged without note content, and makes a
retry of that `op_id` return a clear retriable `sync_recovery_required` error
until an operator resolves it. It must never manufacture an acknowledgement.
Recovery tests must crash after each step and prove the server ends in either
the old state or the new journalled state.

### Failed-intent resolution

A failed intent does not make the whole vault unavailable: reads and unrelated
paths continue to work. The sync admin page lists it only to an administrator,
with the path, operation id, actor/device, timestamps, expected/actual hashes
and no note body by default. It offers three deliberate resolutions:

1. **Keep current file** marks the intent resolved-as-conflict. The original
   device's retry receives the normal conflict response with the current server
   version; its durable local outbox entry is still intact, so the user chooses
   whether to keep, replace or copy their edit.
2. **Retry original operation** is enabled only after the current on-disk state
   again equals the intent's recorded previous state and ACL/base-hash checks
   still pass. It runs the normal controlled-write sequence; it never retries
   over a third-party edit.
3. **Save intended version as conflict copy** writes the persisted intent body
   to a generated `Conflicts/` path through a new controlled operation, then
   resolves the original as a conflict. This preserves both versions without
   overwriting either one.

Every resolution is journalled and audited with the acting administrator. It
requires an explicit confirmation; there is no automatic "force" action.

### Administration, warnings, and backup

The admin UI gains a Sync Health page, guarded by the existing settings/admin
permission. It polls a read-only health endpoint and shows database size,
snapshot cursor/time, oldest retained operation, active-device cursors,
unpublished outbox count, failed-intent count, the last backup, and configured
warning/hard limits. At the warning threshold the page shows a persistent
warning badge and notification; at the hard threshold it shows that new sync
writes are paused. This product has no mail/alerting integration today, so the
first release intentionally uses the admin UI and server logs; external alerts
can be added later from the same metrics.

**Download backup** is an admin-only button. The server first makes an
application-consistent SQLite online backup to a server-generated temporary
file (never a path supplied by the browser), verifies its SQLite integrity and
checksum, then streams it as a download and removes the temporary file. The
administrator stores that downloaded file wherever their normal backup policy
requires; no shell access or manual copying from `/data` is needed. The audit
log records that a backup was created, but never its contents.

Restore is intentionally not a one-click overwrite in the first release:
replacing a journal while the vault has newer files can orphan acknowledged
operations. The Sync Health page will explain this and offer validation of an
uploaded backup as a future maintenance-mode workflow. A restore design must
pause sync, verify the backup schema/checksum, reconcile it against the vault,
and require a second admin confirmation before it may replace live data.

The publisher reads unpublished rows from `sync_outbox` after commit and sends
only a cursor wake-up signal to WebSocket clients. Marking `published_at` is
best effort: duplicate signals are allowed because every client pulls by its
cursor. A restart republishes rows that were committed but not marked.

External watcher changes become one journalled `source: filesystem` operation
after watcher debounce. Sync-controlled events are suppressed as above. A
30-day tombstone retention is recorded in the operation payload; physical purge
remains Plan 3 work.

## Consequences

- The database is backed up and monitored alongside `/data`, separately from
  the vault/git repository.
- The sync core port owns operation validation and journal semantics; SQLite,
  filesystem and WebSocket stay adapters. HTTP handlers remain transport-only.
- ACL filtering happens both in push and pull. Pull advances the global cursor
  across inaccessible operations without returning their path, content or
  count.
- The journal has an explicit recovery failure state, which is safer than
  silently overwriting a third-party filesystem edit and has a deliberate
  administrator resolution workflow.
- Storage remains bounded without abandoning offline devices: compaction keeps
  a verified bootstrap snapshot before it removes covered operations.

## Decisions and open work before stage 1

1. The recovery rule is accepted: when startup finds a state different from
   both the recorded previous and desired states, retain a failed intent and
   return a clear error; do not auto-complete or overwrite that external state.
2. Rename/move is a required future sync operation, not an optional feature to
   forget. It is deliberately excluded from the first upsert/delete slice
   because the current `VaultFS` and `NoteService` have no rename capability.
   Before enabling it, add a dedicated move design and tests covering source
   and destination path locks, read/write ACL checks on both paths, atomic
   filesystem replacement semantics, a two-path recovery intent, tombstones,
   and pending local outbox operations. Until then the client must not expose
   offline rename/move as a syncable action.
3. Review and implement the controlled-writer/watcher suppression interface
   before adding `packages/sync`; its exact ownership (`filesystem` versus a
   small core port) needs code review, but its state-verified contract above is
   required.
4. Implement snapshot/bootstrap, verified compaction, Sync Health thresholds
   and download backup before the feature flag is enabled by default.
5. Confirm whether Plan 9 history commits are required for sync operations;
   until then, history remains independent and must not be used as a sync ack.
