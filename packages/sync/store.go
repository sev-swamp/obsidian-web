// Package sync implements the durable SQLite adapter for offline-first note
// synchronisation. It contains no HTTP or filesystem business logic.
package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/core"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

func Open(dataFile string) (*Store, error) {
	if dataFile == "" {
		return nil, errors.New("sync data file is required")
	}
	abs, err := filepath.Abs(dataFile)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("sync database %s: %w", pragma, err)
		}
	}
	s := &Store{db: db, path: abs}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS sync_operations (
  op_id TEXT PRIMARY KEY, actor TEXT NOT NULL, device_id TEXT NOT NULL,
  path TEXT NOT NULL, kind TEXT NOT NULL, payload BLOB NOT NULL,
  base_hash TEXT, account_seq INTEGER NOT NULL UNIQUE, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sync_outbox (
  account_seq INTEGER PRIMARY KEY, event BLOB NOT NULL, published_at TEXT
);
CREATE TABLE IF NOT EXISTS sync_devices (
  actor TEXT NOT NULL, device_id TEXT NOT NULL, last_cursor INTEGER NOT NULL,
  last_seen_at TEXT NOT NULL, PRIMARY KEY(actor, device_id)
);
CREATE TABLE IF NOT EXISTS sync_snapshots (
  base_cursor INTEGER PRIMARY KEY, state BLOB NOT NULL, checksum TEXT NOT NULL,
  note_count INTEGER NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sync_intents (
  op_id TEXT PRIMARY KEY, actor TEXT NOT NULL, device_id TEXT NOT NULL,
  path TEXT NOT NULL, kind TEXT NOT NULL, operation BLOB NOT NULL,
  before_state BLOB NOT NULL, after_state BLOB NOT NULL, state TEXT NOT NULL,
  created_at TEXT NOT NULL, failure TEXT
);`)
	return err
}

func (s *Store) CreateIntent(ctx context.Context, in core.SyncIntent) error {
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now().UTC()
	}
	if in.State == "" {
		in.State = "pending"
	}
	b, err := json.Marshal(in.SyncOperation)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO sync_intents
 (op_id,actor,device_id,path,kind,operation,before_state,after_state,state,created_at,failure)
 VALUES (?,?,?,?,?,?,?,?,?,?,?)`, in.OpID, in.Actor, in.DeviceID, in.Path, in.Kind, b,
		in.BeforeState, in.AfterState, in.State, in.CreatedAt.Format(time.RFC3339Nano), nullIfEmpty(in.Failure))
	return err
}

func (s *Store) Operation(ctx context.Context, opID string) (core.SyncChange, bool, error) {
	var seq int64
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT account_seq,payload FROM sync_operations WHERE op_id=?`, opID).Scan(&seq, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return core.SyncChange{}, false, nil
	}
	if err != nil {
		return core.SyncChange{}, false, err
	}
	var op core.SyncOperation
	if err := json.Unmarshal(payload, &op); err != nil {
		return core.SyncChange{}, false, err
	}
	return core.SyncChange{SyncOperation: op, AccountSeq: seq}, true, nil
}

func (s *Store) CommitOperation(ctx context.Context, op core.SyncOperation) (core.SyncChange, bool, error) {
	changes, duplicates, err := s.CommitOperations(ctx, []core.SyncOperation{op})
	if err != nil {
		return core.SyncChange{}, false, err
	}
	return changes[0], duplicates[0], nil
}

func (s *Store) CommitOperations(ctx context.Context, operations []core.SyncOperation) ([]core.SyncChange, []bool, error) {
	if len(operations) == 0 {
		return nil, nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	changes := make([]core.SyncChange, len(operations))
	duplicates := make([]bool, len(operations))
	var next int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(account_seq), 0) FROM sync_operations`).Scan(&next); err != nil {
		return nil, nil, err
	}
	for i, op := range operations {
		var existing int64
		err = tx.QueryRowContext(ctx, `SELECT account_seq FROM sync_operations WHERE op_id=?`, op.OpID).Scan(&existing)
		if err == nil {
			changes[i] = core.SyncChange{SyncOperation: op, AccountSeq: existing}
			duplicates[i] = true
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}
		if op.CreatedAt.IsZero() {
			op.CreatedAt = time.Now().UTC()
		}
		payload, err := json.Marshal(op)
		if err != nil {
			return nil, nil, err
		}
		next++
		change := core.SyncChange{SyncOperation: op, AccountSeq: next}
		event, err := json.Marshal(change)
		if err != nil {
			return nil, nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sync_operations
 (op_id,actor,device_id,path,kind,payload,base_hash,account_seq,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			op.OpID, op.Actor, op.DeviceID, op.Path, op.Kind, payload, nullIfEmpty(op.BaseHash), next, op.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return nil, nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sync_outbox(account_seq,event) VALUES (?,?)`, next, event); err != nil {
			return nil, nil, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM sync_intents WHERE op_id=?`, op.OpID); err != nil {
			return nil, nil, err
		}
		changes[i] = change
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return changes, duplicates, nil
}

func (s *Store) Pull(ctx context.Context, after int64, limit int) ([]core.SyncChange, int64, bool, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT account_seq,payload FROM sync_operations WHERE account_seq>? ORDER BY account_seq LIMIT ?`, after, limit+1)
	if err != nil {
		return nil, after, false, err
	}
	defer rows.Close()
	changes := make([]core.SyncChange, 0, limit)
	for rows.Next() {
		var seq int64
		var payload []byte
		if err := rows.Scan(&seq, &payload); err != nil {
			return nil, after, false, err
		}
		if len(changes) == limit {
			return changes, changes[len(changes)-1].AccountSeq, true, nil
		}
		var op core.SyncOperation
		if err := json.Unmarshal(payload, &op); err != nil {
			return nil, after, false, err
		}
		changes = append(changes, core.SyncChange{SyncOperation: op, AccountSeq: seq})
	}
	if err := rows.Err(); err != nil {
		return nil, after, false, err
	}
	next := after
	if len(changes) > 0 {
		next = changes[len(changes)-1].AccountSeq
	}
	return changes, next, false, nil
}

// Unpublished returns journal commits whose live wake-up signal was not yet
// delivered. Re-delivery is harmless: clients always pull by cursor.
func (s *Store) Unpublished(ctx context.Context, limit int) ([]core.SyncChange, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event FROM sync_outbox WHERE published_at IS NULL ORDER BY account_seq LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var changes []core.SyncChange
	for rows.Next() {
		var event []byte
		if err := rows.Scan(&event); err != nil {
			return nil, err
		}
		var change core.SyncChange
		if err := json.Unmarshal(event, &change); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

func (s *Store) MarkPublished(ctx context.Context, cursor int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_outbox SET published_at=? WHERE account_seq=? AND published_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), cursor)
	return err
}

func (s *Store) Intent(ctx context.Context, opID string) (core.SyncIntent, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT operation,before_state,after_state,state,created_at,COALESCE(failure,'') FROM sync_intents WHERE op_id=?`, opID)
	in, err := scanIntent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.SyncIntent{}, false, nil
	}
	return in, err == nil, err
}

func (s *Store) PendingIntents(ctx context.Context) ([]core.SyncIntent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT operation,before_state,after_state,state,created_at,COALESCE(failure,'') FROM sync_intents WHERE state IN ('pending','failed') ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.SyncIntent
	for rows.Next() {
		in, err := scanIntent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}
func (s *Store) MarkIntentFailed(ctx context.Context, opID, failure string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_intents SET state='failed', failure=? WHERE op_id=?`, failure, opID)
	return err
}
func (s *Store) ResolveIntent(ctx context.Context, opID, resolution string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_intents SET state=?, failure=NULL WHERE op_id=?`, resolution, opID)
	return err
}

func (s *Store) DeleteIntent(ctx context.Context, opID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sync_intents WHERE op_id=?`, opID)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type scanner interface{ Scan(...any) error }

func scanIntent(row scanner) (core.SyncIntent, error) {
	var in core.SyncIntent
	var operation []byte
	var created string
	if err := row.Scan(&operation, &in.BeforeState, &in.AfterState, &in.State, &created, &in.Failure); err != nil {
		return in, err
	}
	if err := json.Unmarshal(operation, &in.SyncOperation); err != nil {
		return in, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return in, err
	}
	in.CreatedAt = createdAt
	return in, nil
}
