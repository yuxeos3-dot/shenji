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
	BrandRegex           string `json:"brand_regex"`
	SiteLanguage         string `json:"site_language"`
	PrimaryCountry       string `json:"primary_country"`
	SiteType             string `json:"site_type"`
}

func ListSites(c *gin.Context) {
	userID := c.GetInt64("user_id")
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	// Admin-only filter: ?user_id=N limits the list to that owner. Used
	// by the 用户监控 page so the admin can drill into a specific employee's
	// sites without seeing the full cross-tenant list.
	filterUser := ""
	if isAdmin {
		filterUser = c.Query("user_id")
	}

	query := `
		SELECT s.id, s.domain, s.name, s.gsc_site_url, s.ga_property_id, s.yandex_host_id,
		       s.status, s.created_at, COALESCE(s.created_by,0),
		       COALESCE(u.name,''), COALESCE(d.name,''), COALESCE(g.name,''),
		       (SELECT status FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) as last_audit_status,
		       (SELECT created_at FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) as last_audit_time
		FROM sites s
		LEFT JOIN users u ON u.id = s.created_by
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id`

	var args []interface{}
	if isAdmin {
		if filterUser != "" {
			query += " WHERE s.created_by = $1"
			args = append(args, filterUser)
		}
	} else if role == "group_leader" {
		// Group leader sees sites owned by any teammate in their group.
		var myGroup int64
		db.DB.QueryRow("SELECT COALESCE(group_id,0) FROM users WHERE id = $1", userID).Scan(&myGroup)
		if myGroup > 0 {
			query += " WHERE s.created_by IN (SELECT id FROM users WHERE COALESCE(group_id,0) = $1)"
			args = append(args, myGroup)
		} else {
			query += " WHERE s.created_by = $1"
			args = append(args, userID)
		}
	} else {
		query += " WHERE s.created_by = $1"
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
		var id, createdBy int64
		var domain, name, gscURL, gaID, yandexID, status, createdAt, ownerName, deptName, groupName string
		var lastAuditStatus, lastAuditTime *string
		if err := rows.Scan(&id, &domain, &name, &gscURL, &gaID, &yandexID, &status, &createdAt,
			&createdBy, &ownerName, &deptName, &groupName,
			&lastAuditStatus, &lastAuditTime); err != nil {
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
			"created_by":        createdBy,
			"owner_name":        ownerName,
			"department_name":   deptName,
			"group_name":        groupName,
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
	var id int64
	err := db.DB.QueryRow(
		`INSERT INTO sites (domain, name, gsc_site_url, ga_property_id, yandex_host_id, bing_site_url,
		 url_category, url_detail, url_news_list, url_news_detail, url_search, url_static,
		 url_pages_json, audit_schedule_enabled, audit_interval_hours, audit_daily_hour, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING id`,
		req.Domain, req.Name, req.GscSiteURL, req.GaPropertyID, req.YandexHostID, req.BingSiteURL,
		req.URLCategory, req.URLDetail, req.URLNewsList, req.URLNewsDetail, req.URLSearch, req.URLStatic,
		req.URLPagesJSON, req.AuditScheduleEnabled, req.AuditIntervalHours, req.AuditDailyHour, userID,
	).Scan(&id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "你已有一个该域名的站点"})
		return
	}

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
		BrandRegex           string `json:"brand_regex"`
		SiteLanguage         string `json:"site_language"`
		PrimaryCountry       string `json:"primary_country"`
		SiteType             string `json:"site_type"`
		CreatedAt            string `json:"created_at"`
	}

	err := db.DB.QueryRow(`SELECT id, domain, name, gsc_site_url, ga_property_id, yandex_host_id,
		url_category, url_detail, url_news_list, url_news_detail, url_search, url_static,
		status, COALESCE(audit_schedule_enabled,'false'), COALESCE(audit_interval_hours,0), COALESCE(audit_daily_hour,-1),
		COALESCE(brand_regex,''), COALESCE(site_language,'zh'), COALESCE(primary_country,'chn'), COALESCE(site_type,''),
		created_at FROM sites WHERE id = $1`, id).Scan(
		&site.ID, &site.Domain, &site.Name, &site.GscSiteURL, &site.GaPropertyID, &site.YandexHostID,
		&site.URLCategory, &site.URLDetail, &site.URLNewsList, &site.URLNewsDetail,
		&site.URLSearch, &site.URLStatic, &site.Status,
		&site.AuditScheduleEnabled, &site.AuditIntervalHours, &site.AuditDailyHour,
		&site.BrandRegex, &site.SiteLanguage, &site.PrimaryCountry, &site.SiteType,
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
		`UPDATE sites SET domain=$1, name=$2, gsc_site_url=$3, ga_property_id=$4, yandex_host_id=$5, bing_site_url=$6,
		 url_category=$7, url_detail=$8, url_news_list=$9, url_news_detail=$10, url_search=$11, url_static=$12,
		 audit_schedule_enabled=$13, audit_interval_hours=$14, audit_daily_hour=$15,
		 brand_regex=$16, site_language=$17, primary_country=$18, site_type=$19
		 WHERE id=$20`,
		req.Domain, req.Name, req.GscSiteURL, req.GaPropertyID, req.YandexHostID, req.BingSiteURL,
		req.URLCategory, req.URLDetail, req.URLNewsList, req.URLNewsDetail, req.URLSearch, req.URLStatic,
		req.AuditScheduleEnabled, req.AuditIntervalHours, req.AuditDailyHour,
		req.BrandRegex, req.SiteLanguage, req.PrimaryCountry, req.SiteType, id,
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
		// Platform data
		{"DELETE FROM dead_link_results WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM external_links WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM site_pages WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM gsc_data WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM ga_data WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM bing_data WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM yandex_data WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM spider_logs WHERE site_id = $1", []interface{}{id}},
		// Audit cascade (results/pages reference tasks)
		{"DELETE FROM audit_results WHERE task_id IN (SELECT id FROM audit_tasks WHERE site_id = $1)", []interface{}{id}},
		{"DELETE FROM audit_pages   WHERE task_id IN (SELECT id FROM audit_tasks WHERE site_id = $1)", []interface{}{id}},
		{"DELETE FROM audit_tasks WHERE site_id = $1", []interface{}{id}},
		// Coordination cascade (events/attachments/test_pages reference tasks)
		{"DELETE FROM coord_test_pages WHERE task_id IN (SELECT id FROM coordination_tasks WHERE site_id = $1)", []interface{}{id}},
		{"DELETE FROM coordination_attachments WHERE task_id IN (SELECT id FROM coordination_tasks WHERE site_id = $1)", []interface{}{id}},
		{"DELETE FROM coordination_events WHERE task_id IN (SELECT id FROM coordination_tasks WHERE site_id = $1)", []interface{}{id}},
		{"DELETE FROM coordination_tasks WHERE site_id = $1", []interface{}{id}},
		// GEO content cascade (titles reference title_libs)
		{"DELETE FROM geo_workflow_jobs WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM geo_articles WHERE site_id = $1", []interface{}{id}},
		{"DELETE FROM geo_titles WHERE lib_id IN (SELECT id FROM geo_title_libs WHERE site_id = $1)", []interface{}{id}},
		{"DELETE FROM geo_title_libs WHERE site_id = $1", []interface{}{id}},
		// Finally, the site itself
		{"DELETE FROM sites WHERE id = $1", []interface{}{id}},
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
