package api

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

var deadlinkProgress sync.Map

type DeadLinkJob struct {
	Status  string `json:"status"`
	Phase   string `json:"phase"`
	Total   int    `json:"total"`
	Checked int    `json:"checked"`
	mu      sync.Mutex
}

// DeadlinkProgressStats returns (tracked_sites, running_count) for UI display.
func DeadlinkProgressStats() (tracked, running int) {
	deadlinkProgress.Range(func(_, v interface{}) bool {
		j, ok := v.(*DeadLinkJob)
		if !ok {
			return true
		}
		tracked++
		j.mu.Lock()
		if j.Status == "running" {
			running++
		}
		j.mu.Unlock()
		return true
	})
	return
}

// ClearDeadlinkProgress drops every per-site job record. Running checks keep
// executing but progress becomes untracked — the endpoint just stops showing
// their status. Useful when the UI thinks a stale job is still running.
func ClearDeadlinkProgress() {
	deadlinkProgress.Range(func(k, v interface{}) bool {
		j, ok := v.(*DeadLinkJob)
		if !ok {
			deadlinkProgress.Delete(k)
			return true
		}
		j.mu.Lock()
		notRunning := j.Status != "running"
		j.mu.Unlock()
		if notRunning {
			deadlinkProgress.Delete(k)
		}
		return true
	})
}

func GetDeadLinks(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	// If a check is running, don't hit DB.
	if v, ok := deadlinkProgress.Load(siteID); ok {
		j, ok := v.(*DeadLinkJob)
		if !ok {
			deadlinkProgress.Delete(siteID)
			return
		}
		j.mu.Lock()
		s := j.Status
		j.mu.Unlock()
		if s == "running" {
			c.JSON(200, gin.H{"code": 0, "data": gin.H{"items": []gin.H{}, "total": 0}, "running": true})
			return
		}
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "100"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 500 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	// Filter
	statusFilter := c.Query("status_filter") // dead / redirect / ok / all
	where := "site_id = ?"
	args := []interface{}{siteID}
	switch statusFilter {
	case "dead":
		where += " AND (status_code >= 400 OR status_code <= 0)"
	case "redirect":
		where += " AND status_code >= 300 AND status_code < 400"
	case "ok":
		where += " AND status_code >= 200 AND status_code < 300"
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM dead_link_results WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, perPage, offset)
	rows, err := db.DB.Query(
		"SELECT id, url, source_page, status_code, ttfb_ms, checked_at FROM dead_link_results WHERE "+where+" ORDER BY status_code ASC, id DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		c.JSON(200, gin.H{"code": 0, "data": gin.H{"items": []gin.H{}, "total": 0}})
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

	// Also return summary counts so frontend doesn't need a second query.
	var totalAll, totalDead, totalRedirect, totalOk int
	db.DB.QueryRow("SELECT COUNT(*) FROM dead_link_results WHERE site_id=?", siteID).Scan(&totalAll)
	db.DB.QueryRow("SELECT COUNT(*) FROM dead_link_results WHERE site_id=? AND (status_code>=400 OR status_code<=0)", siteID).Scan(&totalDead)
	db.DB.QueryRow("SELECT COUNT(*) FROM dead_link_results WHERE site_id=? AND status_code>=300 AND status_code<400", siteID).Scan(&totalRedirect)
	db.DB.QueryRow("SELECT COUNT(*) FROM dead_link_results WHERE site_id=? AND status_code>=200 AND status_code<300", siteID).Scan(&totalOk)

	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"items": results, "total": total, "page": page, "per_page": perPage,
		"total_all": totalAll, "total_dead": totalDead, "total_redirect": totalRedirect, "total_ok": totalOk,
	}})
}

// RetryDeadLinks re-checks only the failed/dead links (status >= 400 or <= 0).
func RetryDeadLinks(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	if v, ok := deadlinkProgress.Load(siteID); ok {
		j, ok := v.(*DeadLinkJob)
		if !ok {
			deadlinkProgress.Delete(siteID)
			return
		}
		j.mu.Lock()
		s := j.Status
		j.mu.Unlock()
		if s == "running" {
			c.JSON(400, gin.H{"code": -1, "error": "检测任务正在进行中"})
			return
		}
	}

	// Get all dead links for this site
	rows, err := db.DB.Query(
		"SELECT id, url FROM dead_link_results WHERE site_id=? AND (status_code>=400 OR status_code<=0)",
		siteID)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	type deadItem struct {
		id  int64
		url string
	}
	var items []deadItem
	for rows.Next() {
		var d deadItem
		if err := rows.Scan(&d.id, &d.url); err != nil {
			log.Printf("[deadlinks] retry scan failed: %v", err)
			continue
		}
		items = append(items, d)
	}

	if len(items) == 0 {
		c.JSON(200, gin.H{"code": 0, "data": "没有需要重查的死链"})
		return
	}

	job := &DeadLinkJob{Status: "running", Phase: "正在重查死链...", Total: len(items)}
	deadlinkProgress.Store(siteID, job)

	go func() {
		defer func() {
			job.mu.Lock()
			job.Status = "done"
			job.Phase = "重查完成"
			job.mu.Unlock()
		}()

		sem := make(chan struct{}, 20)
		var wg sync.WaitGroup
		// Reuse the crawler's SSRF-aware client so retries can't be used to
		// probe internal networks via user-supplied URLs.
		client := crawler.BuildHTTPClient(15 * time.Second)

		for _, item := range items {
			wg.Add(1)
			go func(d deadItem) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				start := time.Now()
				sc := 0
				req, err := http.NewRequest("HEAD", d.url, nil)
				if err != nil {
					sc = -1
				} else {
					req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SEO-DeadLinkChecker/1.0)")
					resp, err := client.Do(req)
					if err != nil {
						sc = -1
					} else {
						sc = resp.StatusCode
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
				}
				ttfb := time.Since(start).Milliseconds()
				now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
				if _, err := db.DB.Exec("UPDATE dead_link_results SET status_code=?, ttfb_ms=?, checked_at=? WHERE id=?",
					sc, ttfb, now, d.id); err != nil {
					log.Printf("[deadlinks] retry update id=%d failed: %v", d.id, err)
				}

				job.mu.Lock()
				job.Checked++
				job.mu.Unlock()
			}(item)
		}
		wg.Wait()
	}()

	c.JSON(200, gin.H{"code": 0, "data": gin.H{"retrying": len(items)}})
}

func GetDeadLinkProgress(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	if v, ok := deadlinkProgress.Load(siteID); ok {
		job, ok := v.(*DeadLinkJob)
		if !ok {
			deadlinkProgress.Delete(siteID)
		} else {
			job.mu.Lock()
			defer job.mu.Unlock()
			c.JSON(200, gin.H{"code": 0, "data": gin.H{
				"status": job.Status, "total": job.Total, "checked": job.Checked, "phase": job.Phase,
			}})
			return
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"status": "idle", "total": 0, "checked": 0, "phase": ""}})
}

func StartDeadLinkCheck(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)
	if domain == "" {
		c.JSON(404, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	if v, ok := deadlinkProgress.Load(siteID); ok {
		j, ok := v.(*DeadLinkJob)
		if !ok {
			deadlinkProgress.Delete(siteID)
			return
		}
		j.mu.Lock()
		s := j.Status
		j.mu.Unlock()
		if s == "running" {
			c.JSON(400, gin.H{"code": -1, "error": "检测任务正在进行中"})
			return
		}
	}

	job := &DeadLinkJob{Status: "running", Phase: "正在解析 Sitemap..."}
	deadlinkProgress.Store(siteID, job)
	go runDeadLinkCheck(siteID, domain, job)
	c.JSON(200, gin.H{"code": 0, "data": "检测任务已启动"})
}

// ─── Sitemap XML ─────────────────────────────────────────────────────────────

type sitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}
type sitemapEntry struct {
	Loc string `xml:"loc"`
}
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}
type urlEntry struct {
	Loc string `xml:"loc"`
}

func fetchSitemapURLs(sitemapURL string, depth int) []string {
	if depth > 3 {
		return nil
	}
	client := crawler.BuildHTTPClient(30 * time.Second)
	resp, err := client.Get(sitemapURL)
	if err != nil {
		log.Printf("[deadlink] fetch failed %s: %v", sitemapURL, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil
	}

	var idx sitemapIndex
	if err := xml.Unmarshal(body, &idx); err == nil && len(idx.Sitemaps) > 0 {
		log.Printf("[deadlink] sitemap index %s → %d 子sitemaps", sitemapURL, len(idx.Sitemaps))
		var all []string
		for _, s := range idx.Sitemaps {
			if s.Loc != "" {
				all = append(all, fetchSitemapURLs(s.Loc, depth+1)...)
			}
		}
		return all
	}

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
	return nil
}

// ─── Main check logic ────────────────────────────────────────────────────────

type checkResult struct {
	url        string
	statusCode int
	ttfb       int64
}

func runDeadLinkCheck(siteID, domain string, job *DeadLinkJob) {
	defer func() {
		job.mu.Lock()
		job.Status = "done"
		job.Phase = "完成"
		job.mu.Unlock()
	}()

	d := strings.TrimRight(domain, "/")
	if !strings.HasPrefix(d, "http") {
		d = "https://" + d
	}

	urls := fetchSitemapURLs(d+"/sitemap.xml", 0)

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] || hasStaticExt(u) {
			continue
		}
		seen[u] = true
		uniqueURLs = append(uniqueURLs, u)
	}

	log.Printf("[deadlink] site %s: %d 个唯一 URL", siteID, len(uniqueURLs))

	if len(uniqueURLs) == 0 {
		job.mu.Lock()
		job.Phase = "Sitemap 未找到 URL"
		job.mu.Unlock()
		return
	}

	job.mu.Lock()
	job.Total = len(uniqueURLs)
	job.Phase = "正在检测链接..."
	job.mu.Unlock()

	// Worker-pool pattern: cap live goroutines to `workers`. Previously each
	// URL spawned its own goroutine which piled up on the semaphore (8KB
	// stack × N), so a 100k-URL sitemap ate ~800MB before a single HEAD ran.
	const workers = 20
	urlCh := make(chan string, workers*2)
	resultCh := make(chan checkResult, workers*4)

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for url := range urlCh {
				start := time.Now()
				sc := 0
				req, err := http.NewRequest("HEAD", url, nil)
				if err != nil {
					sc = -1
				} else {
					req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SEO-DeadLinkChecker/1.0)")
					resp, err := client.Do(req)
					if err != nil {
						sc = -1
					} else {
						sc = resp.StatusCode
						resp.Body.Close()
					}
				}
				ttfb := time.Since(start).Milliseconds()

				resultCh <- checkResult{url: url, statusCode: sc, ttfb: ttfb}

				job.mu.Lock()
				job.Checked++
				job.mu.Unlock()
			}
		}()
	}

	// Feed URLs without blocking the batch writer.
	go func() {
		for _, u := range uniqueURLs {
			urlCh <- u
		}
		close(urlCh)
	}()

	// Close results when every worker has drained urlCh.
	go func() { wg.Wait(); close(resultCh) }()

	// Batch writer — collects results and writes to DB in batches of 50.
	// This avoids 20 concurrent goroutines all fighting for the SQLite write
	// lock, which was causing the "卡死" issue.
	IncDeadlinkRun()
	db.DB.Exec("DELETE FROM dead_link_results WHERE site_id = ?", siteID)

	batch := make([]checkResult, 0, 50)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		tx, err := db.DB.Begin()
		if err != nil {
			log.Printf("[deadlink] batch begin failed: %v", err)
			return
		}
		now := time.Now().Format("2006-01-02T15:04:05Z")
		for _, r := range batch {
			tx.Exec(`INSERT INTO dead_link_results (site_id, url, source_page, status_code, ttfb_ms, checked_at)
				VALUES (?, ?, 'sitemap', ?, ?, ?)`, siteID, r.url, r.statusCode, r.ttfb, now)
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[deadlink] batch commit failed: %v", err)
		}
		batch = batch[:0]
	}

	for r := range resultCh {
		batch = append(batch, r)
		if len(batch) >= 50 {
			flush()
		}
	}
	flush() // remaining
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
