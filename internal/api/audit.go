package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"seo-system/internal/ai"
	"seo-system/internal/crawler"
	"seo-system/internal/db"
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
)

func TriggerAudit(c *gin.Context) {
	siteID := c.Param("id")
	userID := c.GetInt64("user_id")

	// Check site exists
	var domain, name, urlCat, urlDet, urlNL, urlND, urlSearch, urlStatic string
	err := db.DB.QueryRow(
		"SELECT domain, name, url_category, url_detail, url_news_list, url_news_detail, url_search, url_static FROM sites WHERE id = ?",
		siteID,
	).Scan(&domain, &name, &urlCat, &urlDet, &urlNL, &urlND, &urlSearch, &urlStatic)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	result, err := db.DB.Exec(
		"INSERT INTO audit_tasks (site_id, triggered_by, status) VALUES (?, ?, 'running')",
		siteID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建任务失败"})
		return
	}

	taskID, _ := result.LastInsertId()

	// Run audit asynchronously
	go runAuditTask(taskID, siteID, crawler.SiteConfig{
		Domain:       domain,
		URLCategory:  urlCat,
		URLDetail:    urlDet,
		URLNewsList:  urlNL,
		URLNewsDetail: urlND,
		URLSearch:    urlSearch,
		URLStatic:   urlStatic,
	}, name)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"task_id": taskID}})
}

func runAuditTask(taskID int64, siteID string, site crawler.SiteConfig, siteName string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Audit task %d panicked: %v", taskID, r)
			db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
				"internal panic", time.Now(), taskID)
		}
	}()

	updateProgress := func(progress int) {
		db.DB.Exec("UPDATE audit_tasks SET progress=? WHERE id=?", progress, taskID)
	}

	// Step 1: Crawl standard pages
	updateProgress(10)
	crawlPages := crawler.CrawlAllPages(site)
	updateProgress(35)

	// Save crawled pages
	for _, p := range crawlPages {
		db.DB.Exec(
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		)
	}
	updateProgress(45)

	// Step 2: Fetch site_pages (user-managed pages with pre-fetched HTML)
	var sitePages []ai.SitePageForPrompt
	rows, err := db.DB.Query(
		"SELECT name, url, last_html, last_fetched_at, status_code, ttfb_ms FROM site_pages WHERE site_id = ? AND last_html != ''",
		siteID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p ai.SitePageForPrompt
			var fetchedAt *string
			if err := rows.Scan(&p.Name, &p.URL, &p.HTML, &fetchedAt, &p.StatusCode, &p.TTFB); err == nil {
				if fetchedAt != nil {
					p.LastFetchedAt = *fetchedAt
				}
				sitePages = append(sitePages, p)
			}
		}
	}
	updateProgress(55)

	// Step 3: Fetch analytics data
	var gscData *ai.GSCDataForPrompt
	var gaData *ai.GADataForPrompt
	var yandexData *ai.YandexDataForPrompt

	var gscClicks, gscImpressions int
	var gscCTR, gscPosition float64
	var gscKeywords, gscPages string
	if err := db.DB.QueryRow(
		"SELECT clicks, impressions, ctr, position, keywords_json, pages_json FROM gsc_data WHERE site_id = ? ORDER BY date DESC LIMIT 1",
		siteID,
	).Scan(&gscClicks, &gscImpressions, &gscCTR, &gscPosition, &gscKeywords, &gscPages); err == nil {
		gscData = &ai.GSCDataForPrompt{
			Clicks: gscClicks, Impressions: gscImpressions,
			CTR: gscCTR, Position: gscPosition,
			Keywords: gscKeywords, Pages: gscPages,
		}
	}

	var gaSessions, gaUsers, gaPageviews int
	var gaChannels string
	if err := db.DB.QueryRow(
		"SELECT sessions, users, pageviews, channels_json FROM ga_data WHERE site_id = ? ORDER BY date DESC LIMIT 1",
		siteID,
	).Scan(&gaSessions, &gaUsers, &gaPageviews, &gaChannels); err == nil {
		gaData = &ai.GADataForPrompt{
			Sessions: gaSessions, Users: gaUsers, Pageviews: gaPageviews, Channels: gaChannels,
		}
	}

	var yandexIndexCount int
	var yandexErrors string
	if err := db.DB.QueryRow(
		"SELECT index_count, errors_json FROM yandex_data WHERE site_id = ? ORDER BY date DESC LIMIT 1",
		siteID,
	).Scan(&yandexIndexCount, &yandexErrors); err == nil {
		yandexData = &ai.YandexDataForPrompt{IndexCount: yandexIndexCount, Errors: yandexErrors}
	}

	updateProgress(60)

	// Step 4: Build v6 prompt and call AI
	prompt := ai.BuildV6AuditPrompt(site.Domain, sitePages, crawlPages, gscData, gaData, yandexData)
	updateProgress(65)

	analysisResult, model, err := ai.CallAI(prompt)
	if err != nil {
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			err.Error(), time.Now(), taskID)
		telegram.SendNotification("审计失败", siteName+" 审计失败: "+err.Error())
		return
	}
	updateProgress(90)

	// Step 5: Save results (including v6_report)
	seoJSON, _ := json.Marshal(analysisResult.SEOIssues)
	codeJSON, _ := json.Marshal(analysisResult.CodeIssues)
	secJSON, _ := json.Marshal(analysisResult.SecurityIssues)
	perfJSON, _ := json.Marshal(analysisResult.PerfIssues)

	db.DB.Exec(
		`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, string(seoJSON), string(codeJSON), string(secJSON), string(perfJSON),
		analysisResult.Suggestions, analysisResult.Summary, model, analysisResult.V6Report,
	)

	db.DB.Exec("UPDATE audit_tasks SET status='done', progress=100, finished_at=? WHERE id=?", time.Now(), taskID)
	telegram.SendNotification("审计完成", siteName+" 审计完成: "+analysisResult.Summary)
}

func ListAudits(c *gin.Context) {
	siteID := c.Param("id")
	rows, err := db.DB.Query(
		"SELECT id, site_id, triggered_by, status, progress, error_msg, created_at, finished_at FROM audit_tasks WHERE site_id = ? ORDER BY id DESC",
		siteID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var audits []gin.H
	for rows.Next() {
		var id, siteIDVal, triggeredBy int64
		var status string
		var progress int
		var errorMsg, createdAt string
		var finishedAt *string
		if err := rows.Scan(&id, &siteIDVal, &triggeredBy, &status, &progress, &errorMsg, &createdAt, &finishedAt); err != nil {
			continue
		}
		audits = append(audits, gin.H{
			"id":           id,
			"site_id":      siteIDVal,
			"triggered_by": triggeredBy,
			"status":       status,
			"progress":     progress,
			"error_msg":    errorMsg,
			"created_at":   createdAt,
			"finished_at":  finishedAt,
		})
	}
	if audits == nil {
		audits = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": audits})
}

func GetAudit(c *gin.Context) {
	taskID := c.Param("task_id")

	// Get task info
	var id, siteID, triggeredBy int64
	var status string
	var progress int
	var errorMsg, createdAt string
	var finishedAt *string
	err := db.DB.QueryRow(
		"SELECT id, site_id, triggered_by, status, progress, error_msg, created_at, finished_at FROM audit_tasks WHERE id = ?",
		taskID,
	).Scan(&id, &siteID, &triggeredBy, &status, &progress, &errorMsg, &createdAt, &finishedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}

	task := gin.H{
		"id":           id,
		"site_id":      siteID,
		"triggered_by": triggeredBy,
		"status":       status,
		"progress":     progress,
		"error_msg":    errorMsg,
		"created_at":   createdAt,
		"finished_at":  finishedAt,
	}

	// Get results if exists
	var seoIssues, codeIssues, secIssues, perfIssues, suggestions, summary, aiModel, v6Report string
	err = db.DB.QueryRow(
		"SELECT seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, COALESCE(v6_report,'') FROM audit_results WHERE task_id = ?",
		taskID,
	).Scan(&seoIssues, &codeIssues, &secIssues, &perfIssues, &suggestions, &summary, &aiModel, &v6Report)

	var resultData interface{}
	if err == nil {
		resultData = gin.H{
			"seo_issues":      json.RawMessage(seoIssues),
			"code_issues":     json.RawMessage(codeIssues),
			"security_issues": json.RawMessage(secIssues),
			"perf_issues":     json.RawMessage(perfIssues),
			"suggestions":     suggestions,
			"summary":         summary,
			"ai_model":        aiModel,
			"v6_report":       v6Report,
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"task": task, "result": resultData}})
}

func GetAuditPages(c *gin.Context) {
	taskID := c.Param("task_id")
	rows, err := db.DB.Query(
		"SELECT id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg, fetched_at FROM audit_pages WHERE task_id = ?",
		taskID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var pages []gin.H
	for rows.Next() {
		var id int64
		var pageType, url, html, errorMsg, fetchedAt string
		var statusCode, ttfb, htmlSize int
		if err := rows.Scan(&id, &pageType, &url, &statusCode, &ttfb, &html, &htmlSize, &errorMsg, &fetchedAt); err != nil {
			continue
		}
		pages = append(pages, gin.H{
			"id":          id,
			"page_type":   pageType,
			"url":         url,
			"status_code": statusCode,
			"ttfb_ms":     ttfb,
			"html":        html,
			"html_size":   htmlSize,
			"error_msg":   errorMsg,
			"fetched_at":  fetchedAt,
		})
	}
	if pages == nil {
		pages = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": pages})
}

func GetAuditPageByType(c *gin.Context) {
	taskID := c.Param("task_id")
	pageType := c.Param("type")

	var id int64
	var pt, url, html, errorMsg, fetchedAt string
	var statusCode, ttfb, htmlSize int
	err := db.DB.QueryRow(
		"SELECT id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg, fetched_at FROM audit_pages WHERE task_id = ? AND page_type = ?",
		taskID, pageType,
	).Scan(&id, &pt, &url, &statusCode, &ttfb, &html, &htmlSize, &errorMsg, &fetchedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "页面不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"id": id, "page_type": pt, "url": url, "status_code": statusCode,
		"ttfb_ms": ttfb, "html": html, "html_size": htmlSize,
		"error_msg": errorMsg, "fetched_at": fetchedAt,
	}})
}
