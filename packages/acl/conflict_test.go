package acl

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSaveDetectsExternalEdit: an API write must not clobber manual
// users.yaml edits made after the store was loaded.
func TestSaveDetectsExternalEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.yaml")
	if err := os.WriteFile(path, []byte("users:\n  - username: alice\n    role: editor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Someone edits the file by hand behind the store's back.
	edited := []byte("users:\n  - username: alice\n    role: admin\n")
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	// Force a distinct mtime even on coarse-grained file systems.
	if err := os.Chtimes(path, time.Now(), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	err = store.UpsertUser(UserRecord{Username: "bob", Role: "viewer"})
	if !errors.Is(err, ErrConcurrentEdit) {
		t.Fatalf("expected ErrConcurrentEdit, got %v", err)
	}
	// The manual edit must survive.
	data, _ := os.ReadFile(path)
	if string(data) != string(edited) {
		t.Fatalf("manual edit clobbered: %s", data)
	}

	// Reload picks up the edit and saving works again.
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(UserRecord{Username: "bob", Role: "viewer"}); err != nil {
		t.Fatalf("save after reload: %v", err)
	}
	if u, ok := store.User("alice"); !ok || u.Role != "admin" {
		t.Fatalf("manual edit lost after reload: %+v", u)
	}
}

// TestSaveCreatesMissingFile: the very first Save may create the file.
func TestSaveCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.yaml")
	store, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(UserRecord{Username: "alice", Role: "editor"}); err != nil {
		t.Fatal(err)
	}
	// Subsequent saves keep working (fingerprint updated after write).
	if err := store.UpsertUser(UserRecord{Username: "bob", Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
}

func TestUserBySubject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.yaml")
	store, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(UserRecord{Username: "alice", Role: "editor", OIDCSubject: "idp-123"}); err != nil {
		t.Fatal(err)
	}
	if u, ok := store.UserBySubject("idp-123"); !ok || u.Username != "alice" {
		t.Fatalf("lookup by subject failed: %+v ok=%v", u, ok)
	}
	if _, ok := store.UserBySubject("unknown"); ok {
		t.Fatal("unknown subject must not match")
	}
	if _, ok := store.UserBySubject(""); ok {
		t.Fatal("empty subject must never match")
	}
}
