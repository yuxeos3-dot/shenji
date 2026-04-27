package api

import (
	"net/http"
	"strconv"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// WriteAuditLog used to record per-action audit trail rows. In single-user
// mode there's no audit log feature; the function is kept as a no-op so
// existing call sites compile without a sweeping rewrite.
func WriteAuditLog(c *gin.Context, action, resource, resourceID, detail string) {
	_ = c
	_ = action
	_ = resource
	_ = resourceID
	_ = detail
}

// canAccessSite used to enforce per-user / per-role site access. In single-
// user mode the only check that still makes sense is "the site exists" —
// otherwise a typo'd ID would silently no-op. Returns false (and writes a
// 404 response) if the site row is missing; true otherwise.
func canAccessSite(c *gin.Context, siteID string) bool {
	id, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "无效的站点ID"})
		return false
	}
	var exists int
	if err := db.DB.QueryRow("SELECT 1 FROM sites WHERE id = $1", id).Scan(&exists); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return false
	}
	return true
}
