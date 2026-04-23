package api

import (
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// canAccessSite checks whether the current user owns the site or is an admin.
// Group leaders can also access sites owned by any user in their same group.
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
	if createdBy == userID {
		return true
	}
	// group_leader can access teammates' sites (same group_id).
	if role == "group_leader" {
		var myGroup, ownerGroup int64
		db.DB.QueryRow("SELECT COALESCE(group_id,0) FROM users WHERE id = $1", userID).Scan(&myGroup)
		db.DB.QueryRow("SELECT COALESCE(group_id,0) FROM users WHERE id = $1", createdBy).Scan(&ownerGroup)
		if myGroup > 0 && myGroup == ownerGroup {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问此站点"})
	return false
}

// getAccessibleSiteIDs returns the list of site IDs the caller can access
// based on role: admin sees all, group_leader sees their group, others see own.
// Used by handlers that filter by a list of sites (dashboard overview, etc.)
func getAccessibleSiteIDs(userID int64, role string) (string, []interface{}) {
	if role == "admin" {
		return "", nil // no filter
	}
	if role == "group_leader" {
		// Sites where owner is in same group as the leader
		var myGroup int64
		db.DB.QueryRow("SELECT COALESCE(group_id,0) FROM users WHERE id = $1", userID).Scan(&myGroup)
		if myGroup > 0 {
			return "s.created_by IN (SELECT id FROM users WHERE COALESCE(group_id,0) = $1)", []interface{}{myGroup}
		}
	}
	return "s.created_by = $1", []interface{}{userID}
}
