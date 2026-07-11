// Package websocket broadcasts core events to connected browser clients
// so the UI updates in real time without page reloads.
package websocket

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// The API is same-origin in production and proxied in development.
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

// Hub fan-outs domain events to all connected WebSocket clients.
type Hub struct {
	log        *slog.Logger
	register   chan *client
	unregister chan *client
	broadcast  chan []byte
}

// NewHub creates a hub subscribed to the event bus and starts its loop.
func NewHub(bus core.EventBus, log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	h := &Hub{
		log:        log,
		register:   make(chan *client),
		unregister: make(chan *client),
		broadcast:  make(chan []byte, 64),
	}
	go h.run()
	bus.Subscribe(func(e core.Event) {
		data, err := json.Marshal(e)
		if err != nil {
			return
		}
		select {
		case h.broadcast <- data:
		default: // drop if the hub is saturated; clients re-sync via REST
		}
	})
	return h
}

func (h *Hub) run() {
	clients := map[*client]bool{}
	for {
		select {
		case c := <-h.register:
			clients[c] = true
		case c := <-h.unregister:
			if clients[c] {
				delete(clients, c)
				close(c.send)
			}
		case msg := <-h.broadcast:
			for c := range clients {
				select {
				case c.send <- msg:
				default: // slow client: disconnect instead of blocking everyone
					delete(clients, c)
					close(c.send)
				}
			}
		}
	}
}

// ServeWS upgrades an HTTP request to a WebSocket connection.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("websocket upgrade failed", "error", err)
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 16)}
	h.register <- c
	go h.writePump(c)
	go h.readPump(c)
}

func (h *Hub) readPump(c *client) {
	defer func() {
		h.unregister <- c
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(1 << 16)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) writePump(c *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
