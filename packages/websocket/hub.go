// Package websocket broadcasts core events to connected browser clients
// so the UI updates in real time, and tracks presence: who is viewing
// or editing which note.
package websocket

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
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
	// Same-origin only: a foreign page opening a WebSocket to a local
	// instance (auth disabled) would otherwise receive vault events.
	// Non-browser clients send no Origin header and are allowed.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	},
}

// AccessFunc restricts which vault paths a user may learn about through
// events and presence. nil allows everything.
type AccessFunc func(username, path string) bool

type client struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
}

// inbound messages from browsers (presence updates).
type inbound struct {
	c   *client
	msg clientMessage
}

type clientMessage struct {
	Type  string `json:"type"`  // "presence"
	Path  string `json:"path"`
	State string `json:"state"` // viewing | editing | left
}

type outbound struct {
	path string // empty = broadcast to everyone
	data []byte
}

// Hub fan-outs domain events and presence to WebSocket clients.
type Hub struct {
	log        *slog.Logger
	access     AccessFunc
	register   chan *client
	unregister chan *client
	broadcast  chan outbound
	inbox      chan inbound
}

// NewHub creates a hub subscribed to the event bus and starts its loop.
func NewHub(bus core.EventBus, access AccessFunc, log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	h := &Hub{
		log:        log,
		access:     access,
		register:   make(chan *client),
		unregister: make(chan *client),
		broadcast:  make(chan outbound, 64),
		inbox:      make(chan inbound, 64),
	}
	go h.run()
	bus.Subscribe(func(e core.Event) {
		data, err := json.Marshal(e)
		if err != nil {
			return
		}
		select {
		case h.broadcast <- outbound{path: e.Path, data: data}:
		default: // drop if the hub is saturated; clients re-sync via REST
		}
	})
	return h
}

func (h *Hub) allowed(username, path string) bool {
	if h.access == nil || path == "" {
		return true
	}
	return h.access(username, path)
}

func (h *Hub) run() {
	clients := map[*client]bool{}
	// presence: path -> client -> state ("viewing" | "editing")
	presence := map[string]map[*client]string{}

	removeFromPresence := func(c *client) []string {
		var affected []string
		for path, members := range presence {
			if _, ok := members[c]; ok {
				delete(members, c)
				if len(members) == 0 {
					delete(presence, path)
				}
				affected = append(affected, path)
			}
		}
		return affected
	}

	notifyPresence := func(path string) {
		viewers := map[string]bool{}
		editors := map[string]bool{}
		for c, state := range presence[path] {
			if c.username == "" {
				continue
			}
			if state == "editing" {
				editors[c.username] = true
			} else {
				viewers[c.username] = true
			}
		}
		payload, err := json.Marshal(map[string]any{
			"type":    "presence.changed",
			"path":    path,
			"viewers": sortedKeys(viewers),
			"editors": sortedKeys(editors),
		})
		if err != nil {
			return
		}
		for c := range clients {
			if h.allowed(c.username, path) {
				h.trySend(clients, c, payload)
			}
		}
	}

	for {
		select {
		case c := <-h.register:
			clients[c] = true
		case c := <-h.unregister:
			if clients[c] {
				delete(clients, c)
				close(c.send)
				for _, path := range removeFromPresence(c) {
					notifyPresence(path)
				}
			}
		case in := <-h.inbox:
			if in.msg.Type != "presence" || in.msg.Path == "" {
				continue
			}
			switch in.msg.State {
			case "viewing", "editing":
				if presence[in.msg.Path] == nil {
					presence[in.msg.Path] = map[*client]string{}
				}
				presence[in.msg.Path][in.c] = in.msg.State
			default: // "left"
				if members, ok := presence[in.msg.Path]; ok {
					delete(members, in.c)
					if len(members) == 0 {
						delete(presence, in.msg.Path)
					}
				}
			}
			notifyPresence(in.msg.Path)
		case msg := <-h.broadcast:
			for c := range clients {
				if h.allowed(c.username, msg.path) {
					h.trySend(clients, c, msg.data)
				}
			}
		}
	}
}

// trySend queues a message, disconnecting slow clients instead of
// blocking everyone.
func (h *Hub) trySend(clients map[*client]bool, c *client, data []byte) {
	select {
	case c.send <- data:
	default:
		delete(clients, c)
		close(c.send)
	}
}

// ServeWS upgrades an HTTP request to a WebSocket connection. username
// may be empty when authentication is disabled.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, username string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("websocket upgrade failed", "error", err)
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 16), username: username}
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
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg clientMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		select {
		case h.inbox <- inbound{c: c, msg: msg}:
		default: // presence updates are best-effort
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
