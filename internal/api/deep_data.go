package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ═══════════════════════════════════════════════════════════════════════
// GSC Deep Sync — query × page × country × device with pagination
// ═══════════════════════════════════════════════════════════════════════

func SyncGSCDeep(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", sid).Scan(&domain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	token, err := getGoogleAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	gscSiteURL, err := resolveGSCProperty(token, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	daysToSync := 7
	if d := c.Query("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 90 {
			daysToSync = n
		}
	}

	totalRows, err := syncGSCDeepInternal(sid, token, gscSiteURL, daysToSync)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"message":    "GSC 深度数据同步成功",
			"total_rows": totalRows,
		},
	})
}

func syncGSCDeepInternal(siteID int64, token, gscSiteURL string, days int) (int, error) {
	// Determine date range: default pulls last 7 days of stable data (delay 3 days)
	// For first-time sync, pull 30 days. For subsequent syncs, use provided days.
	daysToSync := days
	// Check if we already have data — if not, pull 30 days
	var existingCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM gsc_query_page_daily WHERE site_id = $1", siteID).Scan(&existingCount)
	if existingCount == 0 {
		daysToSync = 30 // First sync: pull 30 days of history
	}

	encodedURL := url.QueryEscape(gscSiteURL)
	baseURL := fmt.Sprintf("%s/sites/%s/searchAnalytics/query", gscAPI, encodedURL)

	var totalRows int
	var wg sync.WaitGroup
	var mu sync.Mutex

	// 1. Fetch query × page × country × device for ALL searchTypes × ALL dates
	//    GSC rate limit: 1200 QPM. With 6 searchTypes + 2 appearance calls = 8 calls/day.
	//    30 days × 8 = 240 calls total — well within limits.
	//    We batch 3 days at a time with Wait() to be safe.
	searchTypes := []string{"web", "image", "video", "news", "discover", "googleNews"}
	syncedDays := 0
	for dayOffset := 3; dayOffset < 3+daysToSync; dayOffset++ {
		date := time.Now().AddDate(0, 0, -dayOffset).Format("2006-01-02")

		// Check if we already have data for this date (skip if so)
		var dateCount int
		db.DB.QueryRow("SELECT COUNT(*) FROM gsc_query_page_daily WHERE site_id = $1 AND date = $2", siteID, date).Scan(&dateCount)
		if dateCount > 0 {
			continue // already synced this day
		}
		syncedDays++

		// Primary: web searchType gets full query×page×country×device (the big one)
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			rows := fetchGSCPaged(token, baseURL, d, []string{"query", "page", "country", "device"}, "web")
			n := persistGSCQueryPageDaily(siteID, d, rows, "web")
			mu.Lock()
			totalRows += n
			mu.Unlock()
		}(date)

		// Secondary searchTypes: image/video/news/discover/googleNews
		// These typically have far fewer rows, fetch with query×page only (no country×device breakdown)
		for _, st := range searchTypes[1:] {
			wg.Add(1)
			go func(searchType, d string) {
				defer wg.Done()
				rows := fetchGSCPaged(token, baseURL, d, []string{"query", "page"}, searchType)
				// Convert 2-key rows to 4-key format for unified storage
				for i := range rows {
					if len(rows[i].Keys) == 2 {
						rows[i].Keys = append(rows[i].Keys, "all", "all")
					}
				}
				n := persistGSCQueryPageDaily(siteID, d, rows, searchType)
				mu.Lock()
				totalRows += n
				mu.Unlock()
			}(st, date)
		}

		// searchAppearance aggregated distribution
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			fetchGSCSearchAppearance(token, baseURL, siteID, d)
		}(date)

		// searchAppearance × query × page cross (AI_OVERVIEW, FEATURED_SNIPPET, etc.)
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			fetchGSCAppearanceQueryCross(token, baseURL, siteID, d)
		}(date)

		// Throttle: process 2 days at a time to stay under 1200 QPM
		// Each day fires ~8 concurrent requests. 2 days = ~16 requests.
		if syncedDays%2 == 0 {
			wg.Wait()
		}
	}
	// Flush remaining
	wg.Wait()

	// 4. Fetch sitemaps (once, not per-day)
	wg.Add(1)
	go func() {
		defer wg.Done()
		fetchGSCSitemaps(token, gscSiteURL, siteID, time.Now().Format("2006-01-02"))
	}()

	wg.Wait()

	// 5. Compute rank buckets for the most recent synced date
	latestDate := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	computeRankBuckets(siteID, latestDate)

	// 6. Enrich queries (brand/intent/length/question/comparison)
	enrichQueries(siteID)

	// 7. Compute derived data immediately (don't wait for nightly cron)
	go func() {
		ComputeOpportunities(siteID)
		ComputeCannibalization(siteID)
		ComputeQueryDiff(siteID)
	}()

	return totalRows, nil
}

type gscRow struct {
	Keys        []string `json:"keys"`
	Clicks      int      `json:"clicks"`
	Impressions int      `json:"impressions"`
	CTR         float64  `json:"ctr"`
	Position    float64  `json:"position"`
}

func fetchGSCPaged(token, baseURL, date string, dimensions []string, searchType string) []gscRow {
	var allRows []gscRow
	startRow := 0
	maxRounds := 10 // safety: 25000 × 10 = 250k rows max

	for round := 0; round < maxRounds; round++ {
		body := map[string]interface{}{
			"startDate":       date,
			"endDate":         date,
			"dimensions":      dimensions,
			"rowLimit":        25000,
			"startRow":        startRow,
			"dataState":       "final",
			"type":            searchType,
			"aggregationType": "byPage", // More precise: per-page data, not auto-aggregated
		}

		data, err := gscFetch(token, baseURL, "POST", body)
		if err != nil {
			log.Printf("[gsc-deep] page fetch error round=%d: %v", round, err)
			break
		}

		var resp struct {
			Rows []gscRow `json:"rows"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			log.Printf("[gsc-deep] unmarshal error: %v", err)
			break
		}

		allRows = append(allRows, resp.Rows...)
		if len(resp.Rows) < 25000 {
			break
		}
		startRow += 25000
	}
	return allRows
}

func persistGSCQueryPageDaily(siteID int64, date string, rows []gscRow, searchType string) int {
	if len(rows) == 0 {
		return 0
	}

	// Batch insert using multi-row INSERT
	tx, err := db.DB.Begin()
	if err != nil {
		log.Printf("[gsc-deep] begin tx: %v", err)
		return 0
	}

	count := 0
	for _, r := range rows {
		if len(r.Keys) < 4 {
			continue
		}
		query := r.Keys[0]
		pageURL := r.Keys[1]
		country := r.Keys[2]
		device := r.Keys[3]

		_, err := tx.Exec(
			`INSERT INTO gsc_query_page_daily (site_id, date, query, page_url, country, device, search_type, clicks, impressions, ctr, position)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (site_id, date, query, page_url, country, device, search_type) DO UPDATE SET
			   clicks = EXCLUDED.clicks, impressions = EXCLUDED.impressions,
			   ctr = EXCLUDED.ctr, position = EXCLUDED.position`,
			siteID, date, query, pageURL, country, device, searchType,
			r.Clicks, r.Impressions, r.CTR, r.Position,
		)
		if err != nil {
			log.Printf("[gsc-deep] insert row: %v", err)
			continue
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[gsc-deep] commit: %v", err)
		return 0
	}
	return count
}

func fetchGSCSearchAppearance(token, baseURL string, siteID int64, date string) {
	// Fetch with searchAppearance dimension to see SERP feature distribution
	body := map[string]interface{}{
		"startDate":  date,
		"endDate":    date,
		"dimensions": []string{"searchAppearance"},
		"rowLimit":   25000,
		"dataState":  "final",
		"type":       "web",
	}

	data, err := gscFetch(token, baseURL, "POST", body)
	if err != nil {
		log.Printf("[gsc-deep] searchAppearance fetch: %v", err)
		return
	}

	var resp struct {
		Rows []gscRow `json:"rows"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	for _, r := range resp.Rows {
		if len(r.Keys) < 1 {
			continue
		}
		appearance := r.Keys[0]
		db.DB.Exec(
			`INSERT INTO gsc_appearance_daily (site_id, date, appearance, clicks, impressions, ctr, position)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (site_id, date, appearance) DO UPDATE SET
			   clicks = EXCLUDED.clicks, impressions = EXCLUDED.impressions,
			   ctr = EXCLUDED.ctr, position = EXCLUDED.position`,
			siteID, date, appearance, r.Clicks, r.Impressions, r.CTR, r.Position,
		)
	}
}

// fetchGSCAppearanceQueryCross pulls searchAppearance × query × page for the top SERP features
// so we know WHICH keywords appear in AI Overview, Featured Snippet, FAQ, etc.
func fetchGSCAppearanceQueryCross(token, baseURL string, siteID int64, date string) {
	// Only fetch the most valuable SERP features to avoid excessive API calls
	keyFeatures := []string{"AI_OVERVIEW", "FEATURED_SNIPPET", "TPF_FAQ", "REVIEW_SNIPPET", "VIDEO_RESULT", "RICH_RESULT"}

	for _, feat := range keyFeatures {
		body := map[string]interface{}{
			"startDate":  date,
			"endDate":    date,
			"dimensions": []string{"query", "page"},
			"rowLimit":   1000,
			"dataState":  "final",
			"type":       "web",
			"dimensionFilterGroups": []map[string]interface{}{
				{
					"filters": []map[string]interface{}{
						{
							"dimension":  "searchAppearance",
							"operator":   "equals",
							"expression": feat,
						},
					},
				},
			},
		}

		data, err := gscFetch(token, baseURL, "POST", body)
		if err != nil {
			continue
		}

		var resp struct {
			Rows []gscRow `json:"rows"`
		}
		if json.Unmarshal(data, &resp) != nil {
			continue
		}

		// Store each query×page that appeared in this SERP feature
		// We re-use gsc_query_page_daily but mark via a separate tracking mechanism
		// For now, store in appearance_daily at a finer grain by logging a JSON array
		// of affected queries into a separate table or as enrichment metadata.
		// Simple approach: update the appearance_daily with a query count
		queryCount := len(resp.Rows)
		if queryCount > 0 {
			// Update the appearance row with query-level detail
			topQueriesJSON := "[]"
			type qInfo struct {
				Query       string  `json:"query"`
				Page        string  `json:"page"`
				Clicks      int     `json:"clicks"`
				Impressions int     `json:"impressions"`
				Position    float64 `json:"position"`
			}
			var topQ []qInfo
			for i, r := range resp.Rows {
				if i >= 20 || len(r.Keys) < 2 {
					continue
				}
				topQ = append(topQ, qInfo{
					Query: r.Keys[0], Page: r.Keys[1],
					Clicks: r.Clicks, Impressions: r.Impressions, Position: r.Position,
				})
			}
			if b, err := json.Marshal(topQ); err == nil {
				topQueriesJSON = string(b)
			}
			// Store the query-level detail into gsc_appearance_daily.top_queries_json
			db.DB.Exec(
				`UPDATE gsc_appearance_daily SET top_queries_json = $1
				 WHERE site_id = $2 AND date = $3 AND appearance = $4`,
				topQueriesJSON, siteID, date, feat,
			)
		}
		time.Sleep(100 * time.Millisecond) // Rate limit
	}
}

func fetchGSCSitemaps(token, gscSiteURL string, siteID int64, date string) {
	encodedURL := url.QueryEscape(gscSiteURL)
	apiURL := fmt.Sprintf("%s/sites/%s/sitemaps", gscAPI, encodedURL)

	data, err := gscFetch(token, apiURL, "GET", nil)
	if err != nil {
		log.Printf("[gsc-deep] sitemaps fetch: %v", err)
		return
	}

	var resp struct {
		Sitemap []struct {
			Path            string    `json:"path"`
			LastSubmitted   time.Time `json:"lastSubmitted"`
			LastDownloaded  time.Time `json:"lastDownloaded"`
			IsSitemapsIndex bool      `json:"isSitemapsIndex"`
			IsPending       bool      `json:"isPending"`
			Type            string    `json:"type"`
			Errors          int       `json:"errors"`
			Warnings        int       `json:"warnings"`
			Contents        []struct {
				Type      string `json:"type"`
				Submitted int64  `json:"submitted"`
				Indexed   int64  `json:"indexed"`
			} `json:"contents"`
		} `json:"sitemap"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	for _, sm := range resp.Sitemap {
		contentsJSON, _ := json.Marshal(sm.Contents)
		db.DB.Exec(
			`INSERT INTO gsc_sitemaps (site_id, sitemap_path, snapshot_date, is_sitemaps_index, is_pending, type, last_submitted, last_downloaded, errors, warnings, contents_json)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (site_id, sitemap_path, snapshot_date) DO UPDATE SET
			   is_sitemaps_index = EXCLUDED.is_sitemaps_index, is_pending = EXCLUDED.is_pending,
			   type = EXCLUDED.type, last_submitted = EXCLUDED.last_submitted,
			   last_downloaded = EXCLUDED.last_downloaded, errors = EXCLUDED.errors,
			   warnings = EXCLUDED.warnings, contents_json = EXCLUDED.contents_json`,
			siteID, sm.Path, date, sm.IsSitemapsIndex, sm.IsPending,
			sm.Type, sm.LastSubmitted, sm.LastDownloaded,
			sm.Errors, sm.Warnings, string(contentsJSON),
		)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Rank Buckets Computation
// ═══════════════════════════════════════════════════════════════════════

func computeRankBuckets(siteID int64, date string) {
	buckets := []struct {
		name string
		low  float64
		high float64
	}{
		{"1-3", 0, 3},
		{"4-10", 3, 10},
		{"11-20", 10, 20},
		{"21-50", 20, 50},
		{"51-100", 50, 100},
		{"100+", 100, 99999},
	}

	for _, b := range buckets {
		var queryCount, pageCount, qpCount, totalClicks, totalImpressions int
		var avgCTR, avgPosition float64

		err := db.DB.QueryRow(`
			SELECT
				COUNT(DISTINCT query),
				COUNT(DISTINCT page_url),
				COUNT(*),
				COALESCE(SUM(clicks), 0),
				COALESCE(SUM(impressions), 0),
				CASE WHEN SUM(impressions) > 0 THEN SUM(clicks)::float / SUM(impressions) ELSE 0 END,
				CASE WHEN SUM(impressions) > 0 THEN SUM(position * impressions) / SUM(impressions) ELSE 0 END
			FROM gsc_query_page_daily
			WHERE site_id = $1 AND date = $2 AND search_type = 'web'
			  AND position >= $3 AND position < $4`,
			siteID, date, b.low, b.high,
		).Scan(&queryCount, &pageCount, &qpCount, &totalClicks, &totalImpressions, &avgCTR, &avgPosition)
		if err != nil {
			continue
		}

		// Top queries by clicks in this bucket
		topQRows, _ := db.DB.Query(`
			SELECT query, SUM(clicks) as c, SUM(impressions) as i,
				CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as p
			FROM gsc_query_page_daily
			WHERE site_id = $1 AND date = $2 AND search_type = 'web'
			  AND position >= $3 AND position < $4
			GROUP BY query ORDER BY c DESC LIMIT 10`,
			siteID, date, b.low, b.high)
		var topQueries []map[string]interface{}
		if topQRows != nil {
			defer topQRows.Close()
			for topQRows.Next() {
				var q string
				var c, i int
				var p float64
				topQRows.Scan(&q, &c, &i, &p)
				topQueries = append(topQueries, map[string]interface{}{
					"query": q, "clicks": c, "impressions": i, "position": p,
				})
			}
		}
		topQJSON, _ := json.Marshal(topQueries)

		// Top pages by clicks in this bucket
		topPRows, _ := db.DB.Query(`
			SELECT page_url, SUM(clicks) as c, SUM(impressions) as i,
				CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as p
			FROM gsc_query_page_daily
			WHERE site_id = $1 AND date = $2 AND search_type = 'web'
			  AND position >= $3 AND position < $4
			GROUP BY page_url ORDER BY c DESC LIMIT 10`,
			siteID, date, b.low, b.high)
		var topPages []map[string]interface{}
		if topPRows != nil {
			defer topPRows.Close()
			for topPRows.Next() {
				var u string
				var c, i int
				var p float64
				topPRows.Scan(&u, &c, &i, &p)
				topPages = append(topPages, map[string]interface{}{
					"url": u, "clicks": c, "impressions": i, "position": p,
				})
			}
		}
		topPJSON, _ := json.Marshal(topPages)

		db.DB.Exec(
			`INSERT INTO gsc_rank_buckets_daily (site_id, date, bucket, query_count, page_count, query_page_count, total_clicks, total_impressions, avg_ctr, avg_position, top_queries_json, top_pages_json)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 ON CONFLICT (site_id, date, bucket, search_type, country, device) DO UPDATE SET
			   query_count = EXCLUDED.query_count, page_count = EXCLUDED.page_count,
			   query_page_count = EXCLUDED.query_page_count, total_clicks = EXCLUDED.total_clicks,
			   total_impressions = EXCLUDED.total_impressions, avg_ctr = EXCLUDED.avg_ctr,
			   avg_position = EXCLUDED.avg_position, top_queries_json = EXCLUDED.top_queries_json,
			   top_pages_json = EXCLUDED.top_pages_json`,
			siteID, date, b.name, queryCount, pageCount, qpCount,
			totalClicks, totalImpressions, avgCTR, avgPosition,
			string(topQJSON), string(topPJSON),
		)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Query Enrichment — brand / intent / length / question detection
// ═══════════════════════════════════════════════════════════════════════

var (
	questionWordsZH = []string{"什么", "怎么", "怎样", "如何", "为什么", "为何", "哪里", "哪个", "哪些", "哪种", "多少", "多久", "是否", "能不能", "可不可以"}
	questionWordsEN = []string{"what", "how", "why", "when", "where", "who", "which", "whose", "whom", "can", "could", "should", "would", "will", "is", "are", "does", "do"}
	comparisonWords = []string{"对比", "哪个好", "区别", "差异", "vs", "versus", "compared", "difference", "between", "之间"}
	intentTransact  = []string{"buy", "price", "购买", "下载", "shop", "order", "coupon", "discount", "cheap"}
	intentCommerc   = []string{"best", "top", "review", "评测", "对比", "推荐", "哪个好"}
	intentInfo      = []string{"what", "how", "why", "什么", "怎么", "如何", "原理", "教程", "guide", "tutorial"}
)

func enrichQueries(siteID int64) {
	// Get brand regex for this site
	var brandRegex string
	db.DB.QueryRow("SELECT COALESCE(brand_regex, '') FROM sites WHERE id = $1", siteID).Scan(&brandRegex)

	var compiledBrand *regexp.Regexp
	if brandRegex != "" {
		compiledBrand, _ = regexp.Compile("(?i)" + brandRegex)
	}

	// Get distinct queries that are NOT yet enriched
	rows, err := db.DB.Query(`
		SELECT DISTINCT q.query
		FROM gsc_query_page_daily q
		LEFT JOIN query_enrichment e ON e.site_id = q.site_id AND e.query = q.query AND e.platform = 'gsc'
		WHERE q.site_id = $1 AND e.query IS NULL
		LIMIT 10000`, siteID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var query string
		rows.Scan(&query)
		ql := strings.ToLower(query)

		// Brand classification
		brand := "unknown"
		if compiledBrand != nil {
			if compiledBrand.MatchString(ql) {
				brand = "brand"
			} else {
				brand = "non_brand"
			}
		}

		// Intent classification (rule-based)
		intent := "unknown"
		for _, w := range intentTransact {
			if strings.Contains(ql, w) {
				intent = "transactional"
				break
			}
		}
		if intent == "unknown" {
			for _, w := range intentCommerc {
				if strings.Contains(ql, w) {
					intent = "commercial"
					break
				}
			}
		}
		if intent == "unknown" {
			for _, w := range intentInfo {
				if strings.Contains(ql, w) {
					intent = "informational"
					break
				}
			}
		}
		if intent == "unknown" && brand == "brand" {
			intent = "navigational"
		}

		// Length classification
		wordCount := len(strings.Fields(query))
		charCount := utf8.RuneCountInString(query)
		lengthCat := "body"
		if wordCount <= 1 && charCount <= 3 {
			lengthCat = "head"
		} else if wordCount >= 4 || charCount >= 9 {
			lengthCat = "long_tail"
		}

		// Question detection
		isQuestion := false
		for _, w := range questionWordsZH {
			if strings.Contains(ql, w) {
				isQuestion = true
				break
			}
		}
		if !isQuestion {
			for _, w := range questionWordsEN {
				if strings.Contains(ql, w) {
					isQuestion = true
					break
				}
			}
		}

		// Comparison detection
		isComparison := false
		for _, w := range comparisonWords {
			if strings.Contains(ql, w) {
				isComparison = true
				break
			}
		}

		db.DB.Exec(
			`INSERT INTO query_enrichment (site_id, query, platform, brand_category, intent, length_cat, word_count, char_count, is_question, is_comparison, first_seen_date, last_seen_date)
			 VALUES ($1, $2, 'gsc', $3, $4, $5, $6, $7, $8, $9, CURRENT_DATE, CURRENT_DATE)
			 ON CONFLICT (site_id, query, platform) DO UPDATE SET
			   brand_category = EXCLUDED.brand_category, intent = EXCLUDED.intent,
			   length_cat = EXCLUDED.length_cat, word_count = EXCLUDED.word_count,
			   char_count = EXCLUDED.char_count, is_question = EXCLUDED.is_question,
			   is_comparison = EXCLUDED.is_comparison, last_seen_date = CURRENT_DATE,
			   updated_at = CURRENT_TIMESTAMP`,
			siteID, query, brand, intent, lengthCat, wordCount, charCount, isQuestion, isComparison,
		)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Opportunities Computation
// ═══════════════════════════════════════════════════════════════════════

func ComputeOpportunities(siteID int64) {
	weekStart := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	weekEnd := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	rows, err := db.DB.Query(`
		SELECT query, page_url, SUM(clicks) as clicks, SUM(impressions) as imp,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as pos
		FROM gsc_query_page_daily
		WHERE site_id = $1 AND date >= $2 AND date <= $3 AND search_type = 'web'
		GROUP BY query, page_url
		HAVING SUM(impressions) > 50
		ORDER BY SUM(impressions) DESC
		LIMIT 5000`, siteID, weekStart, weekEnd)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var query, pageURL string
		var clicks, impressions int
		var actualCTR, position float64
		rows.Scan(&query, &pageURL, &clicks, &impressions, &actualCTR, &position)

		posInt := int(math.Round(position))
		if posInt < 1 {
			posInt = 1
		}
		if posInt > 100 {
			posInt = 100
		}

		var baselineCTR float64
		db.DB.QueryRow("SELECT ctr FROM ctr_baseline WHERE position = $1", posInt).Scan(&baselineCTR)

		var ctrGapRatio float64
		if baselineCTR > 0 {
			ctrGapRatio = (baselineCTR - actualCTR) / baselineCTR
		}
		if ctrGapRatio < 0 {
			ctrGapRatio = 0
		}

		// Opportunity score
		imprWeight := math.Log10(float64(impressions) + 1)
		var posPotential float64
		switch {
		case position < 3:
			posPotential = 0.2
		case position < 10:
			posPotential = 1.0
		case position < 20:
			posPotential = 0.7
		case position < 50:
			posPotential = 0.3
		default:
			posPotential = 0.1
		}
		score := math.Round(imprWeight*posPotential*(0.5+ctrGapRatio)*10*100) / 100

		extraClicks := 0
		if baselineCTR > actualCTR {
			extraClicks = int((baselineCTR - actualCTR) * float64(impressions) * 4) // monthly estimate
		}

		oppType := "general"
		if position < 10 && ctrGapRatio > 0.3 {
			oppType = "low_ctr_on_good_position"
		} else if position >= 4 && position < 15 {
			oppType = "close_to_top"
		} else if impressions > 500 && position >= 20 {
			oppType = "high_impression_low_rank"
		}

		db.DB.Exec(
			`INSERT INTO gsc_opportunities_weekly (site_id, week_start, query, page_url, impressions, clicks, actual_ctr, baseline_ctr, ctr_gap_ratio, position, opportunity_score, estimated_extra_clicks, opportunity_type)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			 ON CONFLICT (site_id, week_start, query, page_url) DO UPDATE SET
			   impressions = EXCLUDED.impressions, clicks = EXCLUDED.clicks,
			   actual_ctr = EXCLUDED.actual_ctr, baseline_ctr = EXCLUDED.baseline_ctr,
			   ctr_gap_ratio = EXCLUDED.ctr_gap_ratio, position = EXCLUDED.position,
			   opportunity_score = EXCLUDED.opportunity_score,
			   estimated_extra_clicks = EXCLUDED.estimated_extra_clicks,
			   opportunity_type = EXCLUDED.opportunity_type`,
			siteID, weekStart, query, pageURL, impressions, clicks,
			actualCTR, baselineCTR, ctrGapRatio, position, score, extraClicks, oppType,
		)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Cannibalization Detection
// ═══════════════════════════════════════════════════════════════════════

func ComputeCannibalization(siteID int64) {
	month := time.Now().Format("2006-01")
	sd := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	rows, err := db.DB.Query(`
		WITH query_pages AS (
			SELECT query, page_url, SUM(impressions) as imp, SUM(clicks) as clicks,
				CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as avg_pos
			FROM gsc_query_page_daily
			WHERE site_id = $1 AND date >= $2 AND position < 20 AND search_type = 'web'
			GROUP BY query, page_url
		),
		multi AS (
			SELECT query, COUNT(*) as pc, SUM(imp) as ti, SUM(clicks) as tc
			FROM query_pages GROUP BY query HAVING COUNT(*) >= 2 AND SUM(imp) > 100
		)
		SELECT qp.query, qp.page_url, qp.imp, qp.clicks, qp.avg_pos, m.pc, m.ti, m.tc
		FROM query_pages qp JOIN multi m USING (query)
		ORDER BY m.ti DESC, qp.query, qp.imp DESC
		LIMIT 5000`, siteID, sd)
	if err != nil {
		return
	}
	defer rows.Close()

	type pageInfo struct {
		URL         string  `json:"url"`
		Impressions int     `json:"impressions"`
		Clicks      int     `json:"clicks"`
		Position    float64 `json:"position"`
	}
	queryData := make(map[string]struct {
		pages    []pageInfo
		totalImp int
		totalClk int
		count    int
	})

	for rows.Next() {
		var query, pageURL string
		var imp, clicks, pc, ti, tc int
		var avgPos float64
		rows.Scan(&query, &pageURL, &imp, &clicks, &avgPos, &pc, &ti, &tc)

		d := queryData[query]
		d.pages = append(d.pages, pageInfo{URL: pageURL, Impressions: imp, Clicks: clicks, Position: avgPos})
		d.totalImp = ti
		d.totalClk = tc
		d.count = pc
		queryData[query] = d
	}

	for query, d := range queryData {
		severity := "low"
		if d.count >= 4 {
			severity = "high"
		} else if d.count >= 3 || d.totalImp > 1000 {
			severity = "medium"
		}

		pagesJSON, _ := json.Marshal(d.pages)
		db.DB.Exec(
			`INSERT INTO gsc_cannibalization_monthly (site_id, month, query, page_count, competing_pages, severity, total_impressions, total_clicks)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (site_id, month, query) DO UPDATE SET
			   page_count = EXCLUDED.page_count, competing_pages = EXCLUDED.competing_pages,
			   severity = EXCLUDED.severity, total_impressions = EXCLUDED.total_impressions,
			   total_clicks = EXCLUDED.total_clicks`,
			siteID, month, query, d.count, string(pagesJSON), severity, d.totalImp, d.totalClk,
		)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Query Diff (lost / new / risen / fallen)
// ═══════════════════════════════════════════════════════════════════════

func ComputeQueryDiff(siteID int64) {
	weekStart := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	prevStart := time.Now().AddDate(0, 0, -14).Format("2006-01-02")
	prevEnd := time.Now().AddDate(0, 0, -8).Format("2006-01-02")
	currEnd := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Lost queries
	db.DB.Exec(`
		INSERT INTO gsc_query_diff_weekly (site_id, week_start, diff_type, query, prev_clicks, curr_clicks, click_delta)
		SELECT $1, $2::date, 'lost', p.query, p.clicks, 0, -p.clicks
		FROM (
			SELECT query, SUM(clicks) as clicks
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= $3 AND date <= $4 AND search_type = 'web'
			GROUP BY query HAVING SUM(clicks) > 0
		) p
		LEFT JOIN (
			SELECT DISTINCT query FROM gsc_query_page_daily WHERE site_id = $1 AND date >= $2 AND date <= $5 AND search_type = 'web'
		) c ON c.query = p.query
		WHERE c.query IS NULL
		ON CONFLICT (site_id, week_start, diff_type, query, page_url) DO NOTHING`,
		siteID, weekStart, prevStart, prevEnd, currEnd)

	// New queries
	db.DB.Exec(`
		INSERT INTO gsc_query_diff_weekly (site_id, week_start, diff_type, query, prev_clicks, curr_clicks, click_delta)
		SELECT $1, $2::date, 'new', c.query, 0, c.clicks, c.clicks
		FROM (
			SELECT query, SUM(clicks) as clicks
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= $2 AND date <= $5 AND search_type = 'web'
			GROUP BY query HAVING SUM(impressions) > 5
		) c
		LEFT JOIN (
			SELECT DISTINCT query FROM gsc_query_page_daily WHERE site_id = $1 AND date >= $3 AND date <= $4 AND search_type = 'web'
		) p ON p.query = c.query
		WHERE p.query IS NULL
		ON CONFLICT (site_id, week_start, diff_type, query, page_url) DO NOTHING`,
		siteID, weekStart, prevStart, prevEnd, currEnd)
}

// ═══════════════════════════════════════════════════════════════════════
// GA4 Extended Sync
// ═══════════════════════════════════════════════════════════════════════

func SyncGA4Deep(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", sid).Scan(&domain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在"})
		return
	}
	token, err := getGAAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	gaPropertyID, err := resolveGAProperty(token, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	if err := syncGA4DeepInternal(sid, token, gaPropertyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"message": "GA4 深度数据同步成功"}})
}

func syncGA4DeepInternal(siteID int64, token, gaPropertyID string) error {
	date := "yesterday"
	dateRange := []map[string]string{{"startDate": date, "endDate": date}}

	type queryDef struct {
		name string
		body map[string]interface{}
	}

	queries := []queryDef{
		// Extended daily overview
		{"extended", map[string]interface{}{
			"dateRanges": dateRange,
			"metrics": []map[string]string{
				{"name": "totalUsers"}, {"name": "activeUsers"}, {"name": "newUsers"},
				{"name": "sessions"}, {"name": "engagedSessions"},
				{"name": "sessionsPerUser"}, {"name": "averageSessionDuration"},
				{"name": "engagementRate"}, {"name": "bounceRate"},
				{"name": "userEngagementDuration"},
				{"name": "screenPageViews"}, {"name": "screenPageViewsPerSession"},
				{"name": "eventCount"}, {"name": "eventCountPerUser"},
				{"name": "eventsPerSession"}, {"name": "conversions"},
				{"name": "totalRevenue"},
			},
		}},
		// Landing pages
		{"landing_pages", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "landingPage"}},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
				{"name": "engagedSessions"}, {"name": "engagementRate"},
				{"name": "bounceRate"}, {"name": "averageSessionDuration"},
				{"name": "screenPageViews"}, {"name": "conversions"},
			},
			"orderBys": []map[string]interface{}{{"metric": map[string]string{"metricName": "sessions"}, "desc": true}},
			"limit": 500,
		}},
		// Source / medium
		{"source_medium", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{
				{"name": "sessionSource"}, {"name": "sessionMedium"},
				{"name": "sessionDefaultChannelGroup"},
			},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
				{"name": "engagedSessions"}, {"name": "engagementRate"},
				{"name": "conversions"},
			},
			"orderBys": []map[string]interface{}{{"metric": map[string]string{"metricName": "sessions"}, "desc": true}},
			"limit": 200,
		}},
		// Events
		{"events", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "eventName"}, {"name": "isConversionEvent"}},
			"metrics": []map[string]string{
				{"name": "eventCount"}, {"name": "totalUsers"}, {"name": "eventValue"},
				{"name": "eventCountPerUser"},
			},
			"orderBys": []map[string]interface{}{{"metric": map[string]string{"metricName": "eventCount"}, "desc": true}},
			"limit": 200,
		}},
		// Device/OS/Browser
		{"tech", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "deviceCategory"}, {"name": "operatingSystem"}, {"name": "browser"}},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"},
				{"name": "bounceRate"}, {"name": "engagementRate"},
			},
			"limit": 100,
		}},
		// Geo
		{"geo", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "country"}, {"name": "region"}, {"name": "city"}},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
				{"name": "engagedSessions"}, {"name": "engagementRate"},
			},
			"orderBys": []map[string]interface{}{{"metric": map[string]string{"metricName": "sessions"}, "desc": true}},
			"limit": 200,
		}},
		// Hourly
		{"hourly", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "hour"}},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "screenPageViews"},
			},
		}},
		// Day of week
		{"day_of_week", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "dayOfWeek"}},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "screenPageViews"}, {"name": "conversions"},
			},
		}},
		// Scroll depth (enhanced measurement event)
		{"scroll_depth", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "pagePath"}, {"name": "percentScrolled"}},
			"metrics": []map[string]string{
				{"name": "totalUsers"}, {"name": "eventCount"},
			},
			"dimensionFilter": map[string]interface{}{
				"filter": map[string]interface{}{
					"fieldName":    "eventName",
					"stringFilter": map[string]interface{}{"value": "scroll"},
				},
			},
			"limit": 500,
		}},
		// Outbound clicks
		{"outbound_clicks", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "linkDomain"}, {"name": "linkUrl"}},
			"metrics": []map[string]string{
				{"name": "eventCount"}, {"name": "totalUsers"},
			},
			"dimensionFilter": map[string]interface{}{
				"filter": map[string]interface{}{
					"fieldName":    "outbound",
					"stringFilter": map[string]interface{}{"value": "true"},
				},
			},
			"orderBys": []map[string]interface{}{{"metric": map[string]string{"metricName": "eventCount"}, "desc": true}},
			"limit": 200,
		}},
		// Conversions by source/medium
		{"conversions", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "eventName"}, {"name": "sessionSource"}, {"name": "sessionMedium"}},
			"metrics": []map[string]string{
				{"name": "conversions"}, {"name": "totalRevenue"},
			},
			"dimensionFilter": map[string]interface{}{
				"filter": map[string]interface{}{
					"fieldName":    "isConversionEvent",
					"stringFilter": map[string]interface{}{"value": "true"},
				},
			},
			"limit": 200,
		}},
	}

	results := make(map[string]json.RawMessage)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(name string, body map[string]interface{}) {
			defer wg.Done()
			data, err := ga4Fetch(token, gaPropertyID, body)
			mu.Lock()
			if err != nil {
				log.Printf("[ga4-deep] %s fetch error: %v", name, err)
				results[name] = nil
			} else {
				results[name] = data
			}
			mu.Unlock()
		}(q.name, q.body)
	}
	wg.Wait()

	dateStr := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Parse and store extended overview
	if results["extended"] != nil {
		persistGA4Extended(siteID, dateStr, results["extended"])
	}
	if results["landing_pages"] != nil {
		persistGA4LandingPages(siteID, dateStr, results["landing_pages"])
	}
	if results["source_medium"] != nil {
		persistGA4SourceMedium(siteID, dateStr, results["source_medium"])
	}
	if results["events"] != nil {
		persistGA4Events(siteID, dateStr, results["events"])
	}
	if results["tech"] != nil {
		persistGA4Tech(siteID, dateStr, results["tech"])
	}
	if results["geo"] != nil {
		persistGA4Geo(siteID, dateStr, results["geo"])
	}
	if results["hourly"] != nil {
		persistGA4Hourly(siteID, dateStr, results["hourly"])
	}
	if results["day_of_week"] != nil {
		persistGA4DayOfWeek(siteID, dateStr, results["day_of_week"])
	}
	if results["scroll_depth"] != nil {
		persistGA4ScrollDepth(siteID, dateStr, results["scroll_depth"])
	}
	if results["outbound_clicks"] != nil {
		persistGA4OutboundClicks(siteID, dateStr, results["outbound_clicks"])
	}
	if results["conversions"] != nil {
		persistGA4Conversions(siteID, dateStr, results["conversions"])
	}

	return nil
}

// GA4 row parser helper
type ga4Row struct {
	Dims    []string
	Metrics []string
}

func parseGA4Rows(raw json.RawMessage) []ga4Row {
	var resp struct {
		Rows []struct {
			DimensionValues []struct{ Value string } `json:"dimensionValues"`
			MetricValues    []struct{ Value string } `json:"metricValues"`
		} `json:"rows"`
	}
	json.Unmarshal(raw, &resp)
	var out []ga4Row
	for _, r := range resp.Rows {
		row := ga4Row{}
		for _, d := range r.DimensionValues {
			row.Dims = append(row.Dims, d.Value)
		}
		for _, m := range r.MetricValues {
			row.Metrics = append(row.Metrics, m.Value)
		}
		out = append(out, row)
	}
	return out
}

func ga4Int(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
func ga4Float(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func ga4Int64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func persistGA4Extended(siteID int64, date string, raw json.RawMessage) {
	rows := parseGA4Rows(raw)
	if len(rows) == 0 {
		return
	}
	m := rows[0].Metrics
	if len(m) < 17 {
		return
	}
	db.DB.Exec(`INSERT INTO ga_daily_extended (site_id, date, total_users, active_users, new_users,
		sessions, engaged_sessions, sessions_per_user, average_session_duration,
		engagement_rate, bounce_rate, user_engagement_duration, screen_page_views,
		page_views_per_session, event_count, event_count_per_user, events_per_session,
		conversions, total_revenue)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (site_id,date) DO UPDATE SET
		total_users=EXCLUDED.total_users, active_users=EXCLUDED.active_users, new_users=EXCLUDED.new_users,
		sessions=EXCLUDED.sessions, engaged_sessions=EXCLUDED.engaged_sessions,
		sessions_per_user=EXCLUDED.sessions_per_user, average_session_duration=EXCLUDED.average_session_duration,
		engagement_rate=EXCLUDED.engagement_rate, bounce_rate=EXCLUDED.bounce_rate,
		user_engagement_duration=EXCLUDED.user_engagement_duration, screen_page_views=EXCLUDED.screen_page_views,
		page_views_per_session=EXCLUDED.page_views_per_session, event_count=EXCLUDED.event_count,
		event_count_per_user=EXCLUDED.event_count_per_user, events_per_session=EXCLUDED.events_per_session,
		conversions=EXCLUDED.conversions, total_revenue=EXCLUDED.total_revenue`,
		siteID, date,
		ga4Int(m[0]), ga4Int(m[1]), ga4Int(m[2]),
		ga4Int(m[3]), ga4Int(m[4]),
		ga4Float(m[5]), ga4Float(m[6]),
		ga4Float(m[7]), ga4Float(m[8]),
		ga4Int64(m[9]), ga4Int(m[10]),
		ga4Float(m[11]), ga4Int(m[12]),
		ga4Float(m[13]), ga4Float(m[14]),
		ga4Int(m[15]), ga4Float(m[16]),
	)
}

func persistGA4LandingPages(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 1 || len(r.Metrics) < 9 {
			continue
		}
		db.DB.Exec(`INSERT INTO ga_landing_pages_daily (site_id, date, landing_page, sessions, users, new_users, engaged_sessions, engagement_rate, bounce_rate, average_session_duration, screen_page_views, conversions)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (site_id,date,landing_page) DO UPDATE SET sessions=EXCLUDED.sessions,users=EXCLUDED.users,new_users=EXCLUDED.new_users,engaged_sessions=EXCLUDED.engaged_sessions,engagement_rate=EXCLUDED.engagement_rate,bounce_rate=EXCLUDED.bounce_rate,average_session_duration=EXCLUDED.average_session_duration,screen_page_views=EXCLUDED.screen_page_views,conversions=EXCLUDED.conversions`,
			siteID, date, r.Dims[0],
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]), ga4Int(r.Metrics[2]),
			ga4Int(r.Metrics[3]), ga4Float(r.Metrics[4]), ga4Float(r.Metrics[5]),
			ga4Float(r.Metrics[6]), ga4Int(r.Metrics[7]), ga4Int(r.Metrics[8]),
		)
	}
}

func persistGA4SourceMedium(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 3 || len(r.Metrics) < 6 {
			continue
		}
		db.DB.Exec(`INSERT INTO ga_source_medium_daily (site_id, date, source, medium, channel_group, sessions, users, new_users, engaged_sessions, engagement_rate, conversions)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (site_id,date,source,medium,campaign) DO UPDATE SET channel_group=EXCLUDED.channel_group,sessions=EXCLUDED.sessions,users=EXCLUDED.users,new_users=EXCLUDED.new_users,engaged_sessions=EXCLUDED.engaged_sessions,engagement_rate=EXCLUDED.engagement_rate,conversions=EXCLUDED.conversions`,
			siteID, date, r.Dims[0], r.Dims[1], r.Dims[2],
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]), ga4Int(r.Metrics[2]),
			ga4Int(r.Metrics[3]), ga4Float(r.Metrics[4]), ga4Int(r.Metrics[5]),
		)
	}
}

func persistGA4Events(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 2 || len(r.Metrics) < 4 {
			continue
		}
		isConv := r.Dims[1] == "true"
		db.DB.Exec(`INSERT INTO ga_events_daily (site_id, date, event_name, event_count, users_with_event, event_value_sum, event_count_per_user, is_conversion)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (site_id,date,event_name) DO UPDATE SET event_count=EXCLUDED.event_count,users_with_event=EXCLUDED.users_with_event,event_value_sum=EXCLUDED.event_value_sum,event_count_per_user=EXCLUDED.event_count_per_user,is_conversion=EXCLUDED.is_conversion`,
			siteID, date, r.Dims[0],
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]), ga4Float(r.Metrics[2]), ga4Float(r.Metrics[3]),
			isConv,
		)
	}
}

func persistGA4Tech(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 3 || len(r.Metrics) < 4 {
			continue
		}
		// Store each dimension type separately
		dimTypes := []struct{ typ, val string }{
			{"device", r.Dims[0]},
			{"os", r.Dims[1]},
			{"browser", r.Dims[2]},
		}
		for _, dt := range dimTypes {
			if dt.val == "" || dt.val == "(not set)" {
				continue
			}
			db.DB.Exec(`INSERT INTO ga_tech_daily (site_id, date, dimension_type, dimension_value, sessions, users, bounce_rate, engagement_rate)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (site_id,date,dimension_type,dimension_value) DO UPDATE SET sessions=EXCLUDED.sessions,users=EXCLUDED.users,bounce_rate=EXCLUDED.bounce_rate,engagement_rate=EXCLUDED.engagement_rate`,
				siteID, date, dt.typ, dt.val,
				ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]),
				ga4Float(r.Metrics[2]), ga4Float(r.Metrics[3]),
			)
		}
	}
}

func persistGA4Geo(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 3 || len(r.Metrics) < 5 {
			continue
		}
		db.DB.Exec(`INSERT INTO ga_geo_daily (site_id, date, country, region, city, sessions, users, new_users, engaged_sessions, engagement_rate)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (site_id,date,country,region,city) DO UPDATE SET sessions=EXCLUDED.sessions,users=EXCLUDED.users,new_users=EXCLUDED.new_users,engaged_sessions=EXCLUDED.engaged_sessions,engagement_rate=EXCLUDED.engagement_rate`,
			siteID, date, r.Dims[0], r.Dims[1], r.Dims[2],
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]), ga4Int(r.Metrics[2]),
			ga4Int(r.Metrics[3]), ga4Float(r.Metrics[4]),
		)
	}
}

func persistGA4Hourly(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 1 || len(r.Metrics) < 3 {
			continue
		}
		hour := ga4Int(r.Dims[0])
		db.DB.Exec(`INSERT INTO ga_hourly (site_id, date, hour, sessions, users, screen_page_views)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (site_id,date,hour) DO UPDATE SET sessions=EXCLUDED.sessions,users=EXCLUDED.users,screen_page_views=EXCLUDED.screen_page_views`,
			siteID, date, hour,
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]), ga4Int(r.Metrics[2]),
		)
	}
}

func persistGA4DayOfWeek(siteID int64, date string, raw json.RawMessage) {
	// Compute year_week from date
	t, _ := time.Parse("2006-01-02", date)
	y, w := t.ISOWeek()
	yearWeek := fmt.Sprintf("%d%02d", y, w)

	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 1 || len(r.Metrics) < 4 {
			continue
		}
		dow := ga4Int(r.Dims[0])
		db.DB.Exec(`INSERT INTO ga_day_of_week (site_id, year_week, day_of_week, sessions, users, screen_page_views, conversions)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (site_id,year_week,day_of_week) DO UPDATE SET sessions=EXCLUDED.sessions,users=EXCLUDED.users,screen_page_views=EXCLUDED.screen_page_views,conversions=EXCLUDED.conversions`,
			siteID, yearWeek, dow,
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]), ga4Int(r.Metrics[2]), ga4Int(r.Metrics[3]),
		)
	}
}

func persistGA4ScrollDepth(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 2 || len(r.Metrics) < 2 {
			continue
		}
		db.DB.Exec(`INSERT INTO ga_scroll_depth_daily (site_id, date, page_path, percent_scrolled, users, event_count)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (site_id,date,page_path,percent_scrolled) DO UPDATE SET users=EXCLUDED.users,event_count=EXCLUDED.event_count`,
			siteID, date, r.Dims[0], ga4Int(r.Dims[1]),
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]),
		)
	}
}

func persistGA4OutboundClicks(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 2 || len(r.Metrics) < 2 {
			continue
		}
		db.DB.Exec(`INSERT INTO ga_outbound_clicks_daily (site_id, date, link_domain, link_url, event_count, users)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (site_id,date,link_url) DO UPDATE SET link_domain=EXCLUDED.link_domain,event_count=EXCLUDED.event_count,users=EXCLUDED.users`,
			siteID, date, r.Dims[0], r.Dims[1],
			ga4Int(r.Metrics[0]), ga4Int(r.Metrics[1]),
		)
	}
}

func persistGA4Conversions(siteID int64, date string, raw json.RawMessage) {
	for _, r := range parseGA4Rows(raw) {
		if len(r.Dims) < 3 || len(r.Metrics) < 2 {
			continue
		}
		db.DB.Exec(`INSERT INTO ga_conversions_daily (site_id, date, event_name, source, medium, conversions, total_revenue)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (site_id,date,event_name,source,medium) DO UPDATE SET conversions=EXCLUDED.conversions,total_revenue=EXCLUDED.total_revenue`,
			siteID, date, r.Dims[0], r.Dims[1], r.Dims[2],
			ga4Int(r.Metrics[0]), ga4Float(r.Metrics[1]),
		)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Bing Deep Sync
// ═══════════════════════════════════════════════════════════════════════

func SyncBingDeep(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", sid).Scan(&domain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在"})
		return
	}
	apiKey, _ := LoadUserBingKey(c.GetInt64("user_id"))
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → Bing」填写 API Key"})
		return
	}
	bingSiteURL, err := resolveBingSiteURL(apiKey, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	if err := syncBingDeepInternal(sid, apiKey, bingSiteURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"message": "Bing 深度数据同步成功"}})
}

func syncBingDeepInternal(siteID int64, apiKey, bingSiteURL string) error {
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	sd := time.Now().AddDate(0, 0, -30).Format("2006-01-02T00:00:00")
	ed := time.Now().AddDate(0, 0, -1).Format("2006-01-02T00:00:00")

	var wg sync.WaitGroup

	// 1. Query stats → bing_query_daily
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := bingFetch(apiKey, "GetQueryStats", map[string]string{
			"siteUrl": bingSiteURL, "startDate": sd, "endDate": ed,
		})
		if err != nil {
			return
		}
		var resp struct {
			D struct {
				Results []struct {
					Query       string  `json:"Query"`
					Clicks      int     `json:"Clicks"`
					Impressions int     `json:"Impressions"`
					CTR         float64 `json:"CTR"`
					AvgImprPos  float64 `json:"AvgImpPos"`
					AvgClickPos float64 `json:"AvgClkPos"`
				} `json:"Results"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &resp) != nil {
			return
		}
		for _, r := range resp.D.Results {
			db.DB.Exec(`INSERT INTO bing_query_daily (site_id, date, query, clicks, impressions, ctr, avg_impression_position, avg_click_position)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (site_id,date,query) DO UPDATE SET clicks=EXCLUDED.clicks,impressions=EXCLUDED.impressions,ctr=EXCLUDED.ctr,avg_impression_position=EXCLUDED.avg_impression_position,avg_click_position=EXCLUDED.avg_click_position`,
				siteID, date, r.Query, r.Clicks, r.Impressions, r.CTR, r.AvgImprPos, r.AvgClickPos,
			)
		}
	}()

	// 2. Page stats → bing_page_daily
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := bingFetch(apiKey, "GetPageStats", map[string]string{
			"siteUrl": bingSiteURL, "startDate": sd, "endDate": ed,
		})
		if err != nil {
			return
		}
		var resp struct {
			D struct {
				Results []struct {
					URL         string  `json:"Url"`
					Clicks      int     `json:"Clicks"`
					Impressions int     `json:"Impressions"`
					CTR         float64 `json:"CTR"`
					AvgImprPos  float64 `json:"AvgImpPos"`
					AvgClickPos float64 `json:"AvgClkPos"`
				} `json:"Results"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &resp) != nil {
			return
		}
		for _, r := range resp.D.Results {
			db.DB.Exec(`INSERT INTO bing_page_daily (site_id, date, page_url, clicks, impressions, ctr, avg_impression_position, avg_click_position)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (site_id,date,page_url) DO UPDATE SET clicks=EXCLUDED.clicks,impressions=EXCLUDED.impressions,ctr=EXCLUDED.ctr,avg_impression_position=EXCLUDED.avg_impression_position,avg_click_position=EXCLUDED.avg_click_position`,
				siteID, date, r.URL, r.Clicks, r.Impressions, r.CTR, r.AvgImprPos, r.AvgClickPos,
			)
		}
	}()

	// 3. URL submission quota
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := bingFetch(apiKey, "GetUrlSubmissionQuota", map[string]string{"siteUrl": bingSiteURL})
		if err != nil {
			return
		}
		var resp struct {
			D struct {
				DailyQuota   int `json:"DailyQuota"`
				MonthlyQuota int `json:"MonthlyQuota"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &resp) == nil {
			db.DB.Exec(`UPDATE bing_data SET url_submission_quota_daily=$1, url_submission_quota_monthly=$2 WHERE site_id=$3 AND date=$4`,
				resp.D.DailyQuota, resp.D.MonthlyQuota, siteID, date)
		}
	}()

	// 4. Sitemaps
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := bingFetch(apiKey, "GetFeeds", map[string]string{"siteUrl": bingSiteURL})
		if err != nil {
			return
		}
		var resp struct {
			D struct {
				Results []struct {
					URL       string `json:"Url"`
					Status    string `json:"Status"`
					UrlCount  int    `json:"UrlCount"`
					Submitted string `json:"LastSubmitted"`
				} `json:"Results"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &resp) != nil {
			return
		}
		for _, r := range resp.D.Results {
			db.DB.Exec(`INSERT INTO bing_sitemaps (site_id, sitemap_url, url_count, status)
				VALUES ($1,$2,$3,$4)
				ON CONFLICT (site_id,sitemap_url) DO UPDATE SET url_count=EXCLUDED.url_count,status=EXCLUDED.status`,
				siteID, r.URL, r.UrlCount, r.Status)
		}
	}()

	// 5. Inbound links count
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := bingFetch(apiKey, "GetLinkCounts", map[string]string{"siteUrl": bingSiteURL})
		if err != nil {
			return
		}
		var resp struct {
			D struct {
				InboundLinks int `json:"InboundLinks"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &resp) == nil {
			db.DB.Exec(`UPDATE bing_data SET inbound_links_count=$1 WHERE site_id=$2 AND date=$3`,
				resp.D.InboundLinks, siteID, date)
		}
	}()

	// 6. Blocked URLs count
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := bingFetch(apiKey, "GetBlockedUrls", map[string]string{"siteUrl": bingSiteURL})
		if err != nil {
			return
		}
		var resp struct {
			D struct {
				Results []json.RawMessage `json:"Results"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &resp) == nil {
			db.DB.Exec(`UPDATE bing_data SET blocked_urls_count=$1 WHERE site_id=$2 AND date=$3`,
				len(resp.D.Results), siteID, date)
		}
	}()

	// 7. Crawl settings
	wg.Add(1)
	go func() {
		defer wg.Done()
		bingFetch(apiKey, "GetCrawlSettings", map[string]string{"siteUrl": bingSiteURL})
		// Just fetch to populate cache / log, no dedicated table needed
	}()

	// 8. Query × Page cross-reference (TOP queries → pages)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Get top queries first, then fetch page mapping for each
		data, err := bingFetch(apiKey, "GetQueryStats", map[string]string{
			"siteUrl": bingSiteURL, "startDate": sd, "endDate": ed,
		})
		if err != nil {
			return
		}
		var qr struct {
			D struct {
				Results []struct {
					Query string `json:"Query"`
				} `json:"Results"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &qr) != nil {
			return
		}

		// For top 50 queries, fetch page mapping (rate-limited)
		limit := 50
		if len(qr.D.Results) < limit {
			limit = len(qr.D.Results)
		}
		for i := 0; i < limit; i++ {
			q := qr.D.Results[i].Query
			if q == "" {
				continue
			}
			pData, err := bingFetch(apiKey, "GetQueryPageStats", map[string]string{
				"siteUrl": bingSiteURL, "query": q,
			})
			if err != nil {
				continue
			}
			var pr struct {
				D struct {
					Pages []struct {
						URL         string  `json:"Url"`
						Clicks      int     `json:"Clicks"`
						Impressions int     `json:"Impressions"`
						AvgImprPos  float64 `json:"AvgImpPos"`
						AvgClickPos float64 `json:"AvgClkPos"`
					} `json:"Pages"`
				} `json:"d"`
			}
			if json.Unmarshal(pData, &pr) != nil {
				continue
			}
			for _, p := range pr.D.Pages {
				db.DB.Exec(`INSERT INTO bing_query_page_daily (site_id, date, query, page_url, clicks, impressions, avg_impression_position, avg_click_position)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
					ON CONFLICT (site_id,date,query,page_url) DO UPDATE SET clicks=EXCLUDED.clicks,impressions=EXCLUDED.impressions,avg_impression_position=EXCLUDED.avg_impression_position,avg_click_position=EXCLUDED.avg_click_position`,
					siteID, date, q, p.URL, p.Clicks, p.Impressions, p.AvgImprPos, p.AvgClickPos)
			}
			// Rate limit: 5 calls/sec
			time.Sleep(200 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Also enrich Bing queries
	enrichBingQueries(siteID)

	return nil
}

func enrichBingQueries(siteID int64) {
	var brandRegex string
	db.DB.QueryRow("SELECT COALESCE(brand_regex, '') FROM sites WHERE id = $1", siteID).Scan(&brandRegex)
	var compiled *regexp.Regexp
	if brandRegex != "" {
		compiled, _ = regexp.Compile("(?i)" + brandRegex)
	}

	rows, err := db.DB.Query(`
		SELECT DISTINCT q.query FROM bing_query_daily q
		LEFT JOIN query_enrichment e ON e.site_id=q.site_id AND e.query=q.query AND e.platform='bing'
		WHERE q.site_id=$1 AND e.query IS NULL LIMIT 5000`, siteID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var query string
		rows.Scan(&query)
		ql := strings.ToLower(query)

		brand := "unknown"
		if compiled != nil {
			if compiled.MatchString(ql) {
				brand = "brand"
			} else {
				brand = "non_brand"
			}
		}
		wordCount := len(strings.Fields(query))
		charCount := utf8.RuneCountInString(query)
		lengthCat := "body"
		if wordCount <= 1 && charCount <= 3 {
			lengthCat = "head"
		} else if wordCount >= 4 || charCount >= 9 {
			lengthCat = "long_tail"
		}

		db.DB.Exec(`INSERT INTO query_enrichment (site_id, query, platform, brand_category, length_cat, word_count, char_count, first_seen_date, last_seen_date)
			VALUES ($1,$2,'bing',$3,$4,$5,$6,CURRENT_DATE,CURRENT_DATE)
			ON CONFLICT (site_id,query,platform) DO UPDATE SET brand_category=EXCLUDED.brand_category,last_seen_date=CURRENT_DATE,updated_at=CURRENT_TIMESTAMP`,
			siteID, query, brand, lengthCat, wordCount, charCount)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Yandex Deep Sync
// ═══════════════════════════════════════════════════════════════════════

func SyncYandexDeep(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", sid).Scan(&domain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在"})
		return
	}
	token, err := getYandexAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	yandexUserID, err := getYandexUserIDCached(c.GetInt64("user_id"), token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	yandexHostID, err := resolveYandexHost(token, yandexUserID, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	if err := syncYandexDeepInternal(sid, token, yandexUserID, yandexHostID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"message": "Yandex 深度数据同步成功"}})
}

func syncYandexDeepInternal(siteID int64, token, yandexUserID, hostID string) error {
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	sd := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	ed := date
	encodedHost := url.PathEscape(hostID)

	var wg sync.WaitGroup

	// 1. Popular queries → yandex_query_daily (paginated)
	wg.Add(1)
	go func() {
		defer wg.Done()
		offset := 0
		limit := 500
		for {
			path := fmt.Sprintf("/user/%s/hosts/%s/search-queries/popular?date_from=%s&date_to=%s&order_by=TOTAL_CLICKS&limit=%d&offset=%d&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION",
				yandexUserID, encodedHost, sd, ed, limit, offset)
			data, err := yandexFetch(token, path)
			if err != nil {
				break
			}
			var resp struct {
				Count   int `json:"count"`
				Queries []struct {
					QueryID    string `json:"query_id"`
					QueryText  string `json:"query_text"`
					Indicators struct {
						TotalShows       int     `json:"TOTAL_SHOWS"`
						TotalClicks      int     `json:"TOTAL_CLICKS"`
						AvgShowPosition  float64 `json:"AVG_SHOW_POSITION"`
						AvgClickPosition float64 `json:"AVG_CLICK_POSITION"`
					} `json:"indicators"`
				} `json:"queries"`
			}
			if json.Unmarshal(data, &resp) != nil {
				break
			}
			for _, q := range resp.Queries {
				var ctr float64
				if q.Indicators.TotalShows > 0 {
					ctr = float64(q.Indicators.TotalClicks) / float64(q.Indicators.TotalShows)
				}
				db.DB.Exec(`INSERT INTO yandex_query_daily (site_id, date, query, query_id, total_shows, total_clicks, total_ctr, avg_show_position, avg_click_position)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
					ON CONFLICT (site_id,date,query,device_type) DO UPDATE SET query_id=EXCLUDED.query_id,total_shows=EXCLUDED.total_shows,total_clicks=EXCLUDED.total_clicks,total_ctr=EXCLUDED.total_ctr,avg_show_position=EXCLUDED.avg_show_position,avg_click_position=EXCLUDED.avg_click_position`,
					siteID, date, q.QueryText, q.QueryID,
					q.Indicators.TotalShows, q.Indicators.TotalClicks, ctr,
					q.Indicators.AvgShowPosition, q.Indicators.AvgClickPosition,
				)
			}
			if len(resp.Queries) < limit {
				break
			}
			offset += limit
			if offset > 5000 {
				break
			}
		}
	}()

	// 2. Summary (SQI + problems)
	wg.Add(1)
	go func() {
		defer wg.Done()
		path := fmt.Sprintf("/user/%s/hosts/%s/summary", yandexUserID, encodedHost)
		data, err := yandexFetch(token, path)
		if err != nil {
			return
		}
		var resp struct {
			SQI                  int             `json:"sqi"`
			SearchablePageCount  int             `json:"searchable_pages_count"`
			ExcludedPageCount    int             `json:"excluded_pages_count"`
			SiteProblems         json.RawMessage `json:"site_problems"`
		}
		if json.Unmarshal(data, &resp) == nil {
			db.DB.Exec(`UPDATE yandex_data SET sqi=$1, searchable_pages_count=$2, excluded_pages_count=$3, site_problems_json=$4 WHERE site_id=$5 AND date=$6`,
				resp.SQI, resp.SearchablePageCount, resp.ExcludedPageCount,
				string(resp.SiteProblems), siteID, date)
		}
	}()

	// 3. Indexing history → yandex_http_status_daily
	wg.Add(1)
	go func() {
		defer wg.Done()
		path := fmt.Sprintf("/user/%s/hosts/%s/indexing/history", yandexUserID, encodedHost)
		data, err := yandexFetch(token, path)
		if err != nil {
			return
		}
		var resp struct {
			Indicators map[string][]struct {
				Date  string `json:"date"`
				Value int    `json:"value"`
			} `json:"indicators"`
		}
		if json.Unmarshal(data, &resp) != nil {
			return
		}
		for statusCode, entries := range resp.Indicators {
			for _, e := range entries {
				db.DB.Exec(`INSERT INTO yandex_http_status_daily (site_id, date, status_code, count)
					VALUES ($1,$2,$3,$4)
					ON CONFLICT (site_id,date,status_code) DO UPDATE SET count=EXCLUDED.count`,
					siteID, e.Date, statusCode, e.Value)
			}
		}
	}()

	// 4. Index events → yandex_index_events_daily
	wg.Add(1)
	go func() {
		defer wg.Done()
		path := fmt.Sprintf("/user/%s/hosts/%s/search-urls/events/history", yandexUserID, encodedHost)
		data, err := yandexFetch(token, path)
		if err != nil {
			return
		}
		var resp struct {
			Indicators map[string][]struct {
				Date  string `json:"date"`
				Value int    `json:"value"`
			} `json:"indicators"`
		}
		if json.Unmarshal(data, &resp) != nil {
			return
		}
		for eventType, entries := range resp.Indicators {
			for _, e := range entries {
				db.DB.Exec(`INSERT INTO yandex_index_events_daily (site_id, date, event_type, count)
					VALUES ($1,$2,$3,$4)
					ON CONFLICT (site_id,date,event_type) DO UPDATE SET count=EXCLUDED.count`,
					siteID, e.Date, eventType, e.Value)
			}
		}
	}()

	// 5. External links
	wg.Add(1)
	go func() {
		defer wg.Done()
		path := fmt.Sprintf("/user/%s/hosts/%s/links/external/samples?offset=0&limit=500", yandexUserID, encodedHost)
		data, err := yandexFetch(token, path)
		if err != nil {
			return
		}
		var resp struct {
			Links []struct {
				SourceURL string `json:"source_url"`
				TargetURL string `json:"destination_url"`
			} `json:"links"`
		}
		if json.Unmarshal(data, &resp) != nil {
			return
		}
		for _, l := range resp.Links {
			db.DB.Exec(`INSERT INTO yandex_external_links (site_id, target_url, source_url, first_seen, last_seen)
				VALUES ($1,$2,$3,CURRENT_DATE,CURRENT_DATE)
				ON CONFLICT (site_id,target_url,source_url) DO UPDATE SET last_seen=CURRENT_DATE`,
				siteID, l.TargetURL, l.SourceURL)
		}
	}()

	// 6. Recrawl quota
	wg.Add(1)
	go func() {
		defer wg.Done()
		path := fmt.Sprintf("/user/%s/hosts/%s/recrawl/quota", yandexUserID, encodedHost)
		data, err := yandexFetch(token, path)
		if err != nil {
			return
		}
		var resp struct {
			DailyQuota int `json:"daily_quota"`
			QuotaUsed  int `json:"quota_remainder"`
		}
		if json.Unmarshal(data, &resp) == nil {
			db.DB.Exec(`UPDATE yandex_data SET recrawl_quota_daily=$1, recrawl_quota_used=$2 WHERE site_id=$3 AND date=$4`,
				resp.DailyQuota, resp.QuotaUsed, siteID, date)
		}
	}()

	// 7. Indexed URL samples (search-urls/in-search/samples)
	wg.Add(1)
	go func() {
		defer wg.Done()
		offset := 0
		limit := 500
		for {
			path := fmt.Sprintf("/user/%s/hosts/%s/search-urls/in-search/samples?offset=%d&limit=%d",
				yandexUserID, encodedHost, offset, limit)
			data, err := yandexFetch(token, path)
			if err != nil {
				break
			}
			var resp struct {
				Count int `json:"count"`
				URLs  []struct {
					URL        string `json:"url"`
					HTTPCode   int    `json:"http_code"`
					AccessDate string `json:"access_date"`
				} `json:"urls"`
			}
			if json.Unmarshal(data, &resp) != nil {
				break
			}
			for _, u := range resp.URLs {
				var accessTime *time.Time
				if u.AccessDate != "" {
					if t, err := time.Parse(time.RFC3339, u.AccessDate); err == nil {
						accessTime = &t
					}
				}
				db.DB.Exec(`INSERT INTO yandex_indexed_urls (site_id, url, status, http_code, access_date)
					VALUES ($1,$2,'in_search',$3,$4)
					ON CONFLICT (site_id,url) DO UPDATE SET status='in_search',http_code=EXCLUDED.http_code,access_date=EXCLUDED.access_date,fetched_at=CURRENT_TIMESTAMP`,
					siteID, u.URL, u.HTTPCode, accessTime)
			}
			if len(resp.URLs) < limit {
				break
			}
			offset += limit
			if offset > 5000 {
				break
			}
		}
	}()

	// 8. Popular queries with device type breakdown (DESKTOP + MOBILE_AND_TABLET)
	for _, deviceType := range []string{"DESKTOP", "MOBILE_AND_TABLET"} {
		wg.Add(1)
		go func(dt string) {
			defer wg.Done()
			offset := 0
			limit := 500
			for {
				path := fmt.Sprintf("/user/%s/hosts/%s/search-queries/popular?date_from=%s&date_to=%s&order_by=TOTAL_CLICKS&limit=%d&offset=%d&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION&device_type_indicator=%s",
					yandexUserID, encodedHost, sd, ed, limit, offset, dt)
				data, err := yandexFetch(token, path)
				if err != nil {
					break
				}
				var resp struct {
					Count   int `json:"count"`
					Queries []struct {
						QueryID   string `json:"query_id"`
						QueryText string `json:"query_text"`
						Indicators struct {
							TotalShows       int     `json:"TOTAL_SHOWS"`
							TotalClicks      int     `json:"TOTAL_CLICKS"`
							AvgShowPosition  float64 `json:"AVG_SHOW_POSITION"`
							AvgClickPosition float64 `json:"AVG_CLICK_POSITION"`
						} `json:"indicators"`
					} `json:"queries"`
				}
				if json.Unmarshal(data, &resp) != nil {
					break
				}
				for _, q := range resp.Queries {
					var ctr float64
					if q.Indicators.TotalShows > 0 {
						ctr = float64(q.Indicators.TotalClicks) / float64(q.Indicators.TotalShows)
					}
					db.DB.Exec(`INSERT INTO yandex_query_daily (site_id, date, query, query_id, device_type, total_shows, total_clicks, total_ctr, avg_show_position, avg_click_position)
						VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
						ON CONFLICT (site_id,date,query,device_type) DO UPDATE SET query_id=EXCLUDED.query_id,total_shows=EXCLUDED.total_shows,total_clicks=EXCLUDED.total_clicks,total_ctr=EXCLUDED.total_ctr,avg_show_position=EXCLUDED.avg_show_position,avg_click_position=EXCLUDED.avg_click_position`,
						siteID, date, q.QueryText, q.QueryID, dt,
						q.Indicators.TotalShows, q.Indicators.TotalClicks, ctr,
						q.Indicators.AvgShowPosition, q.Indicators.AvgClickPosition,
					)
				}
				if len(resp.Queries) < limit {
					break
				}
				offset += limit
				if offset > 2000 {
					break
				}
			}
		}(deviceType)
	}

	wg.Wait()

	// Enrich Yandex queries
	enrichYandexQueries(siteID)

	return nil
}

func enrichYandexQueries(siteID int64) {
	var brandRegex string
	db.DB.QueryRow("SELECT COALESCE(brand_regex, '') FROM sites WHERE id = $1", siteID).Scan(&brandRegex)
	var compiled *regexp.Regexp
	if brandRegex != "" {
		compiled, _ = regexp.Compile("(?i)" + brandRegex)
	}

	rows, err := db.DB.Query(`
		SELECT DISTINCT q.query FROM yandex_query_daily q
		LEFT JOIN query_enrichment e ON e.site_id=q.site_id AND e.query=q.query AND e.platform='yandex'
		WHERE q.site_id=$1 AND e.query IS NULL LIMIT 5000`, siteID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var query string
		rows.Scan(&query)
		ql := strings.ToLower(query)
		brand := "unknown"
		if compiled != nil {
			if compiled.MatchString(ql) {
				brand = "brand"
			} else {
				brand = "non_brand"
			}
		}
		wordCount := len(strings.Fields(query))
		charCount := utf8.RuneCountInString(query)
		lengthCat := "body"
		if wordCount <= 1 && charCount <= 3 {
			lengthCat = "head"
		} else if wordCount >= 4 || charCount >= 9 {
			lengthCat = "long_tail"
		}
		db.DB.Exec(`INSERT INTO query_enrichment (site_id, query, platform, brand_category, length_cat, word_count, char_count, first_seen_date, last_seen_date)
			VALUES ($1,$2,'yandex',$3,$4,$5,$6,CURRENT_DATE,CURRENT_DATE)
			ON CONFLICT (site_id,query,platform) DO UPDATE SET brand_category=EXCLUDED.brand_category,last_seen_date=CURRENT_DATE,updated_at=CURRENT_TIMESTAMP`,
			siteID, query, brand, lengthCat, wordCount, charCount)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// GSC URL Inspection (batch top pages)
// ═══════════════════════════════════════════════════════════════════════

func SyncGSCURLInspection(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)

	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", sid).Scan(&domain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在"})
		return
	}
	token, err := getGoogleAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	gscSiteURL, err := resolveGSCProperty(token, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	inspected, err := syncGSCURLInspectionInternal(sid, token, gscSiteURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"message":   "URL 检查完成",
		"inspected": inspected,
	}})
}

func syncGSCURLInspectionInternal(siteID int64, token, gscSiteURL string) (int, error) {
	// Get top 100 pages by clicks
	rows, err := db.DB.Query(`
		SELECT DISTINCT page_url FROM gsc_query_page_daily
		WHERE site_id = $1 AND date >= CURRENT_DATE - INTERVAL '7 days'
		ORDER BY page_url LIMIT 100`, siteID)
	if err != nil {
		return 0, fmt.Errorf("查询页面失败: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		urls = append(urls, u)
	}

	inspected := 0
	for _, pageURL := range urls {
		body := map[string]interface{}{
			"inspectionUrl": pageURL,
			"siteUrl":       gscSiteURL,
		}
		data, err := gscFetch(token, "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect", "POST", body)
		if err != nil {
			log.Printf("[gsc-inspect] %s: %v", pageURL, err)
			continue
		}

		var resp struct {
			InspectionResult struct {
				IndexStatusResult struct {
					Verdict       string   `json:"verdict"`
					CoverageState string   `json:"coverageState"`
					RobotsTxtState string  `json:"robotsTxtState"`
					IndexingState string   `json:"indexingState"`
					PageFetchState string  `json:"pageFetchState"`
					LastCrawlTime string   `json:"lastCrawlTime"`
					GoogleCanonical string `json:"googleCanonical"`
					UserCanonical string   `json:"userCanonical"`
					CrawledAs     string   `json:"crawledAs"`
					Sitemap       []string `json:"sitemap"`
					ReferringUrls []string `json:"referringUrls"`
				} `json:"indexStatusResult"`
				MobileUsabilityResult struct {
					Verdict string `json:"verdict"`
				} `json:"mobileUsabilityResult"`
				RichResultsResult json.RawMessage `json:"richResultsResult"`
			} `json:"inspectionResult"`
		}
		if json.Unmarshal(data, &resp) != nil {
			continue
		}

		idx := resp.InspectionResult.IndexStatusResult
		mobileUsable := resp.InspectionResult.MobileUsabilityResult.Verdict == "PASS"
		sitemapJSON, _ := json.Marshal(idx.Sitemap)
		referJSON, _ := json.Marshal(idx.ReferringUrls)
		richJSON := "[]"
		if resp.InspectionResult.RichResultsResult != nil {
			richJSON = string(resp.InspectionResult.RichResultsResult)
		}

		var lastCrawl *time.Time
		if idx.LastCrawlTime != "" {
			if t, err := time.Parse(time.RFC3339, idx.LastCrawlTime); err == nil {
				lastCrawl = &t
			}
		}

		db.DB.Exec(`INSERT INTO gsc_url_inspection (site_id, url, verdict, coverage_state, robots_txt_state, indexing_state, page_fetch_state, last_crawl_time, google_canonical, user_canonical, crawled_as, mobile_usable, rich_results_json, referring_urls_json, sitemap_urls_json)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT DO NOTHING`,
			siteID, pageURL, idx.Verdict, idx.CoverageState, idx.RobotsTxtState,
			idx.IndexingState, idx.PageFetchState, lastCrawl,
			idx.GoogleCanonical, idx.UserCanonical, idx.CrawledAs,
			mobileUsable, richJSON, string(referJSON), string(sitemapJSON),
		)
		inspected++

		// Rate limit: 600 QPM ≈ 10 QPS, be conservative
		time.Sleep(200 * time.Millisecond)
	}

	return inspected, nil
}
