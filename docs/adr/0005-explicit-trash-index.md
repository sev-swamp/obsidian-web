# ADR-0005: Explicit trash index instead of git-log scanning

Status: accepted (2026-07)

## Context

The trash used to be derived data: every listing walked the git log
(capped at 2000 commits) looking for `delete: <path>` commit messages.
On an active vault — where every save is a commit — a file deleted a
few weeks earlier silently fell off the walk horizon: its content was
still in git, but the UI could no longer restore it. Purging was a
side list of bare paths (`.git/obsidianweb-trash-purged.json`), which
permanently hid *any* future deletion of the same path, and "purge all"
operated on a page-sized listing, clearing only the newest 100 entries.

## Decision

The trash is an explicit index, `.git/obsidianweb-trash.json` (stored
inside `.git/` so it is never committed, like the purged list before
it). Each entry is `{path, actor, time, restoreRev, deleteRev}`:

- `History.Record` maintains it commit-side: a delete commit adds (or
  replaces) the path's entry; any other commit for the path (create,
  save, restore, external) means the file exists again and drops it.
- Listing the trash reads the index — O(size of trash), no walk, no
  horizon. `restoreRev` (the delete commit's parent) is stored, so
  restore does not scan either.
- Purge removes entries from the index. The content stays in git
  history — the UI wording is "remove from trash", not "delete
  permanently". Keying entries by deletion (not bare path) means a
  recreated and re-deleted file shows up in the trash again.
- Migration: on the first start without an index, one legacy log scan
  seeds it (honouring the old purged list). On every later start the
  index is validated — entries whose file reappeared on disk or whose
  revision no longer resolves (out-of-band repository edits) are
  dropped; the same check self-heals on each listing.

## Consequences

- The index is derived data too: deleting it loses trash entries older
  than the 2000-commit migration horizon, but never the content (git
  keeps it; `git log --diff-filter=D` still finds everything).
- In `history.mode: external` the platform makes no commits, so the
  trash stays empty and deletion through the UI is immediate and
  unrecoverable — the delete dialog warns about this.
- True irreversible deletion (rewriting history so purged content
  physically leaves `.git`) is a separate, heavier operation — planned
  in plans/03-trash-v2.md §3.4, not implemented here.
