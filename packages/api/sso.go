package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/auth"
)

const ssoStateCookie = "obsidianweb_sso_state"

// ssoProvider caches the OIDC provider, rebuilding it when the admin
// changes the configuration.
type ssoProvider struct {
	mu   sync.Mutex
	prov *auth.OIDC
	cfg  acl.SSOConfig
}

var ssoCache ssoProvider

func (s *Server) sso(c *gin.Context) (*auth.OIDC, acl.SSOConfig, bool) {
	if s.ACL == nil {
		return nil, acl.SSOConfig{}, false
	}
	cfg := s.ACL.SSO()
	if !cfg.Enabled {
		return nil, cfg, false
	}
	redirect := cfg.RedirectURL
	if redirect == "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		redirect = scheme + "://" + c.Request.Host + "/api/auth/sso/callback"
		cfg.RedirectURL = redirect
	}
	ssoCache.mu.Lock()
	defer ssoCache.mu.Unlock()
	if ssoCache.prov == nil || ssoCache.cfg != cfg {
		ssoCache.prov = auth.NewOIDC(cfg.Name, auth.OIDCSettings{
			Issuer:       cfg.Issuer,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  redirect,
		})
		ssoCache.cfg = cfg
	}
	return ssoCache.prov, cfg, true
}

// handleSSOStatus tells the login page whether to show the SSO button.
func (s *Server) handleSSOStatus(c *gin.Context) {
	if s.ACL == nil || !s.Auth.Enabled {
		c.JSON(http.StatusOK, gin.H{"enabled": false})
		return
	}
	cfg := s.ACL.SSO()
	name := cfg.Name
	if name == "" {
		name = "SSO"
	}
	c.JSON(http.StatusOK, gin.H{"enabled": cfg.Enabled, "name": name})
}

// handleSSOLogin redirects the browser to the identity provider.
func (s *Server) handleSSOLogin(c *gin.Context) {
	provider, _, ok := s.sso(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSO is not enabled"})
		return
	}
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	state := hex.EncodeToString(stateBytes)
	c.SetCookie(ssoStateCookie, state, 300, "/", "", false, true)
	authURL := provider.AuthURL(state)
	if authURL == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "identity provider is unreachable (check issuer URL)"})
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

// handleSSOCallback finishes the flow: verifies state, exchanges the
// code, provisions/matches the account and hands a session token to
// the frontend via the login page.
func (s *Server) handleSSOCallback(c *gin.Context) {
	provider, cfg, ok := s.sso(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSO is not enabled"})
		return
	}
	wanted, err := c.Cookie(ssoStateCookie)
	if err != nil || wanted == "" || c.Query("state") != wanted {
		s.loginRedirectError(c, "SSO state mismatch, try again")
		return
	}
	c.SetCookie(ssoStateCookie, "", -1, "/", "", false, true)

	identity, err := provider.Exchange(c.Query("code"))
	if err != nil {
		s.Log.Warn("sso exchange failed", "error", err)
		s.loginRedirectError(c, "SSO sign-in failed")
		return
	}

	// Accounts are matched by the immutable OIDC subject, never by the
	// username claim alone: usernames/emails are often self-editable at
	// the IdP, and matching by them would let anyone claim an existing
	// account (including admin) by renaming themselves.
	rec, exists := s.ACL.UserBySubject(identity.Subject)
	if !exists {
		byName, nameExists := s.ACL.User(identity.Username)
		switch {
		case nameExists && byName.OIDCSubject != "":
			// Linked to a different IdP identity — refuse.
			s.Log.Warn("sso subject mismatch", "username", identity.Username, "subject", identity.Subject)
			s.loginRedirectError(c, "account is linked to a different SSO identity")
			return
		case nameExists && (byName.Password != "" || byName.PasswordHash != ""):
			// Password accounts are never auto-linked: that would let an
			// IdP user impersonate them by choosing the same username.
			s.loginRedirectError(c, "account "+identity.Username+" uses password sign-in; ask an admin to link it to SSO")
			return
		case nameExists:
			// Legacy SSO-provisioned account (no password, no subject yet):
			// adopt the subject on first login after the upgrade.
			byName.OIDCSubject = identity.Subject
			if err := s.ACL.UpsertUser(byName); err != nil {
				s.loginRedirectError(c, "failed to link account")
				return
			}
			s.Log.Info("sso account linked", "username", byName.Username, "subject", identity.Subject)
			rec = byName
		default:
			if !cfg.AutoProvision {
				s.loginRedirectError(c, "account is not provisioned: "+identity.Username)
				return
			}
			role := cfg.DefaultRole
			if !s.roleKnown(role) {
				role = auth.RoleViewer
			}
			// SSO-only account: no password, sign-in works only through the
			// provider (Authenticate rejects empty credentials).
			rec = acl.UserRecord{Username: identity.Username, Role: role, Groups: identity.Groups, OIDCSubject: identity.Subject}
			if err := s.ACL.UpsertUser(rec); err != nil {
				s.loginRedirectError(c, "failed to provision account")
				return
			}
			s.Log.Info("sso user provisioned", "username", identity.Username, "role", role, "subject", identity.Subject)
		}
	}
	role := rec.Role
	if role == "" {
		role = auth.RoleViewer
	}
	token, _, err := s.Auth.IssueSession(auth.User{Username: rec.Username, Role: role}, rec.TokenVersion)
	if err != nil {
		s.loginRedirectError(c, "failed to issue session")
		return
	}
	c.Redirect(http.StatusFound, "/login?sso_token="+url.QueryEscape(token))
}

func (s *Server) loginRedirectError(c *gin.Context, message string) {
	c.Redirect(http.StatusFound, "/login?sso_error="+url.QueryEscape(message))
}

// handleMe returns the claims of the presented token — used by the
// frontend to hydrate the session after an SSO redirect.
func (s *Server) handleMe(c *gin.Context) {
	claims := claimsOf(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"username":    claims.Username,
		"role":        claims.Role,
		"permissions": claims.Permissions,
	})
}
