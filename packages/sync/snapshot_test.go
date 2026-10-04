package sync

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
)

func TestSnapshotAndCompactionPreserveState(t *testing.T) {
	ctx := context.Background()
	vault, err := filesystem.NewVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Write("Before.md", []byte("baseline")); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureInitialSnapshot(ctx, vault); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CommitOperation(ctx, core.SyncOperation{OpID: "one", Actor: "ada", DeviceID: "d", Path: "Before.md", Kind: core.SyncUpsert, Content: "updated"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CommitOperation(ctx, core.SyncOperation{OpID: "two", Actor: "ada", DeviceID: "d", Path: "New.md", Kind: core.SyncUpsert, Content: "new"}); err != nil {
		t.Fatal(err)
	}
	snapshot, changed, err := s.Compact(ctx)
	if err != nil || !changed {
		t.Fatalf("compact changed=%v err=%v", changed, err)
	}
	if snapshot.BaseCursor != 2 || snapshot.State["Before.md"] != "updated" || snapshot.State["New.md"] != "new" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	bootstrap, required, err := s.Bootstrap(ctx, 0, false)
	if err != nil || !required || bootstrap.BaseCursor != 2 || bootstrap.Notes["New.md"] != "new" {
		t.Fatalf("bootstrap=%#v required=%v err=%v", bootstrap, required, err)
	}
	if _, required, err := s.Bootstrap(ctx, 2, false); err != nil || required {
		t.Fatalf("current cursor bootstrap required=%v err=%v", required, err)
	}
	changes, next, more, err := s.Pull(ctx, 0, 10)
	if err != nil || len(changes) != 0 || next != 0 || more {
		t.Fatalf("compacted pull=%#v next=%d more=%v err=%v", changes, next, more, err)
	}
}
