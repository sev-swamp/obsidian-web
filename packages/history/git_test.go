package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newVault(t *testing.T) (string, *Git) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "Note.md", "v1")
	g, err := Open(root, ModeManaged, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root, g
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecordLogAndFileAt(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Note.md", "v2")
	if err := g.Record("igor", "Note.md", "save", ""); err != nil {
		t.Fatal(err)
	}
	write(t, root, "Note.md", "v3")
	if err := g.Record("sev", "Note.md", "save", ""); err != nil {
		t.Fatal(err)
	}
	// No change → no new revision.
	if err := g.Record("sev", "Note.md", "save", ""); err != nil {
		t.Fatal(err)
	}

	revs, err := g.Log("Note.md", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 3 { // init + 2 saves
		t.Fatalf("revisions = %d, want 3: %+v", len(revs), revs)
	}
	if revs[0].Actor != "sev" || revs[1].Actor != "igor" {
		t.Errorf("actors = %s, %s", revs[0].Actor, revs[1].Actor)
	}

	old, err := g.FileAt("Note.md", revs[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != "v2" {
		t.Errorf("FileAt = %q, want v2", old)
	}

	diff, err := g.Diff("Note.md", revs[1].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "- v2") || !strings.Contains(diff, "+ v3") {
		t.Errorf("diff:\n%s", diff)
	}
}

func TestDeletedAndRestoreRev(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Gone.md", "precious")
	if err := g.Record("igor", "Gone.md", "create", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "Gone.md")); err != nil {
		t.Fatal(err)
	}
	if err := g.Record("igor", "Gone.md", "delete", ""); err != nil {
		t.Fatal(err)
	}

	deleted, err := g.Deleted(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Path != "Gone.md" {
		t.Fatalf("deleted = %+v", deleted)
	}
	content, err := g.FileAt("Gone.md", deleted[0].RestoreRev)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "precious" {
		t.Errorf("restored content = %q", content)
	}
}
