package core

import (
	"crypto/sha256"
	"encoding/hex"
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

// ActorExternal marks changes made directly on the file system
// (Obsidian, editors, sync tools) rather than through the platform.
const ActorExternal = "external"

// CreateNoteRequest describes a note to be created through the UI or API.
type CreateNoteRequest struct {
	Title     string            `json:"title"`
	Folder    string            `json:"folder"`
	Type      string            `json:"type"`
	Template  string            `json:"template"`
	Variables map[string]string `json:"variables"`
	Content   string            `json:"content"`
}

// AllowFunc filters paths visible to the requesting user; nil allows all.
type AllowFunc func(path string) bool

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

	history     History
	extDebounce time.Duration

	lockMu sync.Mutex
	locks  map[string]*sync.Mutex

	extMu     sync.Mutex
	extTimers map[string]*time.Timer
}

// NewNoteService wires the core service from its dependencies.
func NewNoteService(fs VaultFS, renderer Renderer, links LinkIndex, search SearchIndex, templates TemplateEngine, bus EventBus, rules NoteRules, log *slog.Logger) *NoteService {
	if log == nil {
		log = slog.Default()
	}
	return &NoteService{
		fs: fs, renderer: renderer, links: links, search: search,
		templates: templates, bus: bus, rules: rules, log: log,
		locks:     map[string]*sync.Mutex{},
		extTimers: map[string]*time.Timer{},
	}
}

// AttachHistory enables change history. externalDebounce coalesces
// bursts of direct file-system edits into single revisions.
func (s *NoteService) AttachHistory(h History, externalDebounce time.Duration) {
	s.history = h
	s.extDebounce = externalDebounce
}

// History returns the attached history backend (nil when disabled).
func (s *NoteService) History() History { return s.history }

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

// pathLock serializes mutations of a single note.
func (s *NoteService) pathLock(p string) *sync.Mutex {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	l, ok := s.locks[p]
	if !ok {
		l = &sync.Mutex{}
		s.locks[p] = l
	}
	return l
}

// HashContent returns the hash used for optimistic locking.
func HashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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

// GetNote loads, renders and enriches a note with backlinks. allow
// filters which backlink sources the caller may see (nil = all).
func (s *NoteService) GetNote(p string, allow AllowFunc) (*Note, error) {
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
		ContentHash: HashContent(data),
		HTML:        html,
		Frontmatter: fm,
	}
	for _, src := range s.links.Backlinks(p) {
		if allow != nil && !allow(src) {
			continue
		}
		note.Backlinks = append(note.Backlinks, Backlink{Source: src, Title: titleFromPath(src)})
	}
	sort.Slice(note.Backlinks, func(i, j int) bool { return note.Backlinks[i].Source < note.Backlinks[j].Source })
	return note, nil
}

// SaveNote writes note content. When baseHash is non-empty and the
// stored note differs, a *ConflictError is returned and nothing is
// written (optimistic locking).
func (s *NoteService) SaveNote(actor, p, content, baseHash string) error {
	p = NormalizeNotePath(p)
	lock := s.pathLock(p)
	lock.Lock()
	defer lock.Unlock()

	existed := s.fs.Exists(p)
	if baseHash != "" && existed {
		current, err := s.fs.Read(p)
		if err == nil && HashContent(current) != baseHash {
			conflict := &ConflictError{
				CurrentHash:    HashContent(current),
				CurrentContent: string(current),
			}
			if s.history != nil {
				if revs, err := s.history.Log(p, 1); err == nil && len(revs) > 0 {
					conflict.ChangedBy = revs[0].Actor
					conflict.ChangedAt = revs[0].Time
				}
			}
			return conflict
		}
	}

	if s.Rules().TrackAuthorship && actor != "" && actor != ActorExternal {
		content = string(shared.UpsertFrontmatterFields([]byte(content), [][2]string{
			{"updated", time.Now().Format(time.RFC3339)},
			{"updated_by", actor},
		}))
	}

	if err := s.fs.Write(p, []byte(content)); err != nil {
		return err
	}
	s.indexNote(p, []byte(content))
	s.record(actor, p, "save")
	if existed {
		s.bus.Publish(Event{Type: EventFileChanged, Path: p, Actor: actor})
	} else {
		s.bus.Publish(Event{Type: EventFileCreated, Path: p, Actor: actor})
		s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
	}
	return nil
}

var unsafeFilename = regexp.MustCompile(`[\\/:*?"<>|]+`)

// CreateNote creates a note according to the configured rules and
// optional template, returning the new vault path.
func (s *NoteService) CreateNote(actor string, req CreateNoteRequest) (string, error) {
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
	if rules.TrackAuthorship && actor != "" && actor != ActorExternal {
		content = string(shared.UpsertFrontmatterFields([]byte(content), [][2]string{
			{"created_by", actor},
		}))
	}

	if err := s.fs.Write(p, []byte(content)); err != nil {
		return "", err
	}
	s.indexNote(p, []byte(content))
	s.record(actor, p, "create")
	s.bus.Publish(Event{Type: EventFileCreated, Path: p, Actor: actor})
	s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
	return p, nil
}

// DeleteNote removes a note from the vault and all indexes. With
// history enabled the note stays restorable from the trash.
func (s *NoteService) DeleteNote(actor, p string) error {
	p = NormalizeNotePath(p)
	lock := s.pathLock(p)
	lock.Lock()
	defer lock.Unlock()

	if err := s.fs.Delete(p); err != nil {
		return err
	}
	s.links.Remove(p)
	s.search.Remove(p)
	s.record(actor, p, "delete")
	s.bus.Publish(Event{Type: EventFileDeleted, Path: p, Actor: actor})
	s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
	return nil
}

// RestoreNote brings a file back to its content at the given revision.
func (s *NoteService) RestoreNote(actor, p, rev string) error {
	if s.history == nil {
		return fmt.Errorf("history is disabled")
	}
	p = NormalizeNotePath(p)
	content, err := s.history.FileAt(p, rev)
	if err != nil {
		return err
	}
	lock := s.pathLock(p)
	lock.Lock()
	defer lock.Unlock()

	existed := s.fs.Exists(p)
	if err := s.fs.Write(p, content); err != nil {
		return err
	}
	s.indexNote(p, content)
	s.record(actor, p, "restore")
	if existed {
		s.bus.Publish(Event{Type: EventFileChanged, Path: p, Actor: actor})
	} else {
		s.bus.Publish(Event{Type: EventFileCreated, Path: p, Actor: actor})
		s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
	}
	return nil
}

// Trash lists restorable deleted files.
func (s *NoteService) Trash(limit int) ([]DeletedFile, error) {
	if s.history == nil {
		return nil, nil
	}
	return s.history.Deleted(limit)
}

// RestoreDeleted restores a file from the trash.
func (s *NoteService) RestoreDeleted(actor, p string) error {
	if s.history == nil {
		return fmt.Errorf("history is disabled")
	}
	deleted, err := s.history.Deleted(0)
	if err != nil {
		return err
	}
	for _, d := range deleted {
		if d.Path == p {
			return s.RestoreNote(actor, p, d.RestoreRev)
		}
	}
	return fmt.Errorf("file %q not found in trash", p)
}

// PurgeTrash permanently removes paths from the trash. Paths that are
// not in the trash are silently ignored.
func (s *NoteService) PurgeTrash(paths []string) error {
	if s.history == nil {
		return fmt.Errorf("history is disabled")
	}
	return s.history.PurgeDeleted(paths)
}

// record writes a history revision (no-op when history is disabled).
func (s *NoteService) record(actor, p, action string) {
	if s.history == nil {
		return
	}
	if actor == "" {
		actor = "local"
	}
	if err := s.history.Record(actor, p, action); err != nil {
		s.log.Warn("history record failed", "path", p, "error", err)
	}
}

// recordExternalDebounced coalesces direct file-system edit bursts.
func (s *NoteService) recordExternalDebounced(p string) {
	if s.history == nil {
		return
	}
	debounce := s.extDebounce
	if debounce <= 0 {
		debounce = 45 * time.Second
	}
	s.extMu.Lock()
	defer s.extMu.Unlock()
	if t, ok := s.extTimers[p]; ok {
		t.Reset(debounce)
		return
	}
	s.extTimers[p] = time.AfterFunc(debounce, func() {
		s.extMu.Lock()
		delete(s.extTimers, p)
		s.extMu.Unlock()
		s.record(ActorExternal, p, "save")
	})
}

// Tree returns the vault directory tree.
func (s *NoteService) Tree() (*TreeNode, error) { return s.fs.Tree() }

// Search runs a full-text query; allow filters results (nil = all).
func (s *NoteService) Search(query string, limit int, allow AllowFunc) []SearchResult {
	if limit <= 0 {
		limit = 20
	}
	results := s.search.Search(query, limit*4)
	if allow == nil {
		if len(results) > limit {
			results = results[:limit]
		}
		return results
	}
	filtered := results[:0]
	for _, r := range results {
		if allow(r.Path) {
			filtered = append(filtered, r)
			if len(filtered) == limit {
				break
			}
		}
	}
	return filtered
}

// ListNotes returns metadata for every markdown note in the vault.
func (s *NoteService) ListNotes(allow AllowFunc) ([]NoteMeta, error) {
	var out []NoteMeta
	err := s.fs.Walk(func(info FileInfo) error {
		if !info.IsDir && IsMarkdown(info.Path) {
			if allow != nil && !allow(info.Path) {
				return nil
			}
			out = append(out, metaFrom(info.Path, info, nil))
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// Recent returns the most recently modified notes.
func (s *NoteService) Recent(limit int, allow AllowFunc) ([]NoteMeta, error) {
	notes, err := s.ListNotes(allow)
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
		s.recordExternalDebounced(p)
		if op == "create" {
			s.bus.Publish(Event{Type: EventFileCreated, Path: p, Actor: ActorExternal})
			s.bus.Publish(Event{Type: EventTreeChanged, Actor: ActorExternal})
		} else {
			s.bus.Publish(Event{Type: EventFileChanged, Path: p, Actor: ActorExternal})
		}
	case "remove", "rename":
		s.links.Remove(p)
		s.search.Remove(p)
		s.record(ActorExternal, p, "delete")
		s.bus.Publish(Event{Type: EventFileDeleted, Path: p, Actor: ActorExternal})
		s.bus.Publish(Event{Type: EventTreeChanged, Actor: ActorExternal})
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
