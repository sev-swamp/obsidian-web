package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeConfig(t, `
server:
  addr: ":9999"
auth:
  enabled: true
  jwtSecret: "s"
  users:
    - username: "bob"
      password: "pw"
      role: "editor"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != ":9999" {
		t.Errorf("addr = %s", cfg.Server.Addr)
	}
	if len(cfg.Auth.Users) != 1 || cfg.Auth.Users[0].Role != "editor" {
		t.Errorf("users = %+v", cfg.Auth.Users)
	}
	// Untouched sections keep defaults.
	if cfg.Vault.TemplatesDir != "Templates" {
		t.Errorf("templatesDir default lost: %s", cfg.Vault.TemplatesDir)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	cases := map[string]string{
		"top-level typo":     "serverr:\n  addr: \":1\"\n",
		"nested typo":        "server:\n  adress: \":1\"\n",
		"misplaced section":  "log:\n  level: info\n  users:\n    - username: x\n",
		"unknown user field": "auth:\n  users:\n    - username: x\n      pasword: y\n",
	}
	for name, content := range cases {
		if _, err := Load(writeConfig(t, content)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		} else if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "field") {
			t.Logf("%s: error (ok): %v", name, err)
		}
	}
}

func TestLoadMissingAndEmptyFiles(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err != nil {
		t.Errorf("missing file must fall back to defaults: %v", err)
	}
	if cfg, err := Load(writeConfig(t, "")); err != nil || cfg.Server.Addr != ":8787" {
		t.Errorf("empty file must keep defaults: cfg=%+v err=%v", cfg, err)
	}
}
