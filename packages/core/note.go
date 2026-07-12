// Package core contains the domain model and business logic of the
// platform. It must not depend on any transport (HTTP, WebSocket) or UI.
package core

import "time"

// NoteMeta is lightweight note metadata used in listings.
type NoteMeta struct {
	Path    string    `json:"path"`
	Title   string    `json:"title"`
	Tags    []string  `json:"tags,omitempty"`
	Aliases []string  `json:"aliases,omitempty"`
	ModTime time.Time `json:"modTime"`
	Size    int64     `json:"size"`
}

// Note is a fully loaded note: raw content plus rendered HTML and links.
type Note struct {
	NoteMeta
	Content     string         `json:"content"`
	ContentHash string         `json:"contentHash"` // sha256 for optimistic locking
	HTML        string         `json:"html,omitempty"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	Backlinks   []Backlink     `json:"backlinks,omitempty"`
	// Access is the caller's effective access ("read" | "write"),
	// filled by the API layer.
	Access string `json:"access,omitempty"`
}

// Link is an outgoing wiki-link extracted from a note.
type Link struct {
	Raw      string `json:"raw"`      // link target as written: "Note", "folder/Note"
	Alias    string `json:"alias,omitempty"`
	Fragment string `json:"fragment,omitempty"` // heading or ^block reference
	Embed    bool   `json:"embed,omitempty"`
}

// Backlink points from another note to the current one.
type Backlink struct {
	Source string `json:"source"`
	Title  string `json:"title"`
}

// TreeNode is a node of the vault directory tree.
type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Children []*TreeNode `json:"children,omitempty"`
}

// SearchResult is a single full-text search hit.
type SearchResult struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Snippet string   `json:"snippet,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Score   float64  `json:"score"`
}

// VaultStats is an aggregate summary of the vault.
type VaultStats struct {
	Notes       int `json:"notes"`
	Attachments int `json:"attachments"`
	Folders     int `json:"folders"`
	Links       int `json:"links"`
	BrokenLinks int `json:"brokenLinks"`
}

// NoteRules configures where new notes are created.
type NoteRules struct {
	DefaultFolder   string            `yaml:"defaultFolder" json:"defaultFolder"`
	TypeFolders     map[string]string `yaml:"typeFolders" json:"typeFolders"`
	AutoFrontmatter bool              `yaml:"autoFrontmatter" json:"autoFrontmatter"`
	// TrackAuthorship maintains created_by / updated_by frontmatter
	// fields on notes saved through the platform.
	TrackAuthorship bool `yaml:"trackAuthorship" json:"trackAuthorship"`
}
