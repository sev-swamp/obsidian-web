package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCSettings configures a generic OpenID Connect provider (covers
// Google, Keycloak, Authentik, Authelia, Azure AD…).
type OIDCSettings struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// OIDC implements IdentityProvider via OpenID Connect discovery.
// Discovery is lazy: the first login attempt reaches the issuer.
type OIDC struct {
	name     string
	settings OIDCSettings

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    oauth2.Config
}

var _ IdentityProvider = (*OIDC)(nil)

// NewOIDC creates a provider; name is the login-button label.
func NewOIDC(name string, settings OIDCSettings) *OIDC {
	if name == "" {
		name = "SSO"
	}
	return &OIDC{name: name, settings: settings}
}

func (o *OIDC) Name() string { return o.name }

func (o *OIDC) init() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, o.settings.Issuer)
	if err != nil {
		return fmt.Errorf("OIDC discovery for %s: %w", o.settings.Issuer, err)
	}
	o.provider = provider
	o.oauth = oauth2.Config{
		ClientID:     o.settings.ClientID,
		ClientSecret: o.settings.ClientSecret,
		RedirectURL:  o.settings.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return nil
}

// AuthURL returns the provider login URL for the CSRF state.
func (o *OIDC) AuthURL(state string) string {
	if err := o.init(); err != nil {
		return ""
	}
	return o.oauth.AuthCodeURL(state)
}

// Exchange trades the authorization code for a verified identity.
func (o *OIDC) Exchange(code string) (Identity, error) {
	if err := o.init(); err != nil {
		return Identity{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := o.oauth.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("code exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, fmt.Errorf("provider returned no id_token")
	}
	idToken, err := o.provider.Verifier(&oidc.Config{ClientID: o.settings.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("id_token verification: %w", err)
	}
	var claims struct {
		Email             string   `json:"email"`
		PreferredUsername string   `json:"preferred_username"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, err
	}
	username := claims.PreferredUsername
	if username == "" && claims.Email != "" {
		username = strings.SplitN(claims.Email, "@", 2)[0]
	}
	if username == "" {
		return Identity{}, fmt.Errorf("provider returned neither preferred_username nor email")
	}
	return Identity{Username: username, Email: claims.Email, Groups: claims.Groups}, nil
}
