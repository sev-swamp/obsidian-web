package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

func (s *Store) SyncHealth(ctx context.Context) (core.SyncHealth, error) {
	var h core.SyncHealth
	info, err := os.Stat(s.path)
	if err != nil {
		return h, err
	}
	h.DataBytes = info.Size()
	if err = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(base_cursor),0) FROM sync_snapshots`).Scan(&h.SnapshotCursor); err != nil {
		return h, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_operations`).Scan(&h.Operations); err != nil {
		return h, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_outbox WHERE published_at IS NULL`).Scan(&h.Outbox); err != nil {
		return h, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_intents WHERE state='failed'`).Scan(&h.FailedIntents); err != nil {
		return h, err
	}
	return h, nil
}

// CreateSyncBackup uses SQLite's online backup command and returns a temporary
// file. Closing the returned reader removes it, so no server-side archive
// accumulates and the browser never chooses the filesystem path.
func (s *Store) CreateSyncBackup(ctx context.Context) (io.ReadCloser, string, error) {
	f, err := os.CreateTemp(filepath.Dir(s.path), "sync-backup-*.db")
	if err != nil {
		return nil, "", err
	}
	target := f.Name()
	if err = f.Close(); err != nil {
		return nil, "", err
	}
	if err = os.Remove(target); err != nil {
		return nil, "", err
	}
	quoted := strings.ReplaceAll(target, "'", "''")
	if _, err = s.db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", quoted)); err != nil {
		return nil, "", err
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, "", err
	}
	return &removeOnClose{File: file, path: target}, "sync-backup.db", nil
}

type removeOnClose struct {
	*os.File
	path string
}

func (f *removeOnClose) Close() error { err := f.File.Close(); _ = os.Remove(f.path); return err }

func (s *Store) CompactSync(ctx context.Context) (core.SyncSnapshot, bool, error) {
	snapshot, changed, err := s.Compact(ctx)
	if err != nil {
		return core.SyncSnapshot{}, false, err
	}
	return core.SyncSnapshot{BaseCursor: snapshot.BaseCursor, Notes: snapshot.State, Checksum: snapshot.Checksum}, changed, nil
}
