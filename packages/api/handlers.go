package api

import (
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
)

// aclProbeName is a fictional child file used to evaluate folder-level
// ACL access: rules match note paths (globs like "Docs/**"), so access
// to a folder is defined as access to a hypothetical note inside it.
const aclProbeName = "__probe__.md"

func pathParam(c *gin.Context) string {
	return strings.TrimPrefix(c.Param("path"), "/")
}

// internalError logs the failure with its details and answers with an
// opaque 500: raw error strings expose internal paths and library
// internals to clients.
func (s *Server) internalError(c *gin.Context, err error) {
	s.Log.Error("request failed",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"error", err,
	)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

// actor returns the authenticated username ("" when auth is disabled).
func actor(c *gin.Context) string {
	if v, ok := c.Get("user"); ok {
		if claims, ok := v.(*auth.Claims); ok {
			return claims.Username
		}
	}
	return ""
}

func limitParam(c *gin.Context, def int) int {
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		return v
	}
	return def
}

func (s *Server) handleListNotes(c *gin.Context) {
	notes, err := s.Notes.ListNotes(s.allowRead(c))
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, notes)
}

func (s *Server) handleGetNote(c *gin.Context) {
	p := core.NormalizeNotePath(pathParam(c))
	access := s.aclAccess(c, p)
	if access < acl.AccessRead {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	note, err := s.Notes.GetNote(p, s.allowRead(c))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		s.internalError(c, err)
		return
	}
	if access >= acl.AccessWrite {
		note.Access = "write"
	} else {
		note.Access = "read"
	}
	c.JSON(http.StatusOK, note)
}

func (s *Server) handleRawNote(c *gin.Context) {
	p := core.NormalizeNotePath(pathParam(c))
	if s.aclAccess(c, p) < acl.AccessRead {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	data, err := s.Vault.Read(p)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		s.internalError(c, err)
		return
	}
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", data)
}

func (s *Server) handleCreateNote(c *gin.Context) {
	var req core.CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// ACL: check the destination folder before the note is created.
	folder := s.Notes.ResolveTargetFolder(req)
	if s.aclAccess(c, path.Join(folder, aclProbeName)) < acl.AccessWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "no write access to folder " + folder})
		return
	}
	p, err := s.Notes.CreateNote(actor(c), req)
	if err != nil {
		// Validation/template errors are the caller's to fix; file-system
		// failures would leak host paths ("open /vault/…: permission
		// denied") and are server trouble.
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			s.internalError(c, err)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	note, err := s.Notes.GetNote(p, nil)
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, note)
}

func (s *Server) handleCreateFolder(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	folder := strings.Trim(req.Path, "/")
	if folder == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folder path is required"})
		return
	}
	if s.aclAccess(c, path.Join(folder, aclProbeName)) < acl.AccessWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "no write access to folder " + folder})
		return
	}
	p, err := s.Notes.CreateFolder(actor(c), folder)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"path": p})
}

// handleAccess reports the calling user's effective access to a path so
// the UI can decide whether to offer creating a missing note there. The
// ACL result is capped by the role's global ceiling.
func (s *Server) handleAccess(c *gin.Context) {
	p := core.NormalizeNotePath(pathParam(c))
	access := s.aclAccess(c, p)
	// The role ceiling only applies when auth is on; with auth disabled
	// aclAccess already grants write and there is no role to cap by.
	if s.Auth.Enabled {
		if ceiling := s.roleAccessCeiling(s.userRole(actor(c))); ceiling < access {
			access = ceiling
		}
	}
	c.JSON(http.StatusOK, gin.H{"path": p, "access": access.String()})
}

func (s *Server) handleSaveNote(c *gin.Context) {
	var req struct {
		Content  string `json:"content"`
		BaseHash string `json:"baseHash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	switch s.aclAccess(c, core.NormalizeNotePath(pathParam(c))) {
	case acl.AccessNone:
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	case acl.AccessRead:
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only access"})
		return
	}
	if err := s.Notes.SaveNote(actor(c), pathParam(c), req.Content, req.BaseHash); err != nil {
		var conflict *core.ConflictError
		if errors.As(err, &conflict) {
			c.JSON(http.StatusConflict, gin.H{
				"error":          "conflict",
				"currentHash":    conflict.CurrentHash,
				"currentContent": conflict.CurrentContent,
				"changedBy":      conflict.ChangedBy,
				"changedAt":      conflict.ChangedAt,
			})
			return
		}
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

func (s *Server) handleDeleteNote(c *gin.Context) {
	switch s.aclAccess(c, core.NormalizeNotePath(pathParam(c))) {
	case acl.AccessNone:
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	case acl.AccessRead:
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only access"})
		return
	}
	if err := s.Notes.DeleteNote(actor(c), pathParam(c)); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) handleTree(c *gin.Context) {
	tree, err := s.Notes.Tree()
	if err != nil {
		s.internalError(c, err)
		return
	}
	if allow := s.allowRead(c); allow != nil {
		tree = filterTree(tree, allow)
	}
	c.JSON(http.StatusOK, tree)
}

// filterTree hides paths the caller may not read. A directory is shown
// when it keeps visible children or its probe child is readable.
func filterTree(node *core.TreeNode, allow core.AllowFunc) *core.TreeNode {
	out := &core.TreeNode{Name: node.Name, Path: node.Path, IsDir: node.IsDir}
	for _, child := range node.Children {
		if child.IsDir {
			filtered := filterTree(child, allow)
			if len(filtered.Children) > 0 || allow(child.Path+"/"+aclProbeName) {
				out.Children = append(out.Children, filtered)
			}
		} else if allow(child.Path) {
			out.Children = append(out.Children, child)
		}
	}
	return out
}

func (s *Server) handleSearch(c *gin.Context) {
	query := c.Query("q")
	results := s.Notes.Search(query, limitParam(c, 20), s.allowRead(c))
	if results == nil {
		results = []core.SearchResult{}
	}
	c.JSON(http.StatusOK, results)
}

func (s *Server) handleRecent(c *gin.Context) {
	notes, err := s.Notes.Recent(limitParam(c, 10), s.allowRead(c))
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, notes)
}

func (s *Server) handleTemplates(c *gin.Context) {
	names, err := s.Notes.Templates()
	if err != nil {
		s.internalError(c, err)
		return
	}
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, names)
}

// inlineSafe reports whether a MIME type may render inline in the
// browser. Everything else is forced to download: an uploaded .html or
// .svg served inline would execute scripts in the app's origin (stored
// XSS). Embedding via <img>/<audio>/<video> ignores Content-Disposition,
// so media in notes keeps working either way.
func inlineSafe(ct string) bool {
	if ct == "image/svg+xml" {
		return false // SVG can carry scripts
	}
	switch {
	case strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "audio/"),
		strings.HasPrefix(ct, "video/"),
		ct == "application/pdf",
		strings.HasPrefix(ct, "text/plain"):
		return true
	}
	return false
}

// handleAttachment streams a vault file (image, PDF, audio, video) with
// range-request support so media seeking works.
func (s *Server) handleAttachment(c *gin.Context) {
	if s.aclAccess(c, pathParam(c)) < acl.AccessRead {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	abs, err := s.Vault.AbsPath(pathParam(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ct := mime.TypeByExtension(path.Ext(abs))
	if ct != "" {
		c.Header("Content-Type", ct)
	}
	c.Header("X-Content-Type-Options", "nosniff")
	if !inlineSafe(ct) {
		c.Header("Content-Disposition", "attachment")
	}
	c.File(abs)
}

// maxUploadBytes bounds a single upload (the file is buffered in memory).
const maxUploadBytes = 100 << 20 // 100 MiB

// blockedUploadExt rejects extensions that browsers execute as active
// content; defense in depth on top of the Content-Disposition policy.
var blockedUploadExt = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true, ".svg": true, ".xml": true,
	".js": true, ".mjs": true,
}

func (s *Server) handleUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	file, err := c.FormFile("file")
	if err != nil {
		status := http.StatusBadRequest
		msg := "file field is required"
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			status = http.StatusRequestEntityTooLarge
			msg = "file exceeds the upload limit"
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}
	if blockedUploadExt[strings.ToLower(path.Ext(file.Filename))] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this file type cannot be uploaded"})
		return
	}
	folder := strings.Trim(c.PostForm("folder"), "/")
	if folder == "" {
		folder = s.Config.Vault.AttachmentsDir
	}
	src, err := file.Open()
	if err != nil {
		s.internalError(c, err)
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		s.internalError(c, err)
		return
	}
	dest := path.Join(folder, path.Base(file.Filename))
	if s.aclAccess(c, dest) < acl.AccessWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "no write access to folder " + folder})
		return
	}
	if err := s.Vault.Write(dest, data); err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"path": dest})
}

// --- history & trash ---------------------------------------------------

func (s *Server) historyOr404(c *gin.Context) core.History {
	h := s.Notes.History()
	if h == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "history is disabled"})
		return nil
	}
	return h
}

func (s *Server) handleHistoryLog(c *gin.Context) {
	h := s.historyOr404(c)
	if h == nil {
		return
	}
	p := core.NormalizeNotePath(pathParam(c))
	if s.aclAccess(c, p) < acl.AccessRead {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	revs, err := h.Log(p, limitParam(c, 50))
	if err != nil {
		s.internalError(c, err)
		return
	}
	if revs == nil {
		revs = []core.Revision{}
	}
	c.JSON(http.StatusOK, revs)
}

func (s *Server) handleHistoryDiff(c *gin.Context) {
	h := s.historyOr404(c)
	if h == nil {
		return
	}
	diffPath := core.NormalizeNotePath(pathParam(c))
	if s.aclAccess(c, diffPath) < acl.AccessRead {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// ?rev= shows what that revision changed (diff against its parent);
	// ?from=&to= compares two arbitrary revisions.
	var diff string
	var err error
	if rev := c.Query("rev"); rev != "" {
		diff, err = h.ChangesIn(diffPath, rev)
	} else {
		from := c.Query("from")
		if from == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "rev or from revision is required"})
			return
		}
		diff, err = h.Diff(diffPath, from, c.Query("to"))
	}
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (s *Server) handleRestore(c *gin.Context) {
	var req struct {
		Rev string `json:"rev"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Rev == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rev is required"})
		return
	}
	if s.aclAccess(c, core.NormalizeNotePath(pathParam(c))) < acl.AccessWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only access"})
		return
	}
	if err := s.Notes.RestoreNote(actor(c), pathParam(c), req.Rev); err != nil {
		if errors.Is(err, core.ErrRestoreUnchanged) {
			c.JSON(http.StatusOK, gin.H{"status": "unchanged"})
			return
		}
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

func (s *Server) handleTrash(c *gin.Context) {
	deleted, err := s.Notes.Trash(limitParam(c, 100))
	if err != nil {
		s.internalError(c, err)
		return
	}
	if allow := s.allowRead(c); allow != nil {
		filtered := deleted[:0]
		for _, d := range deleted {
			if allow(d.Path) {
				filtered = append(filtered, d)
			}
		}
		deleted = filtered
	}
	if deleted == nil {
		deleted = []core.DeletedFile{}
	}
	c.JSON(http.StatusOK, deleted)
}

func (s *Server) handleTrashRestore(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if s.aclAccess(c, req.Path) < acl.AccessWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only access"})
		return
	}
	if err := s.Notes.RestoreDeleted(actor(c), req.Path); err != nil {
		if errors.Is(err, core.ErrRestoreUnchanged) {
			c.JSON(http.StatusOK, gin.H{"status": "unchanged"})
			return
		}
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

func (s *Server) handleTrashPurge(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	// Symmetric with restore: taking an entry out of someone else's
	// trash requires write access to the path.
	if s.aclAccess(c, req.Path) < acl.AccessWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only access"})
		return
	}
	if err := s.Notes.PurgeTrash([]string{req.Path}); err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "purged"})
}

func (s *Server) handleTrashPurgeAll(c *gin.Context) {
	// Trash(0) is the full list — a page-sized limit here would leave
	// older entries behind. Only entries the caller can see in their own
	// trash view (allowRead) are cleared.
	deleted, err := s.Notes.Trash(0)
	if err != nil {
		s.internalError(c, err)
		return
	}
	allow := s.allowRead(c)
	var paths []string
	for _, d := range deleted {
		if allow == nil || allow(d.Path) {
			paths = append(paths, d.Path)
		}
	}
	if err := s.Notes.PurgeTrash(paths); err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "purged"})
}

// Settings API exposes only the runtime-editable subset (note rules)
// plus read-only facts the UI needs (history availability, so deletion
// warns when nothing lands in the trash).
func (s *Server) handleGetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"notes": s.Notes.Rules(),
		"vault": gin.H{
			"templatesDir":   s.Config.Vault.TemplatesDir,
			"attachmentsDir": s.Config.Vault.AttachmentsDir,
		},
		"history": gin.H{
			"enabled": s.Notes.History() != nil,
			"mode":    s.Config.History.Mode,
		},
	})
}

func (s *Server) handlePutSettings(c *gin.Context) {
	var req struct {
		Notes core.NoteRules `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.Notes.SetRules(req.Notes)
	s.Config.Notes = req.Notes
	if err := s.Config.SaveRuntime(); err != nil {
		s.internalError(c, err)
		return
	}
	s.audit(c, "settings.update")
	c.JSON(http.StatusOK, gin.H{"notes": req.Notes})
}

func (s *Server) handleLogin(c *gin.Context) {
	if !s.Auth.Enabled {
		c.JSON(http.StatusOK, gin.H{"authEnabled": false})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limitKey := req.Username + "|" + c.ClientIP()
	if !s.loginLimits.Allow(limitKey) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many failed attempts, try again later"})
		return
	}

	// users.yaml accounts take priority; config.yaml stays the
	// emergency fallback (see plans/02-access-control.md).
	var user auth.User
	tokenVersion := 0
	found := false
	if s.ACL != nil {
		if rec, ok := s.ACL.User(req.Username); ok {
			role := rec.Role
			if role == "" {
				role = auth.RoleViewer
			}
			user = auth.User{Username: rec.Username, Password: rec.Password, PasswordHash: rec.PasswordHash, Role: role}
			tokenVersion = rec.TokenVersion
			found = true
		}
	}
	if !found {
		if su, ok := s.Auth.StaticUser(req.Username); ok {
			user = su
			found = true
		}
	}
	if !found {
		s.loginLimits.Fail(limitKey)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err := auth.Authenticate(user, req.Password); err != nil {
		s.loginLimits.Fail(limitKey)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	s.loginLimits.Reset(limitKey)
	token, claims, err := s.Auth.IssueSession(user, tokenVersion)
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":       token,
		"username":    claims.Username,
		"role":        claims.Role,
		"permissions": claims.Permissions,
	})
}

func (s *Server) handleAuthStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"authEnabled": s.Auth.Enabled})
}

func (s *Server) handleObsidianPlugins(c *gin.Context) {
	if s.Obsidian == nil || !s.Obsidian.Available() {
		c.JSON(http.StatusOK, gin.H{"available": false, "plugins": []any{}})
		return
	}
	plugins, err := s.Obsidian.CommunityPlugins()
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": true, "plugins": plugins})
}
