package filesystem

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v, err := NewVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCanonicalPath(t *testing.T) {
	v := newTestVault(t)
	p, err := v.CanonicalPath(`/Projects\\Idea.md`)
	if err != nil || p != "Projects/Idea.md" {
		t.Fatalf("CanonicalPath = %q, %v", p, err)
	}
	if _, err := v.CanonicalPath("../secret.md"); !errors.Is(err, ErrOutsideVault) {
		t.Fatalf("CanonicalPath escape error = %v, want ErrOutsideVault", err)
	}
	if _, err := v.CanonicalPath("."); err == nil {
		t.Fatal("CanonicalPath root succeeded")
	}
}

func TestControlledWriterSuppressesOnlyMatchingWatcherEvent(t *testing.T) {
	v := newTestVault(t)
	var events []string
	w, err := NewWatcher(v, slog.Default(), func(op, path string) {
		events = append(events, op+":"+path)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	writer := NewControlledWriter(v, w)
	if err := writer.Write("Note.md", []byte("sync version")); err != nil {
		t.Fatal(err)
	}
	w.emit("write", "Note.md")
	w.emit("write", "Note.md") // fsnotify may report a replacement more than once.
	if len(events) != 0 {
		t.Fatalf("matching controlled events = %v, want none", events)
	}

	if err := v.Write("Note.md", []byte("external version")); err != nil {
		t.Fatal(err)
	}
	w.emit("write", "Note.md")
	if len(events) != 1 || events[0] != "write:Note.md" {
		t.Fatalf("external overwrite events = %v", events)
	}
}

func TestControlledWriterDeleteIsDurableAndSuppressed(t *testing.T) {
	v := newTestVault(t)
	if err := v.Write("Gone.md", []byte("content")); err != nil {
		t.Fatal(err)
	}
	var events []string
	w, err := NewWatcher(v, slog.Default(), func(op, path string) {
		events = append(events, op+":"+path)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := NewControlledWriter(v, w).Delete("Gone.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "Gone.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted file stat error = %v", err)
	}
	w.emit("remove", "Gone.md")
	if len(events) != 0 {
		t.Fatalf("controlled delete events = %v, want none", events)
	}
}
