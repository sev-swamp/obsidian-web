// Package acl implements folder-level access control for team vaults:
// users with groups (users.yaml, hot-reloadable), ordered glob rules,
// per-user API tokens and session revocation via token versions.
//
// The global role from the JWT remains the ceiling — ACL rules can only
// narrow access, never widen it (enforced by the permission middleware
// running before ACL checks).
package acl

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// ErrConcurrentEdit is returned by Save when users.yaml changed on disk
// since it was loaded: saving would silently discard the manual edits.
// The caller must reload (POST /api/admin/reload) and retry.
var ErrConcurrentEdit = errors.New("users.yaml was modified outside the API; reload before saving")

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
	// OIDCSubject links the account to an SSO identity by the provider's
	// immutable subject claim. Set on provisioning/first link; SSO logins
	// match by it, never by (user-editable) username claims.
	OIDCSubject string `yaml:"oidcSubject,omitempty" json:"oidcSubject,omitempty"`
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

// RoleRecord is a role managed through the admin API: a named permission
// set with a human description. The three built-in roles (admin, editor,
// viewer) are seeded on install and cannot be deleted.
type RoleRecord struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description"`
	Permissions []string `yaml:"permissions,omitempty" json:"permissions"`
	// BuiltIn is computed, never persisted: it flags the protected defaults.
	BuiltIn bool `yaml:"-" json:"builtin"`
}

// ServiceTokenRecord is a machine credential for external services
// (SSO plugins). Unlike personal API tokens it is not a JWT: the secret
// is bcrypt-hashed here and verified per request, so revocation is
// instant via the users.yaml hot-reload. Permissions are explicit and
// never derived from a role; RoleCeiling caps what roles the token may
// assign when provisioning users.
type ServiceTokenRecord struct {
	ID          string    `yaml:"id" json:"id"`
	Name        string    `yaml:"name" json:"name"`
	TokenHash   string    `yaml:"tokenHash" json:"-"`
	Permissions []string  `yaml:"permissions" json:"permissions"`
	RoleCeiling string    `yaml:"roleCeiling" json:"roleCeiling"`
	CreatedAt   time.Time `yaml:"createdAt" json:"createdAt"`
	Revoked     bool      `yaml:"revoked,omitempty" json:"revoked"`
}

// LoginProvider is an external sign-in option rendered as a button on
// the login page; url points at the provider's (plugin's) start route.
type LoginProvider struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

// PluginState is the persisted per-plugin state. In YAML it accepts the
// legacy plain-bool form (`templates: false`) as well as the full form
// (`templates: {enabled: true, settings: {folder: Notes/Tpl}}`).
type PluginState struct {
	Enabled  bool              `yaml:"enabled"`
	Settings map[string]string `yaml:"settings,omitempty"`
}

func (p *PluginState) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var enabled bool
		if err := value.Decode(&enabled); err != nil {
			return err
		}
		*p = PluginState{Enabled: enabled}
		return nil
	}
	type raw PluginState
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*p = PluginState(r)
	return nil
}

// MarshalYAML keeps the compact bool form when a plugin has no settings.
func (p PluginState) MarshalYAML() (any, error) {
	if len(p.Settings) == 0 {
		return p.Enabled, nil
	}
	type raw PluginState
	return raw(p), nil
}

type fileData struct {
	Users  []UserRecord `yaml:"users"`
	Groups []string     `yaml:"groups"`
	Roles  []RoleRecord `yaml:"roles,omitempty"`
	ACL    []Rule       `yaml:"acl"`
	// A legacy `sso:` block (built-in SSO, removed in plan 4) is ignored
	// by the non-strict parser and dropped on the next Save.
	// Plugins holds per-plugin state; an absent entry means enabled
	// with default settings.
	Plugins        map[string]PluginState `yaml:"plugins,omitempty"`
	ServiceTokens  []ServiceTokenRecord   `yaml:"serviceTokens,omitempty"`
	LoginProviders []LoginProvider        `yaml:"loginProviders,omitempty"`
}

// Store holds users, groups and ACL rules backed by users.yaml.
type Store struct {
	path string

	mu        sync.RWMutex
	users     map[string]*UserRecord
	order     []string // stable listing order
	groups    []string // explicitly declared groups
	roles     []RoleRecord
	rules     []Rule
	plugins   map[string]PluginState
	svcTokens []ServiceTokenRecord
	providers []LoginProvider

	// loadedMod/loadedSize fingerprint the file as last read or written;
	// Save refuses to clobber a file that changed since (optimistic lock
	// against manual edits). Zero values mean "file did not exist".
	loadedMod  time.Time
	loadedSize int64
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
			s.roles = nil
			s.rules = nil
			s.plugins = nil
			s.svcTokens = nil
			s.providers = nil
			s.loadedMod, s.loadedSize = time.Time{}, 0
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
	s.roles = data.Roles
	s.rules = data.ACL
	s.plugins = data.Plugins
	s.svcTokens = data.ServiceTokens
	s.providers = data.LoginProviders
	s.loadedMod, s.loadedSize = time.Time{}, 0
	if info, err := os.Stat(s.path); err == nil {
		s.loadedMod, s.loadedSize = info.ModTime(), info.Size()
	}
	s.mu.Unlock()
	return nil
}

// Save persists the store atomically (tmp + rename). It fails with
// ErrConcurrentEdit when the on-disk file no longer matches the state
// this store was loaded from, so API writes never clobber manual edits.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if info, err := os.Stat(s.path); err == nil {
		if s.loadedMod.IsZero() || !info.ModTime().Equal(s.loadedMod) || info.Size() != s.loadedSize {
			return ErrConcurrentEdit
		}
	} // a missing file is fine: the first Save creates it

	data := fileData{ACL: s.rules, Groups: s.groups, Roles: s.roles, Plugins: s.plugins, ServiceTokens: s.svcTokens, LoginProviders: s.providers}
	for _, name := range s.order {
		data.Users = append(data.Users, *s.users[name])
	}

	raw, err := yaml.Marshal(&data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if info, err := os.Stat(s.path); err == nil {
		s.loadedMod, s.loadedSize = info.ModTime(), info.Size()
	}
	return nil
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

// UserBySubject finds the account linked to an OIDC subject.
func (s *Store) UserBySubject(subject string) (UserRecord, bool) {
	if subject == "" {
		return UserRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, name := range s.order {
		if u := s.users[name]; u.OIDCSubject == subject {
			return *u, true
		}
	}
	return UserRecord{}, false
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
		if u.OIDCSubject == "" {
			u.OIDCSubject = existing.OIDCSubject
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

// --- roles ----------------------------------------------------------------

// builtInRole reports whether a role name is a protected default.
func builtInRole(name string) bool {
	return name == "admin" || name == "editor" || name == "viewer"
}

// SeedRoles installs default role definitions for any that are missing.
// Called once at startup; existing (possibly customized) roles are kept.
func (s *Store) SeedRoles(defaults []RoleRecord) error {
	s.mu.Lock()
	changed := false
	have := map[string]bool{}
	for _, r := range s.roles {
		have[r.Name] = true
	}
	for _, d := range defaults {
		if !have[d.Name] {
			s.roles = append(s.roles, d)
			changed = true
		}
	}
	s.mu.Unlock()
	if !changed {
		return nil
	}
	return s.Save()
}

// Roles lists all roles with the built-in flag computed.
func (s *Store) Roles() []RoleRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RoleRecord, 0, len(s.roles))
	for _, r := range s.roles {
		r.BuiltIn = builtInRole(r.Name)
		out = append(out, r)
	}
	return out
}

// RoleExists reports whether a role is defined.
func (s *Store) RoleExists(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.roles {
		if r.Name == name {
			return true
		}
	}
	return false
}

// PermissionsForRole returns the permissions granted by a stored role.
// The boolean is false when the role is unknown (caller falls back to
// built-in defaults).
func (s *Store) PermissionsForRole(name string) ([]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.roles {
		if r.Name == name {
			out := make([]string, len(r.Permissions))
			copy(out, r.Permissions)
			return out, true
		}
	}
	return nil, false
}

// UpsertRole creates or updates a role. The admin role's permissions are
// fixed (superuser) and only its description may change.
func (s *Store) UpsertRole(r RoleRecord) error {
	if r.Name == "" {
		return fmt.Errorf("role name is required")
	}
	s.mu.Lock()
	for i := range s.roles {
		if s.roles[i].Name == r.Name {
			s.roles[i].Description = r.Description
			if r.Name != "admin" {
				s.roles[i].Permissions = r.Permissions
			}
			s.mu.Unlock()
			return s.Save()
		}
	}
	s.roles = append(s.roles, r)
	s.mu.Unlock()
	return s.Save()
}

// DeleteRole removes a custom role. Built-in roles are protected.
func (s *Store) DeleteRole(name string) error {
	if builtInRole(name) {
		return fmt.Errorf("built-in role %q cannot be deleted", name)
	}
	s.mu.Lock()
	found := false
	for i, r := range s.roles {
		if r.Name == name {
			s.roles = append(s.roles[:i], s.roles[i+1:]...)
			found = true
			break
		}
	}
	// Reassign users on the deleted role to viewer.
	if found {
		for _, u := range s.users {
			if u.Role == name {
				u.Role = "viewer"
			}
		}
	}
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("role %q not found", name)
	}
	return s.Save()
}

// PluginEnabled reports whether a plugin is enabled (default: yes).
func (s *Store) PluginEnabled(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.plugins[id]; ok {
		return v.Enabled
	}
	return true
}

// PluginConfigured reports whether a plugin has an explicit persisted state.
// It lets legacy config provide a bootstrap default without overwriting a
// choice subsequently made in the web interface.
func (s *Store) PluginConfigured(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.plugins[id]
	return ok
}

// SetPluginEnabled toggles and persists a plugin's enabled state,
// keeping its settings intact.
func (s *Store) SetPluginEnabled(id string, enabled bool) error {
	s.mu.Lock()
	if s.plugins == nil {
		s.plugins = map[string]PluginState{}
	}
	state := s.pluginState(id)
	state.Enabled = enabled
	s.plugins[id] = state
	s.mu.Unlock()
	return s.Save()
}

// PluginSettings returns a copy of a plugin's stored settings.
func (s *Store) PluginSettings(id string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	for k, v := range s.plugins[id].Settings {
		out[k] = v
	}
	return out
}

// SetPluginSettings replaces and persists a plugin's settings, keeping
// its enabled state intact.
func (s *Store) SetPluginSettings(id string, settings map[string]string) error {
	s.mu.Lock()
	if s.plugins == nil {
		s.plugins = map[string]PluginState{}
	}
	state := s.pluginState(id)
	state.Settings = settings
	s.plugins[id] = state
	s.mu.Unlock()
	return s.Save()
}

// pluginState returns the stored state or the enabled default; the
// caller must hold s.mu.
func (s *Store) pluginState(id string) PluginState {
	if v, ok := s.plugins[id]; ok {
		return v
	}
	return PluginState{Enabled: true}
}

// --- service tokens --------------------------------------------------------

// ServiceTokens lists service token records (hashes stripped by json tags).
func (s *Store) ServiceTokens() []ServiceTokenRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ServiceTokenRecord, len(s.svcTokens))
	copy(out, s.svcTokens)
	return out
}

// ServiceToken returns the record with the given id.
func (s *Store) ServiceToken(id string) (ServiceTokenRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.svcTokens {
		if t.ID == id {
			return t, true
		}
	}
	return ServiceTokenRecord{}, false
}

// AddServiceToken records a new service token; ids must be unique.
func (s *Store) AddServiceToken(t ServiceTokenRecord) error {
	if t.ID == "" {
		return fmt.Errorf("service token id is required")
	}
	s.mu.Lock()
	for _, existing := range s.svcTokens {
		if existing.ID == t.ID {
			s.mu.Unlock()
			return fmt.Errorf("service token %q already exists", t.ID)
		}
	}
	s.svcTokens = append(s.svcTokens, t)
	s.mu.Unlock()
	return s.Save()
}

// RevokeServiceToken marks a service token revoked; the middleware
// rejects it on the next request (no restart needed).
func (s *Store) RevokeServiceToken(id string) error {
	s.mu.Lock()
	for i := range s.svcTokens {
		if s.svcTokens[i].ID == id {
			s.svcTokens[i].Revoked = true
			s.mu.Unlock()
			return s.Save()
		}
	}
	s.mu.Unlock()
	return fmt.Errorf("service token %q not found", id)
}

// --- login providers -------------------------------------------------------

// LoginProviders lists the external sign-in options for the login page.
func (s *Store) LoginProviders() []LoginProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LoginProvider, len(s.providers))
	copy(out, s.providers)
	return out
}

// SetLoginProviders validates and replaces the provider list.
func (s *Store) SetLoginProviders(list []LoginProvider) error {
	if err := validateProviders(list); err != nil {
		return err
	}
	s.mu.Lock()
	s.providers = list
	s.mu.Unlock()
	return s.Save()
}

// UpsertLoginProvider creates or updates a single provider by id — used
// by SSO plugins registering themselves with login-providers:write.
func (s *Store) UpsertLoginProvider(p LoginProvider) error {
	if err := validateProviders([]LoginProvider{p}); err != nil {
		return err
	}
	s.mu.Lock()
	replaced := false
	for i := range s.providers {
		if s.providers[i].ID == p.ID {
			s.providers[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		s.providers = append(s.providers, p)
	}
	s.mu.Unlock()
	return s.Save()
}

func validateProviders(list []LoginProvider) error {
	seen := map[string]bool{}
	for i, p := range list {
		if p.ID == "" || p.Name == "" || p.URL == "" {
			return fmt.Errorf("login provider #%d: id, name and url are required", i)
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate login provider id %q", p.ID)
		}
		seen[p.ID] = true
	}
	return nil
}
