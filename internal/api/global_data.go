package api

// Admin global-data endpoint: one platform + one date range → a flat
// list of per-site cards aggregated from the locally synced tables
// (gsc_data / ga_data / yandex_data / bing_data). Powers the 全局数据
// page, which is the admin's cross-user version of the personal 数据
//管理 view — identical card UI, just every user's sites in one grid.
//
// Why local DB instead of calling each provider's API per user:
//   - The per-user dashboards (DataManagement.vue) go through each
//     user's own OAuth token, which the admin does not (and should not)
//     have access to.
//   - Sync jobs already write a daily row per (site, platform) into the
//     local tables, so we have everything we need to aggregate.
//
// Data freshness is therefore whatever the last sync produced. If
// sites show zeros, the owner hasn't synced yet — not an admin issue.

import (
	"fmt"
	"net/http"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

type globalSiteInfo struct {
	id        int64
	name      string
	domain    string
	ownerID   int64
	ownerName string
}

// GetGlobalDataOverview GET /api/v1/admin/global-data?platform=gsc&start_date=X&end_date=Y
//
// platform: gsc | ga4 | bing | yandex
// start_date / end_date: YYYY-MM-DD inclusive
//
// Response:
//
//	{ cards: [{site_id, name, domain, owner_name, clicks, impressions, ctr, position, has_data}] }
//
// Shape kept close to DataManagement's overview so the frontend can
// reuse its existing card component.
func GetGlobalDataOverview(c *gin.Context) {
	platform := strings.ToLower(c.DefaultQuery("platform", "gsc"))
	switch platform {
	case "gsc", "ga4", "bing", "yandex":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "platform 必须是 gsc / ga4 / bing / yandex"})
		return
	}
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if !isYMD(startDate) || !isYMD(endDate) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "start_date / end_date 必须是 YYYY-MM-DD"})
		return
	}
	filterUser := c.Query("user_id") // optional

	// Base: every site + owner name. We ALWAYS list every site, even
	// ones that have no data in the requested window — the card just
	// shows "无数据" in that case, which matches the per-user page.
	siteQ := `SELECT s.id, s.name, s.domain, COALESCE(s.created_by,0), COALESCE(u.name,'')
	          FROM sites s LEFT JOIN users u ON u.id = s.created_by`
	args := []interface{}{}
	if filterUser != "" {
		siteQ += " WHERE s.created_by = ?"
		args = append(args, filterUser)
	}
	siteQ += " ORDER BY u.name, s.id DESC"

	rows, err := db.DB.Query(siteQ, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询站点失败"})
		return
	}
	var sites []globalSiteInfo
	for rows.Next() {
		var s globalSiteInfo
		if err := rows.Scan(&s.id, &s.name, &s.domain, &s.ownerID, &s.ownerName); err != nil {
			continue
		}
		sites = append(sites, s)
	}
	rows.Close()

	// Aggregate the platform data in a single grouped query.
	type agg struct {
		clicks, impressions, indexCount int64
		ctr, position                   float64
		hasData                         bool
	}
	byID := map[int64]*agg{}
	var aggQ string
	switch platform {
	case "gsc":
		aggQ = `SELECT site_id,
		               COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
		               COALESCE(AVG(ctr),0), COALESCE(AVG(position),0),
		               COUNT(*)
		        FROM gsc_data WHERE date >= ? AND date <= ? GROUP BY site_id`
	case "ga4":
		// GA4: clicks → sessions, impressions → users (re-purposed so
		// the shared card UI doesn't need a platform switch). position
		// / ctr stay zero.
		aggQ = `SELECT site_id,
		               COALESCE(SUM(sessions),0), COALESCE(SUM(users),0),
		               0, 0,
		               COUNT(*)
		        FROM ga_data WHERE date >= ? AND date <= ? GROUP BY site_id`
	case "bing":
		aggQ = `SELECT site_id,
		               COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
		               COALESCE(AVG(ctr),0), COALESCE(AVG(avg_position),0),
		               COUNT(*)
		        FROM bing_data WHERE date >= ? AND date <= ? GROUP BY site_id`
	case "yandex":
		// Yandex has no position/CTR in our schema — surface index_count
		// via position slot so the card still has a 4th number.
		aggQ = `SELECT site_id,
		               COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
		               0, COALESCE(MAX(index_count),0),
		               COUNT(*)
		        FROM yandex_data WHERE date >= ? AND date <= ? GROUP BY site_id`
	}

	if r2, err := db.DB.Query(aggQ, startDate, endDate); err == nil {
		for r2.Next() {
			var sid int64
			var clicks, impressions int64
			var ctr, pos float64
			var rowCount int
			if err := r2.Scan(&sid, &clicks, &impressions, &ctr, &pos, &rowCount); err != nil {
				continue
			}
			byID[sid] = &agg{
				clicks:      clicks,
				impressions: impressions,
				ctr:         ctr,
				position:    pos,
				hasData:     rowCount > 0,
			}
		}
		r2.Close()
	}

	cards := make([]gin.H, 0, len(sites))
	var totalClicks, totalImpr int64
	for _, s := range sites {
		a := byID[s.id]
		if a == nil {
			a = &agg{}
		}
		totalClicks += a.clicks
		totalImpr += a.impressions
		card := gin.H{
			"site_id":      s.id,
			"display_name": displayNameForSite(s.name, s.domain),
			"site_url":     s.domain,
			"name":         s.name,
			"domain":       s.domain,
			"owner_id":     s.ownerID,
			"owner_name":   s.ownerName,
			"has_data":     a.hasData,
		}
		switch platform {
		case "ga4":
			card["sessions"] = a.clicks
			card["total_users"] = a.impressions
		default:
			card["clicks"] = a.clicks
			card["impressions"] = a.impressions
			card["ctr"] = a.ctr
			card["position"] = a.position
		}
		if platform == "yandex" {
			// position slot was repurposed for index_count
			card["index_count"] = int64(a.position)
			delete(card, "position")
			delete(card, "ctr")
		}
		cards = append(cards, card)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"cards":    cards,
		"platform": platform,
		"range":    gin.H{"start": startDate, "end": endDate},
		"summary": gin.H{
			"total_sites":  len(cards),
			"total_owners": countDistinctOwners(sites),
			"total_clicks": totalClicks,
			"total_impr":   totalImpr,
		},
	}})
}

func displayNameForSite(name, domain string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return domain
}

func countDistinctOwners(sites []globalSiteInfo) int {
	seen := map[int64]bool{}
	for _, s := range sites {
		if s.ownerID > 0 {
			seen[s.ownerID] = true
		}
	}
	return len(seen)
}

// isYMD validates YYYY-MM-DD strictly — no timezones, no times.
func isYMD(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, r := range s {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// _unused keeps fmt imported without forcing an import line on every
// future tweak; the aggregation queries above use it via printf-style
// helpers added later.
var _ = fmt.Sprintf
