package core_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
	"github.com/obsidianweb/obsidianweb/packages/history"
	"github.com/obsidianweb/obsidianweb/packages/links"
	"github.com/obsidianweb/obsidianweb/packages/search"
)

type nopRenderer struct{}

func (nopRenderer) Render(_ string, src []byte) (string, map[string]any, error) {
	return string(src), nil, nil
}

func newService(t *testing.T, rules core.NoteRules, withHistory bool) *core.NoteService {
	svc, _ := newServiceWithRoot(t, rules, withHistory)
	return svc
}

func newServiceWithRoot(t *testing.T, rules core.NoteRules, withHistory bool) (*core.NoteService, string) {
	t.Helper()
	vault, err := filesystem.NewVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := core.NewNoteService(vault, nopRenderer{}, links.NewIndex(), search.NewIndex(), nil, core.NewEventBus(), rules, nil)
	if withHistory {
		h, err := history.Open(vault.Root(), history.ModeManaged, nil)
		if err != nil {
			t.Fatal(err)
		}
		svc.AttachHistory(h, time.Minute)
	}
	return svc, vault.Root()
}

func TestSaveNoteConflict(t *testing.T) {
	svc := newService(t, core.NoteRules{}, true)

	if err := svc.SaveNote("igor", "Note", "first", ""); err != nil {
		t.Fatal(err)
	}
	note, err := svc.GetNote("Note", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Someone else changes the note after we loaded it.
	if err := svc.SaveNote("sev", "Note", "second", note.ContentHash); err != nil {
		t.Fatal(err)
	}

	// Our save with the stale hash must conflict and not overwrite.
	err = svc.SaveNote("igor", "Note", "mine", note.ContentHash)
	var conflict *core.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
	if conflict.CurrentContent != "second" || conflict.ChangedBy != "sev" {
		t.Errorf("conflict = %+v", conflict)
	}
	current, _ := svc.GetNote("Note", nil)
	if current.Content != "second" {
		t.Errorf("content overwritten despite conflict: %q", current.Content)
	}

	// Saving with the fresh hash succeeds.
	if err := svc.SaveNote("igor", "Note", "mine", conflict.CurrentHash); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAndRestoreFromTrash(t *testing.T) {
	svc := newService(t, core.NoteRules{}, true)

	if err := svc.SaveNote("igor", "Doc", "keep me", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNote("igor", "Doc"); err != nil {
		t.Fatal(err)
	}
	trash, err := svc.Trash(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 || trash[0].Path != "Doc.md" {
		t.Fatalf("trash = %+v", trash)
	}
	if err := svc.RestoreDeleted("igor", "Doc.md"); err != nil {
		t.Fatal(err)
	}
	note, err := svc.GetNote("Doc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if note.Content != "keep me" {
		t.Errorf("restored = %q", note.Content)
	}
}

// Restoring a revision whose content already matches must answer
// ErrRestoreUnchanged instead of silently succeeding, and restores must
// carry their source revision.
func TestRestoreUnchangedAndSourceRev(t *testing.T) {
	svc := newService(t, core.NoteRules{}, true)

	if err := svc.SaveNote("igor", "Note", "v1", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveNote("igor", "Note", "v2", ""); err != nil {
		t.Fatal(err)
	}
	revs, err := svc.History().Log("Note.md", 10)
	if err != nil {
		t.Fatal(err)
	}

	// Top revision holds the current content — nothing to restore.
	err = svc.RestoreNote("igor", "Note", revs[0].ID)
	if !errors.Is(err, core.ErrRestoreUnchanged) {
		t.Fatalf("expected ErrRestoreUnchanged, got %v", err)
	}

	// Restoring the older revision works and records where it came from.
	if err := svc.RestoreNote("igor", "Note", revs[1].ID); err != nil {
		t.Fatal(err)
	}
	note, _ := svc.GetNote("Note", nil)
	if note.Content != "v1" {
		t.Errorf("content = %q, want v1", note.Content)
	}
	revs, err = svc.History().Log("Note.md", 10)
	if err != nil {
		t.Fatal(err)
	}
	if revs[0].Action != "restore" || revs[0].SourceRev != revs[2].ID {
		t.Errorf("restore revision = %+v, want sourceRev %s", revs[0], revs[2].ID)
	}
}

// Unsaved on-disk edits (external editors) are snapshotted before a
// restore overwrites them.
func TestRestoreSnapshotsExternalEdits(t *testing.T) {
	svc, root := newServiceWithRoot(t, core.NoteRules{}, true)

	if err := svc.SaveNote("igor", "Note", "v1", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveNote("igor", "Note", "v2", ""); err != nil {
		t.Fatal(err)
	}
	revs, _ := svc.History().Log("Note.md", 10)

	// Simulate an external edit that never reached history: write to the
	// vault directly, bypassing SaveNote.
	if err := os.WriteFile(filepath.Join(root, "Note.md"), []byte("external edit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.RestoreNote("igor", "Note", revs[1].ID); err != nil {
		t.Fatal(err)
	}
	revs, _ = svc.History().Log("Note.md", 10)
	// newest → oldest: restore, external snapshot, v2, v1
	if len(revs) != 4 {
		t.Fatalf("revisions = %d: %+v", len(revs), revs)
	}
	if revs[1].Actor != core.ActorExternal {
		t.Errorf("snapshot revision = %+v", revs[1])
	}
	content, err := svc.History().FileAt("Note.md", revs[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "external edit" {
		t.Errorf("snapshot content = %q", content)
	}
}

func TestCreateFolder(t *testing.T) {
	svc := newService(t, core.NoteRules{}, false)

	p, err := svc.CreateFolder("igor", "/Projects/2026/")
	if err != nil {
		t.Fatal(err)
	}
	if p != "Projects/2026" {
		t.Errorf("path = %q, want Projects/2026", p)
	}

	tree, err := svc.Tree()
	if err != nil {
		t.Fatal(err)
	}
	var findDir func(n *core.TreeNode, path string) bool
	findDir = func(n *core.TreeNode, path string) bool {
		for _, c := range n.Children {
			if c.IsDir && (c.Path == path || findDir(c, path)) {
				return true
			}
		}
		return false
	}
	if !findDir(tree, "Projects/2026") {
		t.Error("created folder missing from tree")
	}

	if _, err := svc.CreateFolder("igor", ""); err == nil {
		t.Error("empty folder path should error")
	}
}

func TestAuthorshipFrontmatter(t *testing.T) {
	svc := newService(t, core.NoteRules{AutoFrontmatter: true, TrackAuthorship: true}, false)

	p, err := svc.CreateNote("igor", core.CreateNoteRequest{Title: "Authored"})
	if err != nil {
		t.Fatal(err)
	}
	note, _ := svc.GetNote(p, nil)
	if !strings.Contains(note.Content, `created_by: "igor"`) {
		t.Errorf("created_by missing:\n%s", note.Content)
	}

	if err := svc.SaveNote("sev", p, note.Content, ""); err != nil {
		t.Fatal(err)
	}
	note, _ = svc.GetNote(p, nil)
	if !strings.Contains(note.Content, `updated_by: "sev"`) {
		t.Errorf("updated_by missing:\n%s", note.Content)
	}
	if strings.Count(note.Content, "created_by") != 1 {
		t.Errorf("created_by duplicated:\n%s", note.Content)
	}
}
