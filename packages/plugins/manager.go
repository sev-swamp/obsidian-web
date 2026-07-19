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

// UIPlugin describes a frontend feature toggleable like a plugin
// (e.g. the "Recent changes" sidebar section). It has no backend code;
// the frontend consults /api/plugins to show or hide it.
type UIPlugin struct {
	ID          string
	Name        string
	Version     string
	Description string
}

// PluginStatus is the unified view served by GET /api/plugins.
type PluginStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Kind        string `json:"kind"` // backend | ui
	Enabled     bool   `json:"enabled"`
	// Settings are the effective values (stored or manifest default)
	// for the settings declared in SettingsSpec.
	Settings     map[string]string     `json:"settings,omitempty"`
	SettingsSpec []pluginsdk.SettingSpec `json:"settingsSpec,omitempty"`
}

// Manager registers and initializes plugins.
type Manager struct {
	bus       core.EventBus
	notes     *core.NoteService
	vault     core.VaultFS
	log       *slog.Logger
	plugins   []pluginsdk.Plugin
	uiPlugins []UIPlugin
	// settings resolves stored per-plugin settings; nil = nothing stored.
	settings func(id string) map[string]string
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

// RegisterUI adds a toggleable frontend feature to the plugin list.
func (m *Manager) RegisterUI(p UIPlugin) {
	m.uiPlugins = append(m.uiPlugins, p)
}

// SetSettingsSource wires the store that holds per-plugin settings;
// call before InitAll.
func (m *Manager) SetSettingsSource(fn func(id string) map[string]string) {
	m.settings = fn
}

// SettingsSpec returns the settings declared by a plugin's manifest.
func (m *Manager) SettingsSpec(id string) []pluginsdk.SettingSpec {
	for _, p := range m.plugins {
		if mf := p.Manifest(); mf.ID == id {
			return mf.Settings
		}
	}
	return nil
}

// effectiveSettings resolves stored values over manifest defaults.
func (m *Manager) effectiveSettings(mf pluginsdk.Manifest) map[string]string {
	if len(mf.Settings) == 0 {
		return nil
	}
	var stored map[string]string
	if m.settings != nil {
		stored = m.settings(mf.ID)
	}
	out := make(map[string]string, len(mf.Settings))
	for _, spec := range mf.Settings {
		if v, ok := stored[spec.Key]; ok && v != "" {
			out[spec.Key] = v
		} else {
			out[spec.Key] = spec.Default
		}
	}
	return out
}

// Manifests lists registered plugin manifests.
func (m *Manager) Manifests() []pluginsdk.Manifest {
	out := make([]pluginsdk.Manifest, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, p.Manifest())
	}
	return out
}

// Statuses returns every plugin (backend and UI) with its enabled
// state. enabled may be nil (everything enabled).
func (m *Manager) Statuses(enabled func(id string) bool) []PluginStatus {
	isEnabled := func(id string) bool {
		if enabled == nil {
			return true
		}
		return enabled(id)
	}
	out := make([]PluginStatus, 0, len(m.plugins)+len(m.uiPlugins))
	for _, p := range m.plugins {
		mf := p.Manifest()
		out = append(out, PluginStatus{
			ID: mf.ID, Name: mf.Name, Version: mf.Version,
			Description: mf.Description, Kind: "backend", Enabled: isEnabled(mf.ID),
			Settings: m.effectiveSettings(mf), SettingsSpec: mf.Settings,
		})
	}
	for _, p := range m.uiPlugins {
		out = append(out, PluginStatus{
			ID: p.ID, Name: p.Name, Version: p.Version,
			Description: p.Description, Kind: "ui", Enabled: isEnabled(p.ID),
		})
	}
	return out
}

// Known reports whether a plugin id is registered.
func (m *Manager) Known(id string) bool {
	for _, p := range m.plugins {
		if p.Manifest().ID == id {
			return true
		}
	}
	for _, p := range m.uiPlugins {
		if p.ID == id {
			return true
		}
	}
	return false
}

// InitAll initializes every plugin and mounts its routes under
// /api/plugins/<id>/ in the provided router group. Routes of disabled
// plugins answer 404 (enabled is consulted per request, so toggling
// needs no restart).
func (m *Manager) InitAll(routerGroup *gin.RouterGroup, enabled func(id string) bool) error {
	for _, p := range m.plugins {
		manifest := p.Manifest()
		if !compatibleAPI(manifest.APIVersion) {
			m.log.Warn("plugin skipped: incompatible API version",
				"plugin", manifest.ID, "pluginApi", manifest.APIVersion, "hostApi", pluginsdk.APIVersion)
			continue
		}
		id := manifest.ID
		sub := routerGroup.Group("/"+id, func(c *gin.Context) {
			if enabled != nil && !enabled(id) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "plugin disabled"})
				return
			}
			c.Next()
		})
		host := &host{
			manager: m,
			id:      manifest.ID,
			routes:  &ginRoutes{group: sub},
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
func (h *host) Settings() pluginsdk.Settings { return &hostSettings{manager: h.manager, id: h.id} }
func (h *host) Logger() *slog.Logger        { return h.log }

// hostSettings resolves plugin settings per call: stored value first,
// manifest default otherwise, so admin edits apply without a restart.
type hostSettings struct {
	manager *Manager
	id      string
}

func (s *hostSettings) Get(key string) string {
	if s.manager.settings != nil {
		if v, ok := s.manager.settings(s.id)[key]; ok && v != "" {
			return v
		}
	}
	for _, spec := range s.manager.SettingsSpec(s.id) {
		if spec.Key == key {
			return spec.Default
		}
	}
	return ""
}

// ginRoutes adapts gin to the framework-agnostic pluginsdk.Routes.
type ginRoutes struct {
	group *gin.RouterGroup
}

func (r *ginRoutes) Handle(method, path string, handler http.HandlerFunc) {
	r.group.Handle(strings.ToUpper(method), path, func(c *gin.Context) {
		handler(c.Writer, c.Request)
	})
}
