package auth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func newTestService() *Service {
	hash, _ := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	return NewService(true, "test-secret", time.Hour, []User{
		{Username: "admin", Password: "adminpw", Role: RoleAdmin},
		{Username: "writer", PasswordHash: string(hash), Role: RoleEditor},
		{Username: "reader", Password: "readerpw"}, // role omitted → viewer
	})
}

func TestLoginMultipleUsers(t *testing.T) {
	s := newTestService()

	cases := []struct {
		user, pass, wantRole string
	}{
		{"admin", "adminpw", RoleAdmin},
		{"writer", "s3cret", RoleEditor},
		{"reader", "readerpw", RoleViewer},
	}
	for _, c := range cases {
		token, claims, err := s.Login(c.user, c.pass)
		if err != nil {
			t.Fatalf("login %s: %v", c.user, err)
		}
		if claims.Role != c.wantRole {
			t.Errorf("%s role = %s, want %s", c.user, claims.Role, c.wantRole)
		}
		parsed, err := s.Validate(token)
		if err != nil || parsed.Username != c.user {
			t.Errorf("validate %s: %v (username %q)", c.user, err, parsed)
		}
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	s := newTestService()
	for _, c := range [][2]string{
		{"admin", "wrong"},
		{"writer", "wrong"},
		{"ghost", "whatever"},
		{"reader", ""},
	} {
		if _, _, err := s.Login(c[0], c[1]); err == nil {
			t.Errorf("login %q/%q must fail", c[0], c[1])
		}
	}
}

func TestTokenCarriesPermissions(t *testing.T) {
	s := newTestService()
	cases := map[string][]string{
		"reader": {PermNotesRead},
		"writer": {PermNotesRead, PermNotesEdit, PermNotesDelete, PermHistory, PermUpload, PermTrashRead},
		"admin":  {PermNotesRead, PermNotesEdit, PermNotesDelete, PermHistory, PermUpload, PermSettings, PermTrashRead, PermTrashPurge},
	}
	passwords := map[string]string{"reader": "readerpw", "writer": "s3cret", "admin": "adminpw"}
	for user, want := range cases {
		token, _, err := s.Login(user, passwords[user])
		if err != nil {
			t.Fatalf("login %s: %v", user, err)
		}
		claims, err := s.Validate(token)
		if err != nil {
			t.Fatalf("validate %s: %v", user, err)
		}
		if len(claims.Permissions) != len(want) {
			t.Errorf("%s permissions = %v, want %v", user, claims.Permissions, want)
		}
		for _, p := range want {
			if !claims.HasPermission(p) {
				t.Errorf("%s must have %s", user, p)
			}
		}
	}
	// Viewer must not gain write permissions.
	token, _, _ := s.Login("reader", "readerpw")
	claims, _ := s.Validate(token)
	for _, p := range []string{PermNotesEdit, PermNotesDelete, PermHistory, PermUpload, PermSettings, PermTrashRead, PermTrashPurge} {
		if claims.HasPermission(p) {
			t.Errorf("viewer must not have %s", p)
		}
	}
}

func TestHasPermissionLegacyTokenFallsBackToRole(t *testing.T) {
	c := &Claims{Role: RoleEditor} // token issued before the permissions claim existed
	if !c.HasPermission(PermNotesEdit) || c.HasPermission(PermSettings) {
		t.Error("legacy token must fall back to the role permission map")
	}
}

func TestAdminIsSuperuser(t *testing.T) {
	// Even a token missing new permissions must pass every check.
	c := &Claims{Role: RoleAdmin, Permissions: []string{PermNotesRead}}
	for _, p := range []string{PermTrashRead, PermTrashPurge, PermSettings, "future:permission"} {
		if !c.HasPermission(p) {
			t.Errorf("admin must have %s", p)
		}
	}
}

func TestRoleResolverOverridesBuiltins(t *testing.T) {
	s := newTestService()
	s.SetRoleResolver(func(role string) ([]string, bool) {
		if role == RoleViewer {
			return []string{PermNotesRead, PermNotesEdit}, true // custom: viewers can edit
		}
		return nil, false
	})
	_, claims, err := s.Login("reader", "readerpw")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !claims.HasPermission(PermNotesEdit) {
		t.Error("resolver-granted permission must appear in the session")
	}
}

func TestRoleHierarchy(t *testing.T) {
	if !Allows(RoleAdmin, RoleViewer) || !Allows(RoleEditor, RoleViewer) {
		t.Error("higher roles must include lower ones")
	}
	if Allows(RoleViewer, RoleEditor) || Allows(RoleEditor, RoleAdmin) {
		t.Error("lower roles must not satisfy higher requirements")
	}
	if !ValidRole("editor") || ValidRole("superuser") {
		t.Error("ValidRole misclassifies")
	}
}
