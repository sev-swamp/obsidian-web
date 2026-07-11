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

// ErrInvalidCredentials is returned on failed login.
var ErrInvalidCredentials = errors.New("invalid credentials")

// User is a locally configured account.
type User struct {
	Username     string
	Password     string // plaintext (dev only)
	PasswordHash string // bcrypt
	Role         string
}

// Claims are the JWT claims issued by the service.
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
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

// Login verifies credentials and returns a signed JWT.
func (s *Service) Login(username, password string) (string, *Claims, error) {
	u, ok := s.users[username]
	if !ok {
		return "", nil, ErrInvalidCredentials
	}
	switch {
	case u.PasswordHash != "":
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
			return "", nil, ErrInvalidCredentials
		}
	case u.Password != "":
		if u.Password != password {
			return "", nil, ErrInvalidCredentials
		}
	default:
		return "", nil, ErrInvalidCredentials
	}

	claims := &Claims{
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   u.Username,
		},
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
