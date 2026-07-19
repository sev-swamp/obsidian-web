// Package pluginsdk defines the stable, versioned Plugin API of the
// platform. Plugins depend only on this package and packages/core —
// never on the HTTP framework or the Web UI.
//
// Compatibility policy: APIVersion follows semver. Breaking changes bump
// the major version; the host refuses to load plugins built against a
// different major version.
package pluginsdk

import (
	"log/slog"
	"net/http"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// APIVersion is the current Plugin API version.
const APIVersion = "1.1.0"

// Manifest describes a plugin to the host.
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// APIVersion the plugin was built against.
	APIVersion string `json:"apiVersion"`
	// Settings declares the admin-editable settings of the plugin; the
	// host renders them in the UI and persists the values.
	Settings []SettingSpec `json:"settings,omitempty"`
}

// SettingSpec describes one plugin setting.
type SettingSpec struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Default string `json:"default"`
}

// Routes lets a plugin expose REST endpoints. All routes are mounted
// under /api/plugins/<plugin-id>/.
type Routes interface {
	Handle(method, path string, handler http.HandlerFunc)
}

// Settings gives a plugin read access to its stored settings. Values
// are resolved per call, so admin edits apply without a restart.
type Settings interface {
	// Get returns the stored value for a key declared in the manifest,
	// falling back to the manifest default when unset.
	Get(key string) string
}

// Host is the controlled surface a plugin receives from the platform.
type Host interface {
	// Events returns the event bus for subscribing to vault changes.
	Events() core.EventBus
	// Notes exposes the note service (read, create, search…).
	Notes() *core.NoteService
	// Vault gives direct (sandboxed) access to vault files.
	Vault() core.VaultFS
	// Routes registers plugin REST endpoints.
	Routes() Routes
	// Settings exposes the plugin's admin-managed settings.
	Settings() Settings
	// Logger returns a logger namespaced to the plugin.
	Logger() *slog.Logger
}

// Plugin is the contract every plugin implements.
type Plugin interface {
	Manifest() Manifest
	// Init is called once at startup after the vault is indexed.
	Init(host Host) error
	// Close is called on graceful shutdown.
	Close() error
}
