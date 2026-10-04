package sync

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Snapshot is a verified bootstrap state at a global cursor.
type Snapshot struct {
	BaseCursor int64
	State      map[string]string
	Checksum   string
	CreatedAt  time.Time
}

// EnsureInitialSnapshot captures the pre-sync vault exactly once. It prevents
// a later compaction from losing notes that existed before sync was enabled.
func (s *Store) EnsureInitialSnapshot(ctx context.Context, vault core.VaultFS) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_snapshots`).Scan(&count); err != nil || count > 0 {
		return err
	}
	state := map[string]string{}
	if err := vault.Walk(func(info core.FileInfo) error {
		if info.IsDir || !core.IsMarkdown(info.Path) {
			return nil
		}
		data, err := vault.Read(info.Path)
		if err != nil {
			return err
		}
		state[info.Path] = string(data)
		return nil
	}); err != nil {
		return err
	}
	return s.saveSnapshot(ctx, 0, state, false)
}

func (s *Store) LatestSnapshot(ctx context.Context) (Snapshot, bool, error) {
	var base int64
	var blob []byte
	var checksum, created string
	err := s.db.QueryRowContext(ctx, `SELECT base_cursor,state,checksum,created_at FROM sync_snapshots ORDER BY base_cursor DESC LIMIT 1`).Scan(&base, &blob, &checksum, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	if hash(blob) != checksum {
		return Snapshot{}, false, errors.New("sync snapshot checksum mismatch")
	}
	state := map[string]string{}
	if err := json.Unmarshal(blob, &state); err != nil {
		return Snapshot{}, false, err
	}
	at, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Snapshot{}, false, err
	}
	return Snapshot{BaseCursor: base, State: state, Checksum: checksum, CreatedAt: at}, true, nil
}

func (s *Store) Bootstrap(ctx context.Context, cursor int64, force bool) (core.SyncSnapshot, bool, error) {
	snapshot, ok, err := s.LatestSnapshot(ctx)
	if err != nil || !ok || (!force && cursor >= snapshot.BaseCursor) {
		return core.SyncSnapshot{}, false, err
	}
	return core.SyncSnapshot{BaseCursor: snapshot.BaseCursor, Notes: snapshot.State, Checksum: snapshot.Checksum}, true, nil
}

// Compact creates a snapshot from the latest verified snapshot plus all later
// journal operations. Only after that snapshot is committed does it remove the
// covered operations and published outbox rows.
func (s *Store) Compact(ctx context.Context) (Snapshot, bool, error) {
	previous, ok, err := s.LatestSnapshot(ctx)
	if err != nil || !ok {
		return Snapshot{}, false, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT account_seq,payload FROM sync_operations WHERE account_seq>? ORDER BY account_seq`, previous.BaseCursor)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer rows.Close()
	state := previous.State
	cursor := previous.BaseCursor
	changed := false
	for rows.Next() {
		var seq int64
		var payload []byte
		if err := rows.Scan(&seq, &payload); err != nil {
			return Snapshot{}, false, err
		}
		var op core.SyncOperation
		if err := json.Unmarshal(payload, &op); err != nil {
			return Snapshot{}, false, err
		}
		if op.Kind == core.SyncDelete {
			delete(state, op.Path)
		} else {
			state[op.Path] = op.Content
		}
		cursor = seq
		changed = true
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, false, err
	}
	if !changed {
		return previous, false, nil
	}
	if err := s.saveSnapshot(ctx, cursor, state, true); err != nil {
		return Snapshot{}, false, err
	}
	latest, _, err := s.LatestSnapshot(ctx)
	return latest, true, err
}

func (s *Store) saveSnapshot(ctx context.Context, cursor int64, state map[string]string, compact bool) error {
	blob, err := json.Marshal(state)
	if err != nil {
		return err
	}
	checksum := hash(blob)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO sync_snapshots(base_cursor,state,checksum,note_count,created_at) VALUES (?,?,?,?,?)`, cursor, blob, checksum, len(state), now); err != nil {
		return err
	}
	if compact {
		if _, err = tx.ExecContext(ctx, `DELETE FROM sync_operations WHERE account_seq<=?`, cursor); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM sync_outbox WHERE account_seq<=? AND published_at IS NOT NULL`, cursor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
