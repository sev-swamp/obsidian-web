# AGENTS.md

Obsidian Web — self-hosted web platform over an existing Obsidian vault.
Go backend (single binary, embedded React frontend) + React/TS/Vite/Tailwind 4.

## Commands

```bash
go build ./... && go vet ./...      # backend build + lint
go test ./...                       # backend tests (all packages must stay green)
cd apps/web && npm run build        # frontend: tsc strict + vite (this IS the type check)
make build                          # full production build into ./bin
go run ./apps/server -config /dev/null -vault examples/vault   # dev server :8787
cd apps/web && npm run dev          # frontend dev :5173, proxies to :8787
scripts/deploy.sh                   # deploy (needs OBSIDIANWEB_DEPLOY_HOST env)
```

## Architecture (Clean Architecture, dependency rule)

- `packages/core` — domain: `NoteService`, event bus, ALL port interfaces
  (`ports.go`, `history.go`). Never imports transport/UI/adapters.
- Adapters implement core ports: `filesystem` (vault + fsnotify watcher),
  `markdown` (goldmark: wikilinks/callouts/mermaid/mathjax), `links`
  (backlinks graph), `search` (in-memory index), `history` (go-git in
  vault), `templates`, `settings` (strict YAML, KnownFields), `auth`
  (JWT + permissions + OIDC), `acl` (users.yaml: users/groups/rules/
  SSO/plugins state, hot-reload), `plugins` (backend + UI plugin registry).
- `packages/api` + `packages/websocket` — transport only, no business
  logic. Wiring happens in `apps/server/main.go` (constructor injection,
  no DI container).
- `sdk/plugin-sdk` — stable versioned Plugin API.
- New API endpoint = core method → thin handler in `api/handlers.go` →
  route with permission in `api/router.go` → client method in
  `apps/web/src/api/client.ts`.

## Auth model (two layers)

1. JWT permissions per role (auth.rolePermissions): notes:read/edit/
   delete, history:read, files:upload, settings:write. Middleware
   `requirePermission` enforces per route; frontend `useAuthStore.can()`
   hides actions.
2. Folder ACL (packages/acl, users.yaml) narrows access per glob path;
   enforced in handlers AND in tree/search/backlinks/recent/trash/WS
   filtering (`allowRead`). AccessNone → 404, read-only write → 403.

## Conventions

- Every UI string goes through `useT()`; add keys to BOTH `en` and `ru`
  in `apps/web/src/i18n.ts` (typed dict — build fails otherwise).
- Help content is bilingual: `apps/web/src/help/content.ts` `{en, ru}`;
  keep docs/syntax.md in sync.
- Mutating NoteService methods take `actor` (username from JWT) first.
- git commits: NO Co-Authored-By/Codex trailers (owner's requirement).
- docs/adr/ records key decisions; plans/ holds implementation specs.
- Docker: config mounted read-only at /config, writable /data holds
  users.yaml + runtime.yaml (UI-edited note rules; lives next to
  usersFile, config.yaml is never rewritten), vault at /vault (uid 1000
  owns vault and /data).

## Gotchas

- `apps/web/dist/.gitkeep` must exist (go:embed target); build frontend
  before the server binary for production.
- gin wildcards: `/api/note/*path` blocks nested static routes — new
  note-scoped endpoints use their own prefix (`/api/history/*path`).
- goldmark splits `[!note]` across Text nodes ('[' starts a link) —
  see callouts.go transformer.
- go-git: Worktree.Status() hashes the ENTIRE worktree (O(vault) per
  call) — history.Record avoids it by comparing the on-disk blob hash
  against HEAD and staging with SkipStatus. Also: a clean file is
  absent from the Status map (Status.File fabricates Untracked).
- Deploy builds on the VPS are slow (~10 min when go.mod changed);
  always run detached (script does nohup) — a dropped SSH kills builds.
