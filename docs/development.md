# Development guide

## Prerequisites

- Go ≥ 1.24
- Node.js ≥ 20

## Workflow

```bash
# Terminal 1: backend against the demo vault (or your own)
go run ./apps/server -config /dev/null -vault examples/vault

# Terminal 2: frontend with hot reload, proxied to :8787
cd apps/web && npm install && npm run dev
# open http://localhost:5173
```

Production build (`make build`) compiles the frontend into
`apps/web/dist`, which the server embeds via `go:embed` — the result is
one binary.

## Code organization rules

1. `packages/core` must not import transport/UI packages (gin,
   websocket, React). It defines interfaces; adapters implement them.
2. `packages/api` and `packages/websocket` contain no business logic —
   they translate HTTP/WS to `NoteService` calls.
3. New functionality starts with an interface in `core/ports.go` (or the
   plugin SDK) so implementations stay swappable.
4. All wiring happens in the composition roots (`apps/server`,
   `apps/cli`) via constructor injection.

## Testing & checks

```bash
go build ./... && go vet ./...   # backend
go test ./...
cd apps/web && npm run build     # includes tsc type-checking
```

## Adding a markdown feature

Add a goldmark extension in `packages/markdown` and register it in
`NewRenderer`. See [callouts.go](../packages/markdown/callouts.go) for a
complete example (AST transformer + CSS contract with the frontend).

## Adding an API endpoint

1. Add the operation to `core.NoteService` (business logic).
2. Add a thin handler in `packages/api/handlers.go`.
3. Register the route with the right role in `packages/api/router.go`.
4. Add the client call in `apps/web/src/api/client.ts`.
