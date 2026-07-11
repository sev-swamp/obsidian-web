package core

import (
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/shared"
)

// CreateNoteRequest describes a note to be created through the UI or API.
type CreateNoteRequest struct {
	Title     string            `json:"title"`
	Folder    string            `json:"folder"`
	Type      string            `json:"type"`
	Template  string            `json:"template"`
	Variables map[string]string `json:"variables"`
	Content   string            `json:"content"`
}

// NoteService orchestrates the vault, renderer and indexes. It is the
// single entry point for all clients (REST API, CLI, plugins).
type NoteService struct {
	fs        VaultFS
	renderer  Renderer
	links     LinkIndex
	search    SearchIndex
	templates TemplateEngine
	bus       EventBus
	log       *slog.Logger

	mu    sync.RWMutex
	rules NoteRules
}

// NewNoteService wires the core service from its dependencies.
func NewNoteService(fs VaultFS, renderer Renderer, links LinkIndex, search SearchIndex, templates TemplateEngine, bus EventBus, rules NoteRules, log *slog.Logger) *NoteService {
	if log == nil {
		log = slog.Default()
	}
	return &NoteService{fs: fs, renderer: renderer, links: links, search: search, templates: templates, bus: bus, rules: rules, log: log}
}

func (s *NoteService) Rules() NoteRules {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rules
}

func (s *NoteService) SetRules(r NoteRules) {
	s.mu.Lock()
	s.rules = r
	s.mu.Unlock()
}

// NormalizeNotePath cleans a user supplied note path and ensures the
// .md extension.
func NormalizeNotePath(p string) string {
	p = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(p)), "/")
	if p == "" {
		return p
	}
	if !strings.HasSuffix(strings.ToLower(p), ".md") {
		p += ".md"
	}
	return p
}

// IsMarkdown reports whether the path looks like a markdown note.
func IsMarkdown(p string) bool {
	return strings.HasSuffix(strings.ToLower(p), ".md")
}

// GetNote loads, renders and enriches a note with backlinks.
func (s *NoteService) GetNote(p string) (*Note, error) {
	p = NormalizeNotePath(p)
	data, err := s.fs.Read(p)
	if err != nil {
		return nil, err
	}
	info, err := s.fs.Stat(p)
	if err != nil {
		return nil, err
	}
	html, fm, err := s.renderer.Render(p, data)
	if err != nil {
		s.log.Warn("render failed", "path", p, "error", err)
		html = ""
	}
	note := &Note{
		NoteMeta:    metaFrom(p, info, fm),
		Content:     string(data),
		HTML:        html,
		Frontmatter: fm,
	}
	for _, src := range s.links.Backlinks(p) {
		note.Backlinks = append(note.Backlinks, Backlink{Source: src, Title: titleFromPath(src)})
	}
	sort.Slice(note.Backlinks, func(i, j int) bool { return note.Backlinks[i].Source < note.Backlinks[j].Source })
	return note, nil
}

// SaveNote writes note content and synchronously refreshes indexes so
// subsequent reads are consistent.
func (s *NoteService) SaveNote(p, content string) error {
	p = NormalizeNotePath(p)
	existed := s.fs.Exists(p)
	if err := s.fs.Write(p, []byte(content)); err != nil {
		return err
	}
	s.indexNote(p, []byte(content))
	if existed {
		s.bus.Publish(Event{Type: EventFileChanged, Path: p})
	} else {
		s.bus.Publish(Event{Type: EventFileCreated, Path: p})
		s.bus.Publish(Event{Type: EventTreeChanged})
	}
	return nil
}

var unsafeFilename = regexp.MustCompile(`[\\/:*?"<>|]+`)

// CreateNote creates a note according to the configured rules and
// optional template, returning the new vault path.
func (s *NoteService) CreateNote(req CreateNoteRequest) (string, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	name := strings.TrimSpace(unsafeFilename.ReplaceAllString(title, "-"))

	rules := s.Rules()
	folder := strings.Trim(req.Folder, "/")
	if folder == "" && req.Type != "" {
		folder = rules.TypeFolders[req.Type]
	}
	if folder == "" {
		folder = rules.DefaultFolder
	}

	p := path.Join(folder, name+".md")
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	for i := 1; s.fs.Exists(p); i++ {
		p = path.Join(folder, fmt.Sprintf("%s %d.md", name, i))
	}

	content := req.Content
	if req.Template != "" && s.templates != nil {
		vars := map[string]string{"title": title, "filename": name}
		for k, v := range req.Variables {
			vars[k] = v
		}
		rendered, err := s.templates.Render(req.Template, vars)
		if err != nil {
			return "", fmt.Errorf("template %q: %w", req.Template, err)
		}
		content = rendered
	}
	if fm, _ := shared.SplitFrontmatter([]byte(content)); fm == nil && rules.AutoFrontmatter {
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "title: %q\n", title)
		fmt.Fprintf(&b, "created: %s\n", time.Now().Format(time.RFC3339))
		b.WriteString("tags: []\n---\n\n")
		b.WriteString(content)
		content = b.String()
	}

	if err := s.fs.Write(p, []byte(content)); err != nil {
		return "", err
	}
	s.indexNote(p, []byte(content))
	s.bus.Publish(Event{Type: EventFileCreated, Path: p})
	s.bus.Publish(Event{Type: EventTreeChanged})
	return p, nil
}

// DeleteNote removes a note from the vault and all indexes.
func (s *NoteService) DeleteNote(p string) error {
	p = NormalizeNotePath(p)
	if err := s.fs.Delete(p); err != nil {
		return err
	}
	s.links.Remove(p)
	s.search.Remove(p)
	s.bus.Publish(Event{Type: EventFileDeleted, Path: p})
	s.bus.Publish(Event{Type: EventTreeChanged})
	return nil
}

// Tree returns the vault directory tree.
func (s *NoteService) Tree() (*TreeNode, error) { return s.fs.Tree() }

// Search runs a full-text query.
func (s *NoteService) Search(query string, limit int) []SearchResult {
	if limit <= 0 {
		limit = 20
	}
	return s.search.Search(query, limit)
}

// ListNotes returns metadata for every markdown note in the vault.
func (s *NoteService) ListNotes() ([]NoteMeta, error) {
	var out []NoteMeta
	err := s.fs.Walk(func(info FileInfo) error {
		if !info.IsDir && IsMarkdown(info.Path) {
			out = append(out, metaFrom(info.Path, info, nil))
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// Recent returns the most recently modified notes.
func (s *NoteService) Recent(limit int) ([]NoteMeta, error) {
	notes, err := s.ListNotes()
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].ModTime.After(notes[j].ModTime) })
	if limit > 0 && len(notes) > limit {
		notes = notes[:limit]
	}
	return notes, nil
}

// Templates lists available note templates.
func (s *NoteService) Templates() ([]string, error) {
	if s.templates == nil {
		return nil, nil
	}
	return s.templates.List()
}

// BrokenLinks exposes unresolved wiki-links (used by the CLI).
func (s *NoteService) BrokenLinks() map[string][]Link { return s.links.BrokenLinks() }

// Stats aggregates vault numbers.
func (s *NoteService) Stats() (VaultStats, error) {
	var st VaultStats
	err := s.fs.Walk(func(info FileInfo) error {
		switch {
		case info.IsDir:
			st.Folders++
		case IsMarkdown(info.Path):
			st.Notes++
		default:
			st.Attachments++
		}
		return nil
	})
	st.Links, st.BrokenLinks = s.links.Stats()
	return st, err
}

// ReindexAll rebuilds link and search indexes by walking the vault.
func (s *NoteService) ReindexAll() error {
	start := time.Now()
	count := 0
	err := s.fs.Walk(func(info FileInfo) error {
		if info.IsDir {
			return nil
		}
		if IsMarkdown(info.Path) {
			data, err := s.fs.Read(info.Path)
			if err != nil {
				s.log.Warn("reindex: read failed", "path", info.Path, "error", err)
				return nil
			}
			s.indexNote(info.Path, data)
			count++
		} else {
			s.links.RegisterFile(info.Path)
		}
		return nil
	})
	s.log.Info("vault indexed", "notes", count, "duration", time.Since(start).Round(time.Millisecond))
	s.bus.Publish(Event{Type: EventIndexUpdated})
	return err
}

// HandleFSEvent reacts to file watcher notifications and keeps indexes
// and connected clients up to date.
func (s *NoteService) HandleFSEvent(op, p string) {
	switch op {
	case "create", "write":
		if IsMarkdown(p) {
			data, err := s.fs.Read(p)
			if err == nil {
				s.indexNote(p, data)
			}
		} else {
			s.links.RegisterFile(p)
		}
		if op == "create" {
			s.bus.Publish(Event{Type: EventFileCreated, Path: p})
			s.bus.Publish(Event{Type: EventTreeChanged})
		} else {
			s.bus.Publish(Event{Type: EventFileChanged, Path: p})
		}
	case "remove", "rename":
		s.links.Remove(p)
		s.search.Remove(p)
		s.bus.Publish(Event{Type: EventFileDeleted, Path: p})
		s.bus.Publish(Event{Type: EventTreeChanged})
	}
	s.bus.Publish(Event{Type: EventIndexUpdated})
}

func (s *NoteService) indexNote(p string, data []byte) {
	fm, body := shared.SplitFrontmatter(data)
	title := titleFromPath(p)
	if fm != nil {
		if t, ok := fm["title"].(string); ok && t != "" {
			title = t
		}
	}
	tags := shared.StringList(fmValue(fm, "tags"))
	aliases := shared.StringList(fmValue(fm, "aliases"))
	s.links.Update(p, data)
	s.search.Index(SearchDoc{
		Path:        p,
		Title:       title,
		Tags:        tags,
		Aliases:     aliases,
		Body:        string(body),
		Frontmatter: fm,
	})
}

func fmValue(fm map[string]any, key string) any {
	if fm == nil {
		return nil
	}
	return fm[key]
}

func metaFrom(p string, info FileInfo, fm map[string]any) NoteMeta {
	title := titleFromPath(p)
	var tags, aliases []string
	if fm != nil {
		if t, ok := fm["title"].(string); ok && t != "" {
			title = t
		}
		tags = shared.StringList(fm["tags"])
		aliases = shared.StringList(fm["aliases"])
	}
	return NoteMeta{Path: p, Title: title, Tags: tags, Aliases: aliases, ModTime: info.ModTime, Size: info.Size}
}

func titleFromPath(p string) string {
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}
