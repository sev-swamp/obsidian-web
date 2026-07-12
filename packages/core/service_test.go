package core_test

import (
	"errors"
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
	return svc
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
