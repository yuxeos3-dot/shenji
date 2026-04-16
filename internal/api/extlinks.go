package api

import (
	"net/http"
	"strconv"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

func GetExternalLinks(c *gin.Context) {
	siteID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "100"))
	if page < 1 { page = 1 }
	if perPage < 1 || perPage > 500 { perPage = 100 }
	offset := (page - 1) * perPage

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM external_links WHERE site_id = ?", siteID).Scan(&total)

	rows, err := db.DB.Query(`SELECT id, site_id, url, anchor_text, source_url, rel_type, category, note, status_code, checked_at, created_at
		FROM external_links WHERE site_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`, siteID, perPage, offset)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []gin.H{}, "total": 0}})
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
