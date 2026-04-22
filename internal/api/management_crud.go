package api

import (
	"net/http"
	"strconv"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ═══════════════════════════════════════════════════════════════
// Competitors CRUD
// ═══════════════════════════════════════════════════════════════

func ListCompetitors(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var result []map[string]interface{}
	scanMapRows(&result, `SELECT id, domain, name, tier, notes, monitor_enabled, created_at::text FROM competitors WHERE site_id=$1 ORDER BY created_at DESC`, siteID)
	if result == nil { result = []map[string]interface{}{} }
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func CreateCompetitor(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var req struct { Domain string `json:"domain" binding:"required"`; Name string `json:"name"`; Tier string `json:"tier"`; Notes string `json:"notes"` }
	if c.ShouldBindJSON(&req) != nil { c.JSON(400, gin.H{"code": -1, "error": "参数错误"}); return }
	sid, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil { c.JSON(400, gin.H{"code": -1, "error": "无效的站点ID"}); return }
	if _, err := db.DB.Exec(`INSERT INTO competitors (site_id, domain, name, tier, notes, created_by) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
		sid, req.Domain, req.Name, req.Tier, req.Notes, c.GetInt64("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

func DeleteCompetitor(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	competitorID := c.Param("competitorId")
	// Verify the competitor belongs to the site the user has access to
	var ownerSiteID int64
	err := db.DB.QueryRow("SELECT site_id FROM competitors WHERE id=$1", competitorID).Scan(&ownerSiteID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "竞品不存在"})
		return
	}
	if strconv.FormatInt(ownerSiteID, 10) != siteID {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权操作此竞品"})
		return
	}
	if _, err := db.DB.Exec("DELETE FROM competitors WHERE id=$1 AND site_id=$2", competitorID, siteID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

// ═══════════════════════════════════════════════════════════════
// Algorithm Updates CRUD
// ═══════════════════════════════════════════════════════════════

func ListAlgorithmUpdates(c *gin.Context) {
	var result []map[string]interface{}
	scanMapRows(&result, `SELECT id, engine, update_name, update_type, announced_at::text, started_at::text, ended_at::text, official, summary, source_type FROM algorithm_updates ORDER BY announced_at DESC NULLS LAST LIMIT 100`)
	if result == nil { result = []map[string]interface{}{} }
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func CreateAlgorithmUpdate(c *gin.Context) {
	var req struct {
		Engine string `json:"engine" binding:"required"`; UpdateName string `json:"update_name" binding:"required"`
		UpdateType string `json:"update_type"`; AnnouncedAt string `json:"announced_at"`
		Summary string `json:"summary"`; Official bool `json:"official"`; SourceURL string `json:"source_url"`
	}
	if c.ShouldBindJSON(&req) != nil { c.JSON(400, gin.H{"code": -1, "error": "参数错误"}); return }
	if _, err := db.DB.Exec(`INSERT INTO algorithm_updates (engine, update_name, update_type, announced_at, summary, official, source_url, source_type, logged_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'manual',$8) ON CONFLICT DO NOTHING`,
		req.Engine, req.UpdateName, req.UpdateType, req.AnnouncedAt, req.Summary, req.Official, req.SourceURL, c.GetInt64("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

// ═══════════════════════════════════════════════════════════════
// SERP Observations CRUD
// ═══════════════════════════════════════════════════════════════

func ListSERPObservations(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var result []map[string]interface{}
	scanMapRows(&result, `SELECT id, query, engine, country, device, our_position, our_url, observed_at::text, notes
		FROM serp_observations WHERE site_id=$1 ORDER BY observed_at DESC LIMIT 200`, siteID)
	if result == nil { result = []map[string]interface{}{} }
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func CreateSERPObservation(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var req struct {
		Query string `json:"query" binding:"required"`; Engine string `json:"engine"`
		Country string `json:"country"`; Device string `json:"device"`
		OurPosition int `json:"our_position"`; OurURL string `json:"our_url"`; Notes string `json:"notes"`
	}
	if c.ShouldBindJSON(&req) != nil { c.JSON(400, gin.H{"code": -1, "error": "参数错误"}); return }
	sid, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil { c.JSON(400, gin.H{"code": -1, "error": "无效的站点ID"}); return }
	if req.Engine == "" { req.Engine = "google" }
	if req.Device == "" { req.Device = "desktop" }
	if _, err := db.DB.Exec(`INSERT INTO serp_observations (site_id, query, engine, country, device, our_position, our_url, notes, observer_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		sid, req.Query, req.Engine, req.Country, req.Device, req.OurPosition, req.OurURL, req.Notes, c.GetInt64("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

// ═══════════════════════════════════════════════════════════════
// AIO Observations CRUD
// ═══════════════════════════════════════════════════════════════

func ListAIOObservations(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var result []map[string]interface{}
	scanMapRows(&result, `SELECT id, query, has_aio, aio_text, our_domain_cited, observed_at::text, notes
		FROM aio_observations WHERE site_id=$1 ORDER BY observed_at DESC LIMIT 200`, siteID)
	if result == nil { result = []map[string]interface{}{} }
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func CreateAIOObservation(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var req struct {
		Query string `json:"query" binding:"required"`; HasAIO bool `json:"has_aio"`
		AIOText string `json:"aio_text"`; OurDomainCited bool `json:"our_domain_cited"`; Notes string `json:"notes"`
	}
	if c.ShouldBindJSON(&req) != nil { c.JSON(400, gin.H{"code": -1, "error": "参数错误"}); return }
	sid, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil { c.JSON(400, gin.H{"code": -1, "error": "无效的站点ID"}); return }
	if _, err := db.DB.Exec(`INSERT INTO aio_observations (site_id, query, has_aio, aio_text, our_domain_cited, notes, observer_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		sid, req.Query, req.HasAIO, req.AIOText, req.OurDomainCited, req.Notes, c.GetInt64("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

// ═══════════════════════════════════════════════════════════════
// Timeline Events CRUD
// ═══════════════════════════════════════════════════════════════

func ListTimelineEvents(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var result []map[string]interface{}
	scanMapRows(&result, `SELECT id, event_category, event_type, title, severity, occurred_at::text, source_type
		FROM timeline_events WHERE (site_id=$1 OR site_id=0) ORDER BY occurred_at DESC LIMIT 100`, siteID)
	if result == nil { result = []map[string]interface{}{} }
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func CreateTimelineEvent(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) { return }
	var req struct {
		Category string `json:"event_category" binding:"required"`; Type string `json:"event_type" binding:"required"`
		Title string `json:"title" binding:"required"`; Severity string `json:"severity"`
		OccurredAt string `json:"occurred_at"`; Description string `json:"description"`
	}
	if c.ShouldBindJSON(&req) != nil { c.JSON(400, gin.H{"code": -1, "error": "参数错误"}); return }
	sid, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil { c.JSON(400, gin.H{"code": -1, "error": "无效的站点ID"}); return }
	if req.Severity == "" { req.Severity = "info" }
	if req.OccurredAt == "" { req.OccurredAt = time.Now().Format(time.RFC3339) }
	if _, err := db.DB.Exec(`INSERT INTO timeline_events (site_id, event_category, event_type, title, severity, occurred_at, description, source_type, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'manual',$8)`,
		sid, req.Category, req.Type, req.Title, req.Severity, req.OccurredAt, req.Description, c.GetInt64("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}
