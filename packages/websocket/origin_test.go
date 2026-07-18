package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func wsRequest(host, origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		name   string
		host   string // Host header as seen by the backend
		origin string
		want   bool
	}{
		{"no origin (non-browser client)", "example.com", "", true},
		{"same host and port", "example.com:8080", "http://example.com:8080", true},
		// nginx forwards $host without the public port: the backend sees
		// Host "example.com" while the browser origin carries ":8080".
		// This is the production regression: it must be allowed.
		{"reverse proxy strips port", "45.128.96.103", "http://45.128.96.103:8080", true},
		{"origin without port, host with port", "example.com:8787", "https://example.com", true},
		{"case-insensitive hostname", "Example.COM", "http://example.com", true},
		{"vite dev proxy", "localhost:5173", "http://localhost:5173", true},
		{"foreign site", "example.com", "http://evil.com", false},
		{"foreign site with same port", "example.com:8080", "http://evil.com:8080", false},
		{"unparsable origin", "example.com", "://bad", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := originAllowed(wsRequest(tc.host, tc.origin)); got != tc.want {
				t.Fatalf("originAllowed(host=%q, origin=%q) = %v, want %v", tc.host, tc.origin, got, tc.want)
			}
		})
	}
}
