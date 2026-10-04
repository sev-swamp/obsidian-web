package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

type fileState struct {
	Exists bool   `json:"exists"`
	Hash   string `json:"hash,omitempty"`
}

// Recover finalizes only intents whose filesystem outcome can be proven. It
// runs before the watcher starts, so its controlled writes cannot be mistaken
// for external changes. Failed intents require an explicit admin resolution.
func Recover(ctx context.Context, journal core.SyncJournal, vault core.VaultFS) error {
	intents, err := journal.PendingIntents(ctx)
	if err != nil {
		return err
	}
	for _, in := range intents {
		if in.State == "failed" {
			continue
		}
		before, err := decodeState(in.BeforeState)
		if err != nil {
			return fmt.Errorf("intent %s before state: %w", in.OpID, err)
		}
		after, err := decodeState(in.AfterState)
		if err != nil {
			return fmt.Errorf("intent %s after state: %w", in.OpID, err)
		}
		actual, err := stateAt(vault, in.Path)
		if err != nil {
			return fmt.Errorf("intent %s actual state: %w", in.OpID, err)
		}
		switch {
		case actual == after:
			if _, _, err := journal.CommitOperation(ctx, in.SyncOperation); err != nil {
				return fmt.Errorf("intent %s commit: %w", in.OpID, err)
			}
		case actual == before:
			if err := journal.DeleteIntent(ctx, in.OpID); err != nil {
				return fmt.Errorf("intent %s discard: %w", in.OpID, err)
			}
		default:
			if err := journal.MarkIntentFailed(ctx, in.OpID, "filesystem state differs from both intent states"); err != nil {
				return fmt.Errorf("intent %s fail: %w", in.OpID, err)
			}
		}
	}
	return nil
}

func decodeState(data []byte) (fileState, error) {
	var state fileState
	if err := json.Unmarshal(data, &state); err != nil {
		return fileState{}, err
	}
	if !state.Exists && state.Hash != "" {
		return fileState{}, errors.New("absent state cannot have hash")
	}
	return state, nil
}

func stateAt(vault core.VaultFS, path string) (fileState, error) {
	data, err := vault.Read(path)
	if errors.Is(err, core.ErrNotFound) {
		return fileState{}, nil
	}
	if err != nil {
		return fileState{}, err
	}
	return fileState{Exists: true, Hash: core.HashContent(data)}, nil
}
