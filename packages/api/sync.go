package api

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/core"
)

const (
	syncMaxOperations = 100
	syncMaxBytes      = 1 << 20
	syncPullMax       = 500
)

type syncPushRequest struct {
	DeviceID   string             `json:"deviceId"`
	Operations []syncOperationDTO `json:"operations"`
}

type syncOperationDTO struct {
	OpID     string `json:"opId"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	BaseHash string `json:"baseHash"`
}

type syncPushResponse struct {
	Acknowledged []string `json:"acknowledged"`
	Cursor       int64    `json:"cursor"`
}

type syncPullResponse struct {
	Changes    []core.SyncChange  `json:"changes"`
	NextCursor int64              `json:"nextCursor"`
	HasMore    bool               `json:"hasMore"`
	Snapshot   *core.SyncSnapshot `json:"snapshot,omitempty"`
}

func (s *Server) handleSyncPush(c *gin.Context) {
	var req syncPushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sync request"})
		return
	}
	if strings.TrimSpace(req.DeviceID) == "" || len(req.Operations) == 0 || len(req.Operations) > syncMaxOperations {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deviceId and 1 to 100 operations are required"})
		return
	}
	var total int
	for _, dto := range req.Operations {
		total += len(dto.Content) + len(dto.Path) + len(dto.OpID) + len(dto.BaseHash)
	}
	if total > syncMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "sync request exceeds 1 MiB"})
		return
	}
	operations := make([]core.SyncOperation, 0, len(req.Operations))
	for _, dto := range req.Operations {
		p := core.NormalizeNotePath(dto.Path)
		access := s.aclAccess(c, p)
		// A locally-created note has no ACL entry at its exact path yet.  In
		// that case authorise the same way as the regular create endpoint: by
		// probing its destination folder.  Existing paths still need read
		// access first, so sync never discloses a hidden note.
		if dto.Kind == string(core.SyncUpsert) && access < acl.AccessRead {
			access = s.aclAccess(c, path.Join(path.Dir(p), aclProbeName))
		}
		if access < acl.AccessRead {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if access < acl.AccessWrite {
			c.JSON(http.StatusForbidden, gin.H{"error": "write access required"})
			return
		}
		operations = append(operations, core.SyncOperation{
			OpID: dto.OpID, DeviceID: req.DeviceID, Path: dto.Path,
			Kind: core.SyncOperationKind(dto.Kind), Content: dto.Content, BaseHash: dto.BaseHash,
		})
	}
	changes, _, err := s.Notes.ApplySyncBatch(c.Request.Context(), actor(c), operations)
	if err != nil {
		var batch *core.SyncBatchError
		var conflict *core.ConflictError
		if errors.As(err, &batch) && errors.As(batch, &conflict) {
			c.JSON(http.StatusConflict, gin.H{"code": "conflict", "opId": batch.Operation.OpID, "path": batch.Operation.Path, "contentHash": conflict.CurrentHash, "content": conflict.CurrentContent})
			return
		}
		if err.Error() == "sync is disabled" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync is disabled"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	acknowledged := make([]string, len(operations))
	var cursor int64
	for i, change := range changes {
		acknowledged[i] = operations[i].OpID
		if change.AccountSeq > cursor {
			cursor = change.AccountSeq
		}
	}
	c.JSON(http.StatusOK, syncPushResponse{Acknowledged: acknowledged, Cursor: cursor})
}

func (s *Server) handleSyncPull(c *gin.Context) {
	cursor := int64(0)
	if raw := c.Query("cursor"); raw != "" {
		var err error
		if _, err = fmt.Sscan(raw, &cursor); err != nil || cursor < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
			return
		}
	}
	limit := limitParam(c, 100)
	if limit > syncPullMax {
		limit = syncPullMax
	}
	snapshot, required, err := s.Notes.SyncBootstrap(c.Request.Context(), cursor, c.Query("bootstrap") == "1")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync is disabled"})
		return
	}
	if required {
		visible := map[string]string{}
		for path, content := range snapshot.Notes {
			if s.aclAccess(c, path) >= acl.AccessRead {
				visible[path] = content
			}
		}
		snapshot.Notes = visible
		c.JSON(http.StatusOK, syncPullResponse{Changes: []core.SyncChange{}, NextCursor: snapshot.BaseCursor, Snapshot: &snapshot})
		return
	}
	changes, next, more, err := s.Notes.PullSync(c.Request.Context(), cursor, limit)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync is disabled"})
		return
	}
	visible := make([]core.SyncChange, 0, len(changes))
	for _, change := range changes {
		if s.aclAccess(c, change.Path) >= acl.AccessRead {
			visible = append(visible, change)
		}
	}
	c.JSON(http.StatusOK, syncPullResponse{Changes: visible, NextCursor: next, HasMore: more})
}
