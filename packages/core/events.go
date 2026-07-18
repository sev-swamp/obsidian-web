package core

import (
	"log/slog"
	"sync"
)

// Event types published on the bus and forwarded to WebSocket clients.
const (
	EventFileCreated   = "file.created"
	EventFileChanged   = "file.changed"
	EventFileDeleted   = "file.deleted"
	EventTreeChanged   = "tree.changed"
	EventIndexUpdated  = "index.updated"
	EventPluginChanged = "plugin.changed"
)

// Event is a domain event. Path is vault-relative; Actor is the
// username that caused the change ("external" for direct fs edits).
type Event struct {
	Type  string `json:"type"`
	Path  string `json:"path,omitempty"`
	Actor string `json:"actor,omitempty"`
}

// Handler receives published events. Handlers must be fast; long work
// should be dispatched to a goroutine by the subscriber.
type Handler func(Event)

// EventBus is a minimal in-process pub/sub used by the core, the file
// watcher, the WebSocket hub and plugins.
type EventBus interface {
	Publish(Event)
	Subscribe(fn Handler) (unsubscribe func())
}

type memoryBus struct {
	mu   sync.RWMutex
	subs map[int]Handler
	next int
}

// NewEventBus returns an in-memory EventBus.
func NewEventBus() EventBus {
	return &memoryBus{subs: map[int]Handler{}}
}

func (b *memoryBus) Publish(e Event) {
	b.mu.RLock()
	handlers := make([]Handler, 0, len(b.subs))
	for _, h := range b.subs {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()
	for _, h := range handlers {
		safeCall(h, e)
	}
}

// safeCall isolates subscribers: a panicking handler (e.g. a plugin)
// must not take down the publishing request.
func safeCall(h Handler, e Event) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("event handler panicked", "event", e.Type, "panic", r)
		}
	}()
	h(e)
}

func (b *memoryBus) Subscribe(fn Handler) func() {
	b.mu.Lock()
	id := b.next
	b.next++
	b.subs[id] = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
	}
}
