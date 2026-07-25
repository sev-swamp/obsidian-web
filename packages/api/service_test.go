package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// createServiceToken mints a token through the admin API and returns
// the bearer string (svc_<id>_<secret>).
func createServiceToken(t *testing.T, env *testEnv, admin, id, ceiling string, perms []string) string {
	t.Helper()
	permsJSON, _ := json.Marshal(perms)
	body := fmt.Sprintf(`{"id":%q,"name":"test %s","permissions":%s,"roleCeiling":%q}`, id, id, permsJSON, ceiling)
	w := env.do(http.MethodPost, "/api/admin/service-tokens", admin, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create service token: %d (%s)", w.Code, w.Body.String())
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res.Token
}

// TestServiceTokenAuth: the service middleware accepts only a valid,
// unrevoked token carrying the required permission; revocation works
// without a restart.
func TestServiceTokenAuth(t *testing.T) {
	env := newTestEnv(t)
	ada := env.token(t, "ada", "admin")
	svc := createServiceToken(t, env, ada, "sso-test", "editor", []string{"users:provision", "session:code"})

	codeBody := `{"username":"alice"}`
	cases := []struct {
		name  string
		token string
		want  int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"user JWT rejected", ada, http.StatusUnauthorized},
		{"garbage svc token", "svc_sso-test_deadbeef", http.StatusUnauthorized},
		{"unknown id", "svc_nope_deadbeef", http.StatusUnauthorized},
		{"valid token", svc, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := env.do(http.MethodPost, "/api/service/login-code", tc.token, codeBody)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d (%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}

	// A token without login-providers:write may not register providers.
	w := env.do(http.MethodPut, "/api/service/login-providers", svc, `{"id":"x","name":"X","url":"http://x/start"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing permission: got %d, want 403 (%s)", w.Code, w.Body.String())
	}

	// Revocation applies immediately.
	if w := env.do(http.MethodDelete, "/api/admin/service-tokens/sso-test", ada, ""); w.Code != http.StatusOK {
		t.Fatalf("revoke: %d (%s)", w.Code, w.Body.String())
	}
	if w := env.do(http.MethodPost, "/api/service/login-code", svc, codeBody); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token still works: %d", w.Code)
	}
}

// TestServiceProvisionCeiling: a token capped at editor cannot mint or
// promote admins, cannot touch users above its ceiling, and never sets
// passwords (provisioned accounts are SSO-only).
func TestServiceProvisionCeiling(t *testing.T) {
	env := newTestEnv(t)
	ada := env.token(t, "ada", "admin")
	svc := createServiceToken(t, env, ada, "sso-test", "editor", []string{"users:provision"})

	// New editor within the ceiling.
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"carol","role":"editor","groups":["dev"]}`); w.Code != http.StatusCreated {
		t.Fatalf("provision carol: %d (%s)", w.Code, w.Body.String())
	}
	// Update of the same user is 200 and keeps working.
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"carol","role":"viewer"}`); w.Code != http.StatusOK {
		t.Fatalf("update carol: %d (%s)", w.Code, w.Body.String())
	}
	// Password login for an SSO-only account must fail.
	if w := env.do(http.MethodPost, "/api/auth/login", "", `{"username":"carol","password":""}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("SSO-only account accepts password login: %d", w.Code)
	}

	// Creating an admin exceeds the ceiling.
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"mallory","role":"admin"}`); w.Code != http.StatusForbidden {
		t.Fatalf("admin above ceiling: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	// Promoting an existing editor to admin exceeds the ceiling.
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"alice","role":"admin"}`); w.Code != http.StatusForbidden {
		t.Fatalf("promote above ceiling: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	// An existing admin outranks the token entirely — no touching.
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"ada","role":"viewer"}`); w.Code != http.StatusForbidden {
		t.Fatalf("downgrade admin: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	// Statically configured accounts cannot be shadowed.
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"root","role":"viewer"}`); w.Code != http.StatusForbidden {
		t.Fatalf("shadow static user: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	// Existing password accounts keep their password after an upsert.
	if w := env.do(http.MethodPut, "/api/admin/users/alice", ada, `{"password":"alicepw"}`); w.Code != http.StatusOK {
		t.Fatalf("set alice password: %d (%s)", w.Code, w.Body.String())
	}
	if w := env.do(http.MethodPost, "/api/service/users", svc, `{"username":"alice","role":"editor"}`); w.Code != http.StatusOK {
		t.Fatalf("re-provision alice: %d (%s)", w.Code, w.Body.String())
	}
	if w := env.do(http.MethodPost, "/api/auth/login", "", `{"username":"alice","password":"alicepw"}`); w.Code != http.StatusOK {
		t.Fatalf("alice password lost after service upsert: %d (%s)", w.Code, w.Body.String())
	}
}

// TestLoginCodeFlow: a code is exchanged for a working session exactly
// once; expiry, user deletion and tokenVersion bumps all fail closed.
func TestLoginCodeFlow(t *testing.T) {
	env := newTestEnv(t)
	ada := env.token(t, "ada", "admin")
	svc := createServiceToken(t, env, ada, "sso-test", "editor", []string{"session:code"})

	issue := func(username string) string {
		t.Helper()
		w := env.do(http.MethodPost, "/api/service/login-code", svc, fmt.Sprintf(`{"username":%q}`, username))
		if w.Code != http.StatusOK {
			t.Fatalf("issue code: %d (%s)", w.Code, w.Body.String())
		}
		var res struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res.Code
	}

	// Codes are only issued for existing users.
	if w := env.do(http.MethodPost, "/api/service/login-code", svc, `{"username":"ghost"}`); w.Code != http.StatusNotFound {
		t.Fatalf("code for unknown user: got %d, want 404", w.Code)
	}

	code := issue("alice")
	w := env.do(http.MethodPost, "/api/auth/code", "", fmt.Sprintf(`{"code":%q}`, code))
	if w.Code != http.StatusOK {
		t.Fatalf("exchange: %d (%s)", w.Code, w.Body.String())
	}
	var session struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Username != "alice" || session.Role != "editor" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if w := env.do(http.MethodGet, "/api/notes", session.Token, ""); w.Code != http.StatusOK {
		t.Fatalf("session token rejected: %d", w.Code)
	}

	// Second exchange of the same code fails: it burned on first use.
	if w := env.do(http.MethodPost, "/api/auth/code", "", fmt.Sprintf(`{"code":%q}`, code)); w.Code != http.StatusUnauthorized {
		t.Fatalf("code reused: got %d, want 401", w.Code)
	}

	// Expired codes fail.
	expired, _, err := env.srv.loginCodes.issue("alice", -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if w := env.do(http.MethodPost, "/api/auth/code", "", fmt.Sprintf(`{"code":%q}`, expired)); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired code accepted: %d", w.Code)
	}

	// User deleted between issue and exchange fails.
	code = issue("victor")
	if w := env.do(http.MethodDelete, "/api/admin/users/victor", ada, ""); w.Code != http.StatusOK {
		t.Fatalf("delete victor: %d (%s)", w.Code, w.Body.String())
	}
	if w := env.do(http.MethodPost, "/api/auth/code", "", fmt.Sprintf(`{"code":%q}`, code)); w.Code != http.StatusUnauthorized {
		t.Fatalf("code for deleted user accepted: %d", w.Code)
	}

	// tokenVersion revocation applies to code-issued sessions too.
	if _, err := env.store.BumpTokenVersion("alice"); err != nil {
		t.Fatal(err)
	}
	if w := env.do(http.MethodGet, "/api/notes", session.Token, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session still valid: %d", w.Code)
	}
}

// TestLoginProviders: the anonymous providers list reflects admin edits
// and plugin self-registration.
func TestLoginProviders(t *testing.T) {
	env := newTestEnv(t)
	ada := env.token(t, "ada", "admin")

	// Empty by default.
	w := env.do(http.MethodGet, "/api/auth/providers", "", "")
	if w.Code != http.StatusOK || w.Body.String() != `{"providers":[]}` {
		t.Fatalf("default providers: %d %s", w.Code, w.Body.String())
	}

	// Admin registers a provider; the anonymous list shows it.
	body := `{"providers":[{"id":"keycloak","name":"Sign in with Keycloak","url":"https://sso.example.com/start"}]}`
	if w := env.do(http.MethodPut, "/api/admin/login-providers", ada, body); w.Code != http.StatusOK {
		t.Fatalf("put providers: %d (%s)", w.Code, w.Body.String())
	}
	// Non-admins cannot edit the list.
	alice := env.token(t, "alice", "editor")
	if w := env.do(http.MethodPut, "/api/admin/login-providers", alice, body); w.Code != http.StatusForbidden {
		t.Fatalf("editor edits providers: got %d, want 403", w.Code)
	}
	w = env.do(http.MethodGet, "/api/auth/providers", "", "")
	if !strings.Contains(w.Body.String(), "keycloak") {
		t.Fatalf("provider missing: %s", w.Body.String())
	}

	// A plugin with login-providers:write upserts its own entry.
	svc := createServiceToken(t, env, ada, "sso-ldap", "viewer", []string{"login-providers:write"})
	if w := env.do(http.MethodPut, "/api/service/login-providers", svc, `{"id":"ldap","name":"LDAP","url":"https://ldap.example.com/start"}`); w.Code != http.StatusOK {
		t.Fatalf("plugin upsert: %d (%s)", w.Code, w.Body.String())
	}
	w = env.do(http.MethodGet, "/api/auth/providers", "", "")
	if !strings.Contains(w.Body.String(), "ldap") || !strings.Contains(w.Body.String(), "keycloak") {
		t.Fatalf("providers after upsert: %s", w.Body.String())
	}
	// Invalid entries are rejected.
	if w := env.do(http.MethodPut, "/api/service/login-providers", svc, `{"id":"","name":"","url":""}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty provider accepted: %d", w.Code)
	}
}
