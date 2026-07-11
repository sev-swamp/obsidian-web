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
