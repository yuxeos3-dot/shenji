package api

import (
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// canAccessSite checks whether the current user owns the site or is an admin.
// Returns true if access is allowed, false otherwise (and writes 403 response).
func canAccessSite(c *gin.Context, siteID string) bool {
	userID := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if role == "admin" {
		return true
	}
	var createdBy int64
	err := db.DB.QueryRow("SELECT created_by FROM sites WHERE id = $1", siteID).Scan(&createdBy)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return false
	}
	if createdBy != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问此站点"})
		return false
	}
	return true
}
