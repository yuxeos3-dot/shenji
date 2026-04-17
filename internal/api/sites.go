package api

import (
	"log"
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

type CreateSiteRequest struct {
	Domain               string `json:"domain" binding:"required"`
	Name                 string `json:"name" binding:"required"`
	GscSiteURL           string `json:"gsc_site_url"`
	GaPropertyID         string `json:"ga_property_id"`
	YandexHostID         string `json:"yandex_host_id"`
	BingSiteURL          string `json:"bing_site_url"`
	URLCategory          string `json:"url_category"`
	URLDetail            string `json:"url_detail"`
	URLNewsList          string `json:"url_news_list"`
	URLNewsDetail        string `json:"url_news_detail"`
	URLSearch            string `json:"url_search"`
	URLStatic            string `json:"url_static"`
	URLPagesJSON         string `json:"url_pages_json"`
	AuditScheduleEnabled string `json:"audit_schedule_enabled"`
	AuditIntervalHours   int    `json:"audit_interval_hours"`
	AuditDailyHour       int    `json:"audit_daily_hour"`
}

func ListSites(c *gin.Context) {
	userID := c.GetInt64("user_id")
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	query := `
		SELECT s.id, s.domain, s.name, s.gsc_site_url, s.ga_property_id, s.yandex_host_id,
		       s.status, s.created_at,
		       (SELECT status FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) as last_audit_status,
		       (SELECT created_at FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) as last_audit_time
		FROM sites s`

	var args []interface{}
	if !isAdmin {
		query += " WHERE s.created_by = ?"
		args = append(args, userID)
	}
	query += " ORDER BY s.id DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var sites []gin.H
	for rows.Next() {
		var id int64
		var domain, name, gscURL, gaID, yandexID, status, createdAt string
		var lastAuditStatus, lastAuditTime *string
		if err := rows.Scan(&id, &domain, &name, &gscURL, &gaID, &yandexID, &status, &createdAt, &lastAuditStatus, &lastAuditTime); err != nil {
			continue
		}
		sites = append(sites, gin.H{
			"id":                id,
			"domain":            domain,
			"name":              name,
			"gsc_site_url":      gscURL,
			"ga_property_id":    gaID,
			"yandex_host_id":    yandexID,
			"status":            status,
			"created_at":        createdAt,
			"last_audit_status": lastAuditStatus,
			"last_audit_time":   lastAuditTime,
		})
	}
	if sites == nil {
		sites = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": sites})
}

func CreateSite(c *gin.Context) {
	var req CreateSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误: " + err.Error()})
		return
	}

	if req.AuditScheduleEnabled == "" {
		req.AuditScheduleEnabled = "false"
	}
	if req.AuditDailyHour == 0 {
		req.AuditDailyHour = -1
	}

	userID := c.GetInt64("user_id")
	result, err := db.DB.Exec(
		`INSERT INTO sites (domain, name, gsc_site_url, ga_property_id, yandex_host_id, bing_site_url,
		 url_category, url_detail, url_news_list, url_news_detail, url_search, url_static,
		 url_pages_json, audit_schedule_enabled, audit_interval_hours, audit_daily_hour, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Domain, req.Name, req.GscSiteURL, req.GaPropertyID, req.YandexHostID, req.BingSiteURL,
		req.URLCategory, req.URLDetail, req.URLNewsList, req.URLNewsDetail, req.URLSearch, req.URLStatic,
		req.URLPagesJSON, req.AuditScheduleEnabled, req.AuditIntervalHours, req.AuditDailyHour, userID,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "创建失败，域名可能已存在"})
		return
	}

	id, _ := result.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func GetSite(c *gin.Context) {
	id := c.Param("id")
	if !canAccessSite(c, id) {
		return
	}
	var site struct {
		ID                   int64  `json:"id"`
		Domain               string `json:"domain"`
		Name                 string `json:"name"`
		GscSiteURL           string `json:"gsc_site_url"`
		GaPropertyID         string `json:"ga_property_id"`
		YandexHostID         string `json:"yandex_host_id"`
		URLCategory          string `json:"url_category"`
		URLDetail            string `json:"url_detail"`
		URLNewsList          string `json:"url_news_list"`
		URLNewsDetail        string `json:"url_news_detail"`
		URLSearch            string `json:"url_search"`
		URLStatic            string `json:"url_static"`
		Status               string `json:"status"`
		AuditScheduleEnabled string `json:"audit_schedule_enabled"`
		AuditIntervalHours   int    `json:"audit_interval_hours"`
		AuditDailyHour       int    `json:"audit_daily_hour"`
		CreatedAt            string `json:"created_at"`
	}

	err := db.DB.QueryRow(`SELECT id, domain, name, gsc_site_url, ga_property_id, yandex_host_id,
		url_category, url_detail, url_news_list, url_news_detail, url_search, url_static,
		status, COALESCE(audit_schedule_enabled,'false'), COALESCE(audit_interval_hours,0), COALESCE(audit_daily_hour,-1),
		created_at FROM sites WHERE id = ?`, id).Scan(
		&site.ID, &site.Domain, &site.Name, &site.GscSiteURL, &site.GaPropertyID, &site.YandexHostID,
		&site.URLCategory, &site.URLDetail, &site.URLNewsList, &site.URLNewsDetail,
		&site.URLSearch, &site.URLStatic, &site.Status,
		&site.AuditScheduleEnabled, &site.AuditIntervalHours, &site.AuditDailyHour,
		&site.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": site})
}

func UpdateSite(c *gin.Context) {
	id := c.Param("id")
	if !canAccessSite(c, id) {
		return
	}
	var req CreateSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	if req.AuditScheduleEnabled == "" {
		req.AuditScheduleEnabled = "false"
	}

	_, err := db.DB.Exec(
		`UPDATE sites SET domain=?, name=?, gsc_site_url=?, ga_property_id=?, yandex_host_id=?, bing_site_url=?,
		 url_category=?, url_detail=?, url_news_list=?, url_news_detail=?, url_search=?, url_static=?,
		 audit_schedule_enabled=?, audit_interval_hours=?, audit_daily_hour=?
		 WHERE id=?`,
		req.Domain, req.Name, req.GscSiteURL, req.GaPropertyID, req.YandexHostID, req.BingSiteURL,
		req.URLCategory, req.URLDetail, req.URLNewsList, req.URLNewsDetail, req.URLSearch, req.URLStatic,
		req.AuditScheduleEnabled, req.AuditIntervalHours, req.AuditDailyHour, id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}
	WriteAuditLog(c, "update_site", "site", id, req.Domain)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "更新成功"})
}

func DeleteSite(c *gin.Context) {
	id := c.Param("id")
	if !canAccessSite(c, id) {
		return
	}

	// Cascade delete all related data in a single transaction — bail out and
	// rollback on the first error so we never leave orphan rows behind.
	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	execInTx := func(query string, args ...interface{}) error {
		if _, err := tx.Exec(query, args...); err != nil {
			// Log the query (so operators can diagnose) but do NOT include
			// it in the client response — it lowers an attacker's cost of
			// mapping the schema.
			log.Printf("[delete_site] step failed: %s: %v", query, err)
			return err
		}
		return nil
	}

	steps := []struct {
		query string
		args  []interface{}
	}{
		{"DELETE FROM dead_link_results WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM external_links WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM site_pages WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM gsc_data WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM ga_data WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM yandex_data WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM audit_results WHERE task_id IN (SELECT id FROM audit_tasks WHERE site_id = ?)", []interface{}{id}},
		{"DELETE FROM audit_pages   WHERE task_id IN (SELECT id FROM audit_tasks WHERE site_id = ?)", []interface{}{id}},
		{"DELETE FROM audit_tasks WHERE site_id = ?", []interface{}{id}},
		{"DELETE FROM sites WHERE id = ?", []interface{}{id}},
	}
	for _, s := range steps {
		if err := execInTx(s.query, s.args...); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	WriteAuditLog(c, "delete_site", "site", id, "")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}
