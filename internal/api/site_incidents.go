package api

// Site Incident History — "历史错误汇总" on the site detail page.
// Combines two data sources into one timeline:
//   1. coordination_tasks where status='done' (auto, every fix that
//      went through the coord workflow)
//   2. site_incidents (manual, free-form ops log entries — "今天 503"
//      "主域名被封" etc.)
// Owner-or-admin scoped via canAccessSite. Anything else returns 403.

import (
	"net/http"
	"strconv"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetSiteHistory returns combined coord-completed + manual-incident
// rows for a single site, sorted by date desc, capped at 200 each.
//
// Each item carries a `source` string ("coord" / "manual") so the UI
// can render with different icons / actions (manual rows can be
// deleted, coord rows can't).
func GetSiteHistory(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	out := []gin.H{}

	// 1) Completed coord tasks for this site.
	rows, err := db.DB.Query(`
		SELECT t.id, t.issue_title, COALESCE(t.issue_severity,''),
		       COALESCE(t.issue_scope,''), COALESCE(t.issue_module,''),
		       COALESCE(t.note,''),
		       COALESCE(u.name,''), COALESCE(t.verified_at, t.created_at) AS at_time
		FROM coordination_tasks t
		LEFT JOIN users u ON u.id = t.created_by
		WHERE t.site_id::text = $1 AND t.status = 'done'
		ORDER BY at_time DESC
		LIMIT 200`, siteID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var title, severity, scope, module, note, who, at string
			if err := rows.Scan(&id, &title, &severity, &scope, &module, &note, &who, &at); err != nil {
				continue
			}
			out = append(out, gin.H{
				"id":         "coord-" + strconv.FormatInt(id, 10),
				"source":     "coord",
				"coord_id":   id,
				"title":      title,
				"severity":   severity,
				"scope":      scope,
				"module":     module,
				"note":       note,
				"actor_name": who,
				"at":         at,
			})
		}
	}

	// 2) Manual incidents.
	rows2, err := db.DB.Query(`
		SELECT i.id, i.title, COALESCE(i.description,''),
		       COALESCE(i.severity,'medium'),
		       COALESCE(u.name,''),
		       COALESCE(i.occurred_at, i.created_at) AS at_time
		FROM site_incidents i
		LEFT JOIN users u ON u.id = i.created_by
		WHERE i.site_id::text = $1
		ORDER BY at_time DESC
		LIMIT 200`, siteID)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var id int64
			var title, desc, severity, who, at string
			if err := rows2.Scan(&id, &title, &desc, &severity, &who, &at); err != nil {
				continue
			}
			out = append(out, gin.H{
				"id":          "incident-" + strconv.FormatInt(id, 10),
				"source":      "manual",
				"incident_id": id,
				"title":       title,
				"description": desc,
				"severity":    severity,
				"actor_name":  who,
				"at":          at,
			})
		}
	}

	// Sort combined slice by `at` desc — both lists are pre-sorted
	// individually but interleave by date. Insertion sort is fine for
	// ~400 rows; avoids importing sort just for this.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, _ := out[j]["at"].(string)
			b, _ := out[j-1]["at"].(string)
			if a > b {
				out[j], out[j-1] = out[j-1], out[j]
			} else {
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// CreateSiteIncidentRequest is the body for POST /v1/sites/:id/incidents.
type CreateSiteIncidentRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Severity    string `json:"severity"`    // low / medium / high — default medium
	OccurredAt  string `json:"occurred_at"` // RFC3339; defaults to NOW() if empty
}

// CreateSiteIncident logs a manual incident against a site.
func CreateSiteIncident(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	var req CreateSiteIncidentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写事件标题"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)
	req.Severity = strings.ToLower(strings.TrimSpace(req.Severity))
	switch req.Severity {
	case "low", "medium", "high":
	default:
		req.Severity = "medium"
	}
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写事件标题"})
		return
	}
	uid := c.GetInt64("user_id")

	// occurred_at: empty → DB default NOW(); else parse-as-text and
	// let Postgres do the cast. Lets users pick "yesterday 18:00"
	// without us needing to construct timestamps in Go.
	var id int64
	var err error
	if req.OccurredAt == "" {
		err = db.DB.QueryRow(
			`INSERT INTO site_incidents (site_id, title, description, severity, created_by)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			siteID, req.Title, req.Description, req.Severity, uid,
		).Scan(&id)
	} else {
		err = db.DB.QueryRow(
			`INSERT INTO site_incidents (site_id, title, description, severity, occurred_at, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			siteID, req.Title, req.Description, req.Severity, req.OccurredAt, uid,
		).Scan(&id)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "保存失败：" + err.Error()})
		return
	}
	WriteAuditLog(c, "create_site_incident", "site_incident", strconv.FormatInt(id, 10), req.Title)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// DeleteSiteIncident removes a manual incident. Coord-source rows
// aren't deletable through this handler — they belong to coord
// workflow.
func DeleteSiteIncident(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	incidentID := c.Param("incident_id")
	if _, err := db.DB.Exec(
		`DELETE FROM site_incidents WHERE id::text = $1 AND site_id::text = $2`,
		incidentID, siteID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	WriteAuditLog(c, "delete_site_incident", "site_incident", incidentID, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}
