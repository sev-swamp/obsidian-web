// Service API: endpoints for external auth plugins (plan 4). A plugin
// authenticates with a service token (svc_<id>_<secret>, bcrypt-checked
// against users.yaml) and may provision users and mint one-time login
// codes; the browser exchanges a code for a regular session JWT. The
// token itself can never issue a session directly.

package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/auth"
)

// loginCodeTTL is the lifetime of a one-time login code. Codes are
// kept in memory only: after a restart the user simply signs in again.
const loginCodeTTL = 30 * time.Second

const serviceTokenPrefix = "svc_"

// serviceTokenIDRe keeps ids URL- and parse-safe: the secret separator
// is "_", so ids must not contain it.
var serviceTokenIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// --- one-time login codes --------------------------------------------------

type loginCode struct {
	username string
	expires  time.Time
}

// loginCodeStore holds pending one-time codes (map with a lock; pruned
// on every operation).
type loginCodeStore struct {
	mu    sync.Mutex
	codes map[string]loginCode
}

func newLoginCodeStore() *loginCodeStore {
	return &loginCodeStore{codes: map[string]loginCode{}}
}

func (l *loginCodeStore) issue(username string, ttl time.Duration) (string, time.Time, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	code := hex.EncodeToString(raw)
	expires := time.Now().Add(ttl)
	l.mu.Lock()
	l.prune()
	l.codes[code] = loginCode{username: username, expires: expires}
	l.mu.Unlock()
	return code, expires, nil
}

// redeem burns the code on first use; expired codes fail.
func (l *loginCodeStore) redeem(code string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune()
	entry, ok := l.codes[code]
	if !ok {
		return "", false
	}
	delete(l.codes, code)
	return entry.username, true
}

// prune drops expired codes; caller holds the lock.
func (l *loginCodeStore) prune() {
	now := time.Now()
	for code, entry := range l.codes {
		if now.After(entry.expires) {
			delete(l.codes, code)
		}
	}
}

// --- service token middleware ----------------------------------------------

const serviceTokenKey = "serviceToken"

// serviceTokenOf returns the authenticated service token record.
func serviceTokenOf(c *gin.Context) (acl.ServiceTokenRecord, bool) {
	if v, ok := c.Get(serviceTokenKey); ok {
		if rec, ok := v.(acl.ServiceTokenRecord); ok {
			return rec, true
		}
	}
	return acl.ServiceTokenRecord{}, false
}

// requireServicePermission authenticates a service token from the
// Authorization header and checks its explicit permission list. JWTs
// are never accepted here — user sessions cannot call the service API.
func (s *Server) requireServicePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.ACL == nil || !s.Auth.Enabled {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "service API requires auth and a user store"})
			return
		}
		header := c.GetHeader("Authorization")
		tokenString := strings.TrimPrefix(header, "Bearer ")
		if tokenString == header || !strings.HasPrefix(tokenString, serviceTokenPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "service token required"})
			return
		}
		id, secret, ok := strings.Cut(strings.TrimPrefix(tokenString, serviceTokenPrefix), "_")
		if !ok || id == "" || secret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid service token"})
			return
		}
		rec, found := s.ACL.ServiceToken(id)
		if !found || rec.Revoked ||
			bcrypt.CompareHashAndPassword([]byte(rec.TokenHash), []byte(secret)) != nil {
			s.Log.Warn("service token rejected", "id", id, "known", found)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid service token"})
			return
		}
		if !containsString(rec.Permissions, perm) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing permission: " + perm})
			return
		}
		c.Set(serviceTokenKey, rec)
		c.Next()
	}
}

// --- provisioning (users:provision) ----------------------------------------

// roleWithinCeiling reports whether a role may be assigned by a token
// capped at ceiling. Instead of a fixed rank (custom roles have none),
// the role's effective permissions must be a subset of the ceiling's;
// admin is only ever within an admin ceiling.
func (s *Server) roleWithinCeiling(role, ceiling string) bool {
	if ceiling == "" {
		ceiling = auth.RoleViewer
	}
	if role == ceiling {
		return true
	}
	if role == auth.RoleAdmin {
		return false
	}
	if ceiling == auth.RoleAdmin {
		return true
	}
	allowed := map[string]bool{}
	for _, p := range s.effectiveRolePermissions(ceiling) {
		allowed[p] = true
	}
	for _, p := range s.effectiveRolePermissions(role) {
		if !allowed[p] {
			return false
		}
	}
	return true
}

// effectiveRolePermissions resolves a role through the dynamic store
// with the built-in defaults as fallback.
func (s *Server) effectiveRolePermissions(role string) []string {
	if perms, ok := s.rolePermissions(role); ok {
		return perms
	}
	return auth.PermissionsForRole(role)
}

// handleServiceUpsertUser creates or updates an SSO-only account
// {username, role, groups}. It never touches passwords, never deletes,
// and refuses anything above the token's role ceiling — including
// modifying an existing user who already outranks it.
func (s *Server) handleServiceUpsertUser(c *gin.Context) {
	token, _ := serviceTokenOf(c)
	var req struct {
		Username string   `json:"username"`
		Role     string   `json:"role"`
		Groups   []string `json:"groups"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}
	if req.Role == "" {
		req.Role = auth.RoleViewer
	}
	if !s.roleKnown(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown role: " + req.Role})
		return
	}
	if !s.roleWithinCeiling(req.Role, token.RoleCeiling) {
		s.audit(c, "service.provision-denied", "username", req.Username, "role", req.Role, "ceiling", token.RoleCeiling)
		c.JSON(http.StatusForbidden, gin.H{"error": "role exceeds the token's ceiling: " + req.Role})
		return
	}
	// A store record shadows config.yaml accounts at login, so
	// provisioning one for a static user would break (or hijack) the
	// emergency admin sign-in.
	if _, static := s.Auth.StaticUser(req.Username); static {
		c.JSON(http.StatusForbidden, gin.H{"error": "user is statically configured"})
		return
	}
	rec, exists := s.ACL.User(req.Username)
	created := !exists
	if exists {
		if !s.roleWithinCeiling(rec.Role, token.RoleCeiling) {
			s.audit(c, "service.provision-denied", "username", req.Username, "existingRole", rec.Role, "ceiling", token.RoleCeiling)
			c.JSON(http.StatusForbidden, gin.H{"error": "existing user outranks the token's ceiling"})
			return
		}
		rec.Role = req.Role
		if req.Groups != nil {
			rec.Groups = req.Groups
		}
		// Strip secrets so UpsertUser keeps the stored ones.
		rec.Password, rec.PasswordHash = "", ""
	} else {
		// SSO-only account: no password, sign-in only via the provider.
		rec = acl.UserRecord{Username: req.Username, Role: req.Role, Groups: req.Groups}
	}
	if err := s.ACL.UpsertUser(rec); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "service.user-provision", "username", req.Username, "role", req.Role, "created", created)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	rec, _ = s.ACL.User(req.Username)
	c.JSON(status, rec)
}

// --- login codes (session:code) --------------------------------------------

// handleServiceLoginCode mints a one-time code for an existing user.
// The user's existence and tokenVersion are re-checked at exchange
// time, so a deletion between issue and redeem still fails closed.
func (s *Server) handleServiceLoginCode(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}
	if _, ok := s.ACL.User(req.Username); !ok {
		s.audit(c, "service.login-code-denied", "username", req.Username)
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	code, expires, err := s.loginCodes.issue(req.Username, loginCodeTTL)
	if err != nil {
		s.internalError(c, err)
		return
	}
	s.audit(c, "service.login-code", "username", req.Username)
	c.JSON(http.StatusOK, gin.H{"code": code, "expiresAt": expires})
}

// handleAuthCode exchanges a one-time login code for a session JWT
// (anonymous endpoint — the code is the credential).
func (s *Server) handleAuthCode(c *gin.Context) {
	if s.ACL == nil || !s.Auth.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "login codes are not available"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}
	limitKey := "login-code|" + c.ClientIP()
	if !s.loginLimits.Allow(limitKey) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many failed attempts, try again later"})
		return
	}
	username, ok := s.loginCodes.redeem(req.Code)
	if !ok {
		s.loginLimits.Fail(limitKey)
		s.Log.Info("audit", "action", "auth.code-exchange", "result", "invalid")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired code"})
		return
	}
	rec, exists := s.ACL.User(username)
	if !exists {
		s.loginLimits.Fail(limitKey)
		s.Log.Info("audit", "action", "auth.code-exchange", "username", username, "result", "user-gone")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired code"})
		return
	}
	s.loginLimits.Reset(limitKey)
	role := rec.Role
	if role == "" {
		role = auth.RoleViewer
	}
	// The current tokenVersion is embedded, so revoking the user's
	// sessions also kills sessions issued through login codes.
	token, claims, err := s.Auth.IssueSession(auth.User{Username: rec.Username, Role: role}, rec.TokenVersion)
	if err != nil {
		s.internalError(c, err)
		return
	}
	s.Log.Info("audit", "action", "auth.code-exchange", "username", username, "result", "ok")
	c.JSON(http.StatusOK, gin.H{
		"token":       token,
		"username":    claims.Username,
		"role":        claims.Role,
		"permissions": claims.Permissions,
	})
}

// --- login providers --------------------------------------------------------

// handleAuthProviders lists external sign-in options for the login page
// (anonymous — the page renders one button per provider).
func (s *Server) handleAuthProviders(c *gin.Context) {
	providers := []acl.LoginProvider{}
	if s.ACL != nil && s.Auth.Enabled {
		providers = s.ACL.LoginProviders()
	}
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

// handleServiceUpsertProvider lets a plugin register (or update) its
// own login button (login-providers:write).
func (s *Server) handleServiceUpsertProvider(c *gin.Context) {
	var req acl.LoginProvider
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.ACL.UpsertLoginProvider(req); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "login-provider.upsert", "id", req.ID, "url", req.URL)
	c.JSON(http.StatusOK, gin.H{"providers": s.ACL.LoginProviders()})
}

// --- admin: service tokens & login providers -------------------------------

func (s *Server) handleAdminListServiceTokens(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	tokens := store.ServiceTokens()
	if tokens == nil {
		tokens = []acl.ServiceTokenRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens, "permissions": auth.ServiceTokenPermissions()})
}

func (s *Server) handleAdminCreateServiceToken(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
		RoleCeiling string   `json:"roleCeiling"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id and name are required"})
		return
	}
	if !serviceTokenIDRe.MatchString(req.ID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be lowercase letters, digits, dots or dashes"})
		return
	}
	if len(req.Permissions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one permission is required"})
		return
	}
	catalog := auth.ServiceTokenPermissions()
	for _, p := range req.Permissions {
		if !containsString(catalog, p) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown permission: " + p})
			return
		}
	}
	if req.RoleCeiling == "" {
		req.RoleCeiling = auth.RoleViewer
	}
	if !s.roleKnown(req.RoleCeiling) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown roleCeiling: " + req.RoleCeiling})
		return
	}
	secretBytes := make([]byte, 24)
	if _, err := rand.Read(secretBytes); err != nil {
		s.internalError(c, err)
		return
	}
	secret := hex.EncodeToString(secretBytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 10)
	if err != nil {
		s.internalError(c, err)
		return
	}
	rec := acl.ServiceTokenRecord{
		ID:          req.ID,
		Name:        req.Name,
		TokenHash:   string(hash),
		Permissions: req.Permissions,
		RoleCeiling: req.RoleCeiling,
		CreatedAt:   time.Now(),
	}
	if err := store.AddServiceToken(rec); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "service-token.create", "id", req.ID, "permissions", req.Permissions, "roleCeiling", req.RoleCeiling)
	// The full token is shown exactly once and never stored.
	c.JSON(http.StatusCreated, gin.H{
		"token":  serviceTokenPrefix + req.ID + "_" + secret,
		"record": rec,
	})
}

func (s *Server) handleAdminRevokeServiceToken(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	if err := store.RevokeServiceToken(c.Param("id")); err != nil {
		s.storeError(c, err, http.StatusNotFound)
		return
	}
	s.audit(c, "service-token.revoke", "id", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func (s *Server) handleAdminGetLoginProviders(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	providers := store.LoginProviders()
	if providers == nil {
		providers = []acl.LoginProvider{}
	}
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

func (s *Server) handleAdminPutLoginProviders(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req struct {
		Providers []acl.LoginProvider `json:"providers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.SetLoginProviders(req.Providers); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "login-providers.update", "count", len(req.Providers))
	c.JSON(http.StatusOK, gin.H{"providers": store.LoginProviders()})
}
