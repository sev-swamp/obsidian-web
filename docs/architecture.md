# Architecture

Obsidian Web follows Clean Architecture: the domain core knows nothing
about transports or UI, and every capability is expressed as an
interface so implementations can be replaced independently.

## Layers

```
        ┌────────────────────────────────────────────┐
        │ apps/ (composition roots)                  │
        │   server        cli        web (React SPA) │
        └───────┬───────────┬────────────────────────┘
                │           │
        ┌───────▼───────────▼────────────────────────┐
        │ transport: packages/api, packages/websocket│   ← no business logic
        └───────┬────────────────────────────────────┘
                │ calls
        ┌───────▼────────────────────────────────────┐
        │ domain: packages/core                      │
        │   NoteService · EventBus · ports           │   ← depends on nothing below
        └───────┬────────────────────────────────────┘
                │ implemented by (dependency inversion)
        ┌───────▼────────────────────────────────────┐
        │ adapters: filesystem · markdown · links ·  │
        │ search · templates · auth · settings ·     │
        │ plugins · obsidian                         │
        └────────────────────────────────────────────┘
```

Dependency rule: `core` defines the ports ([packages/core/ports.go](../packages/core/ports.go))
— `VaultFS`, `Renderer`, `LinkIndex`, `SearchIndex`, `TemplateEngine`,
`EventBus` — and the adapter packages implement them. The composition
root ([apps/server/main.go](../apps/server/main.go)) wires everything via
constructor injection. No DI container is needed; wiring is explicit and
compile-checked (see ADR-0003).

## Data flow

1. **Read path** — API handler → `NoteService.GetNote` → `VaultFS.Read`
   → `Renderer.Render` (goldmark pipeline, wiki-links resolved through
   `LinkIndex`) → JSON with raw content, HTML and backlinks.
2. **Write path** — `NoteService.SaveNote/CreateNote` → `VaultFS.Write`
   → synchronous index update → events published on the bus.
3. **External changes** — fsnotify watcher (debounced) →
   `NoteService.HandleFSEvent` → incremental index update → events →
   WebSocket hub → connected clients invalidate their queries.

The file system is the single source of truth. Indexes (links, search)
are in-memory projections rebuilt at startup and maintained
incrementally; losing them costs one re-index, never data.

## Module responsibilities

| Module               | Responsibility                                            |
| -------------------- | --------------------------------------------------------- |
| `packages/core`      | Domain types, `NoteService`, event bus, port interfaces   |
| `packages/filesystem`| Sandboxed vault IO, directory tree, recursive watcher     |
| `packages/markdown`  | goldmark pipeline: GFM, wiki-links, callouts, mermaid, math, highlighting |
| `packages/links`     | Wiki-link parsing, name/alias resolution, backlinks, broken links |
| `packages/search`    | Embedded inverted index, incremental, `tag:`/`path:` filters |
| `packages/templates` | Vault-stored templates, `{{var}}` substitution            |
| `packages/settings`  | External YAML config, env overrides, runtime persistence  |
| `packages/auth`      | JWT issuance/validation, role hierarchy                   |
| `packages/api`       | REST handlers and routing (thin transport layer)          |
| `packages/websocket` | Event fan-out hub with per-client send queues             |
| `packages/plugins`   | Plugin runtime hosting the SDK contract                   |
| `packages/obsidian`  | Readers for `.obsidian/` (manifests, community plugins)   |
| `sdk/plugin-sdk`     | Stable, versioned plugin contract (no framework deps)     |

## Extensibility

- **New storage** (e.g. S3, git): implement `core.VaultFS`.
- **New search backend** (e.g. Bleve, Meilisearch): implement `core.SearchIndex`.
- **New markdown features**: add a goldmark extension in `packages/markdown`.
- **New clients** (desktop, mobile, integrations): consume the REST API
  and WebSocket; the core never depends on a specific UI.
- **Plugins**: implement `pluginsdk.Plugin`; see [plugin-sdk.md](plugin-sdk.md).

## Performance

- Lazy loading: notes are rendered on request; the tree endpoint returns
  structure only.
- Incremental indexing: file events update only the affected note.
- Debounced watcher: editor save bursts collapse into one event.
- Background initial index: the server accepts requests while indexing.
- Frontend: TanStack Query caching + WebSocket-driven invalidation.

Key decisions are recorded in [docs/adr/](adr/).
