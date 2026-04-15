package api

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// In-memory progress tracker
var deadlinkProgress sync.Map // key: siteID, val: *DeadLinkJob

type DeadLinkJob struct {
	Status  string `json:"status"`
	Total   int    `json:"total"`
	Checked int    `json:"checked"`
	mu      sync.Mutex
}

func GetDeadLinks(c *gin.Context) {
	siteID := c.Param("id")
	rows, err := db.DB.Query(`
		SELECT id, url, source_page, status_code, ttfb_ms, checked_at
		FROM dead_link_results WHERE site_id = ? ORDER BY status_code ASC, id DESC
	`, siteID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var results []gin.H
	for rows.Next() {
		var id, statusCode, ttfb int64
		var url, sourcePage, checkedAt string
		if err := rows.Scan(&id, &url, &sourcePage, &statusCode, &ttfb, &checkedAt); err != nil {
			continue
		}
		results = append(results, gin.H{
			"id": id, "url": url, "source_page": sourcePage,
			"status_code": statusCode, "ttfb_ms": ttfb, "checked_at": checkedAt,
		})
	}
	if results == nil {
		results = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": results})
}

func GetDeadLinkProgress(c *gin.Context) {
	siteID := c.Param("id")
	if v, ok := deadlinkProgress.Load(siteID); ok {
		job := v.(*DeadLinkJob)
		job.mu.Lock()
		defer job.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"status": job.Status, "total": job.Total, "checked": job.Checked,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": "idle", "total": 0, "checked": 0}})
}

func StartDeadLinkCheck(c *gin.Context) {
	siteID := c.Param("id")

	// Get site domain
	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)
	if domain == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	// Create job
	job := &DeadLinkJob{Status: "running"}
	deadlinkProgress.Store(siteID, job)

	// Run async
	go runDeadLinkCheck(siteID, domain, job)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "检测任务已启动"})
}

func runDeadLinkCheck(siteID, domain string, job *DeadLinkJob) {
	defer func() {
		job.mu.Lock()
		job.Status = "done"
		job.mu.Unlock()
	}()

	// Collect all links from site_pages HTML
	rows, err := db.DB.Query("SELECT name, url, last_html FROM site_pages WHERE site_id = ? AND last_html != ''", siteID)
	if err != nil {
		return
	}
	defer rows.Close()

	type linkItem struct {
		url        string
		sourcePage string
	}

	seen := make(map[string]bool)
	var links []linkItem

	linkRe := regexp.MustCompile(`href="(https?://[^"]+)"`)

	for rows.Next() {
		var name, pageURL, html string
		if err := rows.Scan(&name, &pageURL, &html); err != nil {
			continue
		}
		matches := linkRe.FindAllStringSubmatch(html, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			u := m[1]
			// Skip anchors, js, mailto
			if strings.HasPrefix(u, "#") || strings.HasPrefix(u, "javascript") || strings.HasPrefix(u, "mailto") {
				continue
			}
			// Skip static resources
			if hasStaticExt(u) {
				continue
			}
			if seen[u] {
				continue
			}
			seen[u] = true
			links = append(links, linkItem{url: u, sourcePage: name})
		}
	}

	job.mu.Lock()
	job.Total = len(links)
	job.mu.Unlock()

	if len(links) == 0 {
		return
	}

	// Clear old results
	db.DB.Exec("DELETE FROM dead_link_results WHERE site_id = ?", siteID)

	// Check concurrently (max 10)
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, lnk := range links {
		wg.Add(1)
		go func(l linkItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := crawler.FetchPage(crawler.URLSpec{PageType: l.sourcePage, URL: l.url}, 15*time.Second)
			now := time.Now().Format("2006-01-02 15:04:05")
			db.DB.Exec(`INSERT INTO dead_link_results (site_id, url, source_page, status_code, ttfb_ms, checked_at) VALUES (?, ?, ?, ?, ?, ?)`,
				siteID, l.url, l.sourcePage, result.StatusCode, result.TTFB, now)

			job.mu.Lock()
			job.Checked++
			job.mu.Unlock()
		}(lnk)
	}
	wg.Wait()
}

func hasStaticExt(u string) bool {
	exts := []string{".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".ico",
		".css", ".js", ".woff", ".woff2", ".ttf", ".mp4", ".mp3", ".pdf", ".zip"}
	lower := strings.ToLower(u)
	for _, ext := range exts {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}
