// Package templates renders note templates stored inside the vault
// (e.g. the Templates/ folder). Variables use {{name}} syntax with
// optional {{date:FORMAT}} moment-style formats for Obsidian parity.
package templates

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Engine implements core.TemplateEngine over a vault directory.
type Engine struct {
	fs  core.VaultFS
	dir func() string
}

var _ core.TemplateEngine = (*Engine)(nil)

// NewEngine creates a template engine reading from a fixed dir inside
// the vault.
func NewEngine(fs core.VaultFS, dir string) *Engine {
	return NewEngineFunc(fs, func() string { return dir })
}

// NewEngineFunc creates an engine whose directory is resolved on every
// call, so runtime configuration (e.g. plugin settings) can change it
// without a restart.
func NewEngineFunc(fs core.VaultFS, dir func() string) *Engine {
	return &Engine{fs: fs, dir: func() string { return strings.Trim(dir(), "/") }}
}

// List returns template names (file names without extension).
func (e *Engine) List() ([]string, error) {
	dir := e.dir()
	if !e.fs.Exists(dir) {
		return nil, nil
	}
	entries, err := e.fs.List(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir && strings.HasSuffix(strings.ToLower(entry.Name), ".md") {
			names = append(names, strings.TrimSuffix(entry.Name, ".md"))
		}
	}
	sort.Strings(names)
	return names, nil
}

var varRe = regexp.MustCompile(`\{\{\s*([\w-]+)(?::([^}]+))?\s*\}\}`)

// Render loads a template and substitutes variables. Built-ins: date,
// time, datetime, title, filename, currentuser and actor; anything else
// comes from vars. The last two are supplied by NoteService on creation.
func (e *Engine) Render(name string, vars map[string]string) (string, error) {
	data, err := e.fs.Read(e.dir() + "/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("template not found: %w", err)
	}
	now := time.Now()
	out := varRe.ReplaceAllStringFunc(string(data), func(match string) string {
		m := varRe.FindStringSubmatch(match)
		key, format := m[1], m[2]
		switch key {
		case "date":
			if format == "" {
				format = "YYYY-MM-DD"
			}
			return now.Format(momentToGo(format))
		case "time":
			if format == "" {
				format = "HH:mm"
			}
			return now.Format(momentToGo(format))
		case "datetime":
			return now.Format(time.RFC3339)
		default:
			if v, ok := vars[key]; ok {
				return v
			}
			return match // leave unknown variables as-is
		}
	})
	return out, nil
}

// momentToGo converts common moment.js tokens (used by Obsidian) to a
// Go time layout.
func momentToGo(format string) string {
	r := strings.NewReplacer(
		"YYYY", "2006", "YY", "06",
		"MM", "01", "DD", "02",
		"HH", "15", "mm", "04", "ss", "05",
	)
	return r.Replace(format)
}
