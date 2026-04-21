package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"seo-system/internal/ai"
	"seo-system/internal/crawler"
	"seo-system/internal/db"
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
)

// auditBatchFailure captures every bit of context about a single
// failed technical-audit batch so the aggregated error_msg can show
// the user exactly which batch failed, how long it took, and why.
// Used by both the manual audit path and the scheduler.
type auditBatchFailure struct {
	idx      int
	name     string
	err      error
	duration time.Duration
}

// composeBatchFailureMsg produces a detailed multi-section failure
// message for the 审计详情 banner and the Telegram push. Structure:
//
//	❌ 所有 N 个技术审计批次都失败
//	━━━━━━━━━━━━━━━━━━━━
//	AI 端点：https://api.example.com/v1
//	使用模型：claude-sonnet-4-6
//	━━━━━━━━━━━━━━━━━━━━
//	批次 1 ·「技术审计A」· 耗时 8.9s
//	❌ [classified headline]
//	供应商原文：[extracted error.message]
//	建议处理：[action]
//	原始响应片段：[trimmed raw]
//
//	批次 2 ·「技术审计B」· 耗时 9.2s
//	...
//
// This is verbose ON PURPOSE — the user's complaint was "笼统回答".
// Every bit of context the audit pipeline collected gets surfaced.
func composeBatchFailureMsg(apiURL, model string, failures []auditBatchFailure) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "❌ 所有 %d 个技术审计批次都失败，任务中止\n", len(failures))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(&sb, "🔗 AI 端点：%s\n", apiURL)
	fmt.Fprintf(&sb, "🤖 使用模型：%s\n", model)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━\n")
	for _, f := range failures {
		fmt.Fprintf(&sb, "\n▸ 批次 %d ·「%s」· 耗时 %s\n",
			f.idx, f.name, f.duration.Round(time.Millisecond))
		sb.WriteString(ai.ExplainError(f.err.Error()))
		sb.WriteString("\n")
	}
	return sb.String()
}

// explainAIError is kept as a thin wrapper for any call sites that only
// need single-error translation without the full batch context.
func explainAIError(raw string) string {
	return ai.ExplainError(raw)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TriggerAudit(c *gin.Context) {
	siteID := c.Param("id")
	userID := c.GetInt64("user_id")

	if !canAccessSite(c, siteID) {
		return
	}

	// Check site exists
	var domain, name, urlCat, urlDet, urlNL, urlND, urlSearch, urlStatic string
	err := db.DB.QueryRow(
		"SELECT domain, name, url_category, url_detail, url_news_list, url_news_detail, url_search, url_static FROM sites WHERE id = $1",
		siteID,
	).Scan(&domain, &name, &urlCat, &urlDet, &urlNL, &urlND, &urlSearch, &urlStatic)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "站点不存在"})
		return
	}

	var taskID int64
	err = db.DB.QueryRow(
		"INSERT INTO audit_tasks (site_id, triggered_by, status) VALUES ($1, $2, 'running') RETURNING id",
		siteID, userID,
	).Scan(&taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建任务失败"})
		return
	}


	// Run audit asynchronously. Pass the triggering user's id so
	// completion / failure notices go to *their* Telegram bot rather
	// than a global firehose.
	go runAuditTask(taskID, siteID, userID, crawler.SiteConfig{
		Domain:        domain,
		URLCategory:   urlCat,
		URLDetail:     urlDet,
		URLNewsList:   urlNL,
		URLNewsDetail: urlND,
		URLSearch:     urlSearch,
		URLStatic:     urlStatic,
	}, name)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"task_id": taskID}})
}

func runAuditTask(taskID int64, siteID string, ownerUserID int64, site crawler.SiteConfig, siteName string) {
	IncAuditRun()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Audit task %d panicked: %v", taskID, r)
			// Nested recover: never leave the outer goroutine without logging.
			defer func() {
				if rr := recover(); rr != nil {
					log.Printf("[audit %d] mark-failed panic: %v", taskID, rr)
				}
			}()
			if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
				"internal panic", time.Now(), taskID); err != nil {
				log.Printf("[audit %d] mark-failed exec 失败: %v", taskID, err)
			}
		}
	}()

	updateProgress := func(progress int) {
		if _, err := db.DB.Exec("UPDATE audit_tasks SET progress=$1 WHERE id=$2", progress, taskID); err != nil {
			log.Printf("[audit %d] update progress failed: %v", taskID, err)
		}
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
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		); err != nil {
			log.Printf("[audit %d] insert audit_page failed url=%s: %v", taskID, p.URL, err)
		}
	}
	updateProgress(40)

	// Step 2: Auto re-fetch all site_pages to ensure fresh HTML
	log.Printf("[audit %d] 抓取站点监控页面...", taskID)
	pageRows, err := db.DB.Query(
		"SELECT id, name, url FROM site_pages WHERE site_id = $1", siteID,
	)
	if err == nil {
		defer pageRows.Close()
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

		for i, p := range pageList {
			result := crawler.FetchPage(crawler.URLSpec{PageType: p.name, URL: p.url}, 30*time.Second)
			now := time.Now().UTC().Format("2006-01-02 15:04:05")
			if _, err := db.DB.Exec(`UPDATE site_pages SET last_html=$1, last_fetched_at=$2, status_code=$3, ttfb_ms=$4 WHERE id=$5`,
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
		"SELECT name, url, last_html, last_fetched_at, status_code, ttfb_ms FROM site_pages WHERE site_id = $1 AND last_html != ''",
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
		"SELECT clicks, impressions, ctr, position, keywords_json, pages_json FROM gsc_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1",
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
		"SELECT sessions, users, pageviews, channels_json FROM ga_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1",
		siteID,
	).Scan(&gaSessions, &gaUsers, &gaPageviews, &gaChannels); err == nil {
		gaData = &ai.GADataForPrompt{
			Sessions: gaSessions, Users: gaUsers, Pageviews: gaPageviews, Channels: gaChannels,
		}
	}

	var yandexIndexCount int
	var yandexErrors string
	if err := db.DB.QueryRow(
		"SELECT index_count, errors_json FROM yandex_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1",
		siteID,
	).Scan(&yandexIndexCount, &yandexErrors); err == nil {
		yandexData = &ai.YandexDataForPrompt{IndexCount: yandexIndexCount, Errors: yandexErrors}
	}

	updateProgress(60)

	// Resolve the triggering user's AI config. We deliberately do NOT fall
	// back to a global key — every user owns their own credentials. If the
	// user hasn't configured AI yet we fail fast so the audit doesn't burn
	// crawl + analytics work just to die at the first batch call.
	aiURL, aiKey, aiModel := ai.ResolveUserAI(ownerUserID)
	if aiKey == "" {
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			"AI 未配置：请先在「个人 AI 设置」中填写至少 1 组 API Key 后重试", time.Now(), taskID)
		log.Printf("[audit %d] user %d has no AI config, abort", taskID, ownerUserID)
		telegram.NotifyUser(ownerUserID, "审计失败", siteName+": 你尚未配置 AI API Key")
		return
	}
	// Step 4: Run technical audit agents IN PARALLEL. Each agent focuses on
	// specific categories — shorter prompts, deeper analysis, faster total time.
	type agentResult struct {
		idx      int
		name     string
		chapters []ai.V6Chapter
		raw      string
		model    string
		err      error
		duration time.Duration
	}

	agentCount := len(ai.TechnicalBatches)
	results := make([]agentResult, agentCount)
	var wg sync.WaitGroup

	for i, batch := range ai.TechnicalBatches {
		wg.Add(1)
		go func(idx int, b ai.TechBatch) {
			defer wg.Done()
			start := time.Now()
			log.Printf("[audit %d] Agent %d/%d 启动：%s", taskID, idx+1, agentCount, b.Name)
			bp := ai.BuildTechPromptBatch(b, site.Domain, sitePages, crawlPages, gscData, gaData, yandexData)
			chs, raw, model, err := ai.CallAIChaptersWithConfig(aiURL, aiKey, aiModel, bp)
			dur := time.Since(start)
			results[idx] = agentResult{
				idx: idx + 1, name: b.Name,
				chapters: chs, raw: raw, model: model,
				err: err, duration: dur,
			}
			if err != nil {
				log.Printf("[audit %d] Agent %d 失败（%s）: %v", taskID, idx+1, dur.Round(time.Millisecond), err)
			} else {
				log.Printf("[audit %d] Agent %d 完成: %d 章节（%s）", taskID, idx+1, len(chs), dur.Round(time.Millisecond))
			}
			updateProgress(30 + ((idx + 1) * 50 / agentCount))
		}(i, batch)
	}
	wg.Wait()
	updateProgress(80)

	// Collect results from all agents
	allChapters := make([]ai.V6Chapter, 0, 24)
	var lastModel, lastRaw string
	var batchFailures []auditBatchFailure
	for _, r := range results {
		if r.model != "" { lastModel = r.model }
		if r.raw != "" { lastRaw = r.raw }
		if r.err != nil {
			batchFailures = append(batchFailures, auditBatchFailure{
				idx: r.idx, name: r.name, err: r.err, duration: r.duration,
			})
			continue
		}
		allChapters = append(allChapters, r.chapters...)
	}

	if len(allChapters) == 0 {
		// All batches failed — persist raw response and fail. Build a
		// detailed multi-section error message so the 审计详情 banner shows
		// the user EXACTLY which batch died with what reason, on which
		// model, against which endpoint.
		rawSnippet := lastRaw
		if len(rawSnippet) > 100000 {
			rawSnippet = rawSnippet[:100000]
		}
		db.DB.Exec(
			`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report, raw_ai_response)
			 VALUES ($1, '[]', '[]', '[]', '[]', '', '', $2, '', $3)`,
			taskID, lastModel, rawSnippet,
		)
		errMsg := composeBatchFailureMsg(aiURL, aiModel, batchFailures)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			errMsg, time.Now(), taskID)
		log.Printf("[audit %d] 所有技术审计批次均失败（%d 批次）", taskID, len(batchFailures))
		telegram.NotifyUser(ownerUserID, "审计失败", siteName+"\n"+errMsg)
		return
	}

	// Step 4c: External verification — fetch auxiliary pages, check redirects,
	// validate response headers, and sample additional pages that were not
	// in the original crawl set. This fills the blind spots of source-only audit.
	log.Printf("[audit %d] 开始外部验证抓取...", taskID)
	ev := collectExternalVerification(site.Domain, crawlPages)
	updateProgress(78)

	// Agent-9: AI analysis of external verification data
	log.Printf("[audit %d] Agent-9 外部验证分析启动", taskID)
	verifyPrompt := ai.BuildExternalVerifyPrompt(site.Domain, ev, len(allChapters))
	if verifyChs, _, _, vErr := ai.CallAIChaptersWithConfig(aiURL, aiKey, aiModel, verifyPrompt); vErr == nil {
		log.Printf("[audit %d] Agent-9 完成: %d 章节", taskID, len(verifyChs))
		allChapters = append(allChapters, verifyChs...)
	} else {
		log.Printf("[audit %d] Agent-9 失败（不影响其他结果）: %v", taskID, vErr)
	}
	updateProgress(82)


	// Step 4b: White-hat SEO strategy chapter.
	var strategyChapter *ai.V6Chapter
	log.Printf("[audit %d] 调用策略 AI 生成白帽 SEO 建议...", taskID)
	stratPrompt := ai.BuildStrategyPrompt(site.Domain, sitePages, crawlPages, gscData, gaData, yandexData)
	if strategyCh, _, sErr := ai.CallAIStrategyWithConfig(aiURL, aiKey, aiModel, stratPrompt); sErr == nil && strategyCh != nil {
		strategyChapter = strategyCh
		log.Printf("[audit %d] 策略章节已生成（%d 条建议）", taskID, len(strategyCh.Issues))
	} else {
		log.Printf("[audit %d] 策略章节生成失败（不影响技术审计）: %v", taskID, sErr)
	}
	updateProgress(85)

	// Compose final report (server-side totals, global ID renumber).
	_, v6JSON := ai.ComposeV6Report(site.Domain, allChapters, strategyChapter)

	// Build legacy fields for backwards compat (reading code still expects them).
	analysisResult := &ai.AnalysisResult{
		V6Report:       v6JSON,
		Summary:        "",
		Suggestions:    fmt.Sprintf("共 %d 章节", len(allChapters)+boolToInt(strategyChapter != nil)),
		SEOIssues:      json.RawMessage("[]"),
		CodeIssues:     json.RawMessage("[]"),
		SecurityIssues: json.RawMessage("[]"),
		PerfIssues:     json.RawMessage("[]"),
		RawResponse:    lastRaw,
	}
	model := lastModel

	// Step 5: Save results in a transaction (INSERT + UPDATE must be atomic).
	seoJSON, _ := json.Marshal(analysisResult.SEOIssues)
	codeJSON, _ := json.Marshal(analysisResult.CodeIssues)
	secJSON, _ := json.Marshal(analysisResult.SecurityIssues)
	perfJSON, _ := json.Marshal(analysisResult.PerfIssues)

	tx, txErr := db.DB.Begin()
	if txErr != nil {
		log.Printf("[audit %d] begin tx failed: %v", taskID, txErr)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			"数据库写入失败: "+txErr.Error(), time.Now(), taskID)
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		taskID, string(seoJSON), string(codeJSON), string(secJSON), string(perfJSON),
		analysisResult.Suggestions, analysisResult.Summary, model, analysisResult.V6Report,
	); err != nil {
		tx.Rollback()
		log.Printf("[audit %d] insert audit_results failed: %v", taskID, err)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			"审计结果写入失败: "+err.Error(), time.Now(), taskID)
		return
	}
	if _, err := tx.Exec("UPDATE audit_tasks SET status='done', progress=100, finished_at=$1 WHERE id=$2", time.Now(), taskID); err != nil {
		tx.Rollback()
		log.Printf("[audit %d] update audit_tasks failed: %v", taskID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[audit %d] commit tx failed: %v", taskID, err)
		return
	}

	log.Printf("[audit %d] 完成: %s", taskID, analysisResult.Summary)
	telegram.NotifyUser(ownerUserID, "审计完成", siteName+" 审计完成: "+analysisResult.Summary)
}

func DeleteAudit(c *gin.Context) {
	taskID := c.Param("task_id")
	// All checks + deletes run inside a single BEGIN IMMEDIATE transaction so
	// a concurrent writer can't flip the task to running between the read and
	// the delete.
	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	var status string
	var deleteSiteID int64
	if err := tx.QueryRow("SELECT site_id, status FROM audit_tasks WHERE id = $1", taskID).Scan(&deleteSiteID, &status); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", deleteSiteID)) {
		_ = tx.Rollback()
		return
	}
	if status == "running" {
		_ = tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "进行中的任务不能删除"})
		return
	}

	for _, q := range []string{
		"DELETE FROM audit_results WHERE task_id = $1",
		"DELETE FROM audit_pages   WHERE task_id = $1",
		"DELETE FROM audit_tasks   WHERE id = $1",
	} {
		if _, err := tx.Exec(q, taskID); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}

func ListAudits(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	page, perPage := parsePageQuery(c, 50, 500)
	offset := (page - 1) * perPage

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM audit_tasks WHERE site_id = $1", siteID).Scan(&total)

	rows, err := db.DB.Query(
		"SELECT id, site_id, triggered_by, status, progress, error_msg, created_at, finished_at FROM audit_tasks WHERE site_id = $1 ORDER BY id DESC LIMIT $2 OFFSET $3",
		siteID, perPage, offset,
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
	// Backward-compatible shape: data stays an array, pagination metadata
	// lives in response headers so existing callers keep working.
	c.Header("X-Total-Count", fmt.Sprintf("%d", total))
	c.Header("X-Page", fmt.Sprintf("%d", page))
	c.Header("X-Per-Page", fmt.Sprintf("%d", perPage))
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
		"SELECT id, site_id, triggered_by, status, progress, error_msg, created_at, finished_at FROM audit_tasks WHERE id = $1",
		taskID,
	).Scan(&id, &siteID, &triggeredBy, &status, &progress, &errorMsg, &createdAt, &finishedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", siteID)) {
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
		"SELECT seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, COALESCE(v6_report,''), COALESCE(raw_ai_response,'') FROM audit_results WHERE task_id = $1",
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
	var auditSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM audit_tasks WHERE id = $1", taskID).Scan(&auditSiteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", auditSiteID)) {
		return
	}
	rows, err := db.DB.Query(
		"SELECT id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg, fetched_at FROM audit_pages WHERE task_id = $1",
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
	var auditSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM audit_tasks WHERE id = $1", taskID).Scan(&auditSiteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", auditSiteID)) {
		return
	}

	var id int64
	var pt, url, html, errorMsg, fetchedAt string
	var statusCode, ttfb, htmlSize int
	err := db.DB.QueryRow(
		"SELECT id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg, fetched_at FROM audit_pages WHERE task_id = $1 AND page_type = $2",
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

// collectExternalVerification fetches auxiliary pages, checks redirects,
// and validates resources that source-code-only analysis cannot cover.
func collectExternalVerification(domain string, crawlPages []crawler.PageResult) *ai.ExternalVerification {
	ev := &ai.ExternalVerification{}
	timeout := 15 * time.Second
	base := "https://" + domain

	// 1. robots.txt
	r := crawler.FetchVerification(base+"/robots.txt", "robots.txt", timeout)
	ev.RobotsTxt = ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, Snippet: r.Snippet, Error: r.Error, TTFB: r.TTFB}

	// 2. sitemap.xml
	r = crawler.FetchVerification(base+"/sitemap.xml", "sitemap.xml", timeout)
	ev.SitemapXML = ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, Snippet: r.Snippet, Error: r.Error, TTFB: r.TTFB}

	// 3. HTTP redirect (no follow)
	httpURL := "http://" + domain + "/"
	r = crawler.FetchVerificationNoFollow(httpURL, "HTTP->HTTPS redirect", timeout)
	ev.HTTPRedirect = ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, RedirectTo: r.RedirectTo, Error: r.Error, TTFB: r.TTFB}

	// 4. www redirect (no follow)
	wwwURL := "https://www." + domain + "/"
	r = crawler.FetchVerificationNoFollow(wwwURL, "www redirect", timeout)
	ev.WWWRedirect = ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, RedirectTo: r.RedirectTo, Error: r.Error, TTFB: r.TTFB}

	// 5. Auxiliary pages
	auxPaths := []struct{ path, label string }{{"about", "关于我们"}, {"privacy", "隐私政策"}, {"contact", "联系我们"}, {"terms", "服务条款"}, {"dmca", "DMCA"}, {"sitemap", "站点地图页"}}
	for _, aux := range auxPaths {
		r = crawler.FetchVerification(base+"/"+aux.path, aux.label, timeout)
		ev.AuxPages = append(ev.AuxPages, ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Error: r.Error, TTFB: r.TTFB})
	}

	// 6. Sample additional pages from crawled page links
	sampled := 0
	for _, p := range crawlPages {
		if sampled >= 5 { break }
		if p.URL == "" || p.StatusCode != 200 { continue }
		r = crawler.FetchVerification(p.URL, "抽样-"+p.PageType, timeout)
		ev.SamplePages = append(ev.SamplePages, ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, Snippet: r.Snippet, TTFB: r.TTFB})
		sampled++
	}

	// 7. Resource checks
	resURLs := []struct{ url, label string }{{base + "/favicon.ico", "favicon.ico"}, {base + "/static/web/image/logo.jpg", "og:image logo"}}
	for _, res := range resURLs {
		r = crawler.FetchVerification(res.url, res.label, timeout)
		ev.Resources = append(ev.Resources, ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, Error: r.Error, TTFB: r.TTFB})
	}

	// 8. Homepage response headers
	r = crawler.FetchVerification(base+"/", "首页响应头", timeout)
	ev.ResponseHdrs = append(ev.ResponseHdrs, ai.VerifyItem{URL: r.URL, Label: r.Label, StatusCode: r.StatusCode, Headers: r.Headers, TTFB: r.TTFB})

	return ev
}
