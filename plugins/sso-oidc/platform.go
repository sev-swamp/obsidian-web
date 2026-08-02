package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Platform is a thin client for the Obsidian Web service API. Every call
// carries the service token; the token can provision users and mint
// one-time login codes, but can never issue a session directly.
type Platform struct {
	base  string
	token string
	http  *http.Client
}

func NewPlatform(base, token string) *Platform {
	return &Platform{base: base, token: token, http: &http.Client{Timeout: 15 * time.Second}}
}

func (p *Platform) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return p.http.Do(req)
}

// apiError extracts the platform's {"error": …} message for logging and
// carries the status so callers can distinguish 401/403/409.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("platform %d: %s", e.Status, e.Message) }

func readError(resp *http.Response) error {
	defer resp.Body.Close()
	var payload struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(raw, &payload)
	msg := payload.Error
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &apiError{Status: resp.StatusCode, Message: msg}
}

// UpsertUser provisions (or updates) an SSO-only account. Roles above
// the token ceiling and users that already outrank it return 403.
func (p *Platform) UpsertUser(ctx context.Context, username, role string, groups []string) error {
	resp, err := p.do(ctx, http.MethodPost, "/api/service/users", map[string]any{
		"username": username, "role": role, "groups": groups,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return readError(resp)
	}
	resp.Body.Close()
	return nil
}

// LoginCode mints a one-time code for an existing user; the browser
// exchanges it for a session at POST /api/auth/code.
func (p *Platform) LoginCode(ctx context.Context, username string) (string, error) {
	resp, err := p.do(ctx, http.MethodPost, "/api/service/login-code", map[string]any{"username": username})
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", readError(resp)
	}
	defer resp.Body.Close()
	var out struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Code, nil
}

// RegisterProvider adds/updates the login button pointing at this
// plugin's /start route (needs the login-providers:write permission).
func (p *Platform) RegisterProvider(ctx context.Context, id, name, url string) error {
	resp, err := p.do(ctx, http.MethodPut, "/api/service/login-providers", map[string]any{
		"id": id, "name": name, "url": url,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return readError(resp)
	}
	resp.Body.Close()
	return nil
}
