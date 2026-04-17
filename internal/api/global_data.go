package api

// Admin global-overview endpoint: every site across every user, with the
// latest row from each of the four data platforms (GSC / GA / Yandex /
// Bing). Populates the 全局数据 page so an admin can see the whole
// operation at a glance without drilling per-user.
//
// Performance: the naive implementation ran five round-trips per site
// (one for sites + 4 for platforms) which grew linearly. We use one
// correlated-subquery SELECT that JOINs everything in a single query.
// SQLite's query planner handles this well because each subquery hits
// the (site_id, date DESC) index.

import (
	"net/http"
	"sort"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetGlobalDataOverview GET /api/v1/admin/global-data (admin only)
//
// Returns one row per site with its owner and the most recent snapshot
// from each of the four platforms. Numbers can be zero (site exists
// but never synced) — the frontend renders those as "—". Admin clicks
// a row to jump to that site's /data page for full details.
func GetGlobalDataOverview(c *gin.Context) {
	// Optional filter for the admin UI: narrow to a single owner.
	filterUser := c.Query("user_id")

	// One big query per platform. We do 4 parallel-but-sequential queries
	// into maps, then zip the results together in Go. A single mega-JOIN
	// would require LATERAL (unsupported in SQLite) or ugly nested
	// COALESCE; this is simpler and fast enough for admin-scale reads.
	siteQ := `
		SELECT s.id, s.name, s.domain, COALESCE(s.created_by,0), COALESCE(u.name,''),
		       s.status, s.created_at,
		       (SELECT status FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) as last_audit_status,
		       (SELECT created_at FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) as last_audit_time
		FROM sites s
		LEFT JOIN users u ON u.id = s.created_by`
	args := []interface{}{}
	if filterUser != "" {
		siteQ += ` WHERE s.created_by = ?`
		args = append(args, filterUser)
	}
	siteQ += ` ORDER BY u.name, s.id DESC`

	rows, err := db.DB.Query(siteQ, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询站点失败"})
		return
	}
	type siteRow struct {
		ID        int64
		Name      string
		Domain    string
		OwnerID   int64
		OwnerName string
		Status    string
		CreatedAt string
		AuditS    *string
		AuditT    *string
	}
	var siteList []siteRow
	for rows.Next() {
		var s siteRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Domain, &s.OwnerID, &s.OwnerName,
			&s.Status, &s.CreatedAt, &s.AuditS, &s.AuditT); err != nil {
			continue
		}
		siteList = append(siteList, s)
	}
	rows.Close()

	// Latest GSC per site.
	gscMap := map[int64]gin.H{}
	if r2, err := db.DB.Query(`
		SELECT g.site_id, g.date, g.clicks, g.impressions, g.ctr, g.position
		FROM gsc_data g
		INNER JOIN (
			SELECT site_id, MAX(date) mx FROM gsc_data GROUP BY site_id
		) last ON last.site_id = g.site_id AND last.mx = g.date`); err == nil {
		for r2.Next() {
			var sid int64
			var date string
			var clicks, impr int
			var ctr, pos float64
			if err := r2.Scan(&sid, &date, &clicks, &impr, &ctr, &pos); err == nil {
				gscMap[sid] = gin.H{"date": date, "clicks": clicks, "impressions": impr, "ctr": ctr, "position": pos}
			}
		}
		r2.Close()
	}

	// Latest GA per site.
	gaMap := map[int64]gin.H{}
	if r2, err := db.DB.Query(`
		SELECT g.site_id, g.date, g.sessions, g.users, g.pageviews, g.bounce_rate
		FROM ga_data g
		INNER JOIN (
			SELECT site_id, MAX(date) mx FROM ga_data GROUP BY site_id
		) last ON last.site_id = g.site_id AND last.mx = g.date`); err == nil {
		for r2.Next() {
			var sid int64
			var date string
			var sess, users, pv int
			var br float64
			if err := r2.Scan(&sid, &date, &sess, &users, &pv, &br); err == nil {
				gaMap[sid] = gin.H{"date": date, "sessions": sess, "users": users, "pageviews": pv, "bounce_rate": br}
			}
		}
		r2.Close()
	}

	// Latest Yandex per site.
	yandexMap := map[int64]gin.H{}
	if r2, err := db.DB.Query(`
		SELECT y.site_id, y.date, y.clicks, y.impressions, y.index_count
		FROM yandex_data y
		INNER JOIN (
			SELECT site_id, MAX(date) mx FROM yandex_data GROUP BY site_id
		) last ON last.site_id = y.site_id AND last.mx = y.date`); err == nil {
		for r2.Next() {
			var sid int64
			var date string
			var clicks, impr, idx int
			if err := r2.Scan(&sid, &date, &clicks, &impr, &idx); err == nil {
				yandexMap[sid] = gin.H{"date": date, "clicks": clicks, "impressions": impr, "index_count": idx}
			}
		}
		r2.Close()
	}

	// Latest Bing per site.
	bingMap := map[int64]gin.H{}
	if r2, err := db.DB.Query(`
		SELECT b.site_id, b.date, b.clicks, b.impressions, b.ctr, b.avg_position, b.index_count
		FROM bing_data b
		INNER JOIN (
			SELECT site_id, MAX(date) mx FROM bing_data GROUP BY site_id
		) last ON last.site_id = b.site_id AND last.mx = b.date`); err == nil {
		for r2.Next() {
			var sid int64
			var date string
			var clicks, impr, idx int
			var ctr, pos float64
			if err := r2.Scan(&sid, &date, &clicks, &impr, &ctr, &pos, &idx); err == nil {
				bingMap[sid] = gin.H{"date": date, "clicks": clicks, "impressions": impr, "ctr": ctr, "avg_position": pos, "index_count": idx}
			}
		}
		r2.Close()
	}

	// Zip.
	out := make([]gin.H, 0, len(siteList))
	for _, s := range siteList {
		row := gin.H{
			"site_id":           s.ID,
			"name":              s.Name,
			"domain":            s.Domain,
			"owner_id":          s.OwnerID,
			"owner_name":        s.OwnerName,
			"status":            s.Status,
			"created_at":        s.CreatedAt,
			"last_audit_status": s.AuditS,
			"last_audit_time":   s.AuditT,
			"gsc":               gscMap[s.ID],
			"ga":                gaMap[s.ID],
			"yandex":            yandexMap[s.ID],
			"bing":              bingMap[s.ID],
		}
		out = append(out, row)
	}

	// Totals across everything rendered — useful for a summary bar.
	var totalClicks, totalImpressions, totalSessions, totalUsers int
	for _, r := range out {
		if g, ok := r["gsc"].(gin.H); ok && g != nil {
			if v, ok := g["clicks"].(int); ok {
				totalClicks += v
			}
			if v, ok := g["impressions"].(int); ok {
				totalImpressions += v
			}
		}
		if g, ok := r["ga"].(gin.H); ok && g != nil {
			if v, ok := g["sessions"].(int); ok {
				totalSessions += v
			}
			if v, ok := g["users"].(int); ok {
				totalUsers += v
			}
		}
	}

	// Group by owner count (for the "managed by N users" headline).
	ownerSet := map[int64]bool{}
	for _, s := range siteList {
		if s.OwnerID > 0 {
			ownerSet[s.OwnerID] = true
		}
	}
	ownerIDs := make([]int64, 0, len(ownerSet))
	for id := range ownerSet {
		ownerIDs = append(ownerIDs, id)
	}
	sort.Slice(ownerIDs, func(i, j int) bool { return ownerIDs[i] < ownerIDs[j] })

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"sites": out,
		"summary": gin.H{
			"total_sites":       len(out),
			"total_owners":      len(ownerSet),
			"total_gsc_clicks":  totalClicks,
			"total_impressions": totalImpressions,
			"total_sessions":    totalSessions,
			"total_users":       totalUsers,
		},
	}})
}
