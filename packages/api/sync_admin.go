package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleSyncHealth(c *gin.Context) {
	if s.Sync == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync is disabled"})
		return
	}
	health, err := s.Sync.SyncHealth(c.Request.Context())
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, health)
}
func (s *Server) handleSyncBackup(c *gin.Context) {
	if s.Sync == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync is disabled"})
		return
	}
	file, name, err := s.Sync.CreateSyncBackup(c.Request.Context())
	if err != nil {
		s.internalError(c, err)
		return
	}
	defer file.Close()
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.DataFromReader(http.StatusOK, -1, "application/x-sqlite3", file, nil)
}

func (s *Server) handleSyncCompact(c *gin.Context) {
	if s.Sync == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync is disabled"})
		return
	}
	snapshot, changed, err := s.Sync.CompactSync(c.Request.Context())
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changed": changed, "baseCursor": snapshot.BaseCursor})
}
