// Package api exposes the platform over REST and mounts the WebSocket
// endpoint and the static frontend. It contains no business logic.
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/obsidian"
	"github.com/obsidianweb/obsidianweb/packages/plugins"
	"github.com/obsidianweb/obsidianweb/packages/settings"
	"github.com/obsidianweb/obsidianweb/packages/websocket"
)

// Server aggregates the dependencies of the HTTP layer.
type Server struct {
	Notes    *core.NoteService
	Vault    core.VaultFS
	Config   *settings.Config
	Auth     *auth.Service
	Hub      *websocket.Hub
	Plugins  *plugins.Manager
	Obsidian *obsidian.Compat
	// WebFS is the embedded (or on-disk) frontend; nil means API-only.
	WebFS fs.FS
	Log   *slog.Logger
}

// Router builds the gin engine with all routes attached.
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), s.requestLogger())
	if s.Config.Server.DevCORS {
		r.Use(devCORS())
	}

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
	})
	r.POST("/api/auth/login", s.handleLogin)
	r.GET("/api/auth/status", s.handleAuthStatus)

	read := r.Group("/api", s.requirePermission(auth.PermNotesRead))
	{
		read.GET("/notes", s.handleListNotes)
		read.GET("/note/*path", s.handleGetNote)
		read.GET("/raw/*path", s.handleRawNote)
		read.GET("/tree", s.handleTree)
		read.GET("/search", s.handleSearch)
		read.GET("/recent", s.handleRecent)
		read.GET("/templates", s.handleTemplates)
		read.GET("/attachment/*path", s.handleAttachment)
		read.GET("/settings", s.handleGetSettings)
		read.GET("/obsidian/plugins", s.handleObsidianPlugins)
	}

	edit := r.Group("/api", s.requirePermission(auth.PermNotesEdit))
	{
		edit.POST("/note", s.handleCreateNote)
		edit.PUT("/note/*path", s.handleSaveNote)
	}

	r.DELETE("/api/note/*path", s.requirePermission(auth.PermNotesDelete), s.handleDeleteNote)
	r.POST("/api/upload", s.requirePermission(auth.PermUpload), s.handleUpload)
	r.PUT("/api/settings", s.requirePermission(auth.PermSettings), s.handlePutSettings)

	// Plugin routes live under /api/plugins/<id>/ (read access).
	pluginGroup := r.Group("/api/plugins", s.requirePermission(auth.PermNotesRead))
	if err := s.Plugins.InitAll(pluginGroup); err != nil {
		s.Log.Error("plugin init failed", "error", err)
	}

	r.GET("/ws", func(c *gin.Context) { s.Hub.ServeWS(c.Writer, c.Request) })

	s.mountFrontend(r)
	return r
}

// mountFrontend serves the SPA with an index.html fallback for routes.
func (s *Server) mountFrontend(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/ws" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if s.WebFS == nil {
			c.String(http.StatusOK, "Obsidian Web API is running. Frontend build not found.")
			return
		}
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := s.WebFS.Open(p); err != nil {
			p = "index.html" // SPA fallback
		} else {
			_ = f.Close()
		}
		http.ServeFileFS(c.Writer, c.Request, s.WebFS, p)
	})
}

func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			s.Log.Debug("http",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", c.Writer.Status(),
				"duration", time.Since(start).Round(time.Microsecond),
			)
		}
	}
}

func devCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// requirePermission validates the JWT (when auth is enabled) and checks
// the permission set embedded in the token.
func (s *Server) requirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.Auth.Enabled {
			c.Next()
			return
		}
		header := c.GetHeader("Authorization")
		tokenString := strings.TrimPrefix(header, "Bearer ")
		if tokenString == "" || tokenString == header {
			// Allow ?token= for WebSocket and media elements.
			tokenString = c.Query("token")
		}
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		claims, err := s.Auth.Validate(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		if !claims.HasPermission(perm) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing permission: " + perm})
			return
		}
		c.Set("user", claims)
		c.Next()
	}
}
