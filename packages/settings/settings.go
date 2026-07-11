// Package settings loads and persists the external YAML configuration.
package settings

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Config is the top-level application configuration.
type Config struct {
	Server ServerConfig   `yaml:"server"`
	Vault  VaultConfig    `yaml:"vault"`
	Notes  core.NoteRules `yaml:"notes"`
	Auth   AuthConfig     `yaml:"auth"`
	Web    WebConfig      `yaml:"web"`
	Log    LogConfig      `yaml:"log"`

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
		},
		Auth: AuthConfig{
			TokenTTLHours: 24,
			Admin:         AdminConfig{Username: "admin"},
		},
		Log: LogConfig{Level: "info"},
	}
}

// Load reads the config file (if present) over the defaults. Environment
// variables OBSIDIANWEB_VAULT and OBSIDIANWEB_ADDR take precedence.
func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.path = path
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config: %w", err)
			}
		} else if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
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
	return cfg, nil
}

// Save persists the configuration back to its file.
func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("config was not loaded from a file")
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o644)
}
