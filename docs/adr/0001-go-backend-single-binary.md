# ADR-0001: Go backend shipped as a single binary

- Status: accepted
- Date: 2026-07-11

## Context

The platform must watch a file system efficiently, serve HTTP/WebSocket
to many clients, deploy trivially on a VPS or in Docker, and stay cheap
to run for years. Candidates: Go, C# (.NET 9), TypeScript (Node).

## Decision

Go for the entire backend. The React frontend is embedded into the
server binary with `go:embed`, so deployment is one executable plus an
external YAML config.

## Rationale

- First-class fsnotify support and cheap goroutine-per-watch concurrency.
- Static single-file binaries (~tens of MB, no runtime to install).
- Low memory footprint suits always-on personal servers.
- Mature ecosystem for the exact needs: gin, gorilla/websocket,
  goldmark (extensible markdown), fsnotify, slog.

.NET 9 offered no decisive architectural advantage to justify the
heavier runtime. TypeScript remains an option for a future
Obsidian-plugin adapter service (see docs/obsidian-compat.md), which is
allowed by the spec as a satellite service while the core stays in Go.

## Consequences

- JavaScript-based Obsidian community plugins cannot run in-process;
  compatibility is delivered incrementally via adapters.
- Frontend changes require re-embedding for production builds
  (`web.staticDir` bypasses this during development).
