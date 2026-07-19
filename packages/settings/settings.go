// Package settings loads and persists the external YAML configuration.
package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Config is the top-level application configuration.
type Config struct {
	Server ServerConfig   `yaml:"server"`
	Vault  VaultConfig    `yaml:"vault"`
	Notes  core.NoteRules `yaml:"notes"`
	Auth    AuthConfig     `yaml:"auth"`
	History HistoryConfig  `yaml:"history"`
	Web     WebConfig      `yaml:"web"`
	Log     LogConfig      `yaml:"log"`

	path string // file the config was loaded from
}

type ServerConfig struct {
	Addr    string `yaml:"addr"`
	DevCORS bool   `yaml:"devCors"`
}

type VaultConfig struct {
	Path           string `yaml:"path"`
	TemplatesDir   string `yaml:"templatesDir"`
	AttachmentsDir string `yaml:"attachmentsDir"`
}

type AuthConfig struct {
	Enabled       bool        `yaml:"enabled"`
	JWTSecret     string      `yaml:"jwtSecret"`
	TokenTTLHours int         `yaml:"tokenTtlHours"`
	Admin         AdminConfig `yaml:"admin"`
	// Users are additional accounts beside the admin. Role defaults to
	// "viewer" when omitted.
	Users []UserConfig `yaml:"users"`
	// UsersFile is the hot-reloadable store for team accounts, groups
	// and folder ACL rules, managed through the admin API/UI.
	UsersFile string `yaml:"usersFile"`
}

type AdminConfig struct {
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`     // plaintext (dev only) …
	PasswordHash string `yaml:"passwordHash"` // … or bcrypt hash (preferred)
}

type UserConfig struct {
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`     // plaintext (dev only) …
	PasswordHash string `yaml:"passwordHash"` // … or bcrypt hash (preferred)
	Role         string `yaml:"role"`         // viewer | editor | admin
}

type HistoryConfig struct {
	// Enabled turns on git-backed change history (managed mode creates
	// a .git repository inside the vault).
	Enabled bool `yaml:"enabled"`
	// Mode: managed (platform commits) | external (existing repo, read-only).
	Mode string `yaml:"mode"`
	// ExternalDebounceSec coalesces direct file-system edits into one
	// revision per file per interval.
	ExternalDebounceSec int `yaml:"externalDebounceSec"`
}

type WebConfig struct {
	// StaticDir overrides the embedded frontend (useful in development).
	StaticDir string `yaml:"staticDir"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

// Default returns the configuration defaults.
func Default() *Config {
	return &Config{
		Server: ServerConfig{Addr: ":8787"},
		Vault: VaultConfig{
			Path:           "./vault",
			TemplatesDir:   "Templates",
			AttachmentsDir: "attachments",
		},
		Notes: core.NoteRules{
			DefaultFolder:   "Inbox",
			TypeFolders:     map[string]string{},
			AutoFrontmatter: true,
			ShowProperties:  true,
		},
		Auth: AuthConfig{
			TokenTTLHours: 24,
			Admin:         AdminConfig{Username: "admin"},
			UsersFile:     "users.yaml",
		},
		History: HistoryConfig{
			Enabled:             true,
			Mode:                "managed",
			ExternalDebounceSec: 45,
		},
		Log: LogConfig{Level: "info"},
	}
}

// Load reads the config file (if present) over the defaults. Environment
// variables OBSIDIANWEB_VAULT and OBSIDIANWEB_ADDR take precedence. The
// runtime overlay (UI-editable note rules) is applied last.
func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.path = path
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config: %w", err)
			}
		} else if err := parseStrict(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if v := os.Getenv("OBSIDIANWEB_VAULT"); v != "" {
		cfg.Vault.Path = v
	}
	if v := os.Getenv("OBSIDIANWEB_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("OBSIDIANWEB_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if err := cfg.loadRuntime(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseStrict decodes YAML rejecting unknown keys, so typos and
// mis-indented sections (e.g. `users:` landing under `log:`) fail at
// startup instead of being silently ignored.
func parseStrict(data []byte, cfg *Config) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty config file: keep defaults
		}
		return err
	}
	return nil
}

// runtimeSettings is the runtime-editable subset persisted separately
// from config.yaml: saving it never rewrites the main config, which may
// hold secrets (injected via env) and be mounted read-only in Docker.
type runtimeSettings struct {
	Notes core.NoteRules `yaml:"notes"`
}

// RuntimePath is the runtime-settings file, living next to the (always
// writable) users file.
func (c *Config) RuntimePath() string {
	return filepath.Join(filepath.Dir(c.Auth.UsersFile), "runtime.yaml")
}

// loadRuntime overlays runtime.yaml (if present) over the config.
func (c *Config) loadRuntime() error {
	data, err := os.ReadFile(c.RuntimePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read runtime settings: %w", err)
	}
	var rt runtimeSettings
	if err := yaml.Unmarshal(data, &rt); err != nil {
		return fmt.Errorf("parse %s: %w", c.RuntimePath(), err)
	}
	c.Notes = rt.Notes
	return nil
}

// SaveRuntime persists the runtime-editable settings atomically.
func (c *Config) SaveRuntime() error {
	data, err := yaml.Marshal(runtimeSettings{Notes: c.Notes})
	if err != nil {
		return err
	}
	tmp := c.RuntimePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.RuntimePath())
}
