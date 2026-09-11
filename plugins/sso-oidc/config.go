package main

import (
	"fmt"
	"os"
	"strings"
)

// Config is read entirely from the environment so the plugin needs no
// config file in a container. See README.md for the full table.
type Config struct {
	// Listen is the address the plugin's HTTP server binds to.
	Listen string
	// PublicURL is the plugin's own base URL as the browser and the IdP
	// see it (used to build the OIDC redirect_uri: <PublicURL>/callback).
	PublicURL string

	// PlatformAPI is the platform base URL for server-to-server calls
	// (internal docker network, e.g. http://obsidianweb:8787).
	PlatformAPI string
	// PlatformPublicURL is where the browser is redirected to finish the
	// login (…/login?code=…). Defaults to PlatformAPI when empty.
	PlatformPublicURL string
	// ServiceToken authenticates every call to the platform service API.
	ServiceToken string

	// Provider registration on the login page (optional).
	ProviderID   string
	ProviderName string
	RegisterSelf bool

	// OIDC settings.
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// GroupsClaim is the id-token claim carrying group membership.
	GroupsClaim string
	// DefaultRole is assigned to freshly provisioned accounts; it must be
	// at or below the service token's roleCeiling.
	DefaultRole string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// LoadConfig reads and validates the environment configuration.
func LoadConfig() (Config, error) {
	c := Config{
		Listen:            env("LISTEN_ADDR", ":8080"),
		PublicURL:         strings.TrimRight(os.Getenv("PLUGIN_PUBLIC_URL"), "/"),
		PlatformAPI:       strings.TrimRight(os.Getenv("PLATFORM_URL"), "/"),
		PlatformPublicURL: strings.TrimRight(os.Getenv("PLATFORM_PUBLIC_URL"), "/"),
		ServiceToken:      os.Getenv("SERVICE_TOKEN"),
		ProviderID:        env("PROVIDER_ID", "oidc"),
		ProviderName:      env("PROVIDER_NAME", "Sign in with SSO"),
		RegisterSelf:      os.Getenv("REGISTER_PROVIDER") == "true",
		Issuer:            os.Getenv("OIDC_ISSUER"),
		ClientID:          os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret:      os.Getenv("OIDC_CLIENT_SECRET"),
		GroupsClaim:       env("OIDC_GROUPS_CLAIM", "groups"),
		DefaultRole:       env("DEFAULT_ROLE", "viewer"),
	}
	if raw := os.Getenv("OIDC_SCOPES"); raw != "" {
		c.Scopes = strings.Fields(strings.ReplaceAll(raw, ",", " "))
	} else {
		c.Scopes = []string{"openid", "profile", "email"}
	}
	if c.PlatformPublicURL == "" {
		c.PlatformPublicURL = c.PlatformAPI
	}

	var missing []string
	if c.PublicURL == "" {
		missing = append(missing, "PLUGIN_PUBLIC_URL")
	}
	if c.PlatformAPI == "" {
		missing = append(missing, "PLATFORM_URL")
	}
	if c.ServiceToken == "" {
		missing = append(missing, "SERVICE_TOKEN")
	}
	if c.Issuer == "" {
		missing = append(missing, "OIDC_ISSUER")
	}
	if c.ClientID == "" {
		missing = append(missing, "OIDC_CLIENT_ID")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

// RedirectURL is the OIDC redirect_uri registered at the IdP.
func (c Config) RedirectURL() string { return c.PublicURL + "/callback" }
