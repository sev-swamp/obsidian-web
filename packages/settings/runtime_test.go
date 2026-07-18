package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// TestRuntimeSettingsRoundTrip: UI-edited note rules persist to the
// runtime file (next to users.yaml) and are loaded back on start, while
// config.yaml itself is never rewritten.
func TestRuntimeSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := "auth:\n  usersFile: " + filepath.Join(dir, "users.yaml") + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Notes = core.NoteRules{DefaultFolder: "Notes", TrackAuthorship: true}
	if err := cfg.SaveRuntime(); err != nil {
		t.Fatal(err)
	}

	// config.yaml stays untouched — secrets never land there.
	after, _ := os.ReadFile(configPath)
	if string(after) != configBody {
		t.Fatalf("config.yaml was rewritten:\n%s", after)
	}
	raw, err := os.ReadFile(cfg.RuntimePath())
	if err != nil {
		t.Fatalf("runtime file missing: %v", err)
	}
	if !strings.Contains(string(raw), "defaultFolder: Notes") && !strings.Contains(string(raw), "Notes") {
		t.Fatalf("runtime file content: %s", raw)
	}

	// A fresh Load picks the runtime overlay up.
	cfg2, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Notes.DefaultFolder != "Notes" || !cfg2.Notes.TrackAuthorship {
		t.Fatalf("runtime overlay not applied: %+v", cfg2.Notes)
	}
}
