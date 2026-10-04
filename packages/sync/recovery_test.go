package sync

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
)

func TestRecoverIntentStates(t *testing.T) {
	ctx := context.Background()
	vault, err := filesystem.NewVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := func(content string, exists bool) []byte {
		s := fileState{Exists: exists}
		if exists {
			s.Hash = core.HashContent([]byte(content))
		}
		b, _ := json.Marshal(s)
		return b
	}
	makeIntent := func(id, path, before, after string) core.SyncIntent {
		return core.SyncIntent{SyncOperation: core.SyncOperation{OpID: id, Actor: "ada", DeviceID: "phone", Path: path, Kind: core.SyncUpsert, Content: after}, BeforeState: state(before, before != ""), AfterState: state(after, true)}
	}
	// The write happened before the process died: finalise it.
	if err := vault.Write("new.md", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIntent(ctx, makeIntent("new", "new.md", "", "new")); err != nil {
		t.Fatal(err)
	}
	// The write never happened: discard so the client can retry.
	if err := vault.Write("old.md", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIntent(ctx, makeIntent("old", "old.md", "old", "desired")); err != nil {
		t.Fatal(err)
	}
	// A third party changed this path: surface a failed intent.
	if err := vault.Write("race.md", []byte("external")); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIntent(ctx, makeIntent("race", "race.md", "old", "desired")); err != nil {
		t.Fatal(err)
	}
	if err := Recover(ctx, store, vault); err != nil {
		t.Fatal(err)
	}
	changes, _, _, err := store.Pull(ctx, 0, 10)
	if err != nil || len(changes) != 1 || changes[0].OpID != "new" {
		t.Fatalf("changes = %#v, %v", changes, err)
	}
	if _, ok, err := store.Intent(ctx, "old"); err != nil || ok {
		t.Fatalf("old intent ok=%v err=%v", ok, err)
	}
	race, ok, err := store.Intent(ctx, "race")
	if err != nil || !ok || race.State != "failed" {
		t.Fatalf("race intent = %#v ok=%v err=%v", race, ok, err)
	}
}
