package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetUnifiedPlatformData is THE single endpoint for all platform data.
// GET /v1/sites/:id/platform-data?platform=gsc&days=30
//
// Returns everything the frontend needs in ONE response:
//   - overview KPIs (clicks/impressions/ctr/position OR sessions/users/etc)
//   - daily trend
//   - top keywords (with enrichment tags)
//   - top pages
//   - device breakdown
//   - country breakdown
//   - rank buckets (GSC only)
//   - SERP appearance (GSC only)
//   - has_deep_data flag
//
// Data source priority: deep tables first, gsc_data/ga_data/bing_data/yandex_data as fallback.
func GetUnifiedPlatformData(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	plat := c.DefaultQuery("platform", "gsc")
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 {
		days = 30
	}

	switch plat {
	case "gsc":
		unifiedGSC(c, sid, days)
	case "ga4":
		unifiedGA4(c, sid, days)
	case "bing":
		unifiedBing(c, sid, days)
	case "yandex":
		unifiedYandex(c, sid, days)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "未知平台: " + plat})
	}
}

// ═══════════════════════════════════════════════════════════════════
// GSC unified
// ═══════════════════════════════════════════════════════════════════
func unifiedGSC(c *gin.Context, siteID int64, days int) {
	var deepCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM gsc_query_page_daily WHERE site_id = $1", siteID).Scan(&deepCount)
	hasDeep := deepCount > 0

	var totalClicks, totalImpressions, totalQueries, totalPages int
	var avgCTR, avgPosition float64
	var trend, topKeywords, topPages, devices, countries, buckets, appearances []map[string]interface{}

	if hasDeep {
		// ── From deep tables ──
		db.DB.QueryRow(`SELECT COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END,
			COUNT(DISTINCT query), COUNT(DISTINCT page_url)
			FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND search_type='web'`,
			siteID, days).Scan(&totalClicks, &totalImpressions, &avgCTR, &avgPosition, &totalQueries, &totalPages)

		scanMapRows(&trend, `SELECT date::text, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as position
			FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND search_type='web'
			GROUP BY date ORDER BY date`, siteID, days)

		scanMapRows(&topKeywords, `SELECT query, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as position
			FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND search_type='web'
			GROUP BY query ORDER BY SUM(clicks) DESC LIMIT 20`, siteID, days)

		scanMapRows(&topPages, `SELECT page_url, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr,
			CASE WHEN SUM(impressions)>0 THEN SUM(position*impressions)/SUM(impressions) ELSE 0 END as position
			FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND search_type='web'
			GROUP BY page_url ORDER BY SUM(clicks) DESC LIMIT 20`, siteID, days)

		scanMapRows(&devices, `SELECT device as label, SUM(clicks) as clicks
			FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND search_type='web' AND device!='all'
			GROUP BY device ORDER BY SUM(clicks) DESC`, siteID, days)

		scanMapRows(&countries, `SELECT country as label, SUM(clicks) as clicks
			FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND search_type='web' AND country!='all'
			GROUP BY country ORDER BY SUM(clicks) DESC LIMIT 10`, siteID, days)

		// Rank buckets
		scanMapRows(&buckets, `SELECT bucket, query_count, page_count, total_clicks, total_impressions, avg_ctr, avg_position,
			top_queries_json, top_pages_json
			FROM gsc_rank_buckets_daily WHERE site_id=$1 AND date=(SELECT MAX(date) FROM gsc_rank_buckets_daily WHERE site_id=$1)
			ORDER BY CASE bucket WHEN '1-3' THEN 1 WHEN '4-10' THEN 2 WHEN '11-20' THEN 3 WHEN '21-50' THEN 4 WHEN '51-100' THEN 5 ELSE 6 END`,
			siteID)

		// SERP appearances
		scanMapRows(&appearances, `SELECT appearance, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr
			FROM gsc_appearance_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
			GROUP BY appearance ORDER BY SUM(impressions) DESC`, siteID, days)
	} else {
		// ── Fallback: gsc_data table ──
		rows, _ := db.DB.Query(`SELECT date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json
			FROM gsc_data WHERE site_id=$1 ORDER BY date DESC LIMIT $2`, siteID, days)
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
		"platform": "gsc", "has_deep_data": hasDeep,
		"total_clicks": totalClicks, "total_impressions": totalImpressions,
		"avg_ctr": avgCTR, "avg_position": avgPosition,
		"total_queries": totalQueries, "total_pages": totalPages,
		"trend": trend, "top_keywords": topKeywords, "top_pages": topPages,
		"devices": devices, "countries": countries,
		"buckets": buckets, "appearances": appearances,
	}})
}

// ═══════════════════════════════════════════════════════════════════
// GA4 unified
// ═══════════════════════════════════════════════════════════════════
func unifiedGA4(c *gin.Context, siteID int64, days int) {
	// Try deep table first
	var deepCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM ga_daily_extended WHERE site_id=$1", siteID).Scan(&deepCount)
	hasDeep := deepCount > 0

	var sessions, users, newUsers, pageviews int
	var bounceRate, engagementRate, avgDuration float64
	var trend, topPages, devices, countries []map[string]interface{}

	if hasDeep {
		db.DB.QueryRow(`SELECT COALESCE(SUM(sessions),0), COALESCE(SUM(total_users),0), COALESCE(SUM(new_users),0),
			COALESCE(SUM(screen_page_views),0), AVG(bounce_rate), AVG(engagement_rate), AVG(average_session_duration)
			FROM ga_daily_extended WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'`,
			siteID, days).Scan(&sessions, &users, &newUsers, &pageviews, &bounceRate, &engagementRate, &avgDuration)

		scanMapRows(&trend, `SELECT date::text, sessions as clicks, total_users as impressions, engagement_rate as ctr
			FROM ga_daily_extended WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' ORDER BY date`, siteID, days)

		scanMapRows(&topPages, `SELECT landing_page as page_url, SUM(sessions) as clicks, SUM(users) as impressions
			FROM ga_landing_pages_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
			GROUP BY landing_page ORDER BY SUM(sessions) DESC LIMIT 20`, siteID, days)
	} else {
		// Fallback: ga_data
		rows, _ := db.DB.Query(`SELECT date, sessions, users, pageviews, bounce_rate, avg_duration, channels_json, pages_json
			FROM ga_data WHERE site_id=$1 ORDER BY date DESC LIMIT $2`, siteID, days)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var date, chJSON, pgJSON string
				var s, u, pv int
				var br, ad float64
				rows.Scan(&date, &s, &u, &pv, &br, &ad, &chJSON, &pgJSON)
				sessions += s; users += u; pageviews += pv
				trend = append(trend, map[string]interface{}{"date": date, "clicks": s, "impressions": u})
				if len(topPages) == 0 {
					parseGSCRows(&topPages, pgJSON, "page_url")
				}
			}
		}
		bounceRate = 0; avgDuration = 0
		for i, j := 0, len(trend)-1; i < j; i, j = i+1, j-1 {
			trend[i], trend[j] = trend[j], trend[i]
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform": "ga4", "has_deep_data": hasDeep,
		"sessions": sessions, "users": users, "new_users": newUsers, "pageviews": pageviews,
		"bounce_rate": bounceRate, "engagement_rate": engagementRate, "avg_duration": avgDuration,
		"trend": trend, "top_pages": topPages, "devices": devices, "countries": countries,
	}})
}

// ═══════════════════════════════════════════════════════════════════
// Bing unified
// ═══════════════════════════════════════════════════════════════════
func unifiedBing(c *gin.Context, siteID int64, days int) {
	var deepCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM bing_query_daily WHERE site_id=$1", siteID).Scan(&deepCount)
	hasDeep := deepCount > 0

	var totalClicks, totalImpressions int
	var avgCTR, avgPosition float64
	var trend, topKeywords, topPages []map[string]interface{}

	if hasDeep {
		db.DB.QueryRow(`SELECT COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
			AVG(avg_impression_position)
			FROM bing_query_daily WHERE site_id=$1`, siteID).Scan(&totalClicks, &totalImpressions, &avgCTR, &avgPosition)

		scanMapRows(&topKeywords, `SELECT query, SUM(clicks) as clicks, SUM(impressions) as impressions,
			AVG(avg_impression_position) as position
			FROM bing_query_daily WHERE site_id=$1 GROUP BY query ORDER BY SUM(clicks) DESC LIMIT 20`, siteID)

		scanMapRows(&topPages, `SELECT page_url, SUM(clicks) as clicks, SUM(impressions) as impressions
			FROM bing_page_daily WHERE site_id=$1 GROUP BY page_url ORDER BY SUM(clicks) DESC LIMIT 20`, siteID)
	} else {
		// Fallback: bing_data
		var qJSON, pJSON string
		db.DB.QueryRow(`SELECT clicks, impressions, ctr, avg_position, COALESCE(queries_json::text,'null'), COALESCE(pages_json::text,'null')
			FROM bing_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
			&totalClicks, &totalImpressions, &avgCTR, &avgPosition, &qJSON, &pJSON)
		if qJSON != "" && qJSON != "null" {
			var qs struct {
				D []struct {
					Query string `json:"Query"`
					Clicks int `json:"Clicks"`
					Impressions int `json:"Impressions"`
					Pos float64 `json:"AvgImpPos"`
				} `json:"d"`
			}
			if json.Unmarshal([]byte(qJSON), &qs) == nil {
				for _, q := range qs.D {
					topKeywords = append(topKeywords, map[string]interface{}{"query": q.Query, "clicks": q.Clicks, "impressions": q.Impressions, "position": q.Pos})
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform": "bing", "has_deep_data": hasDeep,
		"total_clicks": totalClicks, "total_impressions": totalImpressions,
		"avg_ctr": avgCTR, "avg_position": avgPosition,
		"trend": trend, "top_keywords": topKeywords, "top_pages": topPages,
	}})
}

// ═══════════════════════════════════════════════════════════════════
// Yandex unified
// ═══════════════════════════════════════════════════════════════════
func unifiedYandex(c *gin.Context, siteID int64, days int) {
	var deepCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM yandex_query_daily WHERE site_id=$1", siteID).Scan(&deepCount)
	hasDeep := deepCount > 0

	var totalClicks, totalImpressions int
	var avgPosition float64
	var trend, topKeywords []map[string]interface{}

	if hasDeep {
		db.DB.QueryRow(`SELECT COALESCE(SUM(total_clicks),0), COALESCE(SUM(total_shows),0), AVG(avg_show_position)
			FROM yandex_query_daily WHERE site_id=$1 AND device_type='ALL'`, siteID).Scan(&totalClicks, &totalImpressions, &avgPosition)

		scanMapRows(&topKeywords, `SELECT query, SUM(total_clicks) as clicks, SUM(total_shows) as impressions, AVG(avg_show_position) as position
			FROM yandex_query_daily WHERE site_id=$1 AND device_type='ALL'
			GROUP BY query ORDER BY SUM(total_clicks) DESC LIMIT 20`, siteID)
	} else {
		// Fallback: yandex_data
		var kwJSON string
		db.DB.QueryRow(`SELECT clicks, impressions, COALESCE(keywords_json::text,'[]')
			FROM yandex_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
			&totalClicks, &totalImpressions, &kwJSON)
		if kwJSON != "" && kwJSON != "[]" {
			var kws []struct {
				QueryText  string `json:"query_text"`
				Indicators struct {
					TotalClicks int     `json:"TOTAL_CLICKS"`
					TotalShows  int     `json:"TOTAL_SHOWS"`
					AvgPos      float64 `json:"AVG_SHOW_POSITION"`
				} `json:"indicators"`
			}
			if json.Unmarshal([]byte(kwJSON), &kws) == nil {
				for _, k := range kws {
					topKeywords = append(topKeywords, map[string]interface{}{
						"query": k.QueryText, "clicks": k.Indicators.TotalClicks,
						"impressions": k.Indicators.TotalShows, "position": k.Indicators.AvgPos,
					})
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform": "yandex", "has_deep_data": hasDeep,
		"total_clicks": totalClicks, "total_impressions": totalImpressions,
		"avg_position": avgPosition,
		"trend": trend, "top_keywords": topKeywords,
	}})
}

// SyncAndLoad triggers deep sync then returns unified data — the "one button" endpoint.
// POST /v1/sites/:id/sync-and-load?platform=gsc&days=30
func SyncAndLoad(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	plat := c.DefaultQuery("platform", "gsc")
	_ = c.DefaultQuery("days", "30") // days param forwarded via query string to GetUnifiedPlatformData

	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id=$1", sid).Scan(&domain)

	userID := c.GetInt64("user_id")

	// Step 1: Run basic platform sync (existing sync handler logic)
	switch plat {
	case "gsc":
		if token, err := getGoogleAccessToken(userID); err == nil {
			if gscURL, err := resolveGSCProperty(token, domain, userID); err == nil {
				syncGSCBasic(sid, token, gscURL)
			}
		}
	case "ga4":
		if token, err := getGAAccessToken(userID); err == nil {
			if gaID, err := resolveGAProperty(token, domain, userID); err == nil {
				syncGA4Basic(sid, token, gaID)
			}
		}
	case "bing":
		if apiKey, _ := LoadUserBingKey(userID); apiKey != "" {
			if bingURL, err := resolveBingSiteURL(apiKey, domain, userID); err == nil {
				syncBingBasic(sid, apiKey, bingURL)
			}
		}
	case "yandex":
		if token, err := getYandexAccessToken(userID); err == nil {
			if yID, err := getYandexUserIDCached(userID, token); err == nil {
				if hostID, err := resolveYandexHost(token, yID, domain, userID); err == nil {
					syncYandexBasic(sid, token, yID, hostID)
				}
			}
		}
	}

	// Step 2: Return unified data
	GetUnifiedPlatformData(c)
}
