package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
)

func pathParam(c *gin.Context) string {
	return strings.TrimPrefix(c.Param("path"), "/")
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
	notes, err := s.Notes.ListNotes(nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notes)
}

func (s *Server) handleGetNote(c *gin.Context) {
	note, err := s.Notes.GetNote(pathParam(c), nil)
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func (s *Server) handleRawNote(c *gin.Context) {
	data, err := s.Vault.Read(core.NormalizeNotePath(pathParam(c)))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
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
	p, err := s.Notes.CreateNote(actor(c), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	note, err := s.Notes.GetNote(p, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, note)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

func (s *Server) handleDeleteNote(c *gin.Context) {
	if err := s.Notes.DeleteNote(actor(c), pathParam(c)); err != nil {
		status := http.StatusInternalServerError
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) handleTree(c *gin.Context) {
	tree, err := s.Notes.Tree()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tree)
}

func (s *Server) handleSearch(c *gin.Context) {
	query := c.Query("q")
	results := s.Notes.Search(query, limitParam(c, 20), nil)
	if results == nil {
		results = []core.SearchResult{}
	}
	c.JSON(http.StatusOK, results)
}

func (s *Server) handleRecent(c *gin.Context) {
	notes, err := s.Notes.Recent(limitParam(c, 10), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notes)
}

func (s *Server) handleTemplates(c *gin.Context) {
	names, err := s.Notes.Templates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, names)
}

// handleAttachment streams a vault file (image, PDF, audio, video) with
// range-request support so media seeking works.
func (s *Server) handleAttachment(c *gin.Context) {
	abs, err := s.Vault.AbsPath(pathParam(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ct := mime.TypeByExtension(path.Ext(abs)); ct != "" {
		c.Header("Content-Type", ct)
	}
	c.File(abs)
}

func (s *Server) handleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required"})
		return
	}
	folder := strings.Trim(c.PostForm("folder"), "/")
	if folder == "" {
		folder = s.Config.Vault.AttachmentsDir
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dest := path.Join(folder, path.Base(file.Filename))
	if err := s.Vault.Write(dest, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	revs, err := h.Log(core.NormalizeNotePath(pathParam(c)), limitParam(c, 50))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	from := c.Query("from")
	if from == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from revision is required"})
		return
	}
	diff, err := h.Diff(core.NormalizeNotePath(pathParam(c)), from, c.Query("to"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	if err := s.Notes.RestoreNote(actor(c), pathParam(c), req.Rev); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

func (s *Server) handleTrash(c *gin.Context) {
	deleted, err := s.Notes.Trash(limitParam(c, 100))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
	if err := s.Notes.RestoreDeleted(actor(c), req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

// Settings API exposes only the runtime-editable subset (note rules).
func (s *Server) handleGetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"notes": s.Notes.Rules(),
		"vault": gin.H{
			"templatesDir":   s.Config.Vault.TemplatesDir,
			"attachmentsDir": s.Config.Vault.AttachmentsDir,
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
	if err := s.Config.Save(); err != nil {
		s.Log.Warn("settings not persisted", "error", err)
	}
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
	token, claims, err := s.Auth.Login(req.Username, req.Password)
	if err != nil {
		if err == auth.ErrInvalidCredentials {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": true, "plugins": plugins})
}
