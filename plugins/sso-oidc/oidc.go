package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Identity is the verified user the IdP hands back after code exchange.
type Identity struct {
	// Subject is the provider's immutable user id (OIDC `sub`). A stable
	// username is derived from it; account matching on the platform never
	// relies on a user-editable claim alone.
	Subject  string
	Username string
	Email    string
	Groups   []string
}

// OIDC wraps OpenID Connect discovery + code exchange. Discovery is
// lazy: the first login attempt reaches the issuer, so the plugin
// starts even when the IdP is briefly unavailable.
type OIDC struct {
	cfg Config

	mu          sync.Mutex
	provider    *oidc.Provider
	oauth       oauth2.Config
	initialized bool
}

func NewOIDC(cfg Config) *OIDC { return &OIDC{cfg: cfg} }

func (o *OIDC) init() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.initialized {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, o.cfg.Issuer)
	if err != nil {
		return fmt.Errorf("OIDC discovery for %s: %w", o.cfg.Issuer, err)
	}
	o.provider = provider
	o.oauth = oauth2.Config{
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
		RedirectURL:  o.cfg.RedirectURL(),
		Endpoint:     provider.Endpoint(),
		Scopes:       o.cfg.Scopes,
	}
	o.initialized = true
	return nil
}

// AuthURL returns the provider login URL for the given CSRF state.
func (o *OIDC) AuthURL(state string) (string, error) {
	if err := o.init(); err != nil {
		return "", err
	}
	return o.oauth.AuthCodeURL(state), nil
}

// Exchange trades an authorization code for a verified identity.
func (o *OIDC) Exchange(ctx context.Context, code string) (Identity, error) {
	if err := o.init(); err != nil {
		return Identity{}, err
	}
	token, err := o.oauth.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("code exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, fmt.Errorf("provider returned no id_token")
	}
	idToken, err := o.provider.Verifier(&oidc.Config{ClientID: o.cfg.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("id_token verification: %w", err)
	}

	// Claims are decoded generically so a custom groups claim name works
	// without recompiling.
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, err
	}
	email, _ := claims["email"].(string)
	username, _ := claims["preferred_username"].(string)
	if username == "" && email != "" {
		username = strings.SplitN(email, "@", 2)[0]
	}
	if username == "" {
		return Identity{}, fmt.Errorf("provider returned neither preferred_username nor email")
	}
	if idToken.Subject == "" {
		return Identity{}, fmt.Errorf("provider returned no subject")
	}
	return Identity{
		Subject:  idToken.Subject,
		Username: username,
		Email:    email,
		Groups:   stringSlice(claims[o.cfg.GroupsClaim]),
	}, nil
}

// stringSlice coerces a JSON claim (array of strings, or a single
// string) into a []string; anything else yields nil.
func stringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case json.RawMessage:
		var s []string
		_ = json.Unmarshal(t, &s)
		return s
	}
	return nil
}
