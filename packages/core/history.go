package core

import (
	"fmt"
	"time"
)

// Revision is a recorded change of a vault file.
type Revision struct {
	ID      string    `json:"id"`
	Actor   string    `json:"actor"`
	Action  string    `json:"action"` // save | create | delete | restore | external | init
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
	// SourceRev is the revision a restore was taken from (restore
	// revisions only).
	SourceRev string `json:"sourceRev,omitempty"`
}

// DeletedFile is a trash entry: a note removed from the vault that can
// be restored from history.
type DeletedFile struct {
	Path       string    `json:"path"`
	Actor      string    `json:"actor"`
	Time       time.Time `json:"time"`
	RestoreRev string    `json:"restoreRev"` // revision holding the last content
	DeleteRev  string    `json:"deleteRev"`  // revision that removed the file
}

// History records and serves per-file change history. Implementations
// must be safe for concurrent use. A nil History disables the feature.
type History interface {
	// Record captures the current state of path as a revision. detail
	// carries action-specific context (the source revision for
	// "restore"); empty for ordinary actions.
	Record(actor, path, action, detail string) error
	// Log lists revisions of a file, newest first.
	Log(path string, limit int) ([]Revision, error)
	// FileAt returns the file content at a revision.
	FileAt(path, rev string) ([]byte, error)
	// Diff renders a line diff of the file between two revisions;
	// empty `to` means the current on-disk content.
	Diff(path, from, to string) (string, error)
	// ChangesIn renders what the given revision changed in the file
	// (diff against its parent; the first revision diffs against empty).
	ChangesIn(path, rev string) (string, error)
	// Deleted lists files removed from the vault, newest first.
	Deleted(limit int) ([]DeletedFile, error)
	// PurgeDeleted permanently removes the given paths from the trash so
	// they no longer appear in Deleted results. An empty slice is a no-op.
	PurgeDeleted(paths []string) error
}

// ErrRestoreUnchanged is returned by RestoreNote when the current
// content already matches the requested revision, so nothing was
// written. The API reports it as {status: "unchanged"} instead of
// silently answering "restored".
var ErrRestoreUnchanged = fmt.Errorf("content already matches the revision")

// ConflictError is returned by SaveNote when the note changed since the
// client loaded it (optimistic locking).
type ConflictError struct {
	CurrentHash    string    `json:"currentHash"`
	CurrentContent string    `json:"currentContent"`
	ChangedBy      string    `json:"changedBy,omitempty"`
	ChangedAt      time.Time `json:"changedAt,omitempty"`
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("note was modified concurrently (by %s)", e.ChangedBy)
}
