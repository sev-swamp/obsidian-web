package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// The trash is an explicit index in .git/obsidianweb-trash.json (never
// committed, like the purged list before it). Record maintains it on
// every delete/restore commit, so listing the trash is a file read
// instead of a log scan — deletions never age out of a walk horizon
// (see ADR-0005).

// trashFilePath returns the on-disk location of the trash index.
func (g *Git) trashFilePath() string {
	return filepath.Join(g.root, ".git", "obsidianweb-trash.json")
}

// loadTrash reads the index. Callers must hold g.mu.
func (g *Git) loadTrash() []core.DeletedFile {
	data, err := os.ReadFile(g.trashFilePath())
	if err != nil {
		return nil
	}
	var entries []core.DeletedFile
	if json.Unmarshal(data, &entries) != nil {
		return nil
	}
	return entries
}

// saveTrash writes the index atomically (tmp + rename). Callers must
// hold g.mu.
func (g *Git) saveTrash(entries []core.DeletedFile) error {
	if entries == nil {
		entries = []core.DeletedFile{}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	tmp := g.trashFilePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, g.trashFilePath())
}

// updateTrashIndex keeps the index in sync with the commit Record just
// made: a delete adds (or replaces) the path's entry, any other action
// means the file exists again and drops it. Callers must hold g.mu.
func (g *Git) updateTrashIndex(entry core.DeletedFile, deleted bool) {
	entries := g.loadTrash()
	kept := entries[:0]
	for _, e := range entries {
		if e.Path != entry.Path {
			kept = append(kept, e)
		}
	}
	if deleted {
		kept = append(kept, entry)
	}
	if err := g.saveTrash(kept); err != nil {
		g.log.Warn("trash index write failed", "path", entry.Path, "error", err)
	}
}

// initTrashIndex brings the index up on Open: a missing file triggers a
// one-time migration from the git log, an existing one is validated so
// entries broken by out-of-band repository edits are dropped.
func (g *Git) initTrashIndex() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, err := os.Stat(g.trashFilePath()); errors.Is(err, os.ErrNotExist) {
		entries, err := g.scanDeleted()
		if err != nil {
			return err
		}
		if err := g.saveTrash(entries); err != nil {
			return err
		}
		if len(entries) > 0 {
			g.log.Info("trash index migrated from git log", "entries", len(entries))
		}
		return nil
	}
	entries := g.loadTrash()
	valid := entries[:0]
	for _, e := range entries {
		if g.validTrashEntry(e) {
			valid = append(valid, e)
		}
	}
	if len(valid) != len(entries) {
		g.log.Warn("trash index: dropped invalid entries", "dropped", len(entries)-len(valid))
		return g.saveTrash(valid)
	}
	return nil
}

// validTrashEntry reports whether a trash entry still describes a
// restorable deletion: the file is absent from the worktree and its
// content is reachable at restoreRev.
func (g *Git) validTrashEntry(e core.DeletedFile) bool {
	if e.Path == "" || e.RestoreRev == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(g.root, filepath.FromSlash(e.Path))); err == nil {
		return false // file is back — not deleted anymore
	}
	commit, err := g.repo.CommitObject(plumbing.NewHash(e.RestoreRev))
	if err != nil {
		return false
	}
	if _, err := commit.File(e.Path); err != nil {
		return false
	}
	return true
}

// scanMaxWalk bounds the one-time migration walk over the git log.
// Deletions older than this horizon predate the index and are lost to
// it — the index itself has no horizon once populated.
const scanMaxWalk = 2000

// scanDeleted is the legacy log scan, kept only to seed the index on
// migration. Callers must hold g.mu.
func (g *Git) scanDeleted() ([]core.DeletedFile, error) {
	if _, err := g.repo.Head(); err != nil {
		return nil, nil // no commits yet (empty vault) — nothing to scan
	}
	iter, err := g.repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	purged := g.loadPurged()
	walked := 0
	seen := map[string]bool{}
	var out []core.DeletedFile
	err = iter.ForEach(func(c *object.Commit) error {
		walked++
		if walked > scanMaxWalk {
			return errStopIteration
		}
		msg := strings.SplitN(c.Message, "\n", 2)[0]
		path, ok := strings.CutPrefix(msg, "delete: ")
		if !ok || seen[path] || purged[path] {
			return nil
		}
		seen[path] = true
		// Still deleted? A later create would have brought it back.
		if _, statErr := os.Stat(filepath.Join(g.root, filepath.FromSlash(path))); statErr == nil {
			return nil
		}
		if len(c.ParentHashes) == 0 {
			return nil
		}
		out = append(out, core.DeletedFile{
			Path:       path,
			Actor:      c.Author.Name,
			Time:       c.Author.When,
			RestoreRev: c.ParentHashes[0].String(),
			DeleteRev:  c.Hash.String(),
		})
		return nil
	})
	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, err
	}
	return out, nil
}

// Deleted lists trash entries from the index, newest first.
func (g *Git) Deleted(limit int) ([]core.DeletedFile, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	entries := g.loadTrash()
	// Self-heal: a file recreated outside Record (external tools during
	// the watcher debounce window) is no longer deleted.
	valid := entries[:0]
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(g.root, filepath.FromSlash(e.Path))); err == nil {
			continue
		}
		valid = append(valid, e)
	}
	if len(valid) != len(entries) {
		if err := g.saveTrash(valid); err != nil {
			g.log.Warn("trash index write failed", "error", err)
		}
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Time.After(valid[j].Time) })
	if limit > 0 && len(valid) > limit {
		valid = valid[:limit]
	}
	out := make([]core.DeletedFile, len(valid))
	copy(out, valid)
	return out, nil
}

// PurgeDeleted removes the given paths from the trash index. The
// underlying content stays in git history — this only takes the entry
// out of the trash. Paths not present are silently ignored.
func (g *Git) PurgeDeleted(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	drop := make(map[string]bool, len(paths))
	for _, p := range paths {
		drop[p] = true
	}
	entries := g.loadTrash()
	kept := entries[:0]
	for _, e := range entries {
		if !drop[e.Path] {
			kept = append(kept, e)
		}
	}
	return g.saveTrash(kept)
}
