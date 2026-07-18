package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
)

// audit logs a security-relevant state change with the acting user, so
// account/ACL/role modifications are traceable after the fact.
func (s *Server) audit(c *gin.Context, action string, kv ...any) {
	args := append([]any{"actor", actor(c), "action", action}, kv...)
	s.Log.Info("audit", args...)
}

// storeError maps user-store failures to HTTP. A concurrent users.yaml
// edit is the caller's to resolve (409 → reload and retry); other errors
// keep the handler's usual status, with 500s going through internalError
// so details stay out of responses.
func (s *Server) storeError(c *gin.Context, err error, fallback int) {
	switch {
	case errors.Is(err, acl.ErrConcurrentEdit):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case fallback == http.StatusInternalServerError:
		s.internalError(c, err)
	default:
		c.JSON(fallback, gin.H{"error": err.Error()})
	}
}

// aclOr503 guards admin endpoints that need the users.yaml store.
func (s *Server) aclOr503(c *gin.Context) *acl.Store {
	if s.ACL == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "user store is not configured"})
		return nil
	}
	return s.ACL
}

// --- user management (admin) -------------------------------------------

func (s *Server) handleAdminListUsers(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": store.Users(), "groups": store.SortedGroupNames()})
}

type adminUserRequest struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Role     string   `json:"role"`
	Groups   []string `json:"groups"`
}

func (s *Server) handleAdminCreateUser(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req adminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}
	if req.Role == "" {
		req.Role = auth.RoleViewer
	}
	if !s.roleKnown(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown role: " + req.Role})
		return
	}
	if _, exists := store.User(req.Username); exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user already exists"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		s.internalError(c, err)
		return
	}
	rec := acl.UserRecord{Username: req.Username, PasswordHash: string(hash), Role: req.Role, Groups: req.Groups}
	if err := store.UpsertUser(rec); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "user.create", "username", req.Username, "role", req.Role)
	c.JSON(http.StatusCreated, rec)
}

func (s *Server) handleAdminUpdateUser(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	name := c.Param("name")
	rec, ok := store.User(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	var req adminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role != "" {
		if !s.roleKnown(req.Role) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown role: " + req.Role})
			return
		}
		rec.Role = req.Role
	}
	if req.Groups != nil {
		rec.Groups = req.Groups
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rec.PasswordHash = string(hash)
		rec.Password = ""
	}
	if err := store.UpsertUser(rec); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "user.update", "username", name, "role", rec.Role, "passwordChanged", req.Password != "")
	c.JSON(http.StatusOK, rec)
}

func (s *Server) handleAdminDeleteUser(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	name := c.Param("name")
	if name == actor(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
		return
	}
	if err := store.DeleteUser(name); err != nil {
		s.storeError(c, err, http.StatusNotFound)
		return
	}
	s.audit(c, "user.delete", "username", name)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// handleAdminRevoke bumps the token version: every session and API
// token of the user becomes invalid immediately.
func (s *Server) handleAdminRevoke(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	v, err := store.BumpTokenVersion(c.Param("name"))
	if err != nil {
		s.storeError(c, err, http.StatusNotFound)
		return
	}
	s.audit(c, "user.revoke-sessions", "username", c.Param("name"))
	c.JSON(http.StatusOK, gin.H{"tokenVersion": v})
}

// --- ACL rules (admin) ---------------------------------------------------

func (s *Server) handleAdminGetACL(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": store.Rules()})
}

func (s *Server) handleAdminPutACL(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req struct {
		Rules []acl.Rule `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.SetRules(req.Rules); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "acl.update", "rules", len(req.Rules))
	c.JSON(http.StatusOK, gin.H{"rules": store.Rules()})
}

// handleAdminCheck computes the effective access of a user to a path —
// the admin's rule-debugging tool.
func (s *Server) handleAdminCheck(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	username := c.Query("user")
	path := c.Query("path")
	if username == "" || path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user and path query parameters are required"})
		return
	}
	// Folder ACL only narrows access; the role's global permissions are
	// the real ceiling. A viewer (no notes:edit) can never write, even on
	// an unrestricted path, so cap the ACL result by the role ceiling.
	role := s.userRole(username)
	effective := store.Access(username, path)
	if ceiling := s.roleAccessCeiling(role); ceiling < effective {
		effective = ceiling
	}
	c.JSON(http.StatusOK, gin.H{
		"user":   username,
		"path":   path,
		"role":   role,
		"access": effective.String(),
	})
}

// userRole resolves a user's role from the store, falling back to the
// static config accounts, then to viewer.
func (s *Server) userRole(username string) string {
	if s.ACL != nil {
		if rec, ok := s.ACL.User(username); ok && rec.Role != "" {
			return rec.Role
		}
	}
	if su, ok := s.Auth.StaticUser(username); ok && su.Role != "" {
		return su.Role
	}
	return auth.RoleViewer
}

// roleAccessCeiling maps a role's note permissions to the highest folder
// access it could ever have: notes:edit → write, notes:read → read.
func (s *Server) roleAccessCeiling(role string) acl.Access {
	if role == auth.RoleAdmin {
		return acl.AccessWrite
	}
	perms, ok := s.rolePermissions(role)
	if !ok {
		perms = auth.PermissionsForRole(role)
	}
	hasRead, hasEdit := false, false
	for _, p := range perms {
		switch p {
		case auth.PermNotesRead:
			hasRead = true
		case auth.PermNotesEdit:
			hasEdit = true
		}
	}
	switch {
	case hasEdit:
		return acl.AccessWrite
	case hasRead:
		return acl.AccessRead
	default:
		return acl.AccessNone
	}
}

// rolePermissions resolves a role's permissions from the dynamic store.
func (s *Server) rolePermissions(role string) ([]string, bool) {
	if s.ACL != nil {
		return s.ACL.PermissionsForRole(role)
	}
	return nil, false
}

func (s *Server) handleAdminReload(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	if err := store.Reload(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "reloaded"})
}

// --- groups (admin) --------------------------------------------------------

func (s *Server) handleAdminGroups(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": store.Groups()})
}

func (s *Server) handleAdminAddGroup(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if err := store.AddGroup(req.Name); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "group.create", "group", req.Name)
	c.JSON(http.StatusCreated, gin.H{"groups": store.Groups()})
}

func (s *Server) handleAdminDeleteGroup(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	if err := store.DeleteGroup(c.Param("name")); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "group.delete", "group", c.Param("name"))
	c.JSON(http.StatusOK, gin.H{"groups": store.Groups()})
}

// --- SSO configuration (admin) ---------------------------------------------

func (s *Server) handleAdminGetSSO(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	cfg := store.SSO()
	// Never expose the secret; report only whether one is set.
	hasSecret := cfg.ClientSecret != ""
	cfg.ClientSecret = ""
	c.JSON(http.StatusOK, gin.H{"sso": cfg, "hasSecret": hasSecret})
}

func (s *Server) handleAdminPutSSO(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req struct {
		SSO acl.SSOConfig `json:"sso"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SSO.DefaultRole != "" && !s.roleKnown(req.SSO.DefaultRole) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown defaultRole: " + req.SSO.DefaultRole})
		return
	}
	if err := store.SetSSO(req.SSO); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "sso.update", "enabled", req.SSO.Enabled, "issuer", req.SSO.Issuer)
	cfg := store.SSO()
	cfg.ClientSecret = ""
	c.JSON(http.StatusOK, gin.H{"sso": cfg})
}

// --- roles ------------------------------------------------------------------

// roleKnown reports whether a role name is defined (dynamic store first,
// then the built-in defaults for setups without a users.yaml).
func (s *Server) roleKnown(name string) bool {
	if s.ACL != nil && s.ACL.RoleExists(name) {
		return true
	}
	return auth.ValidRole(name)
}

func (s *Server) handleAdminListRoles(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"roles":       store.Roles(),
		"permissions": auth.AllPermissions(),
	})
}

type adminRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (s *Server) handleAdminCreateRole(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	var req adminRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role name is required"})
		return
	}
	if store.RoleExists(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role already exists"})
		return
	}
	if bad := invalidPermissions(req.Permissions); bad != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown permission: " + bad})
		return
	}
	rec := acl.RoleRecord{Name: req.Name, Description: req.Description, Permissions: req.Permissions}
	if err := store.UpsertRole(rec); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "role.create", "role", req.Name, "permissions", req.Permissions)
	c.JSON(http.StatusCreated, rec)
}

func (s *Server) handleAdminUpdateRole(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	name := c.Param("name")
	if !store.RoleExists(name) {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	var req adminRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if bad := invalidPermissions(req.Permissions); bad != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown permission: " + bad})
		return
	}
	rec := acl.RoleRecord{Name: name, Description: req.Description, Permissions: req.Permissions}
	if err := store.UpsertRole(rec); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "role.update", "role", name, "permissions", req.Permissions)
	c.JSON(http.StatusOK, rec)
}

func (s *Server) handleAdminDeleteRole(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	if err := store.DeleteRole(c.Param("name")); err != nil {
		s.storeError(c, err, http.StatusBadRequest)
		return
	}
	s.audit(c, "role.delete", "role", c.Param("name"))
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// invalidPermissions returns the first permission not in the catalog, or
// "" when all are valid.
func invalidPermissions(perms []string) string {
	known := map[string]bool{}
	for _, p := range auth.AllPermissions() {
		known[p] = true
	}
	for _, p := range perms {
		if !known[p] {
			return p
		}
	}
	return ""
}

// --- plugins ----------------------------------------------------------------

// pluginEnabled resolves the persisted enabled state (default: on).
func (s *Server) pluginEnabled(id string) bool {
	if s.ACL == nil {
		return true
	}
	return s.ACL.PluginEnabled(id)
}

func (s *Server) handleListPlugins(c *gin.Context) {
	c.JSON(http.StatusOK, s.Plugins.Statuses(s.pluginEnabled))
}

func (s *Server) handleAdminSetPlugin(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	id := c.Param("id")
	if !s.Plugins.Known(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown plugin: " + id})
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled (boolean) is required"})
		return
	}
	if err := store.SetPluginEnabled(id, *req.Enabled); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "plugin.toggle", "plugin", id, "enabled", *req.Enabled)
	if s.Bus != nil {
		s.Bus.Publish(core.Event{Type: core.EventPluginChanged, Actor: actor(c)})
	}
	c.JSON(http.StatusOK, s.Plugins.Statuses(s.pluginEnabled))
}

// --- personal API tokens --------------------------------------------------

func claimsOf(c *gin.Context) *auth.Claims {
	if v, ok := c.Get("user"); ok {
		if claims, ok := v.(*auth.Claims); ok {
			return claims
		}
	}
	return nil
}

func (s *Server) handleListTokens(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	rec, ok := store.User(actor(c))
	if !ok {
		c.JSON(http.StatusOK, []acl.TokenRecord{})
		return
	}
	if rec.Tokens == nil {
		rec.Tokens = []acl.TokenRecord{}
	}
	c.JSON(http.StatusOK, rec.Tokens)
}

func (s *Server) handleCreateToken(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	claims := claimsOf(c)
	if claims == nil || claims.Kind == auth.KindAPI {
		c.JSON(http.StatusForbidden, gin.H{"error": "API tokens cannot mint tokens"})
		return
	}
	rec, ok := store.User(claims.Username)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API tokens are available only for users from the user store"})
		return
	}
	var req struct {
		Name        string   `json:"name"`
		TTLDays     int      `json:"ttlDays"`
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Requested permissions may only narrow the user's role permissions.
	rolePerms := auth.PermissionsForRole(rec.Role)
	perms := req.Permissions
	if len(perms) == 0 {
		perms = rolePerms
	} else {
		for _, p := range perms {
			if !containsString(rolePerms, p) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "permission exceeds your role: " + p})
				return
			}
		}
	}

	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		s.internalError(c, err)
		return
	}
	jti := hex.EncodeToString(jtiBytes)

	var ttl time.Duration
	var expiresAt *time.Time
	if req.TTLDays > 0 {
		ttl = time.Duration(req.TTLDays) * 24 * time.Hour
		t := time.Now().Add(ttl)
		expiresAt = &t
	}

	user := auth.User{Username: rec.Username, Role: rec.Role}
	token, _, err := s.Auth.IssueAPIToken(user, rec.TokenVersion, jti, perms, ttl)
	if err != nil {
		s.internalError(c, err)
		return
	}
	record := acl.TokenRecord{
		ID:          jti,
		Name:        req.Name,
		Permissions: perms,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	}
	if err := store.AddToken(rec.Username, record); err != nil {
		s.storeError(c, err, http.StatusInternalServerError)
		return
	}
	s.audit(c, "token.create", "name", req.Name, "id", jti, "permissions", perms)
	// The token itself is shown exactly once and never stored.
	c.JSON(http.StatusCreated, gin.H{"token": token, "record": record})
}

func (s *Server) handleRevokeToken(c *gin.Context) {
	store := s.aclOr503(c)
	if store == nil {
		return
	}
	if err := store.RevokeToken(actor(c), c.Param("id")); err != nil {
		s.storeError(c, err, http.StatusNotFound)
		return
	}
	s.audit(c, "token.revoke", "id", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
