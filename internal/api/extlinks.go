package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

func GetExternalLinks(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "100"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 500 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM external_links WHERE site_id = ?", siteID).Scan(&total)

	rows, err := db.DB.Query(`SELECT id, site_id, url, anchor_text, source_url, rel_type, category, note, status_code, checked_at, created_at
		FROM external_links WHERE site_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`, siteID, perPage, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var links []gin.H
	for rows.Next() {
		var id, siteIDVal, statusCode int64
		var url, anchorText, sourceURL, relType, category, note, createdAt string
		var checkedAt *string
		if err := rows.Scan(&id, &siteIDVal, &url, &anchorText, &sourceURL, &relType, &category, &note, &statusCode, &checkedAt, &createdAt); err != nil {
			continue
		}
		links = append(links, gin.H{
			"id": id, "site_id": siteIDVal, "url": url, "anchor_text": anchorText,
			"source_url": sourceURL, "rel_type": relType, "category": category, "note": note,
			"status_code": statusCode, "checked_at": checkedAt, "created_at": createdAt,
		})
	}
	if links == nil {
		links = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": links, "total": total, "page": page, "per_page": perPage}})
}

// ─── Shared Backlink Pool (公共外链库) ────────────────────────────────────────

func ListExtlinkPool(c *gin.Context) {
	rows, err := db.DB.Query("SELECT id, url, name, created_at FROM extlink_pool ORDER BY id DESC LIMIT 500")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var list []gin.H
	for rows.Next() {
		var id int64
		var url, name, cat string
		rows.Scan(&id, &url, &name, &cat)
		list = append(list, gin.H{"id": id, "url": url, "name": name, "created_at": cat})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": list})
}

func ImportExtlinkPool(c *gin.Context) {
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "text 必填"})
		return
	}
	userID := c.GetInt64("user_id")
	lines := strings.Split(req.Text, "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		url := strings.TrimSpace(parts[0])
		name := ""
		if len(parts) > 1 {
			name = strings.TrimSpace(parts[1])
		}
		if url == "" {
			continue
		}
		// Skip duplicates
		var exists int
		db.DB.QueryRow("SELECT COUNT(*) FROM extlink_pool WHERE url=?", url).Scan(&exists)
		if exists > 0 {
			continue
		}
		if _, err := db.DB.Exec("INSERT INTO extlink_pool (url, name, created_by) VALUES (?, ?, ?)", url, name, userID); err == nil {
			count++
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"imported": count}})
}

func DeleteExtlinkPoolItem(c *gin.Context) {
	db.DB.Exec("DELETE FROM extlink_pool WHERE id=?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

// ─── Per-site External Links ─────────────────────────────────────────────────

type ExternalLinkRequest struct {
	SiteID     int64  `json:"site_id"`
	URL        string `json:"url" binding:"required"`
	AnchorText string `json:"anchor_text"`
	SourceURL  string `json:"source_url"`
	RelType    string `json:"rel_type"`
	Category   string `json:"category"`
	Note       string `json:"note"`
}

func CreateExternalLink(c *gin.Context) {
	var req ExternalLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if req.SiteID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "site_id 必填"})
		return
	}
	if !canAccessSite(c, strconv.FormatInt(req.SiteID, 10)) {
		return
	}
	if req.RelType == "" {
		req.RelType = "dofollow"
	}
	result, err := db.DB.Exec(`INSERT INTO external_links (site_id, url, anchor_text, source_url, rel_type, category, note) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.SiteID, req.URL, req.AnchorText, req.SourceURL, req.RelType, req.Category, req.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "添加失败"})
		return
	}
	id, _ := result.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func UpdateExternalLink(c *gin.Context) {
	id := c.Param("id")
	// Verify ownership via site_id
	var siteID string
	if err := db.DB.QueryRow("SELECT site_id FROM external_links WHERE id=?", id).Scan(&siteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "外链不存在"})
		return
	}
	if !canAccessSite(c, siteID) {
		return
	}
	var req ExternalLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	db.DB.Exec(`UPDATE external_links SET url=?, anchor_text=?, source_url=?, rel_type=?, category=?, note=? WHERE id=?`,
		req.URL, req.AnchorText, req.SourceURL, req.RelType, req.Category, req.Note, id)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "更新成功"})
}

func DeleteExternalLink(c *gin.Context) {
	id := c.Param("id")
	// Verify ownership via site_id
	var siteID string
	if err := db.DB.QueryRow("SELECT site_id FROM external_links WHERE id=?", id).Scan(&siteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "外链不存在"})
		return
	}
	if !canAccessSite(c, siteID) {
		return
	}
	db.DB.Exec("DELETE FROM external_links WHERE id = ?", id)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}

func CheckExternalLink(c *gin.Context) {
	id := c.Param("id")
	var url string
	if err := db.DB.QueryRow("SELECT url FROM external_links WHERE id = ?", id).Scan(&url); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "外链不存在"})
		return
	}
	result := crawler.FetchPage(crawler.URLSpec{URL: url}, 15*time.Second)
	now := time.Now().Format("2006-01-02 15:04:05")
	db.DB.Exec("UPDATE external_links SET status_code=?, checked_at=? WHERE id=?", result.StatusCode, now, id)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status_code": result.StatusCode, "ttfb_ms": result.TTFB}})
}
