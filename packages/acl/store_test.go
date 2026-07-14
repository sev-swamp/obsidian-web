package acl

import (
	"os"
	"path/filepath"
	"testing"
)

const testYAML = `
users:
  - username: lena
    passwordHash: "$2a$10$x"
    role: editor
    groups: [hr]
  - username: igor
    passwordHash: "$2a$10$y"
    role: editor
    groups: [dev, docs]
  - username: guest
    passwordHash: "$2a$10$z"
    role: viewer

acl:
  - path: "HR/**"
    allow:
      - { group: hr, access: write }
      - { user: sev, access: read }
    default: none
  - path: "Docs/**"
    allow:
      - { group: docs, access: write }
    default: read
  - path: "Private/*/**"
    special: owner
`

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.yaml")
	if err := os.WriteFile(path, []byte(testYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAccessMatrix(t *testing.T) {
	s := newStore(t)
	cases := []struct {
		user, path string
		want       Access
	}{
		// HR: group hr → write, sev → read, others → none
		{"lena", "HR/Salaries.md", AccessWrite},
		{"sev", "HR/Salaries.md", AccessRead},
		{"igor", "HR/Salaries.md", AccessNone},
		{"guest", "HR/Salaries.md", AccessNone},
		// Docs: docs group writes, others read
		{"igor", "Docs/Guide.md", AccessWrite},
		{"lena", "Docs/Guide.md", AccessRead},
		{"guest", "Docs/Guide.md", AccessRead},
		// Owner rule
		{"igor", "Private/igor/Diary.md", AccessWrite},
		{"lena", "Private/igor/Diary.md", AccessNone},
		// Unmatched paths are unrestricted
		{"guest", "Inbox/Todo.md", AccessWrite},
		// Unknown user follows defaults
		{"stranger", "Docs/Guide.md", AccessRead},
		{"stranger", "HR/Salaries.md", AccessNone},
	}
	for _, c := range cases {
		if got := s.Access(c.user, c.path); got != c.want {
			t.Errorf("Access(%s, %s) = %s, want %s", c.user, c.path, got, c.want)
		}
	}
}

func TestFirstMatchWins(t *testing.T) {
	s := newStore(t)
	rules := append([]Rule{
		{Path: "HR/Public/**", Default: "read"},
	}, s.Rules()...)
	if err := s.SetRules(rules); err != nil {
		t.Fatal(err)
	}
	if got := s.Access("guest", "HR/Public/Handbook.md"); got != AccessRead {
		t.Errorf("specific rule must win: %s", got)
	}
	if got := s.Access("guest", "HR/Salaries.md"); got != AccessNone {
		t.Errorf("general rule must still apply: %s", got)
	}
}

func TestUserCRUDAndPersistence(t *testing.T) {
	s := newStore(t)
	if err := s.UpsertUser(UserRecord{Username: "new", PasswordHash: "$2a$10$n", Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.User("new"); !ok {
		t.Error("user lost after reload")
	}

	v, err := s.BumpTokenVersion("new")
	if err != nil || v != 1 {
		t.Errorf("bump = %d, %v", v, err)
	}
	if err := s.DeleteUser("new"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.User("new"); ok {
		t.Error("user still present after delete")
	}
}

func TestRoleSeedingAndCRUD(t *testing.T) {
	s := newStore(t)
	defaults := []RoleRecord{
		{Name: "admin", Description: "super", Permissions: []string{"notes:read"}},
		{Name: "editor", Description: "edit", Permissions: []string{"notes:read", "notes:edit"}},
		{Name: "viewer", Description: "read", Permissions: []string{"notes:read"}},
	}
	if err := s.SeedRoles(defaults); err != nil {
		t.Fatal(err)
	}
	// Seeding is idempotent: a second call keeps customizations.
	if err := s.UpsertRole(RoleRecord{Name: "viewer", Description: "changed", Permissions: []string{"notes:read"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedRoles(defaults); err != nil {
		t.Fatal(err)
	}

	// Custom role create + resolve.
	if err := s.UpsertRole(RoleRecord{Name: "auditor", Permissions: []string{"notes:read", "history:read"}}); err != nil {
		t.Fatal(err)
	}
	perms, ok := s.PermissionsForRole("auditor")
	if !ok || len(perms) != 2 {
		t.Errorf("auditor perms = %v, ok=%v", perms, ok)
	}

	// Admin permissions are fixed even when an update tries to change them.
	if err := s.UpsertRole(RoleRecord{Name: "admin", Description: "d", Permissions: []string{}}); err != nil {
		t.Fatal(err)
	}
	if perms, _ := s.PermissionsForRole("admin"); len(perms) == 0 {
		t.Error("admin permissions must not be emptied")
	}

	// Built-in roles are protected from deletion.
	if err := s.DeleteRole("viewer"); err == nil {
		t.Error("deleting a built-in role must fail")
	}

	// Deleting a custom role survives reload and reassigns nobody wrongly.
	if err := s.DeleteRole("auditor"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if s.RoleExists("auditor") {
		t.Error("auditor should be gone after delete + reload")
	}
	if !s.RoleExists("viewer") {
		t.Error("viewer should persist")
	}
}

func TestTokenLifecycle(t *testing.T) {
	s := newStore(t)
	if err := s.AddToken("igor", TokenRecord{ID: "abc", Name: "ci"}); err != nil {
		t.Fatal(err)
	}
	if !s.TokenValid("igor", "abc") {
		t.Error("fresh token must be valid")
	}
	if err := s.RevokeToken("igor", "abc"); err != nil {
		t.Fatal(err)
	}
	if s.TokenValid("igor", "abc") {
		t.Error("revoked token must be invalid")
	}
	if s.TokenValid("igor", "nope") || s.TokenValid("ghost", "abc") {
		t.Error("unknown tokens must be invalid")
	}
}

func TestInvalidRulesRejected(t *testing.T) {
	s := newStore(t)
	bad := [][]Rule{
		{{Path: ""}},
		{{Path: "x/**", Default: "banana"}},
		{{Path: "x/**", Allow: []Grant{{Access: "read"}}}},
		{{Path: "x/**", Special: "magic"}},
	}
	for i, rules := range bad {
		if err := s.SetRules(rules); err == nil {
			t.Errorf("case %d: invalid rules accepted", i)
		}
	}
}

func TestMissingFileYieldsEmptyStore(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Access("anyone", "Anything.md"); got != AccessWrite {
		t.Errorf("empty store must be unrestricted, got %s", got)
	}
}
