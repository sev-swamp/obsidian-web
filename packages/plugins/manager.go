// Package plugins hosts the plugin runtime: it wires pluginsdk.Host for
// each registered plugin and mounts plugin routes into the API.
package plugins

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/core"
	pluginsdk "github.com/obsidianweb/obsidianweb/sdk/plugin-sdk"
)

// Manager registers and initializes plugins.
type Manager struct {
	bus     core.EventBus
	notes   *core.NoteService
	vault   core.VaultFS
	log     *slog.Logger
	plugins []pluginsdk.Plugin
}

// NewManager creates a plugin manager.
func NewManager(bus core.EventBus, notes *core.NoteService, vault core.VaultFS, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{bus: bus, notes: notes, vault: vault, log: log}
}

// Register adds a plugin; call before InitAll.
func (m *Manager) Register(p pluginsdk.Plugin) {
	m.plugins = append(m.plugins, p)
}

// Manifests lists registered plugin manifests.
func (m *Manager) Manifests() []pluginsdk.Manifest {
	out := make([]pluginsdk.Manifest, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, p.Manifest())
	}
	return out
}

// InitAll initializes every plugin and mounts its routes under
// /api/plugins/<id>/ in the provided router group.
func (m *Manager) InitAll(routerGroup *gin.RouterGroup) error {
	for _, p := range m.plugins {
		manifest := p.Manifest()
		if !compatibleAPI(manifest.APIVersion) {
			m.log.Warn("plugin skipped: incompatible API version",
				"plugin", manifest.ID, "pluginApi", manifest.APIVersion, "hostApi", pluginsdk.APIVersion)
			continue
		}
		host := &host{
			manager: m,
			id:      manifest.ID,
			routes:  &ginRoutes{group: routerGroup.Group("/" + manifest.ID)},
			log:     m.log.With("plugin", manifest.ID),
		}
		if err := p.Init(host); err != nil {
			return fmt.Errorf("plugin %s: %w", manifest.ID, err)
		}
		m.log.Info("plugin initialized", "plugin", manifest.ID, "version", manifest.Version)
	}
	return nil
}

// CloseAll shuts plugins down.
func (m *Manager) CloseAll() {
	for _, p := range m.plugins {
		if err := p.Close(); err != nil {
			m.log.Warn("plugin close failed", "plugin", p.Manifest().ID, "error", err)
		}
	}
}

// compatibleAPI enforces the semver-major compatibility policy.
func compatibleAPI(v string) bool {
	if v == "" {
		return false
	}
	return strings.SplitN(v, ".", 2)[0] == strings.SplitN(pluginsdk.APIVersion, ".", 2)[0]
}

// host implements pluginsdk.Host.
type host struct {
	manager *Manager
	id      string
	routes  *ginRoutes
	log     *slog.Logger
}

func (h *host) Events() core.EventBus       { return h.manager.bus }
func (h *host) Notes() *core.NoteService    { return h.manager.notes }
func (h *host) Vault() core.VaultFS         { return h.manager.vault }
func (h *host) Routes() pluginsdk.Routes    { return h.routes }
func (h *host) Logger() *slog.Logger        { return h.log }

// ginRoutes adapts gin to the framework-agnostic pluginsdk.Routes.
type ginRoutes struct {
	group *gin.RouterGroup
}

func (r *ginRoutes) Handle(method, path string, handler http.HandlerFunc) {
	r.group.Handle(strings.ToUpper(method), path, func(c *gin.Context) {
		handler(c.Writer, c.Request)
	})
}
