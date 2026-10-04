package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"slices"
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
	historyMu   sync.RWMutex

	lockMu sync.Mutex
	locks  map[string]*sync.Mutex

	extMu     sync.Mutex
	extTimers map[string]*time.Timer

	syncMu      sync.RWMutex
	syncJournal SyncJournal
	syncWriter  SyncWriter
	// externalSyncStates coalesces duplicate fsnotify notifications for the
	// same materialized state. Controlled sync writes are suppressed by the
	// filesystem adapter before they reach this map.
	externalSyncStates map[string]string

	// metas caches per-note metadata so listings don't walk the file
	// system on every request. Populated by ReindexAll and kept fresh by
	// the same code paths that maintain the search index.
	metaMu sync.RWMutex
	metas  map[string]NoteMeta
}

// SetSync attaches the durable journal and controlled writer after adapters
// have been constructed in the composition root.
func (s *NoteService) SetSync(journal SyncJournal, writer SyncWriter) {
	s.syncMu.Lock()
	s.syncJournal, s.syncWriter = journal, writer
	s.syncMu.Unlock()
}

// ApplySync serializes an idempotent sync mutation with ordinary note saves.
// A journal acknowledgement is returned only after the controlled vault write
// and the journal/outbox transaction have both succeeded.
func (s *NoteService) ApplySync(ctx context.Context, actor string, op SyncOperation) (SyncChange, bool, error) {
	s.syncMu.RLock()
	journal, writer := s.syncJournal, s.syncWriter
	s.syncMu.RUnlock()
	if journal == nil || writer == nil {
		return SyncChange{}, false, errors.New("sync is disabled")
	}
	if op.OpID == "" || op.DeviceID == "" {
		return SyncChange{}, false, errors.New("sync opId and deviceId are required")
	}
	if op.Kind != SyncUpsert && op.Kind != SyncDelete {
		return SyncChange{}, false, fmt.Errorf("unsupported sync operation %q", op.Kind)
	}
	p := NormalizeNotePath(op.Path)
	if p == "" {
		return SyncChange{}, false, errors.New("sync path is required")
	}
	op.Path, op.Actor = p, actor
	if existing, ok, err := journal.Operation(ctx, op.OpID); err != nil || ok {
		return existing, ok, err
	}
	lock := s.pathLock(p)
	lock.Lock()
	defer lock.Unlock()
	if existing, ok, err := journal.Operation(ctx, op.OpID); err != nil || ok {
		return existing, ok, err
	}
	before, err := s.syncFileState(p)
	if err != nil {
		return SyncChange{}, false, err
	}
	if op.BaseHash != "" && before.Exists && before.Hash != op.BaseHash {
		data, _ := s.fs.Read(p)
		return SyncChange{}, false, &ConflictError{CurrentHash: before.Hash, CurrentContent: string(data)}
	}
	after := syncStateFor(op)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err := journal.CreateIntent(ctx, SyncIntent{SyncOperation: op, BeforeState: beforeJSON, AfterState: afterJSON}); err != nil {
		return SyncChange{}, false, err
	}
	if op.Kind == SyncUpsert {
		err = writer.Write(p, []byte(op.Content))
	} else {
		err = writer.Delete(p)
	}
	if err != nil {
		return SyncChange{}, false, err
	}
	change, duplicate, err := journal.CommitOperation(ctx, op)
	if err != nil {
		return SyncChange{}, false, err
	}
	if op.Kind == SyncUpsert {
		s.indexNote(p, []byte(op.Content))
		if before.Exists {
			s.bus.Publish(Event{Type: EventFileChanged, Path: p, Actor: actor})
		} else {
			s.bus.Publish(Event{Type: EventFileCreated, Path: p, Actor: actor})
			s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
		}
	} else {
		s.links.Remove(p)
		s.search.Remove(p)
		s.removeMeta(p)
		s.bus.Publish(Event{Type: EventFileDeleted, Path: p, Actor: actor})
		s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
	}
	return change, duplicate, nil
}

// PullSync reads the ordered sync journal. Filtering for a caller's ACL is a
// transport concern: the caller must still retain the returned cursor even
// when every change on that page is hidden.
func (s *NoteService) PullSync(ctx context.Context, after int64, limit int) ([]SyncChange, int64, bool, error) {
	s.syncMu.RLock()
	journal := s.syncJournal
	s.syncMu.RUnlock()
	if journal == nil {
		return nil, after, false, errors.New("sync is disabled")
	}
	return journal.Pull(ctx, after, limit)
}

func (s *NoteService) SyncBootstrap(ctx context.Context, cursor int64, force bool) (SyncSnapshot, bool, error) {
	s.syncMu.RLock()
	journal := s.syncJournal
	s.syncMu.RUnlock()
	if journal == nil {
		return SyncSnapshot{}, false, errors.New("sync is disabled")
	}
	return journal.Bootstrap(ctx, cursor, force)
}

// ApplySyncBatch validates a complete push under every affected path lock,
// writes durable intents/files, then commits all journal and outbox rows in one
// SQLite transaction. A validation failure therefore acknowledges no operation.
func (s *NoteService) ApplySyncBatch(ctx context.Context, actor string, operations []SyncOperation) ([]SyncChange, []bool, error) {
	s.syncMu.RLock()
	journal, writer := s.syncJournal, s.syncWriter
	s.syncMu.RUnlock()
	if journal == nil || writer == nil {
		return nil, nil, errors.New("sync is disabled")
	}
	if len(operations) == 0 {
		return nil, nil, errors.New("sync operations are required")
	}
	paths := make([]string, 0, len(operations))
	seenIDs := map[string]bool{}
	for i := range operations {
		op := &operations[i]
		if op.OpID == "" || op.DeviceID == "" || seenIDs[op.OpID] {
			return nil, nil, &SyncBatchError{Operation: *op, Err: errors.New("opId/deviceId is invalid or duplicated")}
		}
		seenIDs[op.OpID] = true
		if op.Kind != SyncUpsert && op.Kind != SyncDelete {
			return nil, nil, &SyncBatchError{Operation: *op, Err: fmt.Errorf("unsupported sync operation %q", op.Kind)}
		}
		op.Path, op.Actor = NormalizeNotePath(op.Path), actor
		if op.Path == "" {
			return nil, nil, &SyncBatchError{Operation: *op, Err: errors.New("sync path is required")}
		}
		paths = append(paths, op.Path)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	locks := make([]*sync.Mutex, len(paths))
	for i, p := range paths {
		locks[i] = s.pathLock(p)
		locks[i].Lock()
	}
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
	}()
	changes := make([]SyncChange, len(operations))
	duplicates := make([]bool, len(operations))
	states := map[string]syncFileState{}
	before := make([]syncFileState, len(operations))
	for i, op := range operations {
		if existing, ok, err := journal.Operation(ctx, op.OpID); err != nil {
			return nil, nil, err
		} else if ok {
			changes[i], duplicates[i] = existing, true
			continue
		}
		state, ok := states[op.Path]
		if !ok {
			var err error
			state, err = s.syncFileState(op.Path)
			if err != nil {
				return nil, nil, err
			}
			states[op.Path] = state
		}
		if op.BaseHash != "" && state.Exists && state.Hash != op.BaseHash {
			data, _ := s.fs.Read(op.Path)
			return nil, nil, &SyncBatchError{Operation: op, Err: &ConflictError{CurrentHash: state.Hash, CurrentContent: string(data)}}
		}
		before[i] = state
		states[op.Path] = syncStateFor(op)
	}
	for i, op := range operations {
		if duplicates[i] {
			continue
		}
		beforeJSON, _ := json.Marshal(before[i])
		afterJSON, _ := json.Marshal(syncStateFor(op))
		if err := journal.CreateIntent(ctx, SyncIntent{SyncOperation: op, BeforeState: beforeJSON, AfterState: afterJSON}); err != nil {
			return nil, nil, &SyncBatchError{Operation: op, Err: err}
		}
		var err error
		if op.Kind == SyncUpsert {
			err = writer.Write(op.Path, []byte(op.Content))
		} else {
			err = writer.Delete(op.Path)
		}
		if err != nil {
			return nil, nil, &SyncBatchError{Operation: op, Err: err}
		}
	}
	committed, committedDuplicates, err := journal.CommitOperations(ctx, operations)
	if err != nil {
		return nil, nil, err
	}
	for i := range operations {
		changes[i] = committed[i]
		duplicates[i] = committedDuplicates[i]
	}
	for i, op := range operations {
		if duplicates[i] {
			continue
		}
		if op.Kind == SyncUpsert {
			s.indexNote(op.Path, []byte(op.Content))
			if before[i].Exists {
				s.bus.Publish(Event{Type: EventFileChanged, Path: op.Path, Actor: actor})
			} else {
				s.bus.Publish(Event{Type: EventFileCreated, Path: op.Path, Actor: actor})
				s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
			}
		} else {
			s.links.Remove(op.Path)
			s.search.Remove(op.Path)
			s.removeMeta(op.Path)
			s.bus.Publish(Event{Type: EventFileDeleted, Path: op.Path, Actor: actor})
			s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
		}
	}
	return changes, duplicates, nil
}

type syncFileState struct {
	Exists bool   `json:"exists"`
	Hash   string `json:"hash,omitempty"`
}

func (s *NoteService) syncFileState(p string) (syncFileState, error) {
	data, err := s.fs.Read(p)
	if errors.Is(err, ErrNotFound) {
		return syncFileState{}, nil
	}
	if err != nil {
		return syncFileState{}, err
	}
	return syncFileState{Exists: true, Hash: HashContent(data)}, nil
}
func syncStateFor(op SyncOperation) syncFileState {
	if op.Kind == SyncDelete {
		return syncFileState{}
	}
	return syncFileState{Exists: true, Hash: HashContent([]byte(op.Content))}
}

// NewNoteService wires the core service from its dependencies.
func NewNoteService(fs VaultFS, renderer Renderer, links LinkIndex, search SearchIndex, templates TemplateEngine, bus EventBus, rules NoteRules, log *slog.Logger) *NoteService {
	if log == nil {
		log = slog.Default()
	}
	return &NoteService{
		fs: fs, renderer: renderer, links: links, search: search,
		templates: templates, bus: bus, rules: rules, log: log,
		locks:              map[string]*sync.Mutex{},
		extTimers:          map[string]*time.Timer{},
		externalSyncStates: map[string]string{},
		metas:              map[string]NoteMeta{},
	}
}

// AttachHistory enables change history. externalDebounce coalesces
// bursts of direct file-system edits into single revisions.
func (s *NoteService) AttachHistory(h History, externalDebounce time.Duration) {
	s.SetHistory(h, externalDebounce)
}

// SetHistory attaches an optional history capability. It is safe to call at
// runtime, which lets a built-in plugin enable history without restarting the
// note service.
func (s *NoteService) SetHistory(h History, externalDebounce time.Duration) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.history = h
	s.extDebounce = externalDebounce
}

// DetachHistory removes h only when it is the currently attached backend.
// Timers for external filesystem edits are cancelled so disabling history
// cannot schedule a later Git write.
func (s *NoteService) DetachHistory(h History) {
	s.historyMu.Lock()
	if s.history != h {
		s.historyMu.Unlock()
		return
	}
	s.history = nil
	s.extDebounce = 0
	s.historyMu.Unlock()

	s.extMu.Lock()
	for p, timer := range s.extTimers {
		timer.Stop()
		delete(s.extTimers, p)
	}
	s.extMu.Unlock()
}

// History returns the attached history backend (nil when disabled).
func (s *NoteService) History() History {
	s.historyMu.RLock()
	defer s.historyMu.RUnlock()
	return s.history
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
			if h := s.History(); h != nil {
				if revs, err := h.Log(p, 1); err == nil && len(revs) > 0 {
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

// ResolveTargetFolder returns the folder a CreateNoteRequest will land
// in under the current rules. The API layer uses it for the pre-create
// ACL check, so the checked folder can never diverge from the one
// CreateNote actually writes to.
func (s *NoteService) ResolveTargetFolder(req CreateNoteRequest) string {
	rules := s.Rules()
	folder := strings.Trim(req.Folder, "/")
	if folder == "" && req.Type != "" {
		folder = rules.TypeFolders[req.Type]
	}
	if folder == "" {
		folder = rules.DefaultFolder
	}
	return folder
}

// CreateNote creates a note according to the configured rules and
// optional template, returning the new vault path.
func (s *NoteService) CreateNote(actor string, req CreateNoteRequest) (string, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	name := strings.TrimSpace(unsafeFilename.ReplaceAllString(title, "-"))

	rules := s.Rules()
	folder := s.ResolveTargetFolder(req)
	p := path.Join(folder, name+".md")
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	for i := 1; s.fs.Exists(p); i++ {
		p = path.Join(folder, fmt.Sprintf("%s %d.md", name, i))
	}

	content := req.Content
	if req.Template != "" && s.templates != nil {
		// currentuser comes from the authenticated actor, never from request
		// variables, so callers cannot attribute a newly created note to someone
		// else. actor is kept as a short alias for API/template compatibility.
		vars := map[string]string{"title": title, "filename": name, "currentuser": actor, "actor": actor}
		for k, v := range req.Variables {
			if k == "currentuser" || k == "actor" {
				continue
			}
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

// CreateFolder creates an (empty) directory in the vault. The path is
// cleaned and must stay inside the vault; creating an existing folder is
// a no-op (MkdirAll).
func (s *NoteService) CreateFolder(actor, p string) (string, error) {
	p = strings.TrimPrefix(path.Clean("/"+strings.Trim(p, "/")), "/")
	if p == "" || p == "." {
		return "", fmt.Errorf("folder path is required")
	}
	if err := s.fs.Mkdir(p); err != nil {
		return "", err
	}
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
	s.removeMeta(p)
	s.record(actor, p, "delete")
	s.bus.Publish(Event{Type: EventFileDeleted, Path: p, Actor: actor})
	s.bus.Publish(Event{Type: EventTreeChanged, Actor: actor})
	return nil
}

// RestoreNote brings a file back to its content at the given revision.
// When the current content already matches, nothing is written and
// ErrRestoreUnchanged is returned so the caller can say so instead of
// pretending a restore happened.
func (s *NoteService) RestoreNote(actor, p, rev string) error {
	s.historyMu.RLock()
	h := s.history
	if h == nil {
		s.historyMu.RUnlock()
		return ErrHistoryDisabled
	}
	defer s.historyMu.RUnlock()
	p = NormalizeNotePath(p)
	content, err := h.FileAt(p, rev)
	if err != nil {
		return err
	}
	lock := s.pathLock(p)
	lock.Lock()
	defer lock.Unlock()

	existed := s.fs.Exists(p)
	if existed {
		current, err := s.fs.Read(p)
		if err != nil {
			return err
		}
		if string(current) == string(content) {
			return ErrRestoreUnchanged
		}
		// Edits not yet in history (external editors, watcher debounce)
		// would be lost by the overwrite — snapshot them first. Record
		// is a no-op when the content already matches the last revision.
		s.record(ActorExternal, p, "save")
	}
	if err := s.fs.Write(p, content); err != nil {
		return err
	}
	s.indexNote(p, content)
	s.recordDetail(actor, p, "restore", rev)
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
	h := s.History()
	if h == nil {
		return nil, nil
	}
	return h.Deleted(limit)
}

// RestoreDeleted restores a file from the trash.
func (s *NoteService) RestoreDeleted(actor, p string) error {
	h := s.History()
	if h == nil {
		return ErrHistoryDisabled
	}
	deleted, err := h.Deleted(0)
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
	h := s.History()
	if h == nil {
		return ErrHistoryDisabled
	}
	return h.PurgeDeleted(paths)
}

// record writes a history revision (no-op when history is disabled).
func (s *NoteService) record(actor, p, action string) {
	s.recordDetail(actor, p, action, "")
}

// recordDetail is record with action-specific context (the source
// revision for restores).
func (s *NoteService) recordDetail(actor, p, action, detail string) {
	s.historyMu.RLock()
	defer s.historyMu.RUnlock()
	if s.history == nil {
		return
	}
	if actor == "" {
		actor = "local"
	}
	if err := s.history.Record(actor, p, action, detail); err != nil {
		s.log.Warn("history record failed", "path", p, "error", err)
	}
}

// recordExternalDebounced coalesces direct file-system edit bursts.
func (s *NoteService) recordExternalDebounced(p string) {
	s.historyMu.RLock()
	enabled := s.history != nil
	debounce := s.extDebounce
	s.historyMu.RUnlock()
	if !enabled {
		return
	}
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

// Properties lists frontmatter keys observed across the vault, for the
// settings UI and search autocomplete. allow filters per-user visibility.
func (s *NoteService) Properties(allow AllowFunc) []PropertyInfo {
	return s.search.Properties(allow)
}

// ListNotes returns metadata for every markdown note in the vault. It
// serves from the in-memory meta cache (populated by ReindexAll, kept
// fresh by saves and watcher events) instead of walking the file system
// on every request.
func (s *NoteService) ListNotes(allow AllowFunc) ([]NoteMeta, error) {
	s.metaMu.RLock()
	out := make([]NoteMeta, 0, len(s.metas))
	for _, m := range s.metas {
		if allow != nil && !allow(m.Path) {
			continue
		}
		out = append(out, m)
	}
	s.metaMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
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

// ReindexAll rebuilds link, search and meta indexes by walking the vault.
func (s *NoteService) ReindexAll() error {
	start := time.Now()
	count := 0
	seen := map[string]bool{}
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
			seen[info.Path] = true
			count++
		} else {
			s.links.RegisterFile(info.Path)
		}
		return nil
	})
	// Prune cache entries whose files disappeared while we weren't
	// looking (bulk deletes, missed watcher events).
	s.metaMu.Lock()
	for p := range s.metas {
		if !seen[p] {
			delete(s.metas, p)
		}
	}
	s.metaMu.Unlock()
	s.log.Info("vault indexed", "notes", count, "duration", time.Since(start).Round(time.Millisecond))
	s.bus.Publish(Event{Type: EventIndexUpdated})
	return err
}

// HandleFSEvent reacts to file watcher notifications and keeps indexes
// and connected clients up to date.
func (s *NoteService) HandleFSEvent(op, p string) {
	switch op {
	case "create", "write":
		var markdown []byte
		if IsMarkdown(p) {
			data, err := s.fs.Read(p)
			if err == nil {
				markdown = data
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
		if IsMarkdown(p) && markdown != nil {
			s.recordExternalSync(p, SyncUpsert, string(markdown))
		}
	case "remove", "rename":
		s.links.Remove(p)
		s.search.Remove(p)
		s.removeMeta(p)
		s.record(ActorExternal, p, "delete")
		s.bus.Publish(Event{Type: EventFileDeleted, Path: p, Actor: ActorExternal})
		s.bus.Publish(Event{Type: EventTreeChanged, Actor: ActorExternal})
		if IsMarkdown(p) {
			s.recordExternalSync(p, SyncDelete, "")
		}
	}
	s.bus.Publish(Event{Type: EventIndexUpdated})
}

// recordExternalSync turns a direct filesystem change into the same ordered
// journal stream used by API-originated changes. fsnotify commonly reports a
// create followed by write for one save, hence the last-state guard.
func (s *NoteService) recordExternalSync(p string, kind SyncOperationKind, content string) {
	s.syncMu.RLock()
	journal := s.syncJournal
	s.syncMu.RUnlock()
	if journal == nil {
		return
	}
	state := string(kind)
	if kind == SyncUpsert {
		state += ":" + HashContent([]byte(content))
	}
	lock := s.pathLock(p)
	lock.Lock()
	defer lock.Unlock()
	s.syncMu.RLock()
	if s.externalSyncStates[p] == state {
		s.syncMu.RUnlock()
		return
	}
	s.syncMu.RUnlock()
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		s.log.Warn("sync external operation id", "path", p, "error", err)
		return
	}
	_, _, err := journal.CommitOperation(context.Background(), SyncOperation{
		OpID: hex.EncodeToString(id[:]), Actor: ActorExternal, DeviceID: "filesystem",
		Path: p, Kind: kind, Content: content, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		s.log.Warn("journal external filesystem change", "path", p, "error", err)
		return
	}
	s.syncMu.Lock()
	s.externalSyncStates[p] = state
	s.syncMu.Unlock()
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
	meta := NoteMeta{Path: p, Title: title, Tags: tags, Aliases: aliases, Size: int64(len(data))}
	if info, err := s.fs.Stat(p); err == nil {
		meta.ModTime = info.ModTime
		meta.Size = info.Size
	} else {
		meta.ModTime = time.Now()
	}
	s.metaMu.Lock()
	s.metas[p] = meta
	s.metaMu.Unlock()
}

func (s *NoteService) removeMeta(p string) {
	s.metaMu.Lock()
	delete(s.metas, p)
	s.metaMu.Unlock()
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
