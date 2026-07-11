# Plugin SDK

Plugins extend the platform without touching its code. The contract
lives in [sdk/plugin-sdk/sdk.go](../sdk/plugin-sdk/sdk.go) and depends
only on `packages/core` — never on the HTTP framework or the Web UI.

## Contract

```go
type Plugin interface {
    Manifest() Manifest      // id, name, version, apiVersion
    Init(host Host) error    // called once after the vault is indexed
    Close() error            // called on graceful shutdown
}
```

Through `Host` a plugin can:

| Capability          | How                                                     |
| ------------------- | ------------------------------------------------------- |
| Subscribe to events | `host.Events().Subscribe(func(e core.Event) { … })`     |
| Read/create notes   | `host.Notes()` (`GetNote`, `CreateNote`, `Search`, …)   |
| Access vault files  | `host.Vault()` (sandboxed `core.VaultFS`)               |
| Add REST endpoints  | `host.Routes().Handle("GET", "/path", handler)` → mounted at `/api/plugins/<id>/path` |
| Log                 | `host.Logger()` (namespaced `slog`)                     |

## Versioning

`pluginsdk.APIVersion` follows semver. The host loads a plugin only when
its manifest's major API version matches; a mismatch is logged and the
plugin is skipped. Breaking SDK changes require a major bump.

## Minimal plugin

```go
package myplugin

import (
    "net/http"
    pluginsdk "github.com/obsidianweb/obsidianweb/sdk/plugin-sdk"
)

type Plugin struct{}

func (p *Plugin) Manifest() pluginsdk.Manifest {
    return pluginsdk.Manifest{
        ID: "hello", Name: "Hello", Version: "0.1.0",
        APIVersion: pluginsdk.APIVersion,
    }
}

func (p *Plugin) Init(host pluginsdk.Host) error {
    host.Routes().Handle(http.MethodGet, "/greet", func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte(`{"hello":"vault"}`))
    })
    return nil
}

func (p *Plugin) Close() error { return nil }
```

Register it in the composition root ([apps/server/main.go](../apps/server/main.go)):

```go
pluginManager.Register(&myplugin.Plugin{})
```

A working reference is the built-in
[vault-stats plugin](../packages/plugins/builtin/stats.go)
(`GET /api/plugins/vault-stats/summary`).

## Roadmap

- Compile-time registration (current) → out-of-process plugins over a
  gRPC/stdio protocol so plugins can be written in any language and
  loaded without recompiling.
- Frontend extension points (custom panels, pages and commands) exposed
  through a manifest-driven UI registry.
