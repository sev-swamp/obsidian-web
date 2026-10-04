package core

import (
	"context"
	"fmt"
	"io"
	"time"
)

type SyncHealth struct {
	DataBytes      int64 `json:"dataBytes"`
	SnapshotCursor int64 `json:"snapshotCursor"`
	Operations     int64 `json:"operations"`
	Outbox         int64 `json:"outbox"`
	FailedIntents  int64 `json:"failedIntents"`
}
type SyncAdministration interface {
	SyncHealth(context.Context) (SyncHealth, error)
	CreateSyncBackup(context.Context) (io.ReadCloser, string, error)
	CompactSync(context.Context) (SyncSnapshot, bool, error)
}

// SyncBatchError identifies the operation that prevented a push batch from
// being acknowledged. Its cause can be a ConflictError.
type SyncBatchError struct {
	Operation SyncOperation
	Err       error
}

func (e *SyncBatchError) Error() string {
	return fmt.Sprintf("sync operation %s: %v", e.Operation.OpID, e.Err)
}
func (e *SyncBatchError) Unwrap() error { return e.Err }

// SyncOperationKind is the durable action replicated between devices.
type SyncOperationKind string

const (
	SyncUpsert SyncOperationKind = "upsert"
	SyncDelete SyncOperationKind = "delete"
)

// SyncOperation is a client-originated, idempotent note change. OpID is a
// client UUID; AccountSeq is assigned only by the server.
type SyncOperation struct {
	OpID      string            `json:"opId"`
	Actor     string            `json:"-"`
	DeviceID  string            `json:"deviceId"`
	Path      string            `json:"path"`
	Kind      SyncOperationKind `json:"kind"`
	Content   string            `json:"content,omitempty"`
	BaseHash  string            `json:"baseHash,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

// SyncChange is the ordered journal representation returned by pull.
type SyncChange struct {
	SyncOperation
	AccountSeq int64 `json:"accountSeq"`
}

// SyncSnapshot is a complete bootstrap state at BaseCursor. Clients whose
// cursor is older than BaseCursor must apply it before ordinary pull results.
type SyncSnapshot struct {
	BaseCursor int64             `json:"baseCursor"`
	Notes      map[string]string `json:"notes"`
	Checksum   string            `json:"checksum"`
}

// SyncIntent is persisted before a controlled filesystem write. State values
// are adapter-independent JSON so recovery can compare them to the vault.
type SyncIntent struct {
	SyncOperation
	BeforeState []byte    `json:"beforeState"`
	AfterState  []byte    `json:"afterState"`
	State       string    `json:"state"`
	Failure     string    `json:"failure,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SyncWriter is a controlled, durable vault mutator. Its adapter suppresses
// only matching watcher notifications; core remains independent of fsnotify.
type SyncWriter interface {
	Write(path string, data []byte) error
	Delete(path string) error
}

// SyncJournal is the core port for durable sync metadata. The filesystem
// operation itself is intentionally outside this interface so the use case can
// enforce intent -> controlled write -> journal commit ordering.
type SyncJournal interface {
	CreateIntent(ctx context.Context, intent SyncIntent) error
	Operation(ctx context.Context, opID string) (SyncChange, bool, error)
	Intent(ctx context.Context, opID string) (SyncIntent, bool, error)
	PendingIntents(ctx context.Context) ([]SyncIntent, error)
	CommitOperation(ctx context.Context, operation SyncOperation) (SyncChange, bool, error)
	CommitOperations(ctx context.Context, operations []SyncOperation) (changes []SyncChange, duplicates []bool, err error)
	MarkIntentFailed(ctx context.Context, opID, failure string) error
	ResolveIntent(ctx context.Context, opID, resolution string) error
	DeleteIntent(ctx context.Context, opID string) error
	Pull(ctx context.Context, after int64, limit int) (changes []SyncChange, next int64, hasMore bool, err error)
	Bootstrap(ctx context.Context, cursor int64, force bool) (snapshot SyncSnapshot, required bool, err error)
}
