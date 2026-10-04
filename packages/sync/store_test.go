package sync

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

func TestJournalCommitIsIdempotentAndPullIsOrdered(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	op1 := core.SyncOperation{OpID: "one", Actor: "ada", DeviceID: "phone", Path: "Inbox/a.md", Kind: core.SyncUpsert, Content: "one"}
	change, duplicate, err := s.CommitOperation(ctx, op1)
	if err != nil || duplicate || change.AccountSeq != 1 {
		t.Fatalf("first commit = %#v duplicate=%v err=%v", change, duplicate, err)
	}
	change, duplicate, err = s.CommitOperation(ctx, op1)
	if err != nil || !duplicate || change.AccountSeq != 1 {
		t.Fatalf("duplicate commit = %#v duplicate=%v err=%v", change, duplicate, err)
	}
	_, _, err = s.CommitOperation(ctx, core.SyncOperation{OpID: "two", Actor: "ada", DeviceID: "laptop", Path: "Inbox/b.md", Kind: core.SyncUpsert, Content: "two"})
	if err != nil {
		t.Fatal(err)
	}
	changes, next, more, err := s.Pull(ctx, 0, 1)
	if err != nil || !more || next != 1 || len(changes) != 1 || changes[0].OpID != "one" {
		t.Fatalf("pull page = %#v next=%d more=%v err=%v", changes, next, more, err)
	}
	changes, next, more, err = s.Pull(ctx, next, 10)
	if err != nil || more || next != 2 || len(changes) != 1 || changes[0].OpID != "two" {
		t.Fatalf("pull tail = %#v next=%d more=%v err=%v", changes, next, more, err)
	}
}

func TestJournalIntentLifecycle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	in := core.SyncIntent{SyncOperation: core.SyncOperation{OpID: "intent", Actor: "ada", DeviceID: "phone", Path: "a.md", Kind: core.SyncUpsert, Content: "new"}, BeforeState: []byte(`{"exists":true}`), AfterState: []byte(`{"exists":true,"hash":"x"}`)}
	if err := s.CreateIntent(ctx, in); err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingIntents(ctx)
	if err != nil || len(pending) != 1 || pending[0].OpID != "intent" {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	if err := s.MarkIntentFailed(ctx, "intent", "unexpected state"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Intent(ctx, "intent")
	if err != nil || !ok || got.State != "failed" || got.Failure == "" {
		t.Fatalf("failed intent = %#v ok=%v err=%v", got, ok, err)
	}
	if _, _, err := s.CommitOperation(ctx, in.SyncOperation); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Intent(ctx, "intent"); err != nil || ok {
		t.Fatalf("intent after commit ok=%v err=%v", ok, err)
	}
}

func TestPublisherDrainsOutbox(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err := s.CommitOperation(context.Background(), core.SyncOperation{OpID: "publish", Actor: "ada", DeviceID: "phone", Path: "a.md", Kind: core.SyncUpsert, Content: "x"}); err != nil {
		t.Fatal(err)
	}
	bus := core.NewEventBus()
	events := make(chan core.Event, 1)
	bus.Subscribe(func(e core.Event) { events <- e })
	if err := NewPublisher(s, bus, nil).Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.Type != core.EventSyncCursor || event.Cursor != 1 || event.Path != "a.md" {
			t.Fatalf("event = %#v", event)
		}
	default:
		t.Fatal("no cursor signal")
	}
	remaining, err := s.Unpublished(context.Background(), 10)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("remaining = %#v, %v", remaining, err)
	}
}
