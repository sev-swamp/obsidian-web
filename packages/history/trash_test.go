package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

func deleteFile(t *testing.T, root string, g *Git, actor, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	if err := g.Record(actor, rel, "delete", ""); err != nil {
		t.Fatal(err)
	}
}

// The index has no walk horizon: a deletion stays in the trash no
// matter how many commits pile up afterwards.
func TestTrashSurvivesManyLaterCommits(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Old.md", "keep me")
	if err := g.Record("igor", "Old.md", "create", ""); err != nil {
		t.Fatal(err)
	}
	deleteFile(t, root, g, "igor", "Old.md")

	for i := 0; i < 30; i++ {
		write(t, root, "Busy.md", fmt.Sprintf("v%d", i))
		if err := g.Record("igor", "Busy.md", "save", ""); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := g.Deleted(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Path != "Old.md" {
		t.Fatalf("deleted = %+v", deleted)
	}
	if deleted[0].DeleteRev == "" || deleted[0].RestoreRev == "" {
		t.Fatalf("missing revs: %+v", deleted[0])
	}
	content, err := g.FileAt("Old.md", deleted[0].RestoreRev)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "keep me" {
		t.Errorf("content = %q", content)
	}
}

// Purge → recreate same name → delete: the second deletion must show up
// in the trash (the old purged-list keyed by bare path hid it forever).
func TestPurgeThenRecreateThenDelete(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Name.md", "first life")
	if err := g.Record("igor", "Name.md", "create", ""); err != nil {
		t.Fatal(err)
	}
	deleteFile(t, root, g, "igor", "Name.md")
	if err := g.PurgeDeleted([]string{"Name.md"}); err != nil {
		t.Fatal(err)
	}
	if deleted, _ := g.Deleted(0); len(deleted) != 0 {
		t.Fatalf("after purge: %+v", deleted)
	}

	write(t, root, "Name.md", "second life")
	if err := g.Record("igor", "Name.md", "create", ""); err != nil {
		t.Fatal(err)
	}
	deleteFile(t, root, g, "igor", "Name.md")

	deleted, err := g.Deleted(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Path != "Name.md" {
		t.Fatalf("second deletion hidden: %+v", deleted)
	}
	content, err := g.FileAt("Name.md", deleted[0].RestoreRev)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "second life" {
		t.Errorf("content = %q", content)
	}
}

// Recreating a deleted file (create/save/restore commit) drops its
// trash entry.
func TestRecreateRemovesTrashEntry(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Back.md", "v1")
	if err := g.Record("igor", "Back.md", "create", ""); err != nil {
		t.Fatal(err)
	}
	deleteFile(t, root, g, "igor", "Back.md")
	write(t, root, "Back.md", "v2")
	if err := g.Record("igor", "Back.md", "create", ""); err != nil {
		t.Fatal(err)
	}

	deleted, err := g.Deleted(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("entry survived recreation: %+v", deleted)
	}
}

// Opening a repository without an index migrates it from the git log,
// honouring the legacy purged list.
func TestTrashIndexMigration(t *testing.T) {
	root, g := newVault(t)

	for _, name := range []string{"A.md", "B.md"} {
		write(t, root, name, "content of "+name)
		if err := g.Record("igor", name, "create", ""); err != nil {
			t.Fatal(err)
		}
		deleteFile(t, root, g, "igor", name)
	}

	// Simulate a pre-index installation: no index, B.md already purged
	// through the legacy list.
	if err := os.Remove(g.trashFilePath()); err != nil {
		t.Fatal(err)
	}
	purged, _ := json.Marshal([]string{"B.md"})
	if err := os.WriteFile(g.purgedFilePath(), purged, 0o644); err != nil {
		t.Fatal(err)
	}

	g2, err := Open(root, ModeManaged, nil)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := g2.Deleted(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Path != "A.md" {
		t.Fatalf("migrated trash = %+v", deleted)
	}
	if deleted[0].RestoreRev == "" || deleted[0].DeleteRev == "" {
		t.Fatalf("migration lost revs: %+v", deleted[0])
	}
}

// Opening validates the index: entries whose file is back on disk or
// whose revision no longer resolves are dropped.
func TestTrashIndexValidationOnOpen(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Real.md", "real")
	if err := g.Record("igor", "Real.md", "create", ""); err != nil {
		t.Fatal(err)
	}
	deleteFile(t, root, g, "igor", "Real.md")

	entries := g.loadTrash()
	entries = append(entries,
		core.DeletedFile{Path: "Note.md", RestoreRev: entries[0].RestoreRev},                        // file exists on disk
		core.DeletedFile{Path: "Ghost.md", RestoreRev: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"}, // bogus revision
	)
	if err := g.saveTrash(entries); err != nil {
		t.Fatal(err)
	}

	g2, err := Open(root, ModeManaged, nil)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := g2.Deleted(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Path != "Real.md" {
		t.Fatalf("validation kept bad entries: %+v", deleted)
	}
}

// A restore commit records its source revision; Log surfaces it as
// SourceRev.
func TestRestoredFromTrailer(t *testing.T) {
	root, g := newVault(t)

	write(t, root, "Note.md", "v2")
	if err := g.Record("igor", "Note.md", "save", ""); err != nil {
		t.Fatal(err)
	}
	revs, err := g.Log("Note.md", 10)
	if err != nil {
		t.Fatal(err)
	}
	source := revs[1].ID

	write(t, root, "Note.md", "v1")
	if err := g.Record("igor", "Note.md", "restore", source); err != nil {
		t.Fatal(err)
	}

	revs, err = g.Log("Note.md", 10)
	if err != nil {
		t.Fatal(err)
	}
	if revs[0].Action != "restore" {
		t.Fatalf("action = %q", revs[0].Action)
	}
	if revs[0].SourceRev != source {
		t.Fatalf("sourceRev = %q, want %q", revs[0].SourceRev, source)
	}
	if revs[1].SourceRev != "" {
		t.Fatalf("plain revision has sourceRev %q", revs[1].SourceRev)
	}
}
