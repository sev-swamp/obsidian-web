// Package acl implements folder-level access control for team vaults:
// users with groups (users.yaml, hot-reloadable), ordered glob rules,
// per-user API tokens and session revocation via token versions.
//
// The global role from the JWT remains the ceiling — ACL rules can only
// narrow access, never widen it (enforced by the permission middleware
// running before ACL checks).
package acl

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Access levels, ordered.
type Access int

const (
	AccessNone Access = iota
	AccessRead
	AccessWrite
)

func (a Access) String() string {
	switch a {
	case AccessRead:
		return "read"
	case AccessWrite:
		return "write"
	default:
		return "none"
	}
}

func parseAccess(s string) Access {
	switch s {
	case "write":
		return AccessWrite
	case "read":
		return AccessRead
	default:
		return AccessNone
	}
}

// UserRecord is an account managed through the admin API.
type UserRecord struct {
	Username     string        `yaml:"username" json:"username"`
	Password     string        `yaml:"password,omitempty" json:"-"`
	PasswordHash string        `yaml:"passwordHash,omitempty" json:"-"`
	Role         string        `yaml:"role" json:"role"`
	Groups       []string      `yaml:"groups,omitempty" json:"groups"`
	TokenVersion int           `yaml:"tokenVersion,omitempty" json:"tokenVersion"`
	Tokens       []TokenRecord `yaml:"tokens,omitempty" json:"tokens,omitempty"`
}

// TokenRecord is metadata of an issued personal API token (the JWT
// itself is never stored).
type TokenRecord struct {
	ID          string     `yaml:"id" json:"id"`
	Name        string     `yaml:"name" json:"name"`
	Permissions []string   `yaml:"permissions" json:"permissions"`
	CreatedAt   time.Time  `yaml:"createdAt" json:"createdAt"`
	ExpiresAt   *time.Time `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	Revoked     bool       `yaml:"revoked,omitempty" json:"revoked"`
}

// Grant gives a user or group an access level within a rule.
type Grant struct {
	User   string `yaml:"user,omitempty" json:"user,omitempty"`
	Group  string `yaml:"group,omitempty" json:"group,omitempty"`
	Access string `yaml:"access" json:"access"` // read | write
}

// Rule restricts a glob of vault paths. Rules are evaluated in order;
// the first matching rule decides. Paths without a matching rule are
// unrestricted.
type Rule struct {
	Path    string  `yaml:"path" json:"path"`
	Allow   []Grant `yaml:"allow,omitempty" json:"allow,omitempty"`
	Default string  `yaml:"default,omitempty" json:"default,omitempty"` // none | read | write ("" = none)
	Special string  `yaml:"special,omitempty" json:"special,omitempty"` // "owner": Private/<user>/…
}

// SSOConfig is the OIDC single-sign-on configuration, editable from
// the settings UI and stored alongside users.
type SSOConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	Name          string `yaml:"name" json:"name"` // button label, e.g. "Keycloak"
	Issuer        string `yaml:"issuer" json:"issuer"`
	ClientID      string `yaml:"clientId" json:"clientId"`
	ClientSecret  string `yaml:"clientSecret" json:"clientSecret,omitempty"`
	RedirectURL   string `yaml:"redirectUrl" json:"redirectUrl"`
	DefaultRole   string `yaml:"defaultRole" json:"defaultRole"`
	AutoProvision bool   `yaml:"autoProvision" json:"autoProvision"`
}

type fileData struct {
	Users  []UserRecord `yaml:"users"`
	Groups []string     `yaml:"groups"`
	ACL    []Rule       `yaml:"acl"`
	SSO    *SSOConfig   `yaml:"sso,omitempty"`
}

// Store holds users, groups and ACL rules backed by users.yaml.
type Store struct {
	path string

	mu     sync.RWMutex
	users  map[string]*UserRecord
	order  []string // stable listing order
	groups []string // explicitly declared groups
	rules  []Rule
	sso    SSOConfig
}

// Load reads users.yaml; a missing file yields an empty store that will
// be created on the first Save.
func Load(path string) (*Store, error) {
	s := &Store{path: path, users: map[string]*UserRecord{}}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// Reload re-reads the backing file (manual edits + POST /api/admin/reload).
func (s *Store) Reload() error {
	var data fileData
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.users = map[string]*UserRecord{}
			s.order = nil
			s.groups = nil
			s.rules = nil
			s.sso = SSOConfig{}
			s.mu.Unlock()
			return nil
		}
		return err
	}
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse %s: %w", s.path, err)
	}
	if err := validateRules(data.ACL); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	users := map[string]*UserRecord{}
	var order []string
	for i := range data.Users {
		u := data.Users[i]
		if u.Username == "" {
			return fmt.Errorf("%s: user #%d has no username", s.path, i)
		}
		if _, dup := users[u.Username]; dup {
			return fmt.Errorf("%s: duplicate username %q", s.path, u.Username)
		}
		users[u.Username] = &u
		order = append(order, u.Username)
	}
	s.mu.Lock()
	s.users = users
	s.order = order
	s.groups = data.Groups
	s.rules = data.ACL
	if data.SSO != nil {
		s.sso = *data.SSO
	} else {
		s.sso = SSOConfig{}
	}
	s.mu.Unlock()
	return nil
}

// Save persists the store atomically (tmp + rename).
func (s *Store) Save() error {
	s.mu.RLock()
	data := fileData{ACL: s.rules, Groups: s.groups}
	if s.sso != (SSOConfig{}) {
		sso := s.sso
		data.SSO = &sso
	}
	for _, name := range s.order {
		data.Users = append(data.Users, *s.users[name])
	}
	s.mu.RUnlock()

	raw, err := yaml.Marshal(&data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func validateRules(rules []Rule) error {
	for i, r := range rules {
		if r.Path == "" {
			return fmt.Errorf("acl rule #%d: path is required", i)
		}
		if !doublestar.ValidatePattern(r.Path) {
			return fmt.Errorf("acl rule #%d: invalid glob %q", i, r.Path)
		}
		if r.Default != "" && r.Default != "none" && r.Default != "read" && r.Default != "write" {
			return fmt.Errorf("acl rule #%d: unknown default %q", i, r.Default)
		}
		if r.Special != "" && r.Special != "owner" {
			return fmt.Errorf("acl rule #%d: unknown special %q", i, r.Special)
		}
		for j, g := range r.Allow {
			if g.User == "" && g.Group == "" {
				return fmt.Errorf("acl rule #%d grant #%d: user or group is required", i, j)
			}
			if g.Access != "read" && g.Access != "write" {
				return fmt.Errorf("acl rule #%d grant #%d: access must be read or write", i, j)
			}
		}
	}
	return nil
}

// --- users --------------------------------------------------------------

// User returns a copy of the record.
func (s *Store) User(username string) (UserRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return UserRecord{}, false
	}
	return *u, true
}

// Users lists records in stable order (secrets stripped by json tags).
func (s *Store) Users() []UserRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UserRecord, 0, len(s.order))
	for _, name := range s.order {
		out = append(out, *s.users[name])
	}
	return out
}

// UpsertUser creates or updates a record and persists the store.
func (s *Store) UpsertUser(u UserRecord) error {
	s.mu.Lock()
	if existing, ok := s.users[u.Username]; ok {
		if u.PasswordHash == "" {
			u.PasswordHash = existing.PasswordHash
			u.Password = existing.Password
		}
		u.TokenVersion = existing.TokenVersion
		u.Tokens = existing.Tokens
	} else {
		s.order = append(s.order, u.Username)
	}
	s.users[u.Username] = &u
	s.mu.Unlock()
	return s.Save()
}

// DeleteUser removes a record and persists the store.
func (s *Store) DeleteUser(username string) error {
	s.mu.Lock()
	if _, ok := s.users[username]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("user %q not found", username)
	}
	delete(s.users, username)
	for i, name := range s.order {
		if name == username {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return s.Save()
}

// BumpTokenVersion invalidates every session and API token of a user.
func (s *Store) BumpTokenVersion(username string) (int, error) {
	s.mu.Lock()
	u, ok := s.users[username]
	if !ok {
		s.mu.Unlock()
		return 0, fmt.Errorf("user %q not found", username)
	}
	u.TokenVersion++
	v := u.TokenVersion
	s.mu.Unlock()
	return v, s.Save()
}

// --- API tokens ----------------------------------------------------------

// AddToken records issued API token metadata.
func (s *Store) AddToken(username string, t TokenRecord) error {
	s.mu.Lock()
	u, ok := s.users[username]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("user %q not found", username)
	}
	u.Tokens = append(u.Tokens, t)
	s.mu.Unlock()
	return s.Save()
}

// RevokeToken marks a token revoked.
func (s *Store) RevokeToken(username, id string) error {
	s.mu.Lock()
	u, ok := s.users[username]
	if ok {
		for i := range u.Tokens {
			if u.Tokens[i].ID == id {
				u.Tokens[i].Revoked = true
				s.mu.Unlock()
				return s.Save()
			}
		}
	}
	s.mu.Unlock()
	return fmt.Errorf("token not found")
}

// TokenValid reports whether an API token (by jti) is known, not
// revoked and not expired.
func (s *Store) TokenValid(username, id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return false
	}
	for _, t := range u.Tokens {
		if t.ID == id {
			if t.Revoked {
				return false
			}
			if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
				return false
			}
			return true
		}
	}
	return false
}

// --- rules ----------------------------------------------------------------

// Rules returns a copy of the ACL rules.
func (s *Store) Rules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Rule, len(s.rules))
	copy(out, s.rules)
	return out
}

// SetRules validates and persists a new rule list.
func (s *Store) SetRules(rules []Rule) error {
	if err := validateRules(rules); err != nil {
		return err
	}
	s.mu.Lock()
	s.rules = rules
	s.mu.Unlock()
	return s.Save()
}

// Access resolves the effective access of a user to a vault path. The
// first matching rule decides; unmatched paths are unrestricted.
func (s *Store) Access(username, path string) Access {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var groups []string
	if u, ok := s.users[username]; ok {
		groups = u.Groups
	}
	for _, rule := range s.rules {
		matched, err := doublestar.Match(rule.Path, path)
		if err != nil || !matched {
			continue
		}
		if rule.Special == "owner" {
			if ownerSegment(rule.Path, path) == username && username != "" {
				return AccessWrite
			}
			return AccessNone
		}
		best := parseAccess(rule.Default)
		for _, g := range rule.Allow {
			if g.User != "" && g.User == username || g.Group != "" && contains(groups, g.Group) {
				if a := parseAccess(g.Access); a > best {
					best = a
				}
			}
		}
		return best
	}
	return AccessWrite
}

// AllowRead returns a predicate for filtering search/tree/backlinks.
func (s *Store) AllowRead(username string) func(path string) bool {
	return func(path string) bool {
		return s.Access(username, path) >= AccessRead
	}
}

// ownerSegment extracts the path segment matching the single-star
// position of an owner pattern like "Private/*/**".
func ownerSegment(pattern, path string) string {
	pSegs := strings.Split(pattern, "/")
	starIdx := -1
	for i, seg := range pSegs {
		if seg == "*" {
			starIdx = i
			break
		}
	}
	if starIdx < 0 {
		return ""
	}
	segs := strings.Split(path, "/")
	if starIdx >= len(segs) {
		return ""
	}
	return segs[starIdx]
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// SortedGroupNames lists group names declared or referenced anywhere.
func (s *Store) SortedGroupNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	set := map[string]bool{}
	for _, g := range s.groups {
		set[g] = true
	}
	for _, u := range s.users {
		for _, g := range u.Groups {
			set[g] = true
		}
	}
	for _, r := range s.rules {
		for _, g := range r.Allow {
			if g.Group != "" {
				set[g.Group] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// GroupInfo describes a group with its members (for the settings UI).
type GroupInfo struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

// Groups lists all groups with member usernames.
func (s *Store) Groups() []GroupInfo {
	names := s.SortedGroupNames()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GroupInfo, 0, len(names))
	for _, name := range names {
		info := GroupInfo{Name: name, Members: []string{}}
		for _, uname := range s.order {
			if contains(s.users[uname].Groups, name) {
				info.Members = append(info.Members, uname)
			}
		}
		out = append(out, info)
	}
	return out
}

// AddGroup declares a group (persisted even while it has no members).
func (s *Store) AddGroup(name string) error {
	if name == "" {
		return fmt.Errorf("group name is required")
	}
	s.mu.Lock()
	if !contains(s.groups, name) {
		s.groups = append(s.groups, name)
		sort.Strings(s.groups)
	}
	s.mu.Unlock()
	return s.Save()
}

// DeleteGroup removes the group declaration and strips it from users.
func (s *Store) DeleteGroup(name string) error {
	s.mu.Lock()
	for i, g := range s.groups {
		if g == name {
			s.groups = append(s.groups[:i], s.groups[i+1:]...)
			break
		}
	}
	for _, u := range s.users {
		for i, g := range u.Groups {
			if g == name {
				u.Groups = append(u.Groups[:i], u.Groups[i+1:]...)
				break
			}
		}
	}
	s.mu.Unlock()
	return s.Save()
}

// SSO returns the current single-sign-on configuration.
func (s *Store) SSO() SSOConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sso
}

// SetSSO validates and persists the SSO configuration.
func (s *Store) SetSSO(cfg SSOConfig) error {
	if cfg.Enabled && (cfg.Issuer == "" || cfg.ClientID == "") {
		return fmt.Errorf("issuer and clientId are required to enable SSO")
	}
	s.mu.Lock()
	// An empty secret in the update keeps the stored one (the UI never
	// receives the secret back).
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = s.sso.ClientSecret
	}
	s.sso = cfg
	s.mu.Unlock()
	return s.Save()
}
