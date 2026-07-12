// Package auth provides local-admin authentication with JWT tokens and
// role-based access control. OAuth providers can be added as separate
// modules implementing the same token issuance.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Roles, ordered by privilege.
const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

var roleRank = map[string]int{RoleViewer: 1, RoleEditor: 2, RoleAdmin: 3}

// ValidRole reports whether the string names a known role.
func ValidRole(role string) bool {
	_, ok := roleRank[role]
	return ok
}

// Permissions embedded into JWT tokens. The API enforces them and the
// frontend uses them to show or hide actions. The role → permission
// mapping is documented in docs/api.md.
const (
	PermNotesRead   = "notes:read"
	PermNotesEdit   = "notes:edit"
	PermNotesDelete = "notes:delete"
	PermUpload      = "files:upload"
	PermSettings    = "settings:write"
)

var rolePermissions = map[string][]string{
	RoleViewer: {PermNotesRead},
	RoleEditor: {PermNotesRead, PermNotesEdit, PermNotesDelete, PermUpload},
	RoleAdmin:  {PermNotesRead, PermNotesEdit, PermNotesDelete, PermUpload, PermSettings},
}

// PermissionsForRole returns the permission set granted by a role.
func PermissionsForRole(role string) []string {
	perms := rolePermissions[role]
	out := make([]string, len(perms))
	copy(out, perms)
	return out
}

// ErrInvalidCredentials is returned on failed login.
var ErrInvalidCredentials = errors.New("invalid credentials")

// User is a locally configured account.
type User struct {
	Username     string
	Password     string // plaintext (dev only)
	PasswordHash string // bcrypt
	Role         string
}

// Token kinds.
const (
	KindSession = ""    // interactive login
	KindAPI     = "api" // personal API token
)

// Claims are the JWT claims issued by the service.
type Claims struct {
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
	// TokenVersion must match the user's current version; bumping the
	// version revokes every outstanding token at once.
	TokenVersion int `json:"tv,omitempty"`
	// Kind distinguishes sessions from personal API tokens.
	Kind string `json:"knd,omitempty"`
	jwt.RegisteredClaims
}

// HasPermission checks the permission list embedded in the token.
// Tokens issued before permissions existed fall back to the role map.
func (c *Claims) HasPermission(perm string) bool {
	perms := c.Permissions
	if len(perms) == 0 {
		perms = rolePermissions[c.Role]
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// Service issues and validates JWT tokens. When disabled, every request
// is treated as an admin (single-user local mode).
type Service struct {
	Enabled bool
	secret  []byte
	ttl     time.Duration
	users   map[string]User
}

// NewService builds the auth service from configuration. Accounts
// without an explicit role get the least-privileged one (viewer).
func NewService(enabled bool, secret string, ttl time.Duration, users []User) *Service {
	m := map[string]User{}
	for _, u := range users {
		if u.Role == "" {
			u.Role = RoleViewer
		}
		m[u.Username] = u
	}
	return &Service{Enabled: enabled, secret: []byte(secret), ttl: ttl, users: m}
}

// StaticUser looks up an account configured in config.yaml (the
// emergency admin and legacy auth.users entries).
func (s *Service) StaticUser(username string) (User, bool) {
	u, ok := s.users[username]
	return u, ok
}

// Authenticate verifies a password against a user record.
func Authenticate(u User, password string) error {
	switch {
	case u.PasswordHash != "":
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
			return ErrInvalidCredentials
		}
	case u.Password != "":
		if u.Password != password {
			return ErrInvalidCredentials
		}
	default:
		return ErrInvalidCredentials
	}
	return nil
}

// Login verifies credentials of a statically configured user and
// returns a signed session JWT.
func (s *Service) Login(username, password string) (string, *Claims, error) {
	u, ok := s.users[username]
	if !ok {
		return "", nil, ErrInvalidCredentials
	}
	if err := Authenticate(u, password); err != nil {
		return "", nil, err
	}
	return s.IssueSession(u, 0)
}

// IssueSession signs a session token for an already authenticated user.
func (s *Service) IssueSession(u User, tokenVersion int) (string, *Claims, error) {
	role := u.Role
	if role == "" {
		role = RoleViewer
	}
	return s.issue(&Claims{
		Username:     u.Username,
		Role:         role,
		Permissions:  PermissionsForRole(role),
		TokenVersion: tokenVersion,
	}, s.ttl)
}

// IssueAPIToken signs a personal API token. permissions must already be
// narrowed to a subset of the user's role permissions; ttl <= 0 means
// no expiry.
func (s *Service) IssueAPIToken(u User, tokenVersion int, jti string, permissions []string, ttl time.Duration) (string, *Claims, error) {
	claims := &Claims{
		Username:     u.Username,
		Role:         u.Role,
		Permissions:  permissions,
		TokenVersion: tokenVersion,
		Kind:         KindAPI,
	}
	claims.ID = jti
	return s.issue(claims, ttl)
}

func (s *Service) issue(claims *Claims, ttl time.Duration) (string, *Claims, error) {
	now := time.Now()
	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.Subject = claims.Username
	if ttl > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(ttl))
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", nil, err
	}
	return token, claims, nil
}

// Validate parses and verifies a token string.
func (s *Service) Validate(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidCredentials
	}
	return claims, nil
}

// Allows reports whether a role satisfies the required role.
func Allows(role, required string) bool {
	return roleRank[role] >= roleRank[required]
}
