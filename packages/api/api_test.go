package api

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
	"github.com/obsidianweb/obsidianweb/packages/links"
	"github.com/obsidianweb/obsidianweb/packages/markdown"
	"github.com/obsidianweb/obsidianweb/packages/plugins"
	"github.com/obsidianweb/obsidianweb/packages/search"
	"github.com/obsidianweb/obsidianweb/packages/settings"
	"github.com/obsidianweb/obsidianweb/packages/templates"
	"github.com/obsidianweb/obsidianweb/packages/websocket"
)

// usersYAML defines the accounts and ACL rules the matrix runs against:
// per-user private folders plus a read-only Docs area.
const usersYAML = `
users:
  - username: ada
    role: admin
  - username: alice
    role: editor
  - username: bob
    role: editor
  - username: victor
    role: viewer
acl:
  - path: "Private/*/**"
    special: owner
  - path: "Docs/**"
    default: read
`

type testEnv struct {
	router *gin.Engine
	auth   *auth.Service
	store  *acl.Store
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	vaultDir := t.TempDir()
	files := map[string]string{
		"Welcome.md":              "# hi",
		"Docs/guide.md":           "# guide",
		"Private/alice/secret.md": "alice only",
		"Private/bob/secret.md":   "bob only",
		"attachments/img.png":     "not-really-png",
		"attachments/page.html":   "<script>alert(1)</script>",
	}
	for p, content := range files {
		full := filepath.Join(vaultDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	vault, err := filesystem.NewVault(vaultDir)
	if err != nil {
		t.Fatal(err)
	}

	bus := core.NewEventBus()
	linkIndex := links.NewIndex()
	searchIndex := search.NewIndex()
	renderer := markdown.NewRenderer(linkIndex)
	engine := templates.NewEngine(vault, "Templates")
	notes := core.NewNoteService(vault, renderer, linkIndex, searchIndex, engine, bus, core.NoteRules{DefaultFolder: "Inbox"}, log)
	if err := notes.ReindexAll(); err != nil {
		t.Fatal(err)
	}

	usersFile := filepath.Join(t.TempDir(), "users.yaml")
	if err := os.WriteFile(usersFile, []byte(usersYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := acl.Load(usersFile)
	if err != nil {
		t.Fatal(err)
	}
	var roleRecords []acl.RoleRecord
	for _, d := range auth.DefaultRoles() {
		roleRecords = append(roleRecords, acl.RoleRecord{Name: d.Name, Description: d.Description, Permissions: d.Permissions})
	}
	if err := store.SeedRoles(roleRecords); err != nil {
		t.Fatal(err)
	}

	authSvc := auth.NewService(true, "test-secret", time.Hour, []auth.User{
		{Username: "root", Password: "rootpw", Role: auth.RoleAdmin},
	})
	authSvc.SetRoleResolver(store.PermissionsForRole)

	srv := &Server{
		Notes:   notes,
		Vault:   vault,
		Config:  settings.Default(),
		Auth:    authSvc,
		ACL:     store,
		Hub:     websocket.NewHub(bus, nil, log),
		Plugins: plugins.NewManager(bus, notes, vault, log),
		Bus:     bus,
		Log:     log,
	}
	return &testEnv{router: srv.Router(), auth: authSvc, store: store}
}

// token issues a session for a store-managed user.
func (e *testEnv) token(t *testing.T, username, role string) string {
	t.Helper()
	tok, _, err := e.auth.IssueSession(auth.User{Username: username, Role: role}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (e *testEnv) do(method, target, token, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// TestPermissionMatrix drives role × ACL rule × endpoint combinations
// through the real router and asserts the resulting status codes.
func TestPermissionMatrix(t *testing.T) {
	env := newTestEnv(t)
	ada := env.token(t, "ada", auth.RoleAdmin)
	alice := env.token(t, "alice", auth.RoleEditor)
	victor := env.token(t, "victor", auth.RoleViewer)

	saveBody := `{"content":"updated"}`
	cases := []struct {
		name   string
		method string
		target string
		token  string
		body   string
		want   int
	}{
		// Authentication is required everywhere.
		{"no token", http.MethodGet, "/api/notes", "", "", http.StatusUnauthorized},
		{"garbage token", http.MethodGet, "/api/notes", "garbage", "", http.StatusUnauthorized},

		// Role permissions (JWT layer).
		{"viewer reads note", http.MethodGet, "/api/note/Welcome", victor, "", http.StatusOK},
		{"viewer cannot save", http.MethodPut, "/api/note/Welcome", victor, saveBody, http.StatusForbidden},
		{"viewer cannot delete", http.MethodDelete, "/api/note/Welcome", victor, "", http.StatusForbidden},
		{"viewer cannot see trash", http.MethodGet, "/api/trash", victor, "", http.StatusForbidden},
		{"viewer cannot admin", http.MethodGet, "/api/admin/users", victor, "", http.StatusForbidden},
		{"editor saves public note", http.MethodPut, "/api/note/Welcome", alice, saveBody, http.StatusOK},
		{"editor cannot admin", http.MethodGet, "/api/admin/users", alice, "", http.StatusForbidden},
		{"editor cannot purge trash", http.MethodPost, "/api/trash/purge", alice, `{"path":"x.md"}`, http.StatusForbidden},
		{"admin lists users", http.MethodGet, "/api/admin/users", ada, "", http.StatusOK},

		// Folder ACL (owner rule): foreign private notes look absent.
		{"alice reads own private note", http.MethodGet, "/api/note/Private/alice/secret", alice, "", http.StatusOK},
		{"alice saves own private note", http.MethodPut, "/api/note/Private/alice/secret", alice, saveBody, http.StatusOK},
		{"alice cannot read bob's note", http.MethodGet, "/api/note/Private/bob/secret", alice, "", http.StatusNotFound},
		{"alice cannot save bob's note", http.MethodPut, "/api/note/Private/bob/secret", alice, saveBody, http.StatusNotFound},
		{"alice cannot read bob's raw", http.MethodGet, "/api/raw/Private/bob/secret", alice, "", http.StatusNotFound},

		// Folder ACL (default: read): editors may not write to Docs.
		{"editor reads docs", http.MethodGet, "/api/note/Docs/guide", alice, "", http.StatusOK},
		{"editor cannot save docs", http.MethodPut, "/api/note/Docs/guide", alice, saveBody, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := env.do(tc.method, tc.target, tc.token, tc.body)
			if w.Code != tc.want {
				t.Fatalf("%s %s: got %d, want %d (body: %s)", tc.method, tc.target, w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// TestTreeAndSearchFiltering verifies listings hide paths the caller may
// not read instead of only blocking direct access.
func TestTreeAndSearchFiltering(t *testing.T) {
	env := newTestEnv(t)
	alice := env.token(t, "alice", auth.RoleEditor)

	w := env.do(http.MethodGet, "/api/tree", alice, "")
	if w.Code != http.StatusOK {
		t.Fatalf("tree: %d", w.Code)
	}
	tree := w.Body.String()
	if strings.Contains(tree, "Private/bob") {
		t.Fatalf("tree leaks bob's folder: %s", tree)
	}
	if !strings.Contains(tree, "Private/alice") {
		t.Fatalf("tree misses alice's own folder: %s", tree)
	}

	w = env.do(http.MethodGet, "/api/search?q=only", alice, "")
	if w.Code != http.StatusOK {
		t.Fatalf("search: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "Private/bob") {
		t.Fatalf("search leaks bob's note: %s", w.Body.String())
	}
}

// TestQueryTokenOnlyForAttachments: query tokens leak into logs, so only
// the media endpoint may accept them.
func TestQueryTokenOnlyForAttachments(t *testing.T) {
	env := newTestEnv(t)
	alice := env.token(t, "alice", auth.RoleEditor)

	if w := env.do(http.MethodGet, "/api/notes?token="+alice, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("query token on /api/notes: got %d, want 401", w.Code)
	}
	if w := env.do(http.MethodGet, "/api/attachment/attachments/img.png?token="+alice, "", ""); w.Code != http.StatusOK {
		t.Fatalf("query token on attachment: got %d, want 200", w.Code)
	}
}

// TestAttachmentContentDisposition: active content must never render
// inline in the app origin.
func TestAttachmentContentDisposition(t *testing.T) {
	env := newTestEnv(t)
	alice := env.token(t, "alice", auth.RoleEditor)

	w := env.do(http.MethodGet, "/api/attachment/attachments/page.html", alice, "")
	if w.Code != http.StatusOK {
		t.Fatalf("html attachment: %d", w.Code)
	}
	if w.Header().Get("Content-Disposition") != "attachment" {
		t.Fatalf("html served inline: disposition=%q", w.Header().Get("Content-Disposition"))
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}

	w = env.do(http.MethodGet, "/api/attachment/attachments/img.png", alice, "")
	if w.Header().Get("Content-Disposition") != "" {
		t.Fatal("image should render inline")
	}
}

func TestUploadRejectsActiveContent(t *testing.T) {
	env := newTestEnv(t)
	alice := env.token(t, "alice", auth.RoleEditor)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "evil.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("<script>1</script>")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Authorization", "Bearer "+alice)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("html upload: got %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

func TestLoginRateLimit(t *testing.T) {
	env := newTestEnv(t)
	body := `{"username":"root","password":"wrong"}`
	for i := 0; i < 10; i++ {
		if w := env.do(http.MethodPost, "/api/auth/login", "", body); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i, w.Code)
		}
	}
	if w := env.do(http.MethodPost, "/api/auth/login", "", body); w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: got %d, want 429", w.Code)
	}
	// The correct password is blocked too until the window passes.
	if w := env.do(http.MethodPost, "/api/auth/login", "", `{"username":"root","password":"rootpw"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked login: got %d, want 429", w.Code)
	}
}

// TestSessionRevocation: bumping the token version invalidates issued
// sessions immediately.
func TestSessionRevocation(t *testing.T) {
	env := newTestEnv(t)
	alice := env.token(t, "alice", auth.RoleEditor)
	if w := env.do(http.MethodGet, "/api/notes", alice, ""); w.Code != http.StatusOK {
		t.Fatalf("before revoke: %d", w.Code)
	}
	if _, err := env.store.BumpTokenVersion("alice"); err != nil {
		t.Fatal(err)
	}
	if w := env.do(http.MethodGet, "/api/notes", alice, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("after revoke: got %d, want 401", w.Code)
	}
}
