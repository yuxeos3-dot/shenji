package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

// --- Platform push functions ---

// pushSitemapToGSC submits a sitemap to Google Search Console.
func pushSitemapToGSC(userID int64, siteURL, sitemapURL string) (string, error) {
	token, err := getGoogleAccessToken(userID)
	if err != nil {
		return "", fmt.Errorf("GSC token error: %w", err)
	}

	apiURL := fmt.Sprintf("https://www.googleapis.com/webmasters/v3/sites/%s/sitemaps/%s",
		url.PathEscape(siteURL), url.PathEscape(sitemapURL))

	req, err := http.NewRequest("PUT", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpx.Outbound(30 * time.Second).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("GSC API %d: %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode), nil
}

// pushSitemapToBing submits a sitemap to Bing Webmaster.
func pushSitemapToBing(userID int64, siteURL, sitemapURL string) (string, error) {
	apiKey, err := LoadUserBingKey(userID)
	if err != nil || apiKey == "" {
		return "", fmt.Errorf("Bing API key not configured")
	}

	apiURL := fmt.Sprintf("https://ssl.bing.com/webmaster/api.svc/json/SubmitFeed?apikey=%s&siteUrl=%s&feedUrl=%s",
		url.QueryEscape(apiKey), url.QueryEscape(siteURL), url.QueryEscape(sitemapURL))

	resp, err := httpx.Outbound(20 * time.Second).Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Bing API %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

// pushSitemapToYandex submits a sitemap to Yandex Webmaster.
func pushSitemapToYandex(userID int64, yandexUID, hostID, sitemapURL string) (string, error) {
	token, err := getYandexAccessToken(userID)
	if err != nil {
		return "", fmt.Errorf("Yandex token error: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.webmaster.yandex.net/v4/user/%s/hosts/%s/sitemaps",
		url.PathEscape(yandexUID), url.PathEscape(hostID))

	payload, _ := json.Marshal(map[string]string{"url": sitemapURL})
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpx.Outbound(15 * time.Second).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Yandex API %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

// pushURLsToGSC submits URLs to Google Indexing API.
func pushURLsToGSC(userID int64, urls []string) (int, int, error) {
	token, err := getGoogleAccessToken(userID)
	if err != nil {
		return 0, 0, err
	}

	success, failed := 0, 0
	for _, u := range urls {
		payload, _ := json.Marshal(map[string]string{"url": u, "type": "URL_UPDATED"})
		req, err := http.NewRequest("POST",
			"https://indexing.googleapis.com/v3/urlNotifications:publish",
			bytes.NewReader(payload))
		if err != nil {
			failed++
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpx.Outbound(10 * time.Second).Do(req)
		if err != nil {
			failed++
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			success++
		} else {
			failed++
		}
	}
	return success, failed, nil
}

// pushURLsToBing submits URLs to Bing URL Submission API (batch).
func pushURLsToBing(userID int64, siteURL string, urls []string) (string, error) {
	apiKey, err := LoadUserBingKey(userID)
	if err != nil || apiKey == "" {
		return "", fmt.Errorf("Bing API key not configured")
	}

	if len(urls) > 500 {
		urls = urls[:500]
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"siteUrl": siteURL,
		"urlList": urls,
	})

	apiURL := "https://ssl.bing.com/webmaster/api.svc/json/SubmitUrlBatch?apikey=" + url.QueryEscape(apiKey)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpx.Outbound(30 * time.Second).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Bing API %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

// pushURLsToYandex submits URLs for recrawl to Yandex.
func pushURLsToYandex(userID int64, yandexUID, hostID string, urls []string) (int, int, error) {
	token, err := getYandexAccessToken(userID)
	if err != nil {
		return 0, 0, err
	}

	success, failed := 0, 0
	for _, u := range urls {
		apiURL := fmt.Sprintf("https://api.webmaster.yandex.net/v4/user/%s/hosts/%s/recrawl/queue",
			url.PathEscape(yandexUID), url.PathEscape(hostID))
		payload, _ := json.Marshal(map[string]string{"url": u})
		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
		if err != nil {
			failed++
			continue
		}
		req.Header.Set("Authorization", "OAuth "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpx.Outbound(10 * time.Second).Do(req)
		if err != nil {
			failed++
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			success++
		} else {
			failed++
		}
	}
	return success, failed, nil
}

// --- Helper: resolve platform identifiers ---

type platformIDs struct {
	gscProp   string
	bingSite  string
	yandexUID string
	yandexHID string
}

func resolvePlatformIDs(userID int64, domain, platform string) (platformIDs, error) {
	var ids platformIDs
	switch platform {
	case "gsc":
		token, err := getGoogleAccessToken(userID)
		if err != nil {
			return ids, err
		}
		prop, err := resolveGSCProperty(token, domain, userID)
		if err != nil {
			return ids, err
		}
		ids.gscProp = prop
	case "bing":
		key, err := LoadUserBingKey(userID)
		if err != nil || key == "" {
			return ids, fmt.Errorf("Bing API key not configured")
		}
		site, err := resolveBingSiteURL(key, domain, userID)
		if err != nil {
			return ids, err
		}
		ids.bingSite = site
	case "yandex":
		token, err := getYandexAccessToken(userID)
		if err != nil {
			return ids, err
		}
		yUID, err := getYandexUserIDCached(userID, token)
		if err != nil {
			return ids, err
		}
		hid, err := resolveYandexHost(token, yUID, domain, userID)
		if err != nil {
			return ids, err
		}
		ids.yandexUID = yUID
		ids.yandexHID = hid
	}
	return ids, nil
}

// --- HTTP Handlers ---

// SubmitSitemap handles POST /sites/:id/push/sitemap
func SubmitSitemap(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	userID := c.GetInt64("user_id")

	var body struct {
		SitemapURL string   `json:"sitemap_url" binding:"required"`
		Platforms  []string `json:"platforms" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", siteID).Scan(&domain); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	results := make(map[string]interface{})

	for _, platform := range body.Platforms {
		var resp string
		var pushErr error

		ids, err := resolvePlatformIDs(userID, domain, platform)
		if err != nil {
			pushErr = err
		} else {
			switch platform {
			case "gsc":
				resp, pushErr = pushSitemapToGSC(userID, ids.gscProp, body.SitemapURL)
			case "bing":
				resp, pushErr = pushSitemapToBing(userID, ids.bingSite, body.SitemapURL)
			case "yandex":
				resp, pushErr = pushSitemapToYandex(userID, ids.yandexUID, ids.yandexHID, body.SitemapURL)
			default:
				pushErr = fmt.Errorf("不支持的平台: %s", platform)
			}
		}

		status := "success"
		errMsg := ""
		if pushErr != nil {
			status = "failed"
			errMsg = pushErr.Error()
		}

		db.DB.Exec(
			`INSERT INTO push_logs (site_id, user_id, platform, push_type, target_url, status, response, error_msg)
			 VALUES ($1, $2, $3, 'sitemap', $4, $5, $6, $7)`,
			siteID, userID, platform, body.SitemapURL, status, resp, errMsg)

		results[platform] = gin.H{"status": status, "error": errMsg, "response": resp}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": results})
}

// SubmitSitemapAll handles POST /push/sitemap-all
func SubmitSitemapAll(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var body struct {
		SitemapPath string   `json:"sitemap_path" binding:"required"`
		Platforms   []string `json:"platforms" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	if !strings.HasPrefix(body.SitemapPath, "/") {
		body.SitemapPath = "/" + body.SitemapPath
	}

	rows, err := db.DB.Query("SELECT id, domain FROM sites WHERE created_by = $1 AND status = 'active'", userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询站点失败"})
		return
	}
	defer rows.Close()

	type siteRow struct {
		id     string
		domain string
	}
	var sites []siteRow
	for rows.Next() {
		var s siteRow
		if err := rows.Scan(&s.id, &s.domain); err == nil {
			sites = append(sites, s)
		}
	}

	allResults := make(map[string]interface{})

	for _, site := range sites {
		sitemapURL := "https://" + site.domain + body.SitemapPath
		siteResults := make(map[string]interface{})

		for _, platform := range body.Platforms {
			var resp string
			var pushErr error

			ids, err := resolvePlatformIDs(userID, site.domain, platform)
			if err != nil {
				pushErr = err
			} else {
				switch platform {
				case "gsc":
					resp, pushErr = pushSitemapToGSC(userID, ids.gscProp, sitemapURL)
				case "bing":
					resp, pushErr = pushSitemapToBing(userID, ids.bingSite, sitemapURL)
				case "yandex":
					resp, pushErr = pushSitemapToYandex(userID, ids.yandexUID, ids.yandexHID, sitemapURL)
				default:
					pushErr = fmt.Errorf("不支持的平台: %s", platform)
				}
			}

			status := "success"
			errMsg := ""
			if pushErr != nil {
				status = "failed"
				errMsg = pushErr.Error()
			}

			db.DB.Exec(
				`INSERT INTO push_logs (site_id, user_id, platform, push_type, target_url, status, response, error_msg)
				 VALUES ($1, $2, $3, 'sitemap', $4, $5, $6, $7)`,
				site.id, userID, platform, sitemapURL, status, resp, errMsg)

			siteResults[platform] = gin.H{"status": status, "error": errMsg}
		}

		allResults[site.domain] = siteResults
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": allResults})
}

// SubmitURLs handles POST /sites/:id/push/urls
func SubmitURLs(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	userID := c.GetInt64("user_id")

	var body struct {
		URLs      []string `json:"urls" binding:"required"`
		Platforms []string `json:"platforms" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	if len(body.URLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "URL 列表不能为空"})
		return
	}
	if len(body.URLs) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "单次最多提交 500 个 URL"})
		return
	}

	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", siteID).Scan(&domain); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	results := make(map[string]interface{})

	for _, platform := range body.Platforms {
		var pushErr error
		var resp string
		success, failed := 0, 0

		ids, err := resolvePlatformIDs(userID, domain, platform)
		if err != nil {
			pushErr = err
		} else {
			switch platform {
			case "gsc":
				success, failed, pushErr = pushURLsToGSC(userID, body.URLs)
				resp = fmt.Sprintf("success=%d, failed=%d", success, failed)
			case "bing":
				resp, pushErr = pushURLsToBing(userID, ids.bingSite, body.URLs)
				if pushErr == nil {
					success = len(body.URLs)
				} else {
					failed = len(body.URLs)
				}
			case "yandex":
				success, failed, pushErr = pushURLsToYandex(userID, ids.yandexUID, ids.yandexHID, body.URLs)
				resp = fmt.Sprintf("success=%d, failed=%d", success, failed)
			default:
				pushErr = fmt.Errorf("不支持的平台: %s", platform)
			}
		}

		status := "success"
		errMsg := ""
		if pushErr != nil {
			status = "failed"
			errMsg = pushErr.Error()
		}

		urlsJSON, _ := json.Marshal(body.URLs)
		targetURL := string(urlsJSON)
		if len(targetURL) > 4096 {
			targetURL = targetURL[:4096]
		}

		db.DB.Exec(
			`INSERT INTO push_logs (site_id, user_id, platform, push_type, target_url, status, response, error_msg)
			 VALUES ($1, $2, $3, 'urls', $4, $5, $6, $7)`,
			siteID, userID, platform, targetURL, status, resp, errMsg)

		results[platform] = gin.H{
			"status":  status,
			"error":   errMsg,
			"success": success,
			"failed":  failed,
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": results})
}

// ListPushLogs handles GET /sites/:id/push/logs
func ListPushLogs(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	platform := c.Query("platform")
	pushType := c.Query("push_type")

	where := "WHERE site_id = $1"
	args := []interface{}{siteID}
	argIdx := 2

	if platform != "" {
		where += fmt.Sprintf(" AND platform = $%d", argIdx)
		args = append(args, platform)
		argIdx++
	}
	if pushType != "" {
		where += fmt.Sprintf(" AND push_type = $%d", argIdx)
		args = append(args, pushType)
		argIdx++
	}

	var total int
	countSQL := "SELECT COUNT(*) FROM push_logs " + where
	db.DB.QueryRow(countSQL, args...).Scan(&total)

	querySQL := fmt.Sprintf(
		"SELECT id, site_id, user_id, platform, push_type, target_url, status, response, error_msg, created_at FROM push_logs %s ORDER BY id DESC LIMIT $%d OFFSET $%d",
		where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.DB.Query(querySQL, args...)
	if err != nil {
		log.Printf("[push] list logs query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询推送日志失败"})
		return
	}
	defer rows.Close()

	type pushLogRow struct {
		ID        int64  `json:"id"`
		SiteID    int64  `json:"site_id"`
		UserID    int64  `json:"user_id"`
		Platform  string `json:"platform"`
		PushType  string `json:"push_type"`
		TargetURL string `json:"target_url"`
		Status    string `json:"status"`
		Response  string `json:"response"`
		ErrorMsg  string `json:"error_msg"`
		CreatedAt string `json:"created_at"`
	}

	var logs []pushLogRow
	for rows.Next() {
		var r pushLogRow
		if err := rows.Scan(&r.ID, &r.SiteID, &r.UserID, &r.Platform, &r.PushType, &r.TargetURL, &r.Status, &r.Response, &r.ErrorMsg, &r.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, r)
	}
	if logs == nil {
		logs = []pushLogRow{}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     logs,
		},
	})
}

// ListPushSchedules handles GET /sites/:id/push/schedules
func ListPushSchedules(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	rows, err := db.DB.Query(
		`SELECT id, site_id, user_id, platform, push_type, target_url, cron_expr, enabled, last_run_at, created_at, updated_at
		 FROM push_schedules WHERE site_id = $1 ORDER BY id DESC`, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询推送计划失败"})
		return
	}
	defer rows.Close()

	type scheduleRow struct {
		ID        int64  `json:"id"`
		SiteID    int64  `json:"site_id"`
		UserID    int64  `json:"user_id"`
		Platform  string `json:"platform"`
		PushType  string `json:"push_type"`
		TargetURL string `json:"target_url"`
		CronExpr  string `json:"cron_expr"`
		Enabled   bool   `json:"enabled"`
		LastRunAt string `json:"last_run_at"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var schedules []scheduleRow
	for rows.Next() {
		var r scheduleRow
		var lastRunAt, updatedAt *string
		if err := rows.Scan(&r.ID, &r.SiteID, &r.UserID, &r.Platform, &r.PushType,
			&r.TargetURL, &r.CronExpr, &r.Enabled, &lastRunAt, &r.CreatedAt, &updatedAt); err != nil {
			continue
		}
		if lastRunAt != nil {
			r.LastRunAt = *lastRunAt
		}
		if updatedAt != nil {
			r.UpdatedAt = *updatedAt
		}
		schedules = append(schedules, r)
	}
	if schedules == nil {
		schedules = []scheduleRow{}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": schedules})
}

// SavePushSchedule handles POST /sites/:id/push/schedules
func SavePushSchedule(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	userID := c.GetInt64("user_id")

	var body struct {
		ID        int64  `json:"id"`
		Platform  string `json:"platform" binding:"required"`
		PushType  string `json:"push_type" binding:"required"`
		TargetURL string `json:"target_url" binding:"required"`
		CronExpr  string `json:"cron_expr" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	if body.Platform != "gsc" && body.Platform != "bing" && body.Platform != "yandex" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "platform 必须是 gsc / bing / yandex"})
		return
	}
	if body.PushType != "sitemap" && body.PushType != "urls" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "push_type 必须是 sitemap / urls"})
		return
	}

	if body.ID > 0 {
		_, err := db.DB.Exec(
			`UPDATE push_schedules SET platform = $1, push_type = $2, target_url = $3, cron_expr = $4, updated_at = CURRENT_TIMESTAMP
			 WHERE id = $5 AND site_id = $6`,
			body.Platform, body.PushType, body.TargetURL, body.CronExpr, body.ID, siteID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新推送计划失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": body.ID, "message": "已更新"}})
	} else {
		var id int64
		err := db.DB.QueryRow(
			`INSERT INTO push_schedules (site_id, user_id, platform, push_type, target_url, cron_expr, enabled)
			 VALUES ($1, $2, $3, $4, $5, $6, true) RETURNING id`,
			siteID, userID, body.Platform, body.PushType, body.TargetURL, body.CronExpr).Scan(&id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建推送计划失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id, "message": "已创建"}})
	}
}

// DeletePushSchedule handles DELETE /push/schedules/:id
func DeletePushSchedule(c *gin.Context) {
	scheduleID := c.Param("id")
	userID := c.GetInt64("user_id")
	role, _ := c.Get("role")

	var ownerID int64
	err := db.DB.QueryRow("SELECT user_id FROM push_schedules WHERE id = $1", scheduleID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "推送计划不存在"})
		return
	}
	if role != "admin" && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权删除此推送计划"})
		return
	}

	if _, err := db.DB.Exec("DELETE FROM push_schedules WHERE id = $1", scheduleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除推送计划失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}

// TogglePushSchedule handles PUT /push/schedules/:id/toggle
func TogglePushSchedule(c *gin.Context) {
	scheduleID := c.Param("id")
	userID := c.GetInt64("user_id")
	role, _ := c.Get("role")

	var ownerID int64
	var enabled bool
	err := db.DB.QueryRow("SELECT user_id, enabled FROM push_schedules WHERE id = $1", scheduleID).Scan(&ownerID, &enabled)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "推送计划不存在"})
		return
	}
	if role != "admin" && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权操作此推送计划"})
		return
	}

	newEnabled := !enabled
	if _, err := db.DB.Exec("UPDATE push_schedules SET enabled = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		newEnabled, scheduleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新推送计划失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"enabled": newEnabled}})
}

// --- Scheduler helper (called from internal/scheduler) ---

// RunScheduledPush executes a single push schedule. Exported so the
// scheduler package can call it.
func RunScheduledPush(schedID int64, siteID int64, userID int64, platform, pushType, targetURL string) {
	var domain string
	if err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", siteID).Scan(&domain); err != nil {
		log.Printf("[push-scheduler] site %d not found: %v", siteID, err)
		return
	}

	var resp string
	var pushErr error

	ids, err := resolvePlatformIDs(userID, domain, platform)
	if err != nil {
		pushErr = err
	} else {
		switch pushType {
		case "sitemap":
			switch platform {
			case "gsc":
				resp, pushErr = pushSitemapToGSC(userID, ids.gscProp, targetURL)
			case "bing":
				resp, pushErr = pushSitemapToBing(userID, ids.bingSite, targetURL)
			case "yandex":
				resp, pushErr = pushSitemapToYandex(userID, ids.yandexUID, ids.yandexHID, targetURL)
			}
		case "urls":
			var urls []string
			if jsonErr := json.Unmarshal([]byte(targetURL), &urls); jsonErr != nil {
				pushErr = fmt.Errorf("URL list parse error: %w", jsonErr)
			} else {
				switch platform {
				case "gsc":
					s, f, e := pushURLsToGSC(userID, urls)
					pushErr = e
					resp = fmt.Sprintf("success=%d, failed=%d", s, f)
				case "bing":
					resp, pushErr = pushURLsToBing(userID, ids.bingSite, urls)
				case "yandex":
					s, f, e := pushURLsToYandex(userID, ids.yandexUID, ids.yandexHID, urls)
					pushErr = e
					resp = fmt.Sprintf("success=%d, failed=%d", s, f)
				}
			}
		}
	}

	status := "success"
	errMsg := ""
	if pushErr != nil {
		status = "failed"
		errMsg = pushErr.Error()
	}

	db.DB.Exec(
		`INSERT INTO push_logs (site_id, user_id, platform, push_type, target_url, status, response, error_msg)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		siteID, userID, platform, pushType, targetURL, status, resp, errMsg)

	db.DB.Exec("UPDATE push_schedules SET last_run_at = CURRENT_TIMESTAMP WHERE id = $1", schedID)

	if pushErr != nil {
		log.Printf("[push-scheduler] schedule %d failed: %v", schedID, pushErr)
	} else {
		log.Printf("[push-scheduler] schedule %d completed: %s %s/%s", schedID, platform, pushType, status)
	}
}
