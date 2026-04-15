package api

import (
	"net/http"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ListSitePages 获取站点的自定义页面列表
func ListSitePages(c *gin.Context) {
	siteID := c.Param("id")
	rows, err := db.DB.Query(`
		SELECT id, site_id, name, url, last_fetched_at, status_code, ttfb_ms, created_at
		FROM site_pages WHERE site_id = ? ORDER BY created_at ASC
	`, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var pages []gin.H
	for rows.Next() {
		var id, siteIDVal, statusCode, ttfb int64
		var name, url, createdAt string
		var lastFetchedAt *string
		if err := rows.Scan(&id, &siteIDVal, &name, &url, &lastFetchedAt, &statusCode, &ttfb, &createdAt); err != nil {
			continue
		}
		pages = append(pages, gin.H{
			"id":              id,
			"site_id":         siteIDVal,
			"name":            name,
			"url":             url,
			"last_fetched_at": lastFetchedAt,
			"status_code":     statusCode,
			"ttfb_ms":         ttfb,
			"created_at":      createdAt,
		})
	}
	if pages == nil {
		pages = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": pages})
}

// CreateSitePage 添加一个自定义页面
func CreateSitePage(c *gin.Context) {
	siteID := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误：name 和 url 为必填项"})
		return
	}
	result, err := db.DB.Exec(
		"INSERT INTO site_pages (site_id, name, url) VALUES (?, ?, ?)",
		siteID, req.Name, req.URL,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "添加失败"})
		return
	}
	id, _ := result.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// DeleteSitePage 删除一个自定义页面
func DeleteSitePage(c *gin.Context) {
	pageID := c.Param("page_id")
	siteID := c.Param("id")
	_, err := db.DB.Exec("DELETE FROM site_pages WHERE id = ? AND site_id = ?", pageID, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}

// FetchSitePage 实时抓取页面 HTML
func FetchSitePage(c *gin.Context) {
	pageID := c.Param("page_id")
	siteID := c.Param("id")

	var name, url string
	err := db.DB.QueryRow("SELECT name, url FROM site_pages WHERE id = ? AND site_id = ?", pageID, siteID).
		Scan(&name, &url)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "页面不存在"})
		return
	}

	// Real-time fetch
	result := crawler.FetchPage(crawler.URLSpec{PageType: name, URL: url}, 30*time.Second)

	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.DB.Exec(`
		UPDATE site_pages SET last_html=?, last_fetched_at=?, status_code=?, ttfb_ms=? WHERE id=?
	`, result.HTML, now, result.StatusCode, result.TTFB, pageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存抓取结果失败"})
		return
	}

	errMsg := result.Error
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"status_code":     result.StatusCode,
			"ttfb_ms":         result.TTFB,
			"html_size":       result.HTMLSize,
			"last_fetched_at": now,
			"error":           errMsg,
		},
	})
}

// GetSitePageSource 获取页面已抓取的 HTML 源码
func GetSitePageSource(c *gin.Context) {
	pageID := c.Param("page_id")
	siteID := c.Param("id")

	var name, url, lastHTML string
	var lastFetchedAt *string
	var statusCode, ttfb int64
	err := db.DB.QueryRow(`
		SELECT name, url, last_html, last_fetched_at, status_code, ttfb_ms
		FROM site_pages WHERE id = ? AND site_id = ?
	`, pageID, siteID).Scan(&name, &url, &lastHTML, &lastFetchedAt, &statusCode, &ttfb)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "页面不存在"})
		return
	}

	if lastHTML == "" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "error": "尚未抓取，请先点击「实时抓取」"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"name":            name,
			"url":             url,
			"html":            lastHTML,
			"last_fetched_at": lastFetchedAt,
			"status_code":     statusCode,
			"ttfb_ms":         ttfb,
		},
	})
}
