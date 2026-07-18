// Package history records vault changes as git commits, providing
// per-file history, diffs, restore and a trash for deleted notes.
// The repository lives inside the vault (.git) — the file system stays
// the single source of truth, history is derived data (see ADR-0002).
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Mode controls whether the platform writes commits itself.
const (
	ModeManaged  = "managed"  // platform commits every change
	ModeExternal = "external" // repository is managed elsewhere; read-only
)

// Git implements core.History on a git repository at the vault root.
type Git struct {
	mu   sync.Mutex
	repo *git.Repository
	root string
	mode string
	log  *slog.Logger
}

var _ core.History = (*Git)(nil)

// Open opens (or, in managed mode, initializes) the vault repository.
func Open(root, mode string, log *slog.Logger) (*Git, error) {
	if log == nil {
		log = slog.Default()
	}
	repo, err := git.PlainOpen(root)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		if mode != ModeManaged {
			return nil, fmt.Errorf("history mode %q requires an existing git repository in the vault", mode)
		}
		repo, err = git.PlainInit(root, false)
		if err != nil {
			return nil, fmt.Errorf("init vault repository: %w", err)
		}
		g := &Git{repo: repo, root: root, mode: mode, log: log}
		if err := g.commitAll("local", "init: vault snapshot"); err != nil {
			return nil, err
		}
		if err := g.initTrashIndex(); err != nil {
			return nil, err
		}
		log.Info("vault history initialized", "root", root)
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	g := &Git{repo: repo, root: root, mode: mode, log: log}
	if mode == ModeManaged {
		// External repositories are read-only for us: no delete commits of
		// ours to index, so the trash stays empty there.
		if err := g.initTrashIndex(); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// Record captures the current state of path as a commit.
//
// It deliberately avoids go-git's Worktree.Status(), which hashes the
// entire worktree on every call (O(vault size) per save, all under
// g.mu). Instead the on-disk blob is compared against HEAD directly and
// staging uses SkipStatus.
func (g *Git) Record(actor, path, action, detail string) error {
	if g.mode != ModeManaged {
		return nil // external repositories are read-only for us
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	data, readErr := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(path)))
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	headHash, inHead := g.headBlobHash(path)
	switch {
	case !exists && !inHead:
		return nil // never committed and already gone
	case exists && inHead && plumbing.ComputeHash(plumbing.BlobObject, data) == headHash:
		return nil // unchanged since the last commit
	}
	if exists {
		if err := w.AddWithOptions(&git.AddOptions{Path: path, SkipStatus: true}); err != nil {
			return fmt.Errorf("stage %s: %w", path, err)
		}
	} else if _, err := w.Remove(path); err != nil {
		return fmt.Errorf("stage delete %s: %w", path, err)
	}
	// The parent of the commit we are about to make holds the last
	// content of a deleted file — that is what the trash restores.
	var parent plumbing.Hash
	if ref, err := g.repo.Head(); err == nil {
		parent = ref.Hash()
	}
	msg := fmt.Sprintf("%s: %s", action, path)
	if detail != "" {
		msg += "\n\n" + restoredFromTrailer + detail + "\n"
	}
	hash, err := w.Commit(msg, &git.CommitOptions{
		Author: signature(actor),
	})
	if err != nil {
		return err
	}
	g.updateTrashIndex(core.DeletedFile{
		Path:       path,
		Actor:      actor,
		Time:       time.Now(),
		RestoreRev: parent.String(),
		DeleteRev:  hash.String(),
	}, !exists)
	return nil
}

// restoredFromTrailer marks the source revision in a restore commit's
// body; the first message line keeps the "action: path" shape that
// revisionFrom parses.
const restoredFromTrailer = "Restored-From: "

// headBlobHash returns the blob hash of path in the HEAD commit.
func (g *Git) headBlobHash(path string) (plumbing.Hash, bool) {
	ref, err := g.repo.Head()
	if err != nil {
		return plumbing.ZeroHash, false
	}
	commit, err := g.repo.CommitObject(ref.Hash())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	tree, err := commit.Tree()
	if err != nil {
		return plumbing.ZeroHash, false
	}
	entry, err := tree.FindEntry(path)
	if err != nil {
		return plumbing.ZeroHash, false
	}
	return entry.Hash, true
}

// commitAll snapshots the whole worktree (used for the initial commit).
func (g *Git) commitAll(actor, message string) error {
	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	status, err := w.Status()
	if err != nil {
		return err
	}
	if status.IsClean() {
		return nil
	}
	_, err = w.Commit(message, &git.CommitOptions{Author: signature(actor)})
	return err
}

// Log lists revisions of a file, newest first.
func (g *Git) Log(path string, limit int) ([]core.Revision, error) {
	iter, err := g.repo.Log(&git.LogOptions{FileName: &path})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if limit <= 0 {
		limit = 50
	}
	var out []core.Revision
	err = iter.ForEach(func(c *object.Commit) error {
		out = append(out, revisionFrom(c))
		if len(out) >= limit {
			return errStopIteration
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, err
	}
	return out, nil
}

// FileAt returns the file content at a revision.
func (g *Git) FileAt(path, rev string) ([]byte, error) {
	commit, err := g.commitAt(rev)
	if err != nil {
		return nil, err
	}
	f, err := commit.File(path)
	if err != nil {
		return nil, fmt.Errorf("%s not present at %s: %w", path, rev, err)
	}
	content, err := f.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// Diff renders a line diff between two revisions of a file. Empty `to`
// diffs against the current on-disk content.
func (g *Git) Diff(path, from, to string) (string, error) {
	a, err := g.FileAt(path, from)
	if err != nil {
		return "", err
	}
	var b []byte
	if to == "" {
		b, err = os.ReadFile(filepath.Join(g.root, filepath.FromSlash(path)))
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	} else {
		b, err = g.FileAt(path, to)
		if err != nil {
			return "", err
		}
	}
	return lineDiff(string(a), string(b)), nil
}

// ChangesIn renders what a revision changed in the file: the diff
// between the parent commit and the revision itself.
func (g *Git) ChangesIn(path, rev string) (string, error) {
	commit, err := g.commitAt(rev)
	if err != nil {
		return "", err
	}
	var after []byte
	if f, err := commit.File(path); err == nil {
		content, err := f.Contents()
		if err != nil {
			return "", err
		}
		after = []byte(content)
	} // deleted in this revision → empty "after"

	var before []byte
	if len(commit.ParentHashes) > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return "", err
		}
		if f, err := parent.File(path); err == nil {
			content, err := f.Contents()
			if err != nil {
				return "", err
			}
			before = []byte(content)
		}
	}
	return lineDiff(string(before), string(after)), nil
}

// purgedFilePath returns the path to the legacy JSON file of purged
// trash paths. It is only consulted by the migration log scan so
// entries the user already purged do not reappear in the new index.
func (g *Git) purgedFilePath() string {
	return filepath.Join(g.root, ".git", "obsidianweb-trash-purged.json")
}

func (g *Git) loadPurged() map[string]bool {
	data, err := os.ReadFile(g.purgedFilePath())
	if err != nil {
		return map[string]bool{}
	}
	var paths []string
	if json.Unmarshal(data, &paths) != nil {
		return map[string]bool{}
	}
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return set
}

var errStopIteration = errors.New("stop")

func (g *Git) commitAt(rev string) (*object.Commit, error) {
	h, err := g.repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, fmt.Errorf("resolve revision %q: %w", rev, err)
	}
	return g.repo.CommitObject(*h)
}

func revisionFrom(c *object.Commit) core.Revision {
	msg := strings.SplitN(c.Message, "\n", 2)[0]
	action := "save"
	if idx := strings.Index(msg, ": "); idx > 0 {
		action = msg[:idx]
	}
	source := ""
	for _, line := range strings.Split(c.Message, "\n") {
		if v, ok := strings.CutPrefix(line, restoredFromTrailer); ok {
			source = strings.TrimSpace(v)
			break
		}
	}
	return core.Revision{
		ID:        c.Hash.String(),
		Actor:     c.Author.Name,
		Action:    action,
		Message:   msg,
		Time:      c.Author.When,
		SourceRev: source,
	}
}

func signature(actor string) *object.Signature {
	if actor == "" {
		actor = "local"
	}
	return &object.Signature{
		Name:  actor,
		Email: actor + "@obsidianweb.local",
		When:  time.Now(),
	}
}

// lineDiff produces a compact unified-style line diff.
func lineDiff(a, b string) string {
	dmp := diffmatchpatch.New()
	ca, cb, lines := dmp.DiffLinesToChars(a, b)
	diffs := dmp.DiffCharsToLines(dmp.DiffMain(ca, cb, false), lines)
	var sb strings.Builder
	for _, d := range diffs {
		prefix := "  "
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			prefix = "- "
		case diffmatchpatch.DiffInsert:
			prefix = "+ "
		}
		for _, line := range strings.Split(strings.TrimRight(d.Text, "\n"), "\n") {
			sb.WriteString(prefix)
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
