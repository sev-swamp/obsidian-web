// Package obsidian implements the Obsidian compatibility layer: it reads
// vault-level Obsidian configuration (.obsidian/) so the platform can
// report installed community plugins and reuse relevant settings.
//
// Limitations are documented in docs/obsidian-compat.md. Executing
// Obsidian community plugins is out of scope for now; the architecture
// allows adding an adapter service later.
package obsidian

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PluginManifest mirrors Obsidian's manifest.json.
type PluginManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	MinAppVer   string `json:"minAppVersion"`
	Enabled     bool   `json:"enabled"`
}

// AppConfig is the subset of .obsidian/app.json we understand.
type AppConfig struct {
	AttachmentFolderPath string `json:"attachmentFolderPath"`
	NewFileLocation      string `json:"newFileLocation"`
	NewFileFolderPath    string `json:"newFileFolderPath"`
}

// Compat reads Obsidian configuration from a vault root.
type Compat struct {
	vaultRoot string
}

// New creates a compatibility reader for the given vault root.
func New(vaultRoot string) *Compat {
	return &Compat{vaultRoot: vaultRoot}
}

func (c *Compat) dir() string { return filepath.Join(c.vaultRoot, ".obsidian") }

// Available reports whether the vault has Obsidian configuration.
func (c *Compat) Available() bool {
	info, err := os.Stat(c.dir())
	return err == nil && info.IsDir()
}

// AppConfig reads .obsidian/app.json (may be absent).
func (c *Compat) AppConfig() (*AppConfig, error) {
	data, err := os.ReadFile(filepath.Join(c.dir(), "app.json"))
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CommunityPlugins lists installed community plugins with their
// manifests and enabled state.
func (c *Compat) CommunityPlugins() ([]PluginManifest, error) {
	enabled := map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(c.dir(), "community-plugins.json")); err == nil {
		var ids []string
		if json.Unmarshal(data, &ids) == nil {
			for _, id := range ids {
				enabled[id] = true
			}
		}
	}

	pluginsDir := filepath.Join(c.dir(), "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []PluginManifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pluginsDir, e.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var m PluginManifest
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		m.Enabled = enabled[m.ID]
		out = append(out, m)
	}
	return out, nil
}
