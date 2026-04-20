package api

// Per-user "hide this site from the dashboard" list for GSC / GA4 / Bing /
// Yandex overview pages. A user who has 50 Yandex hosts but only cares
// about 3 of them clicks × on the 47 noisy ones and only sees the 3.
//
// Storage: user_platform_hides(user_id, platform, site_key), UNIQUE on
// the triple. site_key is whatever id that platform's overview uses as
// the card identifier — site_url for GSC/Bing, property_id for GA4,
// host_id for Yandex.

import (
	"log"
	"net/http"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

var validHidePlatforms = map[string]bool{
	"gsc": true, "ga4": true, "bing": true, "yandex": true,
}

type PlatformHideRequest struct {
	Platform string `json:"platform" binding:"required"`
	SiteKey  string `json:"site_key" binding:"required"`
}

// ListPlatformHides returns every (platform, site_key) the current user
// has chosen to hide. Frontend loads this once on mount and filters the
// overview cards client-side.
func ListPlatformHides(c *gin.Context) {
	uid := c.GetInt64("user_id")
	rows, err := db.DB.Query(
		`SELECT platform, site_key FROM user_platform_hides WHERE user_id = $1`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var platform, siteKey string
		if err := rows.Scan(&platform, &siteKey); err == nil {
			out = append(out, gin.H{"platform": platform, "site_key": siteKey})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// AddPlatformHide inserts a (platform, site_key) pair for the caller.
// Idempotent — ON CONFLICT DO NOTHING swallows re-submits from a
// double-click or a second tab.
func AddPlatformHide(c *gin.Context) {
	var req PlatformHideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	siteKey := strings.TrimSpace(req.SiteKey)
	if !validHidePlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "platform 必须是 gsc / ga4 / bing / yandex 之一"})
		return
	}
	if siteKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "site_key 必填"})
		return
	}
	if len(siteKey) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "site_key 过长"})
		return
	}
	uid := c.GetInt64("user_id")
	if _, err := db.DB.Exec(
		`INSERT INTO user_platform_hides (user_id, platform, site_key)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, platform, site_key) DO NOTHING`,
		uid, platform, siteKey); err != nil {
		log.Printf("[platform-hide] insert failed user=%d platform=%s: %v", uid, platform, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "隐藏失败"})
		return
	}
	WriteAuditLog(c, "platform_hide", "platform_site", platform+":"+siteKey, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已隐藏"})
}

// RemovePlatformHide un-hides a site. Body: {platform, site_key}.
func RemovePlatformHide(c *gin.Context) {
	var req PlatformHideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	siteKey := strings.TrimSpace(req.SiteKey)
	if !validHidePlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "platform 非法"})
		return
	}
	uid := c.GetInt64("user_id")
	if _, err := db.DB.Exec(
		`DELETE FROM user_platform_hides
		 WHERE user_id = $1 AND platform = $2 AND site_key = $3`,
		uid, platform, siteKey); err != nil {
		log.Printf("[platform-hide] delete failed user=%d: %v", uid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "取消隐藏失败"})
		return
	}
	WriteAuditLog(c, "platform_unhide", "platform_site", platform+":"+siteKey, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已取消隐藏"})
}

// loadPlatformHides returns a set of site_keys hidden by userID for a
// specific platform. Used by overview handlers to skip hidden cards
// server-side (defense in depth — frontend also filters). Empty on any error.
func loadPlatformHides(userID int64, platform string) map[string]bool {
	out := map[string]bool{}
	if userID <= 0 {
		return out
	}
	rows, err := db.DB.Query(
		`SELECT site_key FROM user_platform_hides WHERE user_id = $1 AND platform = $2`,
		userID, platform)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			out[s] = true
		}
	}
	return out
}
