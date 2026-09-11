// Command sso-oidc is the reference SSO plugin for Obsidian Web: a
// standalone service that signs users in against any OpenID Connect
// provider and hands them back to the platform via the service API.
// The platform knows nothing about OIDC — the contract is REST plus a
// browser redirect. See docs/sso-plugin.md and plans/04-sso-plugin.md.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

const stateCookie = "obsidianweb_sso_state"

type server struct {
	cfg      Config
	oidc     *OIDC
	platform *Platform
	log      *slog.Logger
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := LoadConfig()
	if err != nil {
		log.Error("configuration error", "error", err)
		os.Exit(1)
	}
	s := &server{
		cfg:      cfg,
		oidc:     NewOIDC(cfg),
		platform: NewPlatform(cfg.PlatformAPI, cfg.ServiceToken),
		log:      log,
	}

	if cfg.RegisterSelf {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := s.platform.RegisterProvider(ctx, cfg.ProviderID, cfg.ProviderName, cfg.PublicURL+"/start"); err != nil {
			log.Warn("could not self-register login provider (add it manually in Settings → SSO)", "error", err)
		} else {
			log.Info("registered login provider", "id", cfg.ProviderID, "url", cfg.PublicURL+"/start")
		}
		cancel()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/start", s.handleStart)
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	log.Info("sso-oidc plugin listening", "addr", cfg.Listen, "issuer", cfg.Issuer, "redirect", cfg.RedirectURL())
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handleStart begins the flow: set a CSRF state cookie and redirect the
// browser to the identity provider.
func (s *server) handleStart(w http.ResponseWriter, r *http.Request) {
	state, err := randomHex(16)
	if err != nil {
		s.fail(w, r, "could not start sign-in")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", MaxAge: 300,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.URL.Scheme == "https",
	})
	authURL, err := s.oidc.AuthURL(state)
	if err != nil {
		s.log.Warn("auth url", "error", err)
		s.fail(w, r, "identity provider is unreachable")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback finishes the flow: verify state, exchange the code,
// provision the account, mint a login code and bounce the browser to
// the platform login page.
func (s *server) handleCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(stateCookie)
	if err != nil || cookie.Value == "" || r.URL.Query().Get("state") != cookie.Value {
		s.fail(w, r, "sign-in state mismatch, try again")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	identity, err := s.oidc.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		s.log.Warn("exchange failed", "error", err)
		s.fail(w, r, "sign-in failed")
		return
	}
	if err := s.platform.UpsertUser(ctx, identity.Username, s.cfg.DefaultRole, identity.Groups); err != nil {
		s.log.Warn("provisioning failed", "username", identity.Username, "error", err)
		s.fail(w, r, "your account could not be provisioned")
		return
	}
	code, err := s.platform.LoginCode(ctx, identity.Username)
	if err != nil {
		s.log.Warn("login code failed", "username", identity.Username, "error", err)
		s.fail(w, r, "could not complete sign-in")
		return
	}
	s.log.Info("signed in", "username", identity.Username, "subject", identity.Subject)
	http.Redirect(w, r, s.cfg.PlatformPublicURL+"/login?code="+url.QueryEscape(code), http.StatusFound)
}

// fail bounces the browser back to the platform login page with a
// message, matching the built-in flow's ?sso_error contract.
func (s *server) fail(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, s.cfg.PlatformPublicURL+"/login?sso_error="+url.QueryEscape(message), http.StatusFound)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
