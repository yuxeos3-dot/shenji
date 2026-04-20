package scheduler

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"seo-system/internal/ai"
	"seo-system/internal/api"
	"seo-system/internal/crawler"
	"seo-system/internal/db"
	"seo-system/internal/telegram"

	"encoding/json"

	"github.com/robfig/cron/v3"
)

var c *cron.Cron

func Start() {
	c = cron.New()

	// Global cron-based auto audit
	enabled := db.GetSetting("auto_audit_enabled")
	if enabled == "true" {
		cronExpr := db.GetSetting("auto_audit_cron")
		if cronExpr == "" {
			cronExpr = "0 9 * * *"
		}
		_, err := c.AddFunc(cronExpr, runGlobalScheduledAudit)
		if err != nil {
			log.Printf("全局巡检调度设置失败: %v", err)
		} else {
			log.Printf("全局巡检调度已启动: %s", cronExpr)
		}
	}

	// Per-site interval check: every hour at :00
	c.AddFunc("0 * * * *", runPerSiteIntervalCheck)

	// Zombie task cleanup: every 5 minutes, mark any audit_task that's been
	// running > 30 minutes as timed out. Handles AI timeouts / crashes that
	// would otherwise leave progress stuck at 65%.
	c.AddFunc("*/5 * * * *", reapZombieTasks)

	// Daily cleanup: clear HTML from audit_pages older than the 10 most
	// recent audits per site, to save disk space.
	c.AddFunc("30 3 * * *", cleanupOldAuditHTML)

	// Scheduled push: check every 10 minutes for push schedules that need to run.
	c.AddFunc("*/10 * * * *", runScheduledPushes)

	c.Start()
	log.Println("调度器已启动")
}

// reapZombieTasks marks audit tasks running too long as timed out.
func reapZombieTasks() {
	cutoff := time.Now().Add(-30 * time.Minute).Format("2006-01-02 15:04:05")
	res, err := db.DB.Exec(
		`UPDATE audit_tasks
		 SET status='timeout',
		     error_msg='任务超时（超过30分钟未完成，自动标记超时）',
		     finished_at=CURRENT_TIMESTAMP
		 WHERE status='running' AND created_at < $1`, cutoff,
	)
	if err != nil {
		log.Printf("zombie task cleanup failed: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("zombie task cleanup: reaped %d stuck tasks", n)
	}
}

// cleanupOldAuditHTML clears the html column from audit_pages belonging to
// audit tasks that are NOT among the 10 most recent per site. This frees
// significant disk space while keeping recent audits fully browsable.
func cleanupOldAuditHTML() {
	// For each site, find audit task IDs beyond the 10 most recent.
	rows, err := db.DB.Query(`
		SELECT DISTINCT ap.id
		FROM audit_pages ap
		JOIN audit_tasks at ON ap.task_id = at.id
		WHERE ap.html != ''
		  AND at.id NOT IN (
			SELECT id FROM (
				SELECT id, site_id,
				       ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY id DESC) as rn
				FROM audit_tasks
			) ranked
			WHERE rn <= 10
		)
	`)
	if err != nil {
		log.Printf("[html-cleanup] query failed: %v", err)
		return
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return
	}

	// Batch clear in chunks of 500
	for i := 0; i < len(ids); i += 500 {
		end := i + 500
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for j, id := range chunk {
			placeholders[j] = fmt.Sprintf("$%d", j+1)
			args[j] = id
		}
		query := "UPDATE audit_pages SET html = '' WHERE id IN (" + strings.Join(placeholders, ",") + ")"
		if res, err := db.DB.Exec(query, args...); err != nil {
			log.Printf("[html-cleanup] batch update failed: %v", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			log.Printf("[html-cleanup] cleared HTML from %d audit_pages rows", n)
		}
	}
}

// runScheduledPushes checks push_schedules and runs any that are due.
// Uses a simple approach: if last_run_at is older than the cron interval,
// run it. Supports common cron patterns by parsing the expression.
func runScheduledPushes() {
	rows, err := db.DB.Query(
		`SELECT id, site_id, user_id, platform, push_type, target_url, cron_expr, last_run_at
		 FROM push_schedules WHERE enabled = true`)
	if err != nil {
		log.Printf("[push-scheduler] query schedules failed: %v", err)
		return
	}
	defer rows.Close()

	type schedule struct {
		id        int64
		siteID    int64
		userID    int64
		platform  string
		pushType  string
		targetURL string
		cronExpr  string
		lastRunAt *string
	}

	var schedules []schedule
	for rows.Next() {
		var s schedule
		if err := rows.Scan(&s.id, &s.siteID, &s.userID, &s.platform, &s.pushType,
			&s.targetURL, &s.cronExpr, &s.lastRunAt); err != nil {
			continue
		}
		schedules = append(schedules, s)
	}
	rows.Close()

	now := time.Now()

	for _, s := range schedules {
		// Parse the cron expression to determine the interval.
		// Use robfig/cron parser to find next run time from last_run_at.
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		cronSched, err := parser.Parse(s.cronExpr)
		if err != nil {
			log.Printf("[push-scheduler] invalid cron %q for schedule %d: %v", s.cronExpr, s.id, err)
			continue
		}

		var lastRun time.Time
		if s.lastRunAt != nil && *s.lastRunAt != "" {
			// Try common timestamp formats.
			for _, layout := range []string{
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				time.RFC3339,
				"2006-01-02T15:04:05-07:00",
				"2006-01-02 15:04:05.999999-07",
				"2006-01-02T15:04:05.999999Z",
			} {
				if t, err := time.Parse(layout, *s.lastRunAt); err == nil {
					lastRun = t
					break
				}
			}
		}
		if lastRun.IsZero() {
			// Never run before: use a time far in the past so it triggers immediately.
			lastRun = now.Add(-24 * time.Hour)
		}

		nextRun := cronSched.Next(lastRun)
		if now.Before(nextRun) {
			continue // not yet time
		}

		log.Printf("[push-scheduler] running schedule %d: %s %s -> %s", s.id, s.platform, s.pushType, s.targetURL)
		go api.RunScheduledPush(s.id, s.siteID, s.userID, s.platform, s.pushType, s.targetURL)
	}
}

func Stop() {
	if c != nil {
		c.Stop()
	}
}

// runPerSiteIntervalCheck checks each site with independent schedule settings
func runPerSiteIntervalCheck() {
	rows, err := db.DB.Query(`
		SELECT id, domain, name, url_category, url_detail, url_news_list, url_news_detail, url_search, url_static,
		       audit_schedule_enabled, audit_interval_hours, audit_daily_hour
		FROM sites WHERE status = 'active'
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now()
	currentHour := now.Hour()

	type siteInfo struct {
		ID              int64
		Name            string
		Config          crawler.SiteConfig
		ScheduleEnabled string
		IntervalHours   int
		DailyHour       int
	}

	var sitesToCheck []siteInfo
	for rows.Next() {
		var s siteInfo
		var domain, urlCat, urlDet, urlNL, urlND, urlSearch, urlStatic string
		var schedEnabled string
		var intervalHours, dailyHour int
		if err := rows.Scan(&s.ID, &domain, &s.Name, &urlCat, &urlDet, &urlNL, &urlND, &urlSearch, &urlStatic,
			&schedEnabled, &intervalHours, &dailyHour); err != nil {
			continue
		}
		s.Config = crawler.SiteConfig{
			Domain: domain, URLCategory: urlCat, URLDetail: urlDet,
			URLNewsList: urlNL, URLNewsDetail: urlND, URLSearch: urlSearch, URLStatic: urlStatic,
		}
		s.ScheduleEnabled = schedEnabled
		s.IntervalHours = intervalHours
		s.DailyHour = dailyHour
		sitesToCheck = append(sitesToCheck, s)
	}

	var sitesToRun []siteInfo
	for _, s := range sitesToCheck {
		if s.ScheduleEnabled != "true" {
			continue
		}

		// Daily hour mode
		if s.DailyHour >= 0 && s.IntervalHours == 0 {
			if currentHour == s.DailyHour {
				// Check not already run today
				var lastTime *string
				db.DB.QueryRow(
					"SELECT created_at FROM audit_tasks WHERE site_id=$1 ORDER BY id DESC LIMIT 1", s.ID,
				).Scan(&lastTime)
				if lastTime != nil {
					t, err := time.Parse("2006-01-02T15:04:05Z", *lastTime)
					if err != nil {
						t, err = time.Parse("2006-01-02 15:04:05", *lastTime)
					}
					if err != nil {
						log.Printf("站点 %d 日期解析失败，跳过: %s", s.ID, *lastTime)
						continue
					}
					if now.Sub(t) < 23*time.Hour {
						continue // already ran within 23h
					}
				}
				sitesToRun = append(sitesToRun, s)
			}
			continue
		}

		// Interval mode
		if s.IntervalHours > 0 {
			var lastTime *string
			db.DB.QueryRow(
				"SELECT created_at FROM audit_tasks WHERE site_id=$1 ORDER BY id DESC LIMIT 1", s.ID,
			).Scan(&lastTime)
			shouldRun := false
			if lastTime == nil {
				shouldRun = true
			} else {
				t, err := time.Parse("2006-01-02T15:04:05Z", *lastTime)
				if err != nil {
					t, err = time.Parse("2006-01-02 15:04:05", *lastTime)
				}
				if err != nil {
					log.Printf("站点 %d 日期解析失败，跳过: %s", s.ID, *lastTime)
					continue
				}
				if now.Sub(t) >= time.Duration(s.IntervalHours)*time.Hour {
					shouldRun = true
				}
			}
			if shouldRun {
				sitesToRun = append(sitesToRun, s)
			}
		}
	}

	if len(sitesToRun) == 0 {
		return
	}

	maxConcurrent := 3
	if v := db.GetSetting("max_concurrent_audit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConcurrent = n
		}
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for _, site := range sitesToRun {
		wg.Add(1)
		go func(s siteInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			log.Printf("站点独立巡检触发: %s", s.Name)
			runSiteAudit(s.ID, s.Name, s.Config)
		}(site)
	}
	wg.Wait()
}

// runGlobalScheduledAudit runs for all sites without their own schedule
func runGlobalScheduledAudit() {
	log.Println("全局定时巡检开始...")

	rows, err := db.DB.Query(
		`SELECT id, domain, name, url_category, url_detail, url_news_list, url_news_detail, url_search, url_static
		 FROM sites WHERE status = 'active' AND (audit_schedule_enabled IS NULL OR audit_schedule_enabled != 'true')`,
	)
	if err != nil {
		log.Printf("查询站点失败: %v", err)
		return
	}
	defer rows.Close()

	type siteInfo struct {
		ID     int64
		Name   string
		Config crawler.SiteConfig
	}

	var sites []siteInfo
	for rows.Next() {
		var id int64
		var domain, name, urlCat, urlDet, urlNL, urlND, urlSearch, urlStatic string
		if err := rows.Scan(&id, &domain, &name, &urlCat, &urlDet, &urlNL, &urlND, &urlSearch, &urlStatic); err != nil {
			continue
		}
		sites = append(sites, siteInfo{
			ID: id, Name: name,
			Config: crawler.SiteConfig{
				Domain: domain, URLCategory: urlCat, URLDetail: urlDet,
				URLNewsList: urlNL, URLNewsDetail: urlND, URLSearch: urlSearch, URLStatic: urlStatic,
			},
		})
	}

	maxConcurrent := 3
	if v := db.GetSetting("max_concurrent_audit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConcurrent = n
		}
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for _, site := range sites {
		wg.Add(1)
		go func(s siteInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			runSiteAudit(s.ID, s.Name, s.Config)
		}(site)
	}
	wg.Wait()
}

func runSiteAudit(siteID int64, siteName string, config crawler.SiteConfig) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("站点 %d 巡检异常: %v", siteID, r)
			// Nested recover: if the mark-failed exec itself panics (e.g.
			// because the DB handle is unusable), we must not leave the
			// outer recover without logging anything.
			defer func() {
				if rr := recover(); rr != nil {
					log.Printf("站点 %d mark-failed panic: %v", siteID, rr)
				}
			}()
			if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg='internal panic', finished_at=$1 WHERE site_id=$2 AND status='running'", time.Now(), siteID); err != nil {
				log.Printf("站点 %d mark-failed exec 失败: %v", siteID, err)
			}
		}
	}()

	// Route notifications to the site's owner (sites.created_by). Under
	// the per-user model every user has their own Telegram bot; scheduled
	// audits fire off the creator's bot rather than a shared one.
	var ownerUserID int64
	db.DB.QueryRow("SELECT COALESCE(created_by,0) FROM sites WHERE id = $1", siteID).Scan(&ownerUserID)

	// Resolve the site owner's personal AI config for the audit.
	aiURL, aiKey, aiModel := ai.ResolveUserAI(ownerUserID)
	if aiKey == "" {
		log.Printf("[audit] site %d owner %d has no AI config, skipping scheduled audit", siteID, ownerUserID)
		return
	}

	var taskID int64
	err := db.DB.QueryRow(
		"INSERT INTO audit_tasks (site_id, triggered_by, status, progress) VALUES ($1, 0, 'running', 10) RETURNING id", siteID,
	).Scan(&taskID)
	if err != nil {
		log.Printf("创建巡检任务失败 站点%d: %v", siteID, err)
		return
	}

	updateProgress := func(progress int) {
		if _, err := db.DB.Exec("UPDATE audit_tasks SET progress=$1 WHERE id=$2", progress, taskID); err != nil {
			log.Printf("[audit %d] update progress failed: %v", taskID, err)
		}
	}

	// Step 1: standard page crawl
	pages := crawler.CrawlAllPages(config)
	for _, p := range pages {
		if _, err := db.DB.Exec(
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		); err != nil {
			log.Printf("[audit %d] insert audit_page failed: %v", taskID, err)
		}
	}
	updateProgress(35)

	// Step 2: re-fetch monitored site_pages so AI uses fresh HTML
	var sitePages []ai.SitePageForPrompt
	pageRows, err := db.DB.Query("SELECT id, name, url FROM site_pages WHERE site_id = $1", siteID)
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
		for _, p := range pageList {
			fp := crawler.FetchPage(crawler.URLSpec{PageType: p.name, URL: p.url}, 30*time.Second)
			now := time.Now().UTC().Format("2006-01-02 15:04:05")
			if _, err := db.DB.Exec(`UPDATE site_pages SET last_html=$1, last_fetched_at=$2, status_code=$3, ttfb_ms=$4 WHERE id=$5`,
				fp.HTML, now, fp.StatusCode, fp.TTFB, p.id); err != nil {
				log.Printf("[audit %d] update site_pages id=%d failed: %v", taskID, p.id, err)
			}
			sitePages = append(sitePages, ai.SitePageForPrompt{
				Name: p.name, URL: p.url, HTML: fp.HTML,
				LastFetchedAt: now, StatusCode: fp.StatusCode, TTFB: fp.TTFB,
			})
		}
	}
	updateProgress(55)

	// Step 3: fetch analytics snapshots (best-effort)
	var gscData *ai.GSCDataForPrompt
	var gaData *ai.GADataForPrompt
	var yandexData *ai.YandexDataForPrompt
	{
		var clicks, impressions int
		var ctr, position float64
		var kwJSON, pgJSON string
		if err := db.DB.QueryRow(
			"SELECT clicks, impressions, ctr, position, keywords_json, pages_json FROM gsc_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1",
			siteID,
		).Scan(&clicks, &impressions, &ctr, &position, &kwJSON, &pgJSON); err == nil {
			gscData = &ai.GSCDataForPrompt{Clicks: clicks, Impressions: impressions, CTR: ctr, Position: position, Keywords: kwJSON, Pages: pgJSON}
		}
	}
	{
		var sessions, users, pv int
		var channels string
		if err := db.DB.QueryRow(
			"SELECT sessions, users, pageviews, channels_json FROM ga_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1",
			siteID,
		).Scan(&sessions, &users, &pv, &channels); err == nil {
			gaData = &ai.GADataForPrompt{Sessions: sessions, Users: users, Pageviews: pv, Channels: channels}
		}
	}
	{
		var idx int
		var errs string
		if err := db.DB.QueryRow(
			"SELECT index_count, errors_json FROM yandex_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1",
			siteID,
		).Scan(&idx, &errs); err == nil {
			yandexData = &ai.YandexDataForPrompt{IndexCount: idx, Errors: errs}
		}
	}
	updateProgress(60)

	// Step 4: batched technical audit (same logic as manual trigger)
	allChapters := make([]ai.V6Chapter, 0, 8)
	var lastModel string
	var lastRaw string
	var lastBatchErr error
	batchTotal := len(ai.TechnicalBatches)
	for i, batch := range ai.TechnicalBatches {
		log.Printf("[audit %d] scheduler 批次 %d/%d：%s", taskID, i+1, batchTotal, batch.Name)
		bp := ai.BuildTechPromptBatch(batch, config.Domain, sitePages, pages, gscData, gaData, yandexData)
		chs, raw, model, bErr := ai.CallAIChaptersWithConfig(aiURL, aiKey, aiModel, bp)
		lastModel = model
		if raw != "" {
			lastRaw = raw
		}
		if bErr != nil {
			log.Printf("[audit %d] scheduler 批次 %d 失败: %v", taskID, i+1, bErr)
			lastBatchErr = bErr
			continue
		}
		allChapters = append(allChapters, chs...)
		updateProgress(65 + ((i + 1) * 15 / batchTotal))
	}

	if len(allChapters) == 0 {
		snippet := lastRaw
		if len(snippet) > 100000 {
			snippet = snippet[:100000]
		}
		if _, err := db.DB.Exec(
			`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report, raw_ai_response)
			 VALUES ($1, '[]', '[]', '[]', '[]', '', '', $2, '', $3)`,
			taskID, lastModel, snippet,
		); err != nil {
			log.Printf("[audit %d] scheduler insert empty results failed: %v", taskID, err)
		}
		errMsg := "所有技术审计批次均失败"
		if lastBatchErr != nil {
			errMsg = ai.ExplainError(lastBatchErr.Error())
		}
		if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			errMsg, time.Now(), taskID); err != nil {
			log.Printf("[audit %d] scheduler mark-failed failed: %v", taskID, err)
		}
		telegram.NotifyUser(ownerUserID, "巡检失败", siteName+":\n"+errMsg)
		return
	}

	// Step 4b: strategy chapter (independent call, failure not fatal)
	var strategyChapter *ai.V6Chapter
	stratPrompt := ai.BuildStrategyPrompt(config.Domain, sitePages, pages, gscData, gaData, yandexData)
	if strategyCh, _, sErr := ai.CallAIStrategyWithConfig(aiURL, aiKey, aiModel, stratPrompt); sErr == nil && strategyCh != nil {
		strategyChapter = strategyCh
		log.Printf("[audit %d] scheduler 策略章节已生成（%d 条）", taskID, len(strategyCh.Issues))
	} else {
		log.Printf("[audit %d] scheduler 策略章节失败（忽略）: %v", taskID, sErr)
	}
	updateProgress(85)

	_, v6JSON := ai.ComposeV6Report(config.Domain, allChapters, strategyChapter)
	analysisResult := &ai.AnalysisResult{
		V6Report:       v6JSON,
		Summary:        "",
		Suggestions:    "",
		SEOIssues:      json.RawMessage("[]"),
		CodeIssues:     json.RawMessage("[]"),
		SecurityIssues: json.RawMessage("[]"),
		PerfIssues:     json.RawMessage("[]"),
	}

	// Step 5: save results (including v6_report) atomically
	seoJSON, _ := json.Marshal(analysisResult.SEOIssues)
	codeJSON, _ := json.Marshal(analysisResult.CodeIssues)
	secJSON, _ := json.Marshal(analysisResult.SecurityIssues)
	perfJSON, _ := json.Marshal(analysisResult.PerfIssues)

	tx, txErr := db.DB.Begin()
	if txErr != nil {
		log.Printf("[audit %d] scheduler begin tx failed: %v", taskID, txErr)
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			"数据库写入失败: "+txErr.Error(), time.Now(), taskID)
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		taskID, string(seoJSON), string(codeJSON), string(secJSON), string(perfJSON),
		analysisResult.Suggestions, analysisResult.Summary, lastModel, analysisResult.V6Report,
	); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Printf("[audit %d] scheduler rollback failed: %v", taskID, rbErr)
		}
		log.Printf("[audit %d] scheduler insert results failed: %v", taskID, err)
		if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=$1, finished_at=$2 WHERE id=$3",
			"写入失败: "+err.Error(), time.Now(), taskID); err != nil {
			log.Printf("[audit %d] scheduler mark-failed failed: %v", taskID, err)
		}
		return
	}
	if _, err := tx.Exec("UPDATE audit_tasks SET status='done', progress=100, finished_at=$1 WHERE id=$2", time.Now(), taskID); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Printf("[audit %d] scheduler rollback failed: %v", taskID, rbErr)
		}
		log.Printf("[audit %d] scheduler mark-done failed: %v", taskID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[audit %d] scheduler commit failed: %v", taskID, err)
		return
	}

	telegram.NotifyUser(ownerUserID, "巡检完成", siteName+": "+analysisResult.Summary)
}
