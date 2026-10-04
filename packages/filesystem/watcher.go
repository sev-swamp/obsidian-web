package filesystem

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const writeDebounce = 250 * time.Millisecond
const suppressionWindow = 2 * time.Second

type suppression struct {
	expected FileState
	expires  time.Time
}

// Watcher observes the vault directory recursively via fsnotify and
// reports debounced, vault-relative events.
type Watcher struct {
	vault   *Vault
	fsw     *fsnotify.Watcher
	onEvent func(op, path string)
	log     *slog.Logger

	mu         sync.Mutex
	timers     map[string]*time.Timer
	suppressed map[string]suppression
	done       chan struct{}
}

// NewWatcher creates a watcher; onEvent receives ("create"|"write"|"remove"|"rename", relPath).
func NewWatcher(vault *Vault, log *slog.Logger, onEvent func(op, path string)) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{
		vault:      vault,
		fsw:        fsw,
		onEvent:    onEvent,
		log:        log,
		timers:     map[string]*time.Timer{},
		suppressed: map[string]suppression{},
		done:       make(chan struct{}),
	}, nil
}

// Suppress registers a state-verified, short-lived marker for a controlled
// write. Only a watcher event whose on-disk state matches exactly is ignored.
// The cancellation function is used when the corresponding write failed.
func (w *Watcher) Suppress(rel string, expected FileState) func() {
	w.mu.Lock()
	w.suppressed[rel] = suppression{expected: expected, expires: time.Now().Add(suppressionWindow)}
	w.mu.Unlock()
	return func() {
		w.mu.Lock()
		delete(w.suppressed, rel)
		w.mu.Unlock()
	}
}

// Start registers all vault directories and begins the event loop.
func (w *Watcher) Start() error {
	if err := w.addRecursive(w.vault.Root()); err != nil {
		return err
	}
	go w.loop()
	return nil
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	close(w.done)
	return w.fsw.Close()
}

func (w *Watcher) addRecursive(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != w.vault.Root() && hidden(d.Name()) {
				return filepath.SkipDir
			}
			if err := w.fsw.Add(p); err != nil {
				w.log.Warn("watch failed", "dir", p, "error", err)
			}
		}
		return nil
	})
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.log.Warn("watcher error", "error", err)
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(ev)
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	rel, ok := w.vault.Rel(ev.Name)
	if !ok || rel == "." || hiddenPath(rel) {
		return
	}
	switch {
	case ev.Op.Has(fsnotify.Create):
		// New directories must be watched too.
		if info, err := w.vault.Stat(rel); err == nil && info.IsDir {
			_ = w.addRecursive(ev.Name)
			w.emit("create", rel)
			return
		}
		w.emit("create", rel)
	case ev.Op.Has(fsnotify.Write):
		w.debounced(rel)
	case ev.Op.Has(fsnotify.Remove):
		w.emit("remove", rel)
	case ev.Op.Has(fsnotify.Rename):
		w.emit("rename", rel)
	}
}

// debounced coalesces rapid successive writes (editors save in bursts).
func (w *Watcher) debounced(rel string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.timers[rel]; ok {
		t.Reset(writeDebounce)
		return
	}
	w.timers[rel] = time.AfterFunc(writeDebounce, func() {
		w.mu.Lock()
		delete(w.timers, rel)
		w.mu.Unlock()
		w.emit("write", rel)
	})
}

func (w *Watcher) emit(op, rel string) {
	if w.matchesSuppression(rel) {
		return
	}
	if w.onEvent != nil {
		w.onEvent(op, rel)
	}
}

func (w *Watcher) matchesSuppression(rel string) bool {
	w.mu.Lock()
	entry, ok := w.suppressed[rel]
	if !ok {
		w.mu.Unlock()
		return false
	}
	if time.Now().After(entry.expires) {
		delete(w.suppressed, rel)
		w.mu.Unlock()
		return false
	}
	w.mu.Unlock()

	actual := FileState{}
	if data, err := w.vault.Read(rel); err == nil {
		sum := sha256.Sum256(data)
		actual = FileState{Exists: true, Hash: fmt.Sprintf("%x", sum)}
	}
	if actual == entry.expected {
		return true
	}

	// A differently shaped event at the same path is evidence that another
	// writer won the race. It must be surfaced as external, not hidden until
	// the timeout expires.
	w.mu.Lock()
	delete(w.suppressed, rel)
	w.mu.Unlock()
	return false
}

func hiddenPath(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if hidden(seg) {
			return true
		}
	}
	return false
}
