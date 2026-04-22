package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetUnifiedPlatformData returns ALL data for a given platform+site in one response.
// Deep tables first, basic tables as fallback. Single source of truth.
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

		scanMapRows(&buckets, `SELECT bucket, query_count, page_count, total_clicks, total_impressions, avg_ctr, avg_position,
			top_queries_json, top_pages_json
			FROM gsc_rank_buckets_daily WHERE site_id=$1 AND date=(SELECT MAX(date) FROM gsc_rank_buckets_daily WHERE site_id=$1)
			ORDER BY CASE bucket WHEN '1-3' THEN 1 WHEN '4-10' THEN 2 WHEN '11-20' THEN 3 WHEN '21-50' THEN 4 WHEN '51-100' THEN 5 ELSE 6 END`,
			siteID)

		scanMapRows(&appearances, `SELECT appearance, SUM(clicks) as clicks, SUM(impressions) as impressions,
			CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END as ctr
			FROM gsc_appearance_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
			GROUP BY appearance ORDER BY SUM(impressions) DESC`, siteID, days)
	} else {
		// Fallback: gsc_data
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

	if trend == nil { trend = []map[string]interface{}{} }
	if topKeywords == nil { topKeywords = []map[string]interface{}{} }
	if topPages == nil { topPages = []map[string]interface{}{} }
	if devices == nil { devices = []map[string]interface{}{} }
	if countries == nil { countries = []map[string]interface{}{} }
	if buckets == nil { buckets = []map[string]interface{}{} }
	if appearances == nil { appearances = []map[string]interface{}{} }

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
// GA4 unified — FIX #7 #8: return bounce_rate/engagement_rate + devices/countries
// ═══════════════════════════════════════════════════════════════════
func unifiedGA4(c *gin.Context, siteID int64, days int) {
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

		scanMapRows(&trend, `SELECT date::text, sessions as clicks, total_users as impressions,
			bounce_rate, engagement_rate
			FROM ga_daily_extended WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' ORDER BY date`, siteID, days)

		scanMapRows(&topPages, `SELECT landing_page as page_url, SUM(sessions) as clicks, SUM(users) as impressions,
			AVG(engagement_rate) as engagement_rate, AVG(bounce_rate) as bounce_rate
			FROM ga_landing_pages_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
			GROUP BY landing_page ORDER BY SUM(sessions) DESC LIMIT 20`, siteID, days)

		// FIX #8: Load devices from ga_tech_daily
		scanMapRows(&devices, `SELECT dimension_value as label, SUM(sessions) as clicks
			FROM ga_tech_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day' AND dimension_type='device'
			GROUP BY dimension_value ORDER BY SUM(sessions) DESC`, siteID, days)

		// FIX #8: Load countries from ga_geo_daily
		scanMapRows(&countries, `SELECT country as label, SUM(sessions) as clicks
			FROM ga_geo_daily WHERE site_id=$1 AND date>=CURRENT_DATE-$2*INTERVAL '1 day'
			GROUP BY country ORDER BY SUM(sessions) DESC LIMIT 10`, siteID, days)
	} else {
		rows, _ := db.DB.Query(`SELECT date, sessions, users, pageviews, bounce_rate, avg_duration, pages_json
			FROM ga_data WHERE site_id=$1 ORDER BY date DESC LIMIT $2`, siteID, days)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var date, pgJSON string
				var s, u, pv int
				var br, ad float64
				rows.Scan(&date, &s, &u, &pv, &br, &ad, &pgJSON)
				sessions += s
				users += u
				pageviews += pv
				bounceRate = br // Use latest
				avgDuration = ad
				trend = append(trend, map[string]interface{}{"date": date, "clicks": s, "impressions": u})
				if len(topPages) == 0 {
					parseGSCRows(&topPages, pgJSON, "page_url")
				}
			}
		}
		for i, j := 0, len(trend)-1; i < j; i, j = i+1, j-1 {
			trend[i], trend[j] = trend[j], trend[i]
		}
	}

	if trend == nil { trend = []map[string]interface{}{} }
	if topPages == nil { topPages = []map[string]interface{}{} }
	if devices == nil { devices = []map[string]interface{}{} }
	if countries == nil { countries = []map[string]interface{}{} }

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform": "ga4", "has_deep_data": hasDeep,
		"sessions": sessions, "users": users, "new_users": newUsers, "pageviews": pageviews,
		"bounce_rate": bounceRate, "engagement_rate": engagementRate, "avg_duration": avgDuration,
		"trend": trend, "top_pages": topPages, "devices": devices, "countries": countries,
	}})
}

// ═══════════════════════════════════════════════════════════════════
// Bing unified — FIX #4 #5: parse both queries_json AND pages_json correctly
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

		// FIX #15: Bing trend from daily data
		scanMapRows(&trend, `SELECT date::text, SUM(clicks) as clicks, SUM(impressions) as impressions
			FROM bing_query_daily WHERE site_id=$1 GROUP BY date ORDER BY date`, siteID)
	} else {
		// FIX #4 #5: Fallback from bing_data — parse BOTH queries_json and pages_json
		var qJSON, pJSON string
		db.DB.QueryRow(`SELECT clicks, impressions, ctr, avg_position, COALESCE(queries_json::text,'null'), COALESCE(pages_json::text,'null')
			FROM bing_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
			&totalClicks, &totalImpressions, &avgCTR, &avgPosition, &qJSON, &pJSON)

		// Parse queries — try {d:[...]} wrapper first, then direct array
		parseBingJSON(&topKeywords, qJSON, "Query", "query")
		// Parse pages — same approach
		parseBingJSON(&topPages, pJSON, "Page", "page_url")
		// Also try "Url" field for pages
		if len(topPages) == 0 {
			parseBingJSON(&topPages, pJSON, "Url", "page_url")
		}
	}

	if trend == nil { trend = []map[string]interface{}{} }
	if topKeywords == nil { topKeywords = []map[string]interface{}{} }
	if topPages == nil { topPages = []map[string]interface{}{} }

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform": "bing", "has_deep_data": hasDeep,
		"total_clicks": totalClicks, "total_impressions": totalImpressions,
		"avg_ctr": avgCTR, "avg_position": avgPosition,
		"trend": trend, "top_keywords": topKeywords, "top_pages": topPages,
	}})
}

// parseBingJSON handles both {d:[...]} wrapper and direct [...] array formats
func parseBingJSON(dest *[]map[string]interface{}, jsonStr, nameField, outKey string) {
	if jsonStr == "" || jsonStr == "null" {
		return
	}
	// Try {d: {Results: [...]}} format first (Bing API raw response)
	var wrapped struct {
		D struct {
			Results []map[string]interface{} `json:"Results"`
		} `json:"d"`
	}
	if json.Unmarshal([]byte(jsonStr), &wrapped) == nil && len(wrapped.D.Results) > 0 {
		for _, r := range wrapped.D.Results {
			name, _ := r[nameField].(string)
			clicks, _ := r["Clicks"].(float64)
			impressions, _ := r["Impressions"].(float64)
			pos, _ := r["AvgImpPos"].(float64)
			if pos == 0 {
				pos, _ = r["AvgClickPos"].(float64)
			}
			if name != "" {
				*dest = append(*dest, map[string]interface{}{outKey: name, "clicks": int(clicks), "impressions": int(impressions), "position": pos})
			}
		}
		return
	}
	// Try {d: [...]} format
	var wrapped2 struct {
		D []map[string]interface{} `json:"d"`
	}
	if json.Unmarshal([]byte(jsonStr), &wrapped2) == nil && len(wrapped2.D) > 0 {
		for _, r := range wrapped2.D {
			name, _ := r[nameField].(string)
			clicks, _ := r["Clicks"].(float64)
			impressions, _ := r["Impressions"].(float64)
			if name != "" {
				*dest = append(*dest, map[string]interface{}{outKey: name, "clicks": int(clicks), "impressions": int(impressions)})
			}
		}
		return
	}
	// Try direct array [...] format
	var direct []map[string]interface{}
	if json.Unmarshal([]byte(jsonStr), &direct) == nil {
		for _, r := range direct {
			name, _ := r[nameField].(string)
			clicks, _ := r["Clicks"].(float64)
			impressions, _ := r["Impressions"].(float64)
			if name != "" {
				*dest = append(*dest, map[string]interface{}{outKey: name, "clicks": int(clicks), "impressions": int(impressions)})
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════════
// Yandex unified — FIX #9: weighted avg position
// ═══════════════════════════════════════════════════════════════════
func unifiedYandex(c *gin.Context, siteID int64, days int) {
	var deepCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM yandex_query_daily WHERE site_id=$1", siteID).Scan(&deepCount)
	hasDeep := deepCount > 0

	var totalClicks, totalImpressions int
	var avgPosition float64
	var trend, topKeywords []map[string]interface{}

	if hasDeep {
		// FIX #9: weighted average position instead of AVG(AVG)
		db.DB.QueryRow(`SELECT COALESCE(SUM(total_clicks),0), COALESCE(SUM(total_shows),0),
			CASE WHEN SUM(total_shows)>0 THEN SUM(avg_show_position * total_shows) / SUM(total_shows) ELSE 0 END
			FROM yandex_query_daily WHERE site_id=$1 AND device_type='ALL'`, siteID).Scan(&totalClicks, &totalImpressions, &avgPosition)

		scanMapRows(&topKeywords, `SELECT query, SUM(total_clicks) as clicks, SUM(total_shows) as impressions,
			CASE WHEN SUM(total_shows)>0 THEN SUM(avg_show_position*total_shows)/SUM(total_shows) ELSE 0 END as position
			FROM yandex_query_daily WHERE site_id=$1 AND device_type='ALL'
			GROUP BY query ORDER BY SUM(total_clicks) DESC LIMIT 20`, siteID)

		// Trend from daily aggregation
		scanMapRows(&trend, `SELECT date::text, SUM(total_clicks) as clicks, SUM(total_shows) as impressions
			FROM yandex_query_daily WHERE site_id=$1 AND device_type='ALL'
			GROUP BY date ORDER BY date`, siteID)
	} else {
		// Fallback: yandex_data
		rows, _ := db.DB.Query(`SELECT date, clicks, impressions, COALESCE(keywords_json::text,'[]')
			FROM yandex_data WHERE site_id=$1 ORDER BY date DESC LIMIT $2`, siteID, days)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var date, kwJSON string
				var clicks, imp int
				rows.Scan(&date, &clicks, &imp, &kwJSON)
				totalClicks += clicks
				totalImpressions += imp
				trend = append(trend, map[string]interface{}{"date": date, "clicks": clicks, "impressions": imp})
				if len(topKeywords) == 0 {
					parseYandexKeywords(&topKeywords, kwJSON)
				}
			}
		}
		for i, j := 0, len(trend)-1; i < j; i, j = i+1, j-1 {
			trend[i], trend[j] = trend[j], trend[i]
		}
	}

	if trend == nil { trend = []map[string]interface{}{} }
	if topKeywords == nil { topKeywords = []map[string]interface{}{} }

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform": "yandex", "has_deep_data": hasDeep,
		"total_clicks": totalClicks, "total_impressions": totalImpressions,
		"avg_position": avgPosition,
		"trend": trend, "top_keywords": topKeywords,
	}})
}

// parseYandexKeywords handles the Yandex popular queries JSON format
func parseYandexKeywords(dest *[]map[string]interface{}, jsonStr string) {
	if jsonStr == "" || jsonStr == "[]" {
		return
	}
	var kws []struct {
		QueryText  string `json:"query_text"`
		Indicators struct {
			TotalClicks int     `json:"TOTAL_CLICKS"`
			TotalShows  int     `json:"TOTAL_SHOWS"`
			AvgPos      float64 `json:"AVG_SHOW_POSITION"`
		} `json:"indicators"`
	}
	if json.Unmarshal([]byte(jsonStr), &kws) == nil {
		for _, k := range kws {
			*dest = append(*dest, map[string]interface{}{
				"query": k.QueryText, "clicks": k.Indicators.TotalClicks,
				"impressions": k.Indicators.TotalShows, "position": k.Indicators.AvgPos,
			})
		}
		return
	}
	// Fallback: try raw message array
	var raw []json.RawMessage
	if json.Unmarshal([]byte(jsonStr), &raw) == nil {
		for _, r := range raw {
			var q struct {
				QueryText  string `json:"query_text"`
				Indicators struct {
					TotalClicks int     `json:"TOTAL_CLICKS"`
					TotalShows  int     `json:"TOTAL_SHOWS"`
					AvgPos      float64 `json:"AVG_SHOW_POSITION"`
				} `json:"indicators"`
			}
			if json.Unmarshal(r, &q) == nil && q.QueryText != "" {
				*dest = append(*dest, map[string]interface{}{
					"query": q.QueryText, "clicks": q.Indicators.TotalClicks,
					"impressions": q.Indicators.TotalShows, "position": q.Indicators.AvgPos,
				})
			}
		}
	}
}

// SyncAndLoad triggers deep sync then returns unified data.
// FIX #10: forward days parameter correctly
func SyncAndLoad(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	plat := c.DefaultQuery("platform", "gsc")

	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id=$1", sid).Scan(&domain)

	userID := c.GetInt64("user_id")

	// Step 1: Run basic platform sync (best-effort, don't block on failure)
	switch plat {
	case "gsc":
		if token, err := getGoogleAccessToken(userID); err == nil {
			if gscURL, err := resolveGSCProperty(token, domain, userID); err == nil {
				syncGSCBasic(sid, token, gscURL)
			} else {
				log.Printf("[sync-and-load] GSC resolve failed for %s: %v", domain, err)
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

	// Step 2: Return unified data (reads from DB which now has fresh data)
	GetUnifiedPlatformData(c)
}
