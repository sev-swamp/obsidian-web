package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Publisher drains the transactional outbox after the journal transaction has
// committed. WebSocket messages are wake-ups only; a dropped or duplicated
// signal cannot lose a change because clients subsequently call Pull.
type Publisher struct {
	store *Store
	bus   core.EventBus
	log   *slog.Logger
}

func NewPublisher(store *Store, bus core.EventBus, log *slog.Logger) *Publisher {
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{store: store, bus: bus, log: log}
}

func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := p.Flush(ctx); err != nil && ctx.Err() == nil {
			p.log.Error("sync outbox publish", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) Flush(ctx context.Context) error {
	changes, err := p.store.Unpublished(ctx, 100)
	if err != nil {
		return err
	}
	for _, change := range changes {
		p.bus.Publish(core.Event{Type: core.EventSyncCursor, Path: change.Path, Actor: change.Actor, Cursor: change.AccountSeq})
		if err := p.store.MarkPublished(ctx, change.AccountSeq); err != nil {
			return err
		}
	}
	return nil
}
