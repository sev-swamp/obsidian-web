# ADR-0002: File system is the single source of truth (no database)

- Status: accepted
- Date: 2026-07-11

## Context

The product promise is "your existing Obsidian vault, served over the
web, unchanged". Any copy of the data (database, import step) creates
sync problems and breaks interoperability with Obsidian itself.

## Decision

All reads and writes go directly to the vault directory. Link and
search indexes are **in-memory projections**: rebuilt on startup,
updated incrementally from watcher events, and never authoritative.

## Rationale

- Zero migration/import: point the server at a vault and it works.
- Obsidian, git, Syncthing etc. keep working on the same folder.
- Index loss is harmless — a restart re-indexes (a few thousand notes
  index in well under a second).

## Consequences

- Startup cost grows with vault size; mitigated by background indexing
  and, later, an optional on-disk index cache (an implementation detail
  behind `core.SearchIndex`, not a schema).
- Concurrent writers (user + Obsidian) are reconciled by the watcher:
  last write wins, as with Obsidian's own sync.
- Very large vaults (100k+ notes) may eventually want a persistent
  index backend — the `SearchIndex`/`LinkIndex` ports allow swapping in
  Bleve/SQLite without touching the core.
