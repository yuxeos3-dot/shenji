package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// resolveSiteIDByDomain finds the site_id matching a platform domain/URL.
// Exported for use by other handlers.
func resolveSiteIDByDomain(domain string, userID int64, isAdmin bool) int64 {
	clean := strings.TrimPrefix(domain, "sc-domain:")
	clean = strings.TrimPrefix(clean, "https://")
	clean = strings.TrimPrefix(clean, "http://")
	clean = strings.TrimPrefix(clean, "www.")
	clean = strings.TrimRight(clean, "/")
	clean = strings.ToLower(clean)
	if idx := strings.Index(clean, ":"); idx > 0 {
		clean = clean[:idx]
	}
	if clean == "" {
		return 0
	}

	var siteID int64
	likePattern := "%" + clean + "%"

	if isAdmin {
		db.DB.QueryRow(`SELECT id FROM sites WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(domain,'https://',''),'http://',''),'www.',''),'/','')) = $1 LIMIT 1`, clean).Scan(&siteID)
		if siteID == 0 {
			db.DB.QueryRow(`SELECT id FROM sites WHERE LOWER(domain) LIKE $1 LIMIT 1`, likePattern).Scan(&siteID)
		}
	} else {
		db.DB.QueryRow(`SELECT id FROM sites WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(domain,'https://',''),'http://',''),'www.',''),'/','')) = $1 AND created_by = $2 LIMIT 1`, clean, userID).Scan(&siteID)
		if siteID == 0 {
			db.DB.QueryRow(`SELECT id FROM sites WHERE LOWER(domain) LIKE $1 AND created_by = $2 LIMIT 1`, likePattern, userID).Scan(&siteID)
		}
	}
	return siteID
}

// ResolveSiteID is the HTTP endpoint for resolving domain → site_id.
func ResolveSiteID(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "缺少 domain 参数"})
		return
	}
	role, _ := c.Get("role")
	siteID := resolveSiteIDByDomain(domain, c.GetInt64("user_id"), role == "admin")
	if siteID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "未找到匹配的站点"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"site_id": siteID}})
}

// ═══════════════════════════════════════════════════════════════════════
// GSC Deep Query Endpoints
// ═══════════════════════════════════════════════════════════════════════

// GetGSCRankBuckets returns rank bucket distribution for a site
func GetGSCRankBuckets(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 {
		days = 30
	}

	rows, err := db.DB.Query(`
		SELECT date, bucket, query_count, page_count, total_clicks, total_impressions, avg_ctr, avg_position, top_queries_json, top_pages_json
		FROM gsc_rank_buckets_daily
		WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day'
		ORDER BY date DESC, bucket`, siteID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var date, bucket string
		var qc, pc, tc, ti int
		var ctr, pos float64
		var tqJSON, tpJSON string
		rows.Scan(&date, &bucket, &qc, &pc, &tc, &ti, &ctr, &pos, &tqJSON, &tpJSON)
		result = append(result, map[string]interface{}{
			"date": date, "bucket": bucket,
			"query_count": qc, "page_count": pc,
			"total_clicks": tc, "total_impressions": ti,
			"avg_ctr": ctr, "avg_position": pos,
			"top_queries": json.RawMessage(tqJSON),
			"top_pages":   json.RawMessage(tpJSON),
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGSCKeywords returns paginated keyword data with enrichment
func GetGSCKeywords(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 250 {
		pageSize = 25
	}
	offset := (page - 1) * pageSize
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	sortBy := c.DefaultQuery("sort", "clicks")
	sortDir := c.DefaultQuery("dir", "desc")
	search := c.Query("search")
	bucket := c.Query("bucket")
	intent := c.Query("intent")
	brand := c.Query("brand")
	lengthCat := c.Query("length")
	isQuestion := c.Query("is_question")

	// Validate sort column
	validSorts := map[string]string{
		"clicks": "clicks", "impressions": "impressions",
		"ctr": "ctr", "position": "position", "query": "query",
	}
	sortCol, ok := validSorts[sortBy]
	if !ok {
		sortCol = "clicks"
	}
	if sortDir != "asc" {
		sortDir = "desc"
	}

	// Build WHERE clause
	where := "WHERE q.site_id = $1 AND q.date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND q.search_type = 'web'"
	args := []interface{}{siteID, days}
	argIdx := 3

	if search != "" {
		where += " AND q.query ILIKE $" + strconv.Itoa(argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if bucket != "" {
		switch bucket {
		case "1-3":
			where += " AND q.position >= 0 AND q.position < 3"
		case "4-10":
			where += " AND q.position >= 3 AND q.position < 10"
		case "11-20":
			where += " AND q.position >= 10 AND q.position < 20"
		case "21-50":
			where += " AND q.position >= 20 AND q.position < 50"
		case "51-100":
			where += " AND q.position >= 50 AND q.position < 100"
		case "100+":
			where += " AND q.position >= 100"
		}
	}
	if intent != "" {
		where += " AND e.intent = $" + strconv.Itoa(argIdx)
		args = append(args, intent)
		argIdx++
	}
	if brand != "" {
		where += " AND e.brand_category = $" + strconv.Itoa(argIdx)
		args = append(args, brand)
		argIdx++
	}
	if lengthCat != "" {
		where += " AND e.length_cat = $" + strconv.Itoa(argIdx)
		args = append(args, lengthCat)
		argIdx++
	}
	if isQuestion == "true" {
		where += " AND e.is_question = TRUE"
	}

	// Count total
	var total int
	countSQL := `SELECT COUNT(*) FROM (
		SELECT q.query FROM gsc_query_page_daily q
		LEFT JOIN query_enrichment e ON e.site_id = q.site_id AND e.query = q.query AND e.platform = 'gsc'
		` + where + ` GROUP BY q.query, q.page_url
	) sub`
	db.DB.QueryRow(countSQL, args...).Scan(&total)

	// Get paginated data
	dataSQL := `SELECT q.query, q.page_url,
		SUM(q.clicks) as clicks, SUM(q.impressions) as impressions,
		CASE WHEN SUM(q.impressions)>0 THEN SUM(q.clicks)::float/SUM(q.impressions) ELSE 0 END as ctr,
		CASE WHEN SUM(q.impressions)>0 THEN SUM(q.position*q.impressions)/SUM(q.impressions) ELSE 0 END as position,
		COALESCE(e.brand_category,'unknown') as brand_category,
		COALESCE(e.intent,'unknown') as intent,
		COALESCE(e.length_cat,'body') as length_cat,
		COALESCE(e.is_question, FALSE) as is_question,
		COALESCE(e.is_comparison, FALSE) as is_comparison
	FROM gsc_query_page_daily q
	LEFT JOIN query_enrichment e ON e.site_id = q.site_id AND e.query = q.query AND e.platform = 'gsc'
	` + where + `
	GROUP BY q.query, q.page_url, e.brand_category, e.intent, e.length_cat, e.is_question, e.is_comparison
	ORDER BY ` + sortCol + ` ` + sortDir + `
	LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)

	args = append(args, pageSize, offset)
	rows, err := db.DB.Query(dataSQL, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var query, pageURL, brandCat, intentCat, lenCat string
		var clicks, impressions int
		var ctr, position float64
		var isQ, isComp bool
		rows.Scan(&query, &pageURL, &clicks, &impressions, &ctr, &position,
			&brandCat, &intentCat, &lenCat, &isQ, &isComp)
		result = append(result, map[string]interface{}{
			"query": query, "page_url": pageURL,
			"clicks": clicks, "impressions": impressions,
			"ctr": ctr, "position": position,
			"brand": brandCat, "intent": intentCat,
			"length": lenCat, "is_question": isQ, "is_comparison": isComp,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"items":     result,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetGSCKeywordDetail returns detailed data for a single keyword
func GetGSCKeywordDetail(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "缺少关键词参数 q"})
		return
	}

	// Get enrichment info
	var brand, intent, lengthCat sql.NullString
	var isQuestion, isComparison sql.NullBool
	var wordCount, charCount sql.NullInt64
	db.DB.QueryRow(`SELECT brand_category, intent, length_cat, is_question, is_comparison, word_count, char_count
		FROM query_enrichment WHERE site_id = $1 AND query = $2 AND platform = 'gsc'`,
		siteID, query).Scan(&brand, &intent, &lengthCat, &isQuestion, &isComparison, &wordCount, &charCount)

	// Get daily trend (last 90 days)
	trendRows, _ := db.DB.Query(`
		SELECT date, SUM(clicks) as clicks, SUM(impressions) as imp,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as pos
		FROM gsc_query_page_daily
		WHERE site_id = $1 AND query = $2 AND search_type = 'web'
			AND date >= CURRENT_DATE - INTERVAL '90 days'
		GROUP BY date ORDER BY date`, siteID, query)
	var trend []map[string]interface{}
	if trendRows != nil {
		defer trendRows.Close()
		for trendRows.Next() {
			var date string
			var clicks, imp int
			var ctr, pos float64
			trendRows.Scan(&date, &clicks, &imp, &ctr, &pos)
			trend = append(trend, map[string]interface{}{
				"date": date, "clicks": clicks, "impressions": imp, "ctr": ctr, "position": pos,
			})
		}
	}

	// Get corresponding pages
	pageRows, _ := db.DB.Query(`
		SELECT page_url, SUM(clicks) as clicks, SUM(impressions) as imp,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as pos
		FROM gsc_query_page_daily
		WHERE site_id = $1 AND query = $2 AND search_type = 'web'
			AND date >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY page_url ORDER BY clicks DESC LIMIT 20`, siteID, query)
	var pages []map[string]interface{}
	if pageRows != nil {
		defer pageRows.Close()
		for pageRows.Next() {
			var u string
			var clicks, imp int
			var pos float64
			pageRows.Scan(&u, &clicks, &imp, &pos)
			pages = append(pages, map[string]interface{}{
				"page_url": u, "clicks": clicks, "impressions": imp, "position": pos,
			})
		}
	}

	// Device breakdown for this query
	deviceRows, _ := db.DB.Query(`
		SELECT device, SUM(clicks), SUM(impressions),
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END
		FROM gsc_query_page_daily
		WHERE site_id = $1 AND query = $2 AND search_type = 'web'
			AND date >= CURRENT_DATE - INTERVAL '30 days' AND device != 'all'
		GROUP BY device ORDER BY SUM(clicks) DESC`, siteID, query)
	var devices []map[string]interface{}
	if deviceRows != nil {
		defer deviceRows.Close()
		for deviceRows.Next() {
			var dev string
			var c2, i2 int
			var ctr2, pos2 float64
			deviceRows.Scan(&dev, &c2, &i2, &ctr2, &pos2)
			devices = append(devices, map[string]interface{}{"device": dev, "clicks": c2, "impressions": i2, "ctr": ctr2, "position": pos2})
		}
	}

	// Country breakdown for this query
	countryRows, _ := db.DB.Query(`
		SELECT country, SUM(clicks), SUM(impressions),
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END
		FROM gsc_query_page_daily
		WHERE site_id = $1 AND query = $2 AND search_type = 'web'
			AND date >= CURRENT_DATE - INTERVAL '30 days' AND country != 'all'
		GROUP BY country ORDER BY SUM(clicks) DESC LIMIT 10`, siteID, query)
	var countries []map[string]interface{}
	if countryRows != nil {
		defer countryRows.Close()
		for countryRows.Next() {
			var ctry string
			var c3, i3 int
			var pos3 float64
			countryRows.Scan(&ctry, &c3, &i3, &pos3)
			countries = append(countries, map[string]interface{}{"country": ctry, "clicks": c3, "impressions": i3, "position": pos3})
		}
	}

	// Other search types for this query (image/video/news)
	searchTypeRows, _ := db.DB.Query(`
		SELECT search_type, SUM(clicks), SUM(impressions)
		FROM gsc_query_page_daily
		WHERE site_id = $1 AND query = $2 AND search_type != 'web'
			AND date >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY search_type ORDER BY SUM(clicks) DESC`, siteID, query)
	var searchTypes []map[string]interface{}
	if searchTypeRows != nil {
		defer searchTypeRows.Close()
		for searchTypeRows.Next() {
			var st string
			var c4, i4 int
			searchTypeRows.Scan(&st, &c4, &i4)
			searchTypes = append(searchTypes, map[string]interface{}{"search_type": st, "clicks": c4, "impressions": i4})
		}
	}

	// Cannibalization check: does this query have multiple ranking pages?
	var cannibalCount int
	db.DB.QueryRow(`
		SELECT COUNT(DISTINCT page_url) FROM gsc_query_page_daily
		WHERE site_id = $1 AND query = $2 AND search_type = 'web'
			AND date >= CURRENT_DATE - INTERVAL '30 days' AND position < 20`,
		siteID, query).Scan(&cannibalCount)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"query":              query,
		"brand":              brand.String,
		"intent":             intent.String,
		"length":             lengthCat.String,
		"is_question":        isQuestion.Bool,
		"is_comparison":      isComparison.Bool,
		"word_count":         wordCount.Int64,
		"char_count":         charCount.Int64,
		"trend":              trend,
		"pages":              pages,
		"devices":            devices,
		"countries":          countries,
		"other_search_types": searchTypes,
		"cannibalization":    cannibalCount >= 2,
		"cannibal_page_count": cannibalCount,
	}})
}

// GetGSCPages returns paginated page-level data
func GetGSCPages(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if page < 1 {
		page = 1
	}
	if pageSize > 250 {
		pageSize = 25
	}
	offset := (page - 1) * pageSize
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	search := c.Query("search")

	where := "WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND search_type = 'web'"
	args := []interface{}{siteID, days}
	argIdx := 3
	if search != "" {
		where += " AND page_url ILIKE $" + strconv.Itoa(argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(DISTINCT page_url) FROM gsc_query_page_daily "+where, args...).Scan(&total)

	dataSQL := `SELECT page_url,
		SUM(clicks) as clicks, SUM(impressions) as imp,
		CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
		CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as pos,
		COUNT(DISTINCT query) as query_count
	FROM gsc_query_page_daily ` + where + `
	GROUP BY page_url ORDER BY clicks DESC
	LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.DB.Query(dataSQL, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var pageURL string
		var clicks, imp, qc int
		var ctr, pos float64
		rows.Scan(&pageURL, &clicks, &imp, &ctr, &pos, &qc)
		result = append(result, map[string]interface{}{
			"page_url": pageURL, "clicks": clicks, "impressions": imp,
			"ctr": ctr, "position": pos, "query_count": qc,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"items": result, "total": total, "page": page, "page_size": pageSize,
	}})
}

// GetGSCAppearance returns SERP feature distribution
func GetGSCAppearance(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	rows, err := db.DB.Query(`
		SELECT appearance, SUM(clicks) as clicks, SUM(impressions) as imp,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as pos,
			COUNT(*) as days_count
		FROM gsc_appearance_daily
		WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day'
		GROUP BY appearance ORDER BY imp DESC`, siteID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var appearance string
		var clicks, imp, dc int
		var ctr, pos float64
		rows.Scan(&appearance, &clicks, &imp, &ctr, &pos, &dc)
		result = append(result, map[string]interface{}{
			"appearance": appearance, "clicks": clicks, "impressions": imp,
			"ctr": ctr, "position": pos, "days_count": dc,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGSCUrlInspection returns URL inspection data
func GetGSCUrlInspection(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize
	coverageFilter := c.Query("coverage_state")

	where := "WHERE site_id = $1"
	args := []interface{}{siteID}
	argIdx := 2
	if coverageFilter != "" {
		where += " AND coverage_state = $" + strconv.Itoa(argIdx)
		args = append(args, coverageFilter)
		argIdx++
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM gsc_url_inspection "+where, args...).Scan(&total)

	args = append(args, pageSize, offset)
	rows, err := db.DB.Query(`
		SELECT url, verdict, coverage_state, robots_txt_state, indexing_state, page_fetch_state,
			last_crawl_time, google_canonical, user_canonical, crawled_as, mobile_usable, inspected_at
		FROM gsc_url_inspection `+where+`
		ORDER BY inspected_at DESC
		LIMIT $`+strconv.Itoa(argIdx)+` OFFSET $`+strconv.Itoa(argIdx+1), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var u, verdict, coverage, robots, indexing, fetch, gCanon, uCanon, crawledAs string
		var mobileUsable bool
		var lastCrawl, inspectedAt sql.NullTime
		rows.Scan(&u, &verdict, &coverage, &robots, &indexing, &fetch,
			&lastCrawl, &gCanon, &uCanon, &crawledAs, &mobileUsable, &inspectedAt)
		item := map[string]interface{}{
			"url": u, "verdict": verdict, "coverage_state": coverage,
			"robots_txt_state": robots, "indexing_state": indexing,
			"page_fetch_state": fetch, "google_canonical": gCanon,
			"user_canonical": uCanon, "crawled_as": crawledAs,
			"mobile_usable": mobileUsable,
		}
		if lastCrawl.Valid {
			item["last_crawl_time"] = lastCrawl.Time.Format("2006-01-02T15:04:05Z")
		}
		if inspectedAt.Valid {
			item["inspected_at"] = inspectedAt.Time.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, item)
	}

	// Also get coverage state summary
	summaryRows, _ := db.DB.Query(`
		SELECT coverage_state, COUNT(*) FROM gsc_url_inspection
		WHERE site_id = $1 GROUP BY coverage_state ORDER BY COUNT(*) DESC`, siteID)
	var summary []map[string]interface{}
	if summaryRows != nil {
		defer summaryRows.Close()
		for summaryRows.Next() {
			var state string
			var cnt int
			summaryRows.Scan(&state, &cnt)
			summary = append(summary, map[string]interface{}{"state": state, "count": cnt})
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"items": result, "total": total, "page": page, "page_size": pageSize,
		"summary": summary,
	}})
}

// GetGSCSitemapsData returns sitemap status data
func GetGSCSitemapsData(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	rows, err := db.DB.Query(`
		SELECT DISTINCT ON (sitemap_path) sitemap_path, is_sitemaps_index, is_pending, type,
			last_submitted, last_downloaded, errors, warnings, contents_json, snapshot_date
		FROM gsc_sitemaps WHERE site_id = $1
		ORDER BY sitemap_path, snapshot_date DESC`, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var path, smType, contentsJSON, snapshotDate string
		var isIndex, isPending bool
		var lastSub, lastDown sql.NullTime
		var errors, warnings int
		rows.Scan(&path, &isIndex, &isPending, &smType, &lastSub, &lastDown,
			&errors, &warnings, &contentsJSON, &snapshotDate)
		item := map[string]interface{}{
			"path": path, "is_sitemaps_index": isIndex, "is_pending": isPending,
			"type": smType, "errors": errors, "warnings": warnings,
			"contents": json.RawMessage(contentsJSON), "snapshot_date": snapshotDate,
		}
		if lastSub.Valid {
			item["last_submitted"] = lastSub.Time.Format("2006-01-02T15:04:05Z")
		}
		if lastDown.Valid {
			item["last_downloaded"] = lastDown.Time.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, item)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGSCOpportunities returns keyword opportunities
func GetGSCOpportunities(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	offset := (page - 1) * pageSize
	oppType := c.Query("type")

	where := "WHERE site_id = $1"
	args := []interface{}{siteID}
	argIdx := 2
	if oppType != "" {
		where += " AND opportunity_type = $" + strconv.Itoa(argIdx)
		args = append(args, oppType)
		argIdx++
	}

	args = append(args, pageSize, offset)
	rows, err := db.DB.Query(`
		SELECT query, page_url, impressions, clicks, actual_ctr, baseline_ctr,
			ctr_gap_ratio, position, opportunity_score, estimated_extra_clicks, opportunity_type, week_start
		FROM gsc_opportunities_weekly `+where+`
		ORDER BY opportunity_score DESC
		LIMIT $`+strconv.Itoa(argIdx)+` OFFSET $`+strconv.Itoa(argIdx+1), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var query, pageURL, oppT, ws string
		var imp, clicks, extraClicks int
		var aCTR, bCTR, gapR, pos, score float64
		rows.Scan(&query, &pageURL, &imp, &clicks, &aCTR, &bCTR, &gapR, &pos, &score, &extraClicks, &oppT, &ws)
		result = append(result, map[string]interface{}{
			"query": query, "page_url": pageURL, "impressions": imp, "clicks": clicks,
			"actual_ctr": aCTR, "baseline_ctr": bCTR, "ctr_gap_ratio": gapR,
			"position": pos, "opportunity_score": score,
			"estimated_extra_clicks": extraClicks, "type": oppT, "week_start": ws,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGSCCannibalization returns keyword cannibalization data
func GetGSCCannibalization(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	rows, err := db.DB.Query(`
		SELECT query, page_count, competing_pages, severity, total_impressions, total_clicks, month
		FROM gsc_cannibalization_monthly
		WHERE site_id = $1 ORDER BY total_impressions DESC LIMIT 200`, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var query, pagesJSON, severity, month string
		var pc, ti, tc int
		rows.Scan(&query, &pc, &pagesJSON, &severity, &ti, &tc, &month)
		result = append(result, map[string]interface{}{
			"query": query, "page_count": pc, "competing_pages": json.RawMessage(pagesJSON),
			"severity": severity, "total_impressions": ti, "total_clicks": tc, "month": month,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGSCQueryDiff returns lost/new/risen/fallen keywords
func GetGSCQueryDiff(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	diffType := c.DefaultQuery("type", "lost")
	rows, err := db.DB.Query(`
		SELECT query, diff_type, prev_clicks, curr_clicks, click_delta,
			prev_position, curr_position, position_delta, week_start
		FROM gsc_query_diff_weekly
		WHERE site_id = $1 AND diff_type = $2
		ORDER BY ABS(click_delta) DESC LIMIT 200`, siteID, diffType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var query, dt, ws string
		var pc, cc, cd int
		var pp, cp, pd float64
		rows.Scan(&query, &dt, &pc, &cc, &cd, &pp, &cp, &pd, &ws)
		result = append(result, map[string]interface{}{
			"query": query, "diff_type": dt, "prev_clicks": pc, "curr_clicks": cc,
			"click_delta": cd, "prev_position": pp, "curr_position": cp,
			"position_delta": pd, "week_start": ws,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGSCOverviewDeep returns a high-level summary from deep data
// GetGSCOverviewDeep is the UNIFIED overview endpoint for GSC.
// It queries the deep tables (gsc_query_page_daily) first. If those are empty,
// it falls back to gsc_data (basic sync). This is the single source of truth
// for the detail view — no separate "basic" call is needed.
func GetGSCOverviewDeep(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	var deepCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM gsc_query_page_daily WHERE site_id = $1", siteID).Scan(&deepCount)
	hasDeepData := deepCount > 0

	var totalClicks, totalImpressions, totalQueries, totalPages int
	var avgCTR, avgPosition float64
	var trend, topKeywords, topPages []map[string]interface{}
	var devices, countries []map[string]interface{}

	src := "gsc_data" // which table we got data from

	if hasDeepData {
		src = "gsc_query_page_daily"
		db.DB.QueryRow(`
			SELECT COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
				CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
				CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END,
				COUNT(DISTINCT query), COUNT(DISTINCT page_url)
			FROM gsc_query_page_daily
			WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND search_type = 'web'`,
			siteID, days).Scan(&totalClicks, &totalImpressions, &avgCTR, &avgPosition, &totalQueries, &totalPages)

		// Daily trend
		scanMapRows(&trend, `SELECT date, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as position
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND search_type = 'web'
			GROUP BY date ORDER BY date`, siteID, days)

		// Top keywords
		scanMapRows(&topKeywords, `SELECT query, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as position
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND search_type = 'web'
			GROUP BY query ORDER BY SUM(clicks) DESC LIMIT 20`, siteID, days)

		// Top pages
		scanMapRows(&topPages, `SELECT page_url, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as position
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND search_type = 'web'
			GROUP BY page_url ORDER BY SUM(clicks) DESC LIMIT 20`, siteID, days)

		// Device breakdown
		scanMapRows(&devices, `SELECT device as label, SUM(clicks) as clicks, SUM(impressions) as impressions
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day'
			AND search_type = 'web' AND device != 'all'
			GROUP BY device ORDER BY SUM(clicks) DESC`, siteID, days)

		// Country breakdown
		scanMapRows(&countries, `SELECT country as label, SUM(clicks) as clicks, SUM(impressions) as impressions
			FROM gsc_query_page_daily WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day'
			AND search_type = 'web' AND country != 'all'
			GROUP BY country ORDER BY SUM(clicks) DESC LIMIT 10`, siteID, days)

	} else {
		// FALLBACK: gsc_data table
		rows, _ := db.DB.Query(`SELECT date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json
			FROM gsc_data WHERE site_id = $1 ORDER BY date DESC LIMIT $2`, siteID, days)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var date, kwJSON, pgJSON, devJSON, ctryJSON string
				var clicks, imp int
				var ctr, pos float64
				rows.Scan(&date, &clicks, &imp, &ctr, &pos, &kwJSON, &pgJSON, &devJSON, &ctryJSON)
				totalClicks += clicks
				totalImpressions += imp
				trend = append(trend, map[string]interface{}{"date": date, "clicks": clicks, "impressions": imp, "ctr": ctr, "position": pos})
				// Parse keyword/page/device/country JSON from first (latest) row
				if len(topKeywords) == 0 {
					parseGSCRows(&topKeywords, kwJSON, "query")
				}
				if len(topPages) == 0 {
					parseGSCRows(&topPages, pgJSON, "page_url")
				}
				if len(devices) == 0 {
					parseGSCDeviceCountry(&devices, devJSON)
				}
				if len(countries) == 0 {
					parseGSCDeviceCountry(&countries, ctryJSON)
				}
			}
		}
		if totalImpressions > 0 {
			avgCTR = float64(totalClicks) / float64(totalImpressions)
		}
		totalQueries = len(topKeywords)
		totalPages = len(topPages)
		for i, j := 0, len(trend)-1; i < j; i, j = i+1, j-1 {
			trend[i], trend[j] = trend[j], trend[i]
		}
		if len(trend) > 0 {
			if p, ok := trend[len(trend)-1]["position"].(float64); ok {
				avgPosition = p
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"total_clicks": totalClicks, "total_impressions": totalImpressions,
		"avg_ctr": avgCTR, "avg_position": avgPosition,
		"total_queries": totalQueries, "total_pages": totalPages,
		"trend": trend, "top_keywords": topKeywords, "top_pages": topPages,
		"devices": devices, "countries": countries,
		"has_deep_data": hasDeepData, "source": src,
	}})
}

// helper: scan SQL rows into a slice of maps (2-5 column results)
func scanMapRows(dest *[]map[string]interface{}, query string, args ...interface{}) {
	rows, err := db.DB.Query(query, args...)
	if err != nil || rows == nil {
		return
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		m := make(map[string]interface{})
		for i, col := range cols {
			switch v := vals[i].(type) {
			case []byte:
				m[col] = string(v)
			default:
				m[col] = v
			}
		}
		*dest = append(*dest, m)
	}
}

func parseGSCRows(dest *[]map[string]interface{}, jsonStr, keyName string) {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return
	}
	var rows []struct {
		Keys        []string `json:"keys"`
		Clicks      int      `json:"clicks"`
		Impressions int      `json:"impressions"`
		CTR         float64  `json:"ctr"`
		Position    float64  `json:"position"`
	}
	if json.Unmarshal([]byte(jsonStr), &rows) != nil {
		return
	}
	for _, r := range rows {
		k := ""
		if len(r.Keys) > 0 {
			k = r.Keys[0]
		}
		if k != "" {
			*dest = append(*dest, map[string]interface{}{keyName: k, "clicks": r.Clicks, "impressions": r.Impressions, "ctr": r.CTR, "position": r.Position})
		}
	}
}

func parseGSCDeviceCountry(dest *[]map[string]interface{}, jsonStr string) {
	if jsonStr == "" || jsonStr == "{}" || jsonStr == "[]" || jsonStr == "null" {
		return
	}
	// Try array format first [{keys:["MOBILE"], clicks:...}]
	var rows []struct {
		Keys   []string `json:"keys"`
		Clicks int      `json:"clicks"`
	}
	if json.Unmarshal([]byte(jsonStr), &rows) == nil && len(rows) > 0 {
		for _, r := range rows {
			if len(r.Keys) > 0 {
				*dest = append(*dest, map[string]interface{}{"label": r.Keys[0], "clicks": r.Clicks})
			}
		}
		return
	}
	// Try object format {MOBILE: 123, DESKTOP: 456}
	var obj map[string]interface{}
	if json.Unmarshal([]byte(jsonStr), &obj) == nil {
		for k, v := range obj {
			clicks := 0
			switch val := v.(type) {
			case float64:
				clicks = int(val)
			case int:
				clicks = val
			}
			*dest = append(*dest, map[string]interface{}{"label": k, "clicks": clicks})
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════
// GA4 Deep Query Endpoints
// ═══════════════════════════════════════════════════════════════════════

func GetGA4Extended(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, err := db.DB.Query(`SELECT date, total_users, active_users, new_users, sessions,
		engaged_sessions, engagement_rate, bounce_rate, average_session_duration,
		screen_page_views, event_count, conversions, total_revenue
		FROM ga_daily_extended WHERE site_id = $1 AND date >= CURRENT_DATE - $2 * INTERVAL '1 day'
		ORDER BY date DESC`, siteID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var date string
		var tu, au, nu, sess, es, spv, ec, conv int
		var er, br, asd, rev float64
		rows.Scan(&date, &tu, &au, &nu, &sess, &es, &er, &br, &asd, &spv, &ec, &conv, &rev)
		result = append(result, map[string]interface{}{
			"date": date, "total_users": tu, "active_users": au, "new_users": nu,
			"sessions": sess, "engaged_sessions": es, "engagement_rate": er,
			"bounce_rate": br, "average_session_duration": asd,
			"screen_page_views": spv, "event_count": ec, "conversions": conv, "total_revenue": rev,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetGA4LandingPages(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	offset := (page - 1) * pageSize

	var total int
	db.DB.QueryRow(`SELECT COUNT(DISTINCT landing_page) FROM ga_landing_pages_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'`, siteID, days).Scan(&total)

	rows, _ := db.DB.Query(`SELECT landing_page, SUM(sessions), SUM(users), SUM(new_users),
		SUM(engaged_sessions), AVG(engagement_rate), AVG(bounce_rate), AVG(average_session_duration),
		SUM(screen_page_views), SUM(conversions)
		FROM ga_landing_pages_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY landing_page ORDER BY SUM(sessions) DESC LIMIT $3 OFFSET $4`,
		siteID, days, pageSize, offset)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []interface{}{}, "total": 0}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var lp string
		var sess, users, nu, es, spv, conv int
		var er, br, asd float64
		rows.Scan(&lp, &sess, &users, &nu, &es, &er, &br, &asd, &spv, &conv)
		result = append(result, map[string]interface{}{
			"landing_page": lp, "sessions": sess, "users": users, "new_users": nu,
			"engaged_sessions": es, "engagement_rate": er, "bounce_rate": br,
			"average_session_duration": asd, "screen_page_views": spv, "conversions": conv,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": result, "total": total, "page": page, "page_size": pageSize}})
}

func GetGA4SourceMedium(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT source, medium, channel_group, SUM(sessions), SUM(users), SUM(new_users),
		SUM(engaged_sessions), AVG(engagement_rate), SUM(conversions)
		FROM ga_source_medium_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY source, medium, channel_group ORDER BY SUM(sessions) DESC LIMIT 100`,
		siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var src, med, cg string
		var sess, users, nu, es, conv int
		var er float64
		rows.Scan(&src, &med, &cg, &sess, &users, &nu, &es, &er, &conv)
		result = append(result, map[string]interface{}{
			"source": src, "medium": med, "channel_group": cg,
			"sessions": sess, "users": users, "new_users": nu,
			"engaged_sessions": es, "engagement_rate": er, "conversions": conv,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetGA4Events(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT event_name, SUM(event_count), SUM(users_with_event), SUM(event_value_sum), bool_or(is_conversion)
		FROM ga_events_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY event_name ORDER BY SUM(event_count) DESC LIMIT 100`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var name string
		var ec, ue int
		var ev float64
		var isConv bool
		rows.Scan(&name, &ec, &ue, &ev, &isConv)
		result = append(result, map[string]interface{}{
			"event_name": name, "event_count": ec, "users": ue,
			"event_value": ev, "is_conversion": isConv,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetGA4Tech(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	dimType := c.DefaultQuery("dimension", "device")
	rows, _ := db.DB.Query(`SELECT dimension_value, SUM(sessions), SUM(users), AVG(bounce_rate), AVG(engagement_rate)
		FROM ga_tech_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND dimension_type=$3
		GROUP BY dimension_value ORDER BY SUM(sessions) DESC LIMIT 50`,
		siteID, days, dimType)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var val string
		var sess, users int
		var br, er float64
		rows.Scan(&val, &sess, &users, &br, &er)
		result = append(result, map[string]interface{}{
			"value": val, "sessions": sess, "users": users,
			"bounce_rate": br, "engagement_rate": er,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetGA4Geo(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT country, region, city, SUM(sessions), SUM(users), SUM(new_users)
		FROM ga_geo_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY country, region, city ORDER BY SUM(sessions) DESC LIMIT 100`,
		siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var country, region, city string
		var sess, users, nu int
		rows.Scan(&country, &region, &city, &sess, &users, &nu)
		result = append(result, map[string]interface{}{
			"country": country, "region": region, "city": city,
			"sessions": sess, "users": users, "new_users": nu,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetGA4Hourly(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	rows, _ := db.DB.Query(`SELECT hour, SUM(sessions), SUM(users), SUM(screen_page_views)
		FROM ga_hourly WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY hour ORDER BY hour`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var hour, sess, users, spv int
		rows.Scan(&hour, &sess, &users, &spv)
		result = append(result, map[string]interface{}{
			"hour": hour, "sessions": sess, "users": users, "screen_page_views": spv,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGA4DayOfWeek returns session distribution by day of week
func GetGA4DayOfWeek(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	rows, _ := db.DB.Query(`SELECT day_of_week, SUM(sessions), SUM(users), SUM(screen_page_views), SUM(conversions)
		FROM ga_day_of_week WHERE site_id=$1
		GROUP BY day_of_week ORDER BY day_of_week`, siteID)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	dayNames := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	var result []map[string]interface{}
	for rows.Next() {
		var dow, sess, users, spv, conv int
		rows.Scan(&dow, &sess, &users, &spv, &conv)
		name := ""
		if dow >= 0 && dow < 7 {
			name = dayNames[dow]
		}
		result = append(result, map[string]interface{}{
			"day_of_week": dow, "name": name, "sessions": sess, "users": users,
			"screen_page_views": spv, "conversions": conv,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGA4ScrollDepth returns scroll depth data
func GetGA4ScrollDepth(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT page_path, percent_scrolled, SUM(users), SUM(event_count)
		FROM ga_scroll_depth_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY page_path, percent_scrolled ORDER BY SUM(event_count) DESC LIMIT 200`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var pp string
		var ps, users, ec int
		rows.Scan(&pp, &ps, &users, &ec)
		result = append(result, map[string]interface{}{"page_path": pp, "percent_scrolled": ps, "users": users, "event_count": ec})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGA4OutboundClicks returns outbound click data
func GetGA4OutboundClicks(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT link_domain, link_url, SUM(event_count), SUM(users)
		FROM ga_outbound_clicks_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY link_domain, link_url ORDER BY SUM(event_count) DESC LIMIT 200`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var ld, lu string
		var ec, users int
		rows.Scan(&ld, &lu, &ec, &users)
		result = append(result, map[string]interface{}{"link_domain": ld, "link_url": lu, "event_count": ec, "users": users})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetGA4ConversionsDaily returns conversion data by source/medium
func GetGA4ConversionsDaily(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT event_name, source, medium, SUM(conversions), SUM(total_revenue)
		FROM ga_conversions_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		GROUP BY event_name, source, medium ORDER BY SUM(conversions) DESC LIMIT 200`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var en, src, med string
		var conv int
		var rev float64
		rows.Scan(&en, &src, &med, &conv, &rev)
		result = append(result, map[string]interface{}{
			"event_name": en, "source": src, "medium": med, "conversions": conv, "total_revenue": rev,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetYandexIndexedUrls returns indexed URL samples
func GetYandexIndexedUrls(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	offset := (page - 1) * pageSize

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM yandex_indexed_urls WHERE site_id=$1", siteID).Scan(&total)

	rows, _ := db.DB.Query(`SELECT url, status, http_code, access_date, fetched_at
		FROM yandex_indexed_urls WHERE site_id=$1 ORDER BY fetched_at DESC LIMIT $2 OFFSET $3`,
		siteID, pageSize, offset)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []interface{}{}, "total": 0}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var u, status string
		var code int
		var accessDate, fetchedAt sql.NullTime
		rows.Scan(&u, &status, &code, &accessDate, &fetchedAt)
		item := map[string]interface{}{"url": u, "status": status, "http_code": code}
		if accessDate.Valid {
			item["access_date"] = accessDate.Time.Format("2006-01-02")
		}
		if fetchedAt.Valid {
			item["fetched_at"] = fetchedAt.Time.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, item)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": result, "total": total, "page": page, "page_size": pageSize}})
}

// GetGSCKeywordsExport returns keyword data as CSV
func GetGSCKeywordsExport(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	rows, err := db.DB.Query(`
		SELECT q.query, q.page_url,
			SUM(q.clicks) as clicks, SUM(q.impressions) as impressions,
			CASE WHEN SUM(q.impressions)>0 THEN SUM(q.clicks)::float/SUM(q.impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(q.impressions)>0 THEN SUM(q.position*q.impressions)/SUM(q.impressions) ELSE 0 END as position,
			COALESCE(e.brand_category,'') as brand, COALESCE(e.intent,'') as intent,
			COALESCE(e.is_question, FALSE) as is_question
		FROM gsc_query_page_daily q
		LEFT JOIN query_enrichment e ON e.site_id = q.site_id AND e.query = q.query AND e.platform = 'gsc'
		WHERE q.site_id = $1 AND q.date >= CURRENT_DATE - $2 * INTERVAL '1 day' AND q.search_type = 'web'
		GROUP BY q.query, q.page_url, e.brand_category, e.intent, e.is_question
		ORDER BY clicks DESC LIMIT 50000`, siteID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=gsc_keywords.csv")
	c.Writer.WriteString("\xEF\xBB\xBF") // UTF-8 BOM for Excel
	c.Writer.WriteString("关键词,落地页,点击,曝光,CTR,位置,品牌,意图,问题型\n")

	for rows.Next() {
		var query, pageURL, brand, intent string
		var clicks, impressions int
		var ctr, position float64
		var isQ bool
		rows.Scan(&query, &pageURL, &clicks, &impressions, &ctr, &position, &brand, &intent, &isQ)
		qFlag := ""
		if isQ {
			qFlag = "Y"
		}
		line := fmt.Sprintf("\"%s\",\"%s\",%d,%d,%.4f,%.1f,%s,%s,%s\n",
			escapeCSV(query), escapeCSV(pageURL), clicks, impressions, ctr, position, brand, intent, qFlag)
		c.Writer.WriteString(line)
	}
}

func escapeCSV(s string) string {
	return strings.ReplaceAll(s, "\"", "\"\"")
}

// ═══════════════════════════════════════════════════════════════════════
// Bing Deep Query Endpoints
// ═══════════════════════════════════════════════════════════════════════

func GetBingKeywords(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	offset := (page - 1) * pageSize
	search := c.Query("search")

	where := "WHERE site_id = $1"
	args := []interface{}{siteID}
	argIdx := 2
	if search != "" {
		where += " AND query ILIKE $" + strconv.Itoa(argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(DISTINCT query) FROM bing_query_daily "+where, args...).Scan(&total)

	args = append(args, pageSize, offset)
	rows, _ := db.DB.Query(`SELECT query, SUM(clicks), SUM(impressions),
		CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
		AVG(avg_impression_position), AVG(avg_click_position)
		FROM bing_query_daily `+where+`
		GROUP BY query ORDER BY SUM(clicks) DESC
		LIMIT $`+strconv.Itoa(argIdx)+` OFFSET $`+strconv.Itoa(argIdx+1), args...)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []interface{}{}, "total": 0}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var query string
		var clicks, imp int
		var ctr, aip, acp float64
		rows.Scan(&query, &clicks, &imp, &ctr, &aip, &acp)
		result = append(result, map[string]interface{}{
			"query": query, "clicks": clicks, "impressions": imp,
			"ctr": ctr, "avg_impression_position": aip, "avg_click_position": acp,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": result, "total": total, "page": page, "page_size": pageSize}})
}

func GetBingPages(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	offset := (page - 1) * pageSize

	var total int
	db.DB.QueryRow("SELECT COUNT(DISTINCT page_url) FROM bing_page_daily WHERE site_id=$1", siteID).Scan(&total)

	rows, _ := db.DB.Query(`SELECT page_url, SUM(clicks), SUM(impressions),
		CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
		AVG(avg_impression_position)
		FROM bing_page_daily WHERE site_id=$1
		GROUP BY page_url ORDER BY SUM(clicks) DESC LIMIT $2 OFFSET $3`,
		siteID, pageSize, offset)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []interface{}{}, "total": 0}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var u string
		var clicks, imp int
		var ctr, pos float64
		rows.Scan(&u, &clicks, &imp, &ctr, &pos)
		result = append(result, map[string]interface{}{
			"page_url": u, "clicks": clicks, "impressions": imp, "ctr": ctr, "position": pos,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": result, "total": total, "page": page, "page_size": pageSize}})
}

func GetBingInboundLinks(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	rows, _ := db.DB.Query(`SELECT target_page, source_url, anchor_text, last_seen
		FROM bing_inbound_links WHERE site_id=$1 ORDER BY last_seen DESC LIMIT 500`, siteID)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var tp, su, at string
		var ls sql.NullString
		rows.Scan(&tp, &su, &at, &ls)
		result = append(result, map[string]interface{}{
			"target_page": tp, "source_url": su, "anchor_text": at, "last_seen": ls.String,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetBingSitemaps(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	rows, _ := db.DB.Query(`SELECT sitemap_url, url_count, status FROM bing_sitemaps WHERE site_id=$1`, siteID)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var u, status string
		var cnt int
		rows.Scan(&u, &cnt, &status)
		result = append(result, map[string]interface{}{"sitemap_url": u, "url_count": cnt, "status": status})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// ═══════════════════════════════════════════════════════════════════════
// Yandex Deep Query Endpoints
// ═══════════════════════════════════════════════════════════════════════

func GetYandexKeywords(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	offset := (page - 1) * pageSize
	search := c.Query("search")

	where := "WHERE site_id = $1"
	args := []interface{}{siteID}
	argIdx := 2
	if search != "" {
		where += " AND query ILIKE $" + strconv.Itoa(argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(DISTINCT query) FROM yandex_query_daily "+where, args...).Scan(&total)

	args = append(args, pageSize, offset)
	rows, _ := db.DB.Query(`SELECT query, SUM(total_clicks), SUM(total_shows),
		CASE WHEN SUM(total_shows)>0 THEN SUM(total_clicks)::float/SUM(total_shows) ELSE 0 END,
		AVG(avg_show_position), AVG(avg_click_position)
		FROM yandex_query_daily `+where+`
		GROUP BY query ORDER BY SUM(total_clicks) DESC
		LIMIT $`+strconv.Itoa(argIdx)+` OFFSET $`+strconv.Itoa(argIdx+1), args...)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []interface{}{}, "total": 0}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var query string
		var clicks, shows int
		var ctr, asp, acp float64
		rows.Scan(&query, &clicks, &shows, &ctr, &asp, &acp)
		result = append(result, map[string]interface{}{
			"query": query, "clicks": clicks, "shows": shows,
			"ctr": ctr, "avg_show_position": asp, "avg_click_position": acp,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": result, "total": total, "page": page, "page_size": pageSize}})
}

func GetYandexIndexEvents(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT date, event_type, count
		FROM yandex_index_events_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		ORDER BY date DESC, event_type`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var date, et string
		var cnt int
		rows.Scan(&date, &et, &cnt)
		result = append(result, map[string]interface{}{"date": date, "event_type": et, "count": cnt})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetYandexHTTPStatus(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	rows, _ := db.DB.Query(`SELECT date, status_code, count
		FROM yandex_http_status_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
		ORDER BY date DESC, status_code`, siteID, days)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var date, sc string
		var cnt int
		rows.Scan(&date, &sc, &cnt)
		result = append(result, map[string]interface{}{"date": date, "status_code": sc, "count": cnt})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func GetYandexExternalLinks(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	rows, _ := db.DB.Query(`SELECT target_url, source_url, last_seen
		FROM yandex_external_links WHERE site_id=$1 ORDER BY last_seen DESC LIMIT 500`, siteID)
	if rows == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []interface{}{}})
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var tu, su string
		var ls sql.NullString
		rows.Scan(&tu, &su, &ls)
		result = append(result, map[string]interface{}{"target_url": tu, "source_url": su, "last_seen": ls.String})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}
