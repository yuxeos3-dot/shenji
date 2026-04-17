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
			"SELECT id FROM sites WHERE LOWER(domain) = ? OR LOWER(domain) LIKE ? LIMIT 1",
			dom, "%"+dom+"%",
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

// SpiderOverview returns global spider stats and per-site rollups for the
// landing dashboard. Windows: today (UTC), last 7 days, last 30 days.
func SpiderOverview(c *gin.Context) {
	now := time.Now().UTC()
	day1 := now.Add(-24 * time.Hour)
	day7 := now.Add(-7 * 24 * time.Hour)
	day30 := now.Add(-30 * 24 * time.Hour)

	count := func(since time.Time) int {
		var n int
		db.DB.QueryRow("SELECT COUNT(*) FROM spider_logs WHERE visited_at >= ?", since).Scan(&n)
		return n
	}

	// Per-spider breakdown (last 7 days)
	type row struct {
		Spider string `json:"spider"`
		Hits   int    `json:"hits"`
	}
	var breakdown []row
	if rows, err := db.DB.Query(
		"SELECT spider, COUNT(*) FROM spider_logs WHERE visited_at >= ? GROUP BY spider ORDER BY 2 DESC",
		day7,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var r row
			if rows.Scan(&r.Spider, &r.Hits) == nil {
				breakdown = append(breakdown, r)
			}
		}
	}

	// Per-site top 20 (last 7 days)
	type siteRow struct {
		SiteID  int64  `json:"site_id"`
		Domain  string `json:"domain"`
		Name    string `json:"name"`
		Hits    int    `json:"hits"`
	}
	var sites []siteRow
	if rows, err := db.DB.Query(
		`SELECT sl.site_id, s.domain, s.name, COUNT(*) as hits
		 FROM spider_logs sl JOIN sites s ON sl.site_id = s.id
		 WHERE sl.visited_at >= ?
		 GROUP BY sl.site_id, s.domain, s.name
		 ORDER BY hits DESC LIMIT 20`,
		day7,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var r siteRow
			if rows.Scan(&r.SiteID, &r.Domain, &r.Name, &r.Hits) == nil {
				sites = append(sites, r)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"total_24h":  count(day1),
			"total_7d":   count(day7),
			"total_30d":  count(day30),
			"by_spider":  breakdown,
			"top_sites":  sites,
			"generated":  now.Format(time.RFC3339),
		},
	})
}

// SpiderDailyTrend returns one row per day × spider for the last N days.
func SpiderDailyTrend(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "14"))
	if days < 1 {
		days = 14
	}
	if days > 180 {
		days = 180
	}
	siteIDFilter := c.Query("site_id")

	since := time.Now().UTC().AddDate(0, 0, -days)
	q := `SELECT strftime('%Y-%m-%d', visited_at) as d, spider, COUNT(*)
	      FROM spider_logs WHERE visited_at >= ?`
	args := []interface{}{since}
	if siteIDFilter != "" {
		q += " AND site_id = ?"
		args = append(args, siteIDFilter)
	}
	q += " GROUP BY d, spider ORDER BY d ASC"

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
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
