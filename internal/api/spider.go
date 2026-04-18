package api

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Spider identification ──────────────────────────────────────────────────
//
// We keep the matcher list hard-coded (rather than DB-backed) because the set
// of well-known crawlers evolves slowly and a shared list keeps analytics
// consistent across deploys. Matching is case-insensitive substring on the
// User-Agent header. Order matters only for the "Unknown" fallback.

type spiderMatcher struct {
	Name    string
	Needles []string
}

var spiderMatchers = []spiderMatcher{
	{"Googlebot", []string{"googlebot", "google-inspectiontool"}},
	{"Bingbot", []string{"bingbot", "adidxbot"}},
	{"Baiduspider", []string{"baiduspider"}},
	{"YandexBot", []string{"yandex"}},
	{"Sogou", []string{"sogou"}},
	{"360Spider", []string{"360spider", "haosouspider"}},
	{"Bytespider", []string{"bytespider"}},
	{"YisouSpider", []string{"yisouspider"}},
	{"DuckDuckBot", []string{"duckduckbot"}},
	{"AhrefsBot", []string{"ahrefsbot"}},
	{"SemrushBot", []string{"semrushbot"}},
	{"MJ12bot", []string{"mj12bot"}},
	{"AppleBot", []string{"applebot"}},
	{"FacebookBot", []string{"facebookexternalhit", "meta-externalagent"}},
	{"PetalBot", []string{"petalbot"}},
	{"GPTBot", []string{"gptbot", "chatgpt-user", "oai-searchbot"}},
	{"ClaudeBot", []string{"claudebot", "claude-web", "anthropic"}},
	{"PerplexityBot", []string{"perplexitybot", "perplexity-user"}},
}

// IdentifySpider returns the canonical spider name from a User-Agent, or ""
// for non-spider traffic (which callers should skip persisting).
func IdentifySpider(ua string) string {
	lower := strings.ToLower(ua)
	for _, m := range spiderMatchers {
		for _, n := range m.Needles {
			if strings.Contains(lower, n) {
				return m.Name
			}
		}
	}
	if strings.Contains(lower, "bot") || strings.Contains(lower, "spider") || strings.Contains(lower, "crawler") {
		return "Other"
	}
	return ""
}

// ─── Ingest endpoint (public, API-key protected) ────────────────────────────

type spiderIngestRequest struct {
	SiteDomain string `json:"site_domain"` // e.g. "keifukudo.com" — match against sites.domain
	SiteID     int64  `json:"site_id"`     // alternative to SiteDomain
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent" binding:"required"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	TTFBms     int    `json:"ttfb_ms"`
	Referrer   string `json:"referrer"`
	Timestamp  string `json:"timestamp"` // RFC3339; falls back to now() if missing
}

// IngestSpiderLog accepts a single spider hit from an external shipper (nginx
// log parser, edge function, Cloudflare worker, etc.). Auth is a shared
// secret in X-API-Key — the key is set by admin in /settings. Non-spider
// User-Agents are silently accepted but NOT persisted, keeping the table
// focused on search engines only.
func IngestSpiderLog(c *gin.Context) {
	configured := db.GetSetting("spider_ingest_api_key")
	if configured == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": -1, "error": "蜘蛛日志摄入未启用：请在系统管理中配置 spider_ingest_api_key"})
		return
	}
	provided := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(configured), []byte(provided)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "API Key 无效"})
		return
	}

	var req spiderIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	spider := IdentifySpider(req.UserAgent)
	if spider == "" {
		// Non-spider traffic is a no-op so shippers can safely stream every
		// hit without having to classify upstream.
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"skipped": true}})
		return
	}

	siteID := req.SiteID
	if siteID == 0 && req.SiteDomain != "" {
		dom := strings.ToLower(strings.TrimSpace(req.SiteDomain))
		dom = strings.TrimPrefix(dom, "https://")
		dom = strings.TrimPrefix(dom, "http://")
		dom = strings.TrimRight(dom, "/")
		db.DB.QueryRow(
			"SELECT id FROM sites WHERE LOWER(domain) = ? LIMIT 1",
			dom,
		).Scan(&siteID)
	}
	if siteID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点未登记，先在站点管理中创建"})
		return
	}

	visitedAt := time.Now().UTC()
	if req.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			visitedAt = t
		}
	}

	if _, err := db.DB.Exec(
		`INSERT INTO spider_logs (site_id, spider, ip, user_agent, url, status_code, ttfb_ms, referrer, visited_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, spider, req.IP, req.UserAgent, req.URL, req.StatusCode, req.TTFBms, req.Referrer, visitedAt,
	); err != nil {
		log.Printf("[spider] insert failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "写入失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"spider": spider, "site_id": siteID}})
}

// ─── Read endpoints (admin / authenticated) ─────────────────────────────────

// SpiderOverview returns spider stats for ONE site. site_id query parameter
// is required — spider analytics is designed around per-site reporting
// (aggregating across sites mixes unrelated hosts with very different
// crawl budgets, which isn't useful for SEO monitoring).
//
// Response fields:
//   - total_24h / total_7d / total_30d: hit counts for this site
//   - top_url_7d: most-crawled URLs on this site (top 10, last 7 days)
//   - by_spider: per-spider breakdown (last 7 days, this site)
//   - last_visit_by_spider: when each spider last hit this site
func SpiderOverview(c *gin.Context) {
	siteID := c.Query("site_id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "site_id 必填"})
		return
	}
	if !canAccessSite(c, siteID) {
		return
	}

	now := time.Now().UTC()
	day1 := now.Add(-24 * time.Hour)
	day7 := now.Add(-7 * 24 * time.Hour)
	day30 := now.Add(-30 * 24 * time.Hour)

	count := func(since time.Time) int {
		var n int
		db.DB.QueryRow(
			"SELECT COUNT(*) FROM spider_logs WHERE site_id = ? AND visited_at >= ?",
			siteID, since,
		).Scan(&n)
		return n
	}

	// Per-spider breakdown (last 7 days, this site only)
	type row struct {
		Spider string `json:"spider"`
		Hits   int    `json:"hits"`
	}
	var breakdown []row
	if rows, err := db.DB.Query(
		`SELECT spider, COUNT(*) FROM spider_logs
		 WHERE site_id = ? AND visited_at >= ?
		 GROUP BY spider ORDER BY 2 DESC`,
		siteID, day7,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var r row
			if rows.Scan(&r.Spider, &r.Hits) == nil {
				breakdown = append(breakdown, r)
			}
		}
	}

	// Most crawled URLs (top 10, last 7 days, this site)
	type urlRow struct {
		URL  string `json:"url"`
		Hits int    `json:"hits"`
	}
	var topURLs []urlRow
	if rows, err := db.DB.Query(
		`SELECT url, COUNT(*) as hits FROM spider_logs
		 WHERE site_id = ? AND visited_at >= ? AND url != ''
		 GROUP BY url ORDER BY hits DESC LIMIT 10`,
		siteID, day7,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var r urlRow
			if rows.Scan(&r.URL, &r.Hits) == nil {
				topURLs = append(topURLs, r)
			}
		}
	}

	// Last visit per spider (so operators can see "Googlebot last crawled 3h ago")
	type lastVisit struct {
		Spider string `json:"spider"`
		At     string `json:"at"`
	}
	var lastVisits []lastVisit
	if rows, err := db.DB.Query(
		`SELECT spider, MAX(visited_at) FROM spider_logs
		 WHERE site_id = ? GROUP BY spider ORDER BY 2 DESC LIMIT 20`,
		siteID,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var r lastVisit
			if rows.Scan(&r.Spider, &r.At) == nil {
				lastVisits = append(lastVisits, r)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"site_id":              siteID,
			"total_24h":            count(day1),
			"total_7d":             count(day7),
			"total_30d":            count(day30),
			"by_spider":            breakdown,
			"top_url_7d":           topURLs,
			"last_visit_by_spider": lastVisits,
			"generated":            now.Format(time.RFC3339),
		},
	})
}

// SpiderSiteSummary returns a lightweight per-site row for all registered
// sites: last-7-day total + per-spider breakdown. Used by the site selector
// tooltip / overview table so operators can see at a glance which sites are
// being crawled without drilling into each one.
func SpiderSiteSummary(c *gin.Context) {
	role, _ := c.Get("role")
	userID := c.GetInt64("user_id")
	day7 := time.Now().UTC().Add(-7 * 24 * time.Hour)

	// Scalar subqueries keep the main query simple — the GROUP-BY + CASE
	// form was tripping up modernc/sqlite's aggregate handling on an empty
	// spider_logs table (returning an error instead of a zero row).
	rows, err := db.DB.Query(
		`SELECT s.id, s.name, s.domain,
		        (SELECT COUNT(*)                    FROM spider_logs WHERE site_id = s.id AND visited_at >= ?) AS hits_7d,
		        (SELECT COUNT(DISTINCT spider)      FROM spider_logs WHERE site_id = s.id AND visited_at >= ?) AS spiders_7d,
		        (SELECT MAX(visited_at)             FROM spider_logs WHERE site_id = s.id) AS last_visit
		 FROM sites s
		 WHERE (? = 'admin' OR s.created_by = ?)
		 ORDER BY hits_7d DESC, s.id`,
		day7, day7, role, userID,
	)
	if err != nil {
		log.Printf("[spider] SpiderSiteSummary query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败: " + err.Error()})
		return
	}
	defer rows.Close()

	type siteRow struct {
		SiteID    int64   `json:"site_id"`
		Name      string  `json:"name"`
		Domain    string  `json:"domain"`
		Hits7d    int64   `json:"hits_7d"`
		Spiders7d int64   `json:"spiders_7d"`
		LastVisit *string `json:"last_visit"`
	}
	var out []siteRow
	for rows.Next() {
		var r siteRow
		if err := rows.Scan(&r.SiteID, &r.Name, &r.Domain, &r.Hits7d, &r.Spiders7d, &r.LastVisit); err != nil {
			log.Printf("[spider] SpiderSiteSummary scan failed: %v", err)
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[spider] SpiderSiteSummary rows.Err: %v", err)
	}
	if out == nil {
		out = []siteRow{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// SpiderDailyTrend returns one row per day × spider for the last N days.
// site_id is required — same reasoning as SpiderOverview.
func SpiderDailyTrend(c *gin.Context) {
	siteID := c.Query("site_id")
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "site_id 必填"})
		return
	}
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "14"))
	if days < 1 {
		days = 14
	}
	if days > 180 {
		days = 180
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	rows, err := db.DB.Query(
		`SELECT strftime('%Y-%m-%d', visited_at) as d, spider, COUNT(*)
		 FROM spider_logs WHERE site_id = ? AND visited_at >= ?
		 GROUP BY d, spider ORDER BY d ASC`,
		siteID, since,
	)
	if err != nil {
		log.Printf("[spider] SpiderDailyTrend query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败: " + err.Error()})
		return
	}
	defer rows.Close()

	type cell struct {
		Date   string `json:"date"`
		Spider string `json:"spider"`
		Hits   int    `json:"hits"`
	}
	var out []cell
	for rows.Next() {
		var c cell
		if rows.Scan(&c.Date, &c.Spider, &c.Hits) == nil {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []cell{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// SpiderLogs returns paginated raw hits, optionally filtered by site/spider.
func SpiderLogs(c *gin.Context) {
	page, perPage := parsePageQuery(c, 50, 500)
	offset := (page - 1) * perPage

	siteID := c.Query("site_id")
	spider := c.Query("spider")

	where := "1=1"
	args := []interface{}{}
	if siteID != "" {
		if !canAccessSite(c, siteID) {
			return
		}
		where += " AND sl.site_id = ?"
		args = append(args, siteID)
	}
	if spider != "" {
		where += " AND sl.spider = ?"
		args = append(args, spider)
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM spider_logs sl WHERE "+where, args...).Scan(&total)

	q := `SELECT sl.id, sl.site_id, s.domain, sl.spider, sl.ip, sl.user_agent, sl.url,
	             sl.status_code, sl.ttfb_ms, sl.referrer, sl.visited_at
	      FROM spider_logs sl LEFT JOIN sites s ON sl.site_id = s.id
	      WHERE ` + where + ` ORDER BY sl.visited_at DESC LIMIT ? OFFSET ?`
	rows, err := db.DB.Query(q, append(args, perPage, offset)...)
	if err != nil {
		log.Printf("[spider] SpiderLogs query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var out []gin.H
	for rows.Next() {
		var id, siteIDVal int64
		var domain, spiderName, ip, ua, url, referrer, visitedAt string
		var status, ttfb int
		if err := rows.Scan(&id, &siteIDVal, &domain, &spiderName, &ip, &ua, &url, &status, &ttfb, &referrer, &visitedAt); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id": id, "site_id": siteIDVal, "domain": domain, "spider": spiderName,
			"ip": ip, "user_agent": ua, "url": url, "status_code": status,
			"ttfb_ms": ttfb, "referrer": referrer, "visited_at": visitedAt,
		})
	}
	if out == nil {
		out = []gin.H{}
	}
	c.Header("X-Total-Count", fmt.Sprintf("%d", total))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out, "total": total, "page": page, "per_page": perPage})
}
