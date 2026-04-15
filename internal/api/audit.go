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

	log.Printf("[audit %d] 开始巡检 site=%s", taskID, siteName)

	// Step 1: Crawl standard pages
	updateProgress(10)
	log.Printf("[audit %d] 抓取标准页面...", taskID)
	crawlPages := crawler.CrawlAllPages(site)
	updateProgress(30)

	// Save crawled pages
	for _, p := range crawlPages {
		if _, err := db.DB.Exec(
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		); err != nil {
			log.Printf("[audit %d] insert audit_page failed url=%s: %v", taskID, p.URL, err)
		}
	}
	updateProgress(40)

	// Step 2: Auto re-fetch all site_pages to ensure fresh HTML
	log.Printf("[audit %d] 抓取站点监控页面...", taskID)
	pageRows, err := db.DB.Query(
		"SELECT id, name, url FROM site_pages WHERE site_id = ?", siteID,
	)
	if err == nil {
		type pageInfo struct {
			id   int64
			name string
			url  string
		}
		var pageList []pageInfo
		for pageRows.Next() {
			var p pageInfo
			if err := pageRows.Scan(&p.id, &p.name, &p.url); err == nil {
				pageList = append(pageList, p)
			}
		}
		pageRows.Close()

		for i, p := range pageList {
			result := crawler.FetchPage(crawler.URLSpec{PageType: p.name, URL: p.url}, 30*time.Second)
			now := time.Now().Format("2006-01-02 15:04:05")
			if _, err := db.DB.Exec(`UPDATE site_pages SET last_html=?, last_fetched_at=?, status_code=?, ttfb_ms=? WHERE id=?`,
				result.HTML, now, result.StatusCode, result.TTFB, p.id); err != nil {
				log.Printf("[audit %d] update site_pages failed: %v", taskID, err)
			}
			progress := 40 + (i+1)*10/max(len(pageList), 1)
			if progress > 50 {
				progress = 50
			}
			updateProgress(progress)
		}
		log.Printf("[audit %d] 共抓取 %d 个监控页面", taskID, len(pageList))
	}

	// Step 3: Fetch site_pages (now freshly fetched above)
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

	// Step 4: Fetch analytics data
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
	log.Printf("[audit %d] 构建 v6 prompt (监控页面 %d)", taskID, len(sitePages))
	prompt := ai.BuildV6AuditPrompt(site.Domain, sitePages, crawlPages, gscData, gaData, yandexData)
	updateProgress(65)

	log.Printf("[audit %d] 调用 AI (prompt %d chars)...", taskID, len(prompt))
	analysisResult, model, err := ai.CallAI(prompt)
	if err != nil {
		// Persist the raw AI response (if any) so we can debug parse failures
		// without needing to re-run the expensive audit.
		rawSnippet := ""
		if analysisResult != nil {
			rawSnippet = analysisResult.RawResponse
			if len(rawSnippet) > 100000 {
				rawSnippet = rawSnippet[:100000]
			}
		}
		db.DB.Exec(
			`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report, raw_ai_response)
			 VALUES (?, '[]', '[]', '[]', '[]', '', '', ?, '', ?)`,
			taskID, model, rawSnippet,
		)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			err.Error(), time.Now(), taskID)
		log.Printf("[audit %d] AI 调用失败: %v", taskID, err)
		telegram.SendNotification("审计失败", siteName+" 审计失败: "+err.Error())
		return
	}
	updateProgress(85)

	// Step 5: Save results in a transaction (INSERT + UPDATE must be atomic).
	seoJSON, _ := json.Marshal(analysisResult.SEOIssues)
	codeJSON, _ := json.Marshal(analysisResult.CodeIssues)
	secJSON, _ := json.Marshal(analysisResult.SecurityIssues)
	perfJSON, _ := json.Marshal(analysisResult.PerfIssues)

	tx, txErr := db.DB.Begin()
	if txErr != nil {
		log.Printf("[audit %d] begin tx failed: %v", taskID, txErr)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			"数据库写入失败: "+txErr.Error(), time.Now(), taskID)
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, string(seoJSON), string(codeJSON), string(secJSON), string(perfJSON),
		analysisResult.Suggestions, analysisResult.Summary, model, analysisResult.V6Report,
	); err != nil {
		tx.Rollback()
		log.Printf("[audit %d] insert audit_results failed: %v", taskID, err)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			"审计结果写入失败: "+err.Error(), time.Now(), taskID)
		return
	}
	if _, err := tx.Exec("UPDATE audit_tasks SET status='done', progress=100, finished_at=? WHERE id=?", time.Now(), taskID); err != nil {
		tx.Rollback()
		log.Printf("[audit %d] update audit_tasks failed: %v", taskID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[audit %d] commit tx failed: %v", taskID, err)
		return
	}

	log.Printf("[audit %d] 完成: %s", taskID, analysisResult.Summary)
	telegram.SendNotification("审计完成", siteName+" 审计完成: "+analysisResult.Summary)
}

func DeleteAudit(c *gin.Context) {
	taskID := c.Param("task_id")
	// Don't allow deleting running tasks
	var status string
	db.DB.QueryRow("SELECT status FROM audit_tasks WHERE id = ?", taskID).Scan(&status)
	if status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "进行中的任务不能删除"})
		return
	}
	db.DB.Exec("DELETE FROM audit_results WHERE task_id = ?", taskID)
	db.DB.Exec("DELETE FROM audit_pages WHERE task_id = ?", taskID)
	db.DB.Exec("DELETE FROM audit_tasks WHERE id = ?", taskID)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
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
	var seoIssues, codeIssues, secIssues, perfIssues, suggestions, summary, aiModel, v6Report, rawAI string
	err = db.DB.QueryRow(
		"SELECT seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, COALESCE(v6_report,''), COALESCE(raw_ai_response,'') FROM audit_results WHERE task_id = ?",
		taskID,
	).Scan(&seoIssues, &codeIssues, &secIssues, &perfIssues, &suggestions, &summary, &aiModel, &v6Report, &rawAI)

	var resultData interface{}
	if err == nil {
		// Only expose the raw AI response when v6_report is empty (i.e. parse
		// failed) so the frontend can surface it for debugging.
		rawForAPI := ""
		if v6Report == "" && rawAI != "" {
			rawForAPI = rawAI
		}
		resultData = gin.H{
			"seo_issues":      json.RawMessage(seoIssues),
			"code_issues":     json.RawMessage(codeIssues),
			"security_issues": json.RawMessage(secIssues),
			"perf_issues":     json.RawMessage(perfIssues),
			"suggestions":     suggestions,
			"summary":         summary,
			"ai_model":        aiModel,
			"v6_report":       v6Report,
			"raw_ai_response": rawForAPI,
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


func max(a, b int) int { // nolint:unused - kept for clarity against Go builtin shadowing
	if a > b {
		return a
	}
	return b
}
