package core

import (
	"errors"
	"time"
)

// ErrNotFound is returned by adapters when a vault entry does not exist.
// Transport layers match it with errors.Is instead of inspecting
// implementation-specific errors (os.IsNotExist and friends).
var ErrNotFound = errors.New("not found")

// FileInfo describes a vault entry.
type FileInfo struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// VaultFS abstracts access to the Obsidian vault. The file system is the
// single source of truth: implementations must not copy data elsewhere.
type VaultFS interface {
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Delete(path string) error
	// Mkdir creates a directory (and any missing parents) in the vault.
	Mkdir(path string) error
	List(dir string) ([]FileInfo, error)
	Stat(path string) (FileInfo, error)
	Exists(path string) bool
	Tree() (*TreeNode, error)
	Walk(fn func(info FileInfo) error) error
	// AbsPath resolves a vault-relative path to an absolute one after
	// validating it stays inside the vault (used for efficient file serving).
	AbsPath(path string) (string, error)
}

// Renderer converts markdown into HTML and returns parsed frontmatter.
type Renderer interface {
	Render(path string, source []byte) (html string, frontmatter map[string]any, err error)
}

// LinkIndex maintains the wiki-link graph of the vault.
type LinkIndex interface {
	// Update (re)indexes a markdown note.
	Update(path string, content []byte)
	// RegisterFile makes a non-markdown file (attachment) resolvable.
	RegisterFile(path string)
	Remove(path string)
	// Resolve maps a wiki-link target ("Note", "folder/Note", "img.png")
	// to a vault path.
	Resolve(target string) (string, bool)
	Backlinks(path string) []string
	BrokenLinks() map[string][]Link
	Stats() (links int, broken int)
}

// SearchDoc is the indexable projection of a note.
type SearchDoc struct {
	Path        string
	Title       string
	Tags        []string
	Aliases     []string
	Body        string
	Frontmatter map[string]any
}

// PropertyInfo describes one frontmatter key observed across the vault:
// how many notes carry it, the dominant value type (text, number, checkbox,
// date, datetime, list, link) and the most frequent values.
type PropertyInfo struct {
	Key    string          `json:"key"`
	Type   string          `json:"type"`
	Count  int             `json:"count"`
	Values []PropertyValue `json:"values,omitempty"`
}

// PropertyValue is one observed value of a frontmatter key.
type PropertyValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// SearchIndex is an in-memory, incrementally updated full-text index.
type SearchIndex interface {
	Index(doc SearchDoc)
	Remove(path string)
	Search(query string, limit int) []SearchResult
	// Properties aggregates frontmatter keys over notes visible to the
	// caller (allow nil = all) for settings and search autocomplete.
	Properties(allow AllowFunc) []PropertyInfo
}

// TemplateEngine renders note templates stored inside the vault.
type TemplateEngine interface {
	List() ([]string, error)
	Render(name string, vars map[string]string) (string, error)
}
