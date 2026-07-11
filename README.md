# Obsidian Web

An open-source, long-term platform that serves an **existing Obsidian
vault** through a web interface. The vault folder is the single source of
truth: nothing is imported, copied or stored in a database.

```
┌─────────────┐   fsnotify   ┌──────────────────────────────┐
│ Obsidian    │ ───────────▶ │  Go core                     │
│ Vault (fs)  │ ◀─────────── │  links · search · markdown   │
└─────────────┘   read/write └──────┬───────────────┬───────┘
                                    │ REST API      │ WebSocket
                              ┌─────▼───────────────▼───────┐
                              │  React frontend (embedded)  │
                              └─────────────────────────────┘
```

## Features

- **Vault-native** — reads/writes your existing vault in place; watches
  the file system and updates the UI in real time over WebSocket.
- **Obsidian-flavoured markdown** — wiki-links (aliases, headings,
  `^block` refs), backlinks, YAML frontmatter, callouts, Mermaid,
  MathJax, GFM tables, task lists, code highlighting, images/PDF/audio/video.
- **Embedded full-text search** — incremental in-memory index with
  `tag:` and `path:` filters; no external services.
- **Note creation** — folders, per-type folder rules, templates with
  `{{date}}` / `{{time}}` / `{{title}}` / custom variables, automatic
  frontmatter, and the created note opens immediately.
- **Plugin system** — a stable, versioned Go Plugin SDK
  ([sdk/plugin-sdk](sdk/plugin-sdk/sdk.go)); plugins subscribe to events
  and register REST routes.
- **Auth** — optional local admin with JWT and roles (viewer/editor/admin).
- **Single binary** — the React frontend is embedded into the Go server;
  configuration lives in an external YAML file.

## Quick start

Prerequisites: Go ≥ 1.24, Node ≥ 20.

```bash
make build                      # frontend + server + CLI into ./bin
./bin/obsidianweb -vault /path/to/your/vault
# open http://localhost:8787
```

Try it instantly with the bundled demo vault:

```bash
./bin/obsidianweb -config /dev/null -vault examples/vault
```

Configuration: copy [config.example.yaml](config.example.yaml) to
`config.yaml`. Environment overrides: `OBSIDIANWEB_VAULT`,
`OBSIDIANWEB_ADDR`, `OBSIDIANWEB_JWT_SECRET`.

### Docker

```bash
docker compose up --build
# vault is mounted from ./examples/vault — edit docker-compose.yml
```

### CLI

```bash
./bin/obsidianweb-cli -vault /path/to/vault index        # index + stats
./bin/obsidianweb-cli -vault /path/to/vault check-links  # broken wiki-links
./bin/obsidianweb-cli -vault /path/to/vault export -out ./html
```

## Development

```bash
make run    # backend on :8787
make dev    # Vite dev server on :5173 with API/WS proxy
```

## Repository layout

```
apps/
  server/        composition root, single-binary HTTP server
  web/           React + TypeScript + Vite + Tailwind frontend
  cli/           console utility (index, check-links, export)
packages/
  core/          domain model, NoteService, event bus, ports (interfaces)
  filesystem/    vault access + fsnotify watcher
  markdown/      goldmark pipeline (wiki-links, callouts, mermaid, math)
  links/         wiki-link parsing, resolution, backlinks, broken links
  search/        embedded incremental full-text index
  templates/     note templates with variables
  settings/      external YAML configuration
  auth/          JWT + roles (local admin)
  api/           REST handlers (transport only, no business logic)
  websocket/     event hub for live UI updates
  plugins/       plugin runtime + built-in plugins
  obsidian/      Obsidian compatibility layer (.obsidian readers)
  shared/        small cross-cutting utilities
sdk/
  plugin-sdk/    stable, versioned Plugin API for external plugins
docs/            architecture, API reference, guides, ADRs
examples/vault/  demo vault used by the quick start
```

## Documentation

- [Architecture](docs/architecture.md)
- [REST API & WebSocket reference](docs/api.md)
- [Plugin SDK guide](docs/plugin-sdk.md)
- [Deployment guide](docs/deployment.md)
- [Obsidian compatibility](docs/obsidian-compat.md)
- [Development guide](docs/development.md)
- [Architecture Decision Records](docs/adr/)

## License

MIT — see [LICENSE](LICENSE).
