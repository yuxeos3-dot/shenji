package api

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// In-memory progress tracker
var deadlinkProgress sync.Map // key: siteID, val: *DeadLinkJob

type DeadLinkJob struct {
	Status  string `json:"status"`
	Phase   string `json:"phase"`
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
			"status": job.Status, "total": job.Total, "checked": job.Checked, "phase": job.Phase,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": "idle", "total": 0, "checked": 0, "phase": ""}})
}

func StartDeadLinkCheck(c *gin.Context) {
	siteID := c.Param("id")

	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)
	if domain == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	// Check not already running
	if v, ok := deadlinkProgress.Load(siteID); ok {
		j := v.(*DeadLinkJob)
		j.mu.Lock()
		s := j.Status
		j.mu.Unlock()
		if s == "running" {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "检测任务正在进行中"})
			return
		}
	}

	job := &DeadLinkJob{Status: "running", Phase: "正在解析 Sitemap..."}
	deadlinkProgress.Store(siteID, job)
	go runDeadLinkCheck(siteID, domain, job)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "检测任务已启动，正在从 Sitemap 收集链接"})
}

// ─── Sitemap XML structures ──────────────────────────────────────────────────

type sitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

type sitemapEntry struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod"`
}

type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

type urlEntry struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod"`
}

// fetchSitemapURLs recursively fetches sitemap.xml (handles sitemap index with
// sub-sitemaps). Returns all <loc> URLs found across all levels.
func fetchSitemapURLs(sitemapURL string, depth int) []string {
	if depth > 3 {
		return nil // prevent infinite recursion
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sitemapURL)
	if err != nil {
		log.Printf("[deadlink] sitemap fetch failed %s: %v", sitemapURL, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[deadlink] sitemap HTTP %d for %s", resp.StatusCode, sitemapURL)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB max
	if err != nil {
		return nil
	}

	// Try parsing as sitemap index first
	var idx sitemapIndex
	if err := xml.Unmarshal(body, &idx); err == nil && len(idx.Sitemaps) > 0 {
		log.Printf("[deadlink] sitemap index %s → %d 子 sitemaps", sitemapURL, len(idx.Sitemaps))
		var all []string
		for _, s := range idx.Sitemaps {
			if s.Loc != "" {
				sub := fetchSitemapURLs(s.Loc, depth+1)
				all = append(all, sub...)
			}
		}
		return all
	}

	// Try parsing as regular urlset
	var us urlSet
	if err := xml.Unmarshal(body, &us); err == nil && len(us.URLs) > 0 {
		log.Printf("[deadlink] sitemap %s → %d URLs", sitemapURL, len(us.URLs))
		urls := make([]string, 0, len(us.URLs))
		for _, u := range us.URLs {
			if u.Loc != "" {
				urls = append(urls, u.Loc)
			}
		}
		return urls
	}

	log.Printf("[deadlink] sitemap %s: 无法解析为 sitemap index 或 urlset", sitemapURL)
	return nil
}

func runDeadLinkCheck(siteID, domain string, job *DeadLinkJob) {
	defer func() {
		job.mu.Lock()
		job.Status = "done"
		job.Phase = "完成"
		job.mu.Unlock()
	}()

	// Normalize domain
	d := strings.TrimRight(domain, "/")
	if !strings.HasPrefix(d, "http") {
		d = "https://" + d
	}

	// Step 1: Fetch sitemap URLs (recursive, handles sitemap index)
	job.mu.Lock()
	job.Phase = "正在解析 Sitemap..."
	job.mu.Unlock()

	sitemapURL := d + "/sitemap.xml"
	urls := fetchSitemapURLs(sitemapURL, 0)

	// Deduplicate
	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		// Skip static resources
		if hasStaticExt(u) {
			continue
		}
		seen[u] = true
		uniqueURLs = append(uniqueURLs, u)
	}

	log.Printf("[deadlink] site %s: sitemap 收集到 %d 个唯一 URL", siteID, len(uniqueURLs))

	if len(uniqueURLs) == 0 {
		job.mu.Lock()
		job.Phase = "Sitemap 未找到任何 URL"
		job.mu.Unlock()
		return
	}

	job.mu.Lock()
	job.Total = len(uniqueURLs)
	job.Phase = "正在检测链接..."
	job.mu.Unlock()

	// Step 2: Clear old results
	db.DB.Exec("DELETE FROM dead_link_results WHERE site_id = ?", siteID)

	// Step 3: Check all URLs concurrently (max 20 goroutines)
	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	for _, u := range uniqueURLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			statusCode := 0
			ttfb := int64(0)

			req, err := http.NewRequest("HEAD", url, nil)
			if err != nil {
				statusCode = -1
			} else {
				req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SEO-DeadLinkChecker/1.0)")
				resp, err := client.Do(req)
				ttfb = time.Since(start).Milliseconds()
				if err != nil {
					statusCode = -1
				} else {
					statusCode = resp.StatusCode
					resp.Body.Close()
				}
			}

			now := time.Now().Format("2006-01-02T15:04:05Z")
			db.DB.Exec(`INSERT INTO dead_link_results (site_id, url, source_page, status_code, ttfb_ms, checked_at)
				VALUES (?, ?, 'sitemap', ?, ?, ?)`, siteID, url, statusCode, ttfb, now)

			job.mu.Lock()
			job.Checked++
			job.mu.Unlock()
		}(u)
	}
	wg.Wait()
}

func hasStaticExt(u string) bool {
	exts := []string{".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".ico",
		".css", ".js", ".woff", ".woff2", ".ttf", ".mp4", ".mp3", ".pdf", ".zip"}
	lower := strings.ToLower(u)
	for _, ext := range exts {
		if strings.HasSuffix(lower, ext) || strings.Contains(lower, ext+"?") {
			return true
		}
	}
	return false
}
