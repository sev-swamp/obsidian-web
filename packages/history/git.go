// Package history records vault changes as git commits, providing
// per-file history, diffs, restore and a trash for deleted notes.
// The repository lives inside the vault (.git) — the file system stays
// the single source of truth, history is derived data (see ADR-0002).
package history

import (
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
		log.Info("vault history initialized", "root", root)
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	return &Git{repo: repo, root: root, mode: mode, log: log}, nil
}

// Record captures the current state of path as a commit.
func (g *Git) Record(actor, path, action string) error {
	if g.mode != ModeManaged {
		return nil // external repositories are read-only for us
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := w.Add(path); err != nil {
		// Deleted files: modern go-git stages deletions via Add, older
		// versions need Remove. Try both before giving up.
		if _, rmErr := w.Remove(path); rmErr != nil {
			return fmt.Errorf("stage %s: %w", path, err)
		}
	}
	status, err := w.Status()
	if err != nil {
		return err
	}
	// A clean file is absent from the status map (Status.File would
	// fabricate an Untracked entry) — skip to avoid empty commits.
	fs, changed := status[path]
	if !changed || (fs.Staging == git.Unmodified && fs.Worktree == git.Unmodified) {
		return nil
	}
	_, err = w.Commit(fmt.Sprintf("%s: %s", action, path), &git.CommitOptions{
		Author: signature(actor),
	})
	return err
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

// Deleted lists files removed through the platform, newest first.
func (g *Git) Deleted(limit int) ([]core.DeletedFile, error) {
	iter, err := g.repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if limit <= 0 {
		limit = 100
	}
	const maxWalk = 2000
	walked := 0
	seen := map[string]bool{}
	var out []core.DeletedFile
	err = iter.ForEach(func(c *object.Commit) error {
		walked++
		if walked > maxWalk || len(out) >= limit {
			return errStopIteration
		}
		msg := strings.SplitN(c.Message, "\n", 2)[0]
		path, ok := strings.CutPrefix(msg, "delete: ")
		if !ok || seen[path] {
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
		})
		return nil
	})
	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, err
	}
	return out, nil
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
	return core.Revision{
		ID:      c.Hash.String(),
		Actor:   c.Author.Name,
		Action:  action,
		Message: msg,
		Time:    c.Author.When,
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
