package scheduler

import (
	"log"
	"strconv"
	"sync"
	"time"

	"seo-system/internal/ai"
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
	// running > 10 minutes as errored. Handles AI timeouts / crashes that
	// would otherwise leave progress stuck at 65%.
	c.AddFunc("*/5 * * * *", reapZombieTasks)

	c.Start()
	log.Println("调度器已启动")
}

// reapZombieTasks marks audit tasks running too long as errored.
func reapZombieTasks() {
	cutoff := time.Now().Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	res, err := db.DB.Exec(
		`UPDATE audit_tasks
		 SET status='error',
		     error_msg='任务超时（超过10分钟未完成，自动标记失败）',
		     finished_at=CURRENT_TIMESTAMP
		 WHERE status='running' AND created_at < ?`, cutoff,
	)
	if err != nil {
		log.Printf("zombie task cleanup failed: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("zombie task cleanup: reaped %d stuck tasks", n)
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
					"SELECT created_at FROM audit_tasks WHERE site_id=? ORDER BY id DESC LIMIT 1", s.ID,
				).Scan(&lastTime)
				if lastTime != nil {
					t, err := time.Parse("2006-01-02T15:04:05Z", *lastTime)
					if err != nil {
						t, _ = time.Parse("2006-01-02 15:04:05", *lastTime)
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
				"SELECT created_at FROM audit_tasks WHERE site_id=? ORDER BY id DESC LIMIT 1", s.ID,
			).Scan(&lastTime)
			shouldRun := false
			if lastTime == nil {
				shouldRun = true
			} else {
				t, err := time.Parse("2006-01-02T15:04:05Z", *lastTime)
				if err != nil {
					t, _ = time.Parse("2006-01-02 15:04:05", *lastTime)
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
			if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg='internal panic', finished_at=? WHERE site_id=? AND status='running'", time.Now(), siteID); err != nil {
				log.Printf("站点 %d mark-failed exec 失败: %v", siteID, err)
			}
		}
	}()

	// Route notifications to the site's owner (sites.created_by). Under
	// the per-user model every user has their own Telegram bot; scheduled
	// audits fire off the creator's bot rather than a shared one.
	var ownerUserID int64
	db.DB.QueryRow("SELECT COALESCE(created_by,0) FROM sites WHERE id = ?", siteID).Scan(&ownerUserID)

	result, err := db.DB.Exec(
		"INSERT INTO audit_tasks (site_id, triggered_by, status, progress) VALUES (?, 0, 'running', 10)", siteID,
	)
	if err != nil {
		log.Printf("创建巡检任务失败 站点%d: %v", siteID, err)
		return
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		log.Printf("创建巡检任务 LastInsertId 失败 站点%d: %v", siteID, err)
		return
	}

	updateProgress := func(progress int) {
		if _, err := db.DB.Exec("UPDATE audit_tasks SET progress=? WHERE id=?", progress, taskID); err != nil {
			log.Printf("[audit %d] update progress failed: %v", taskID, err)
		}
	}

	// Step 1: standard page crawl
	pages := crawler.CrawlAllPages(config)
	for _, p := range pages {
		if _, err := db.DB.Exec(
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		); err != nil {
			log.Printf("[audit %d] insert audit_page failed: %v", taskID, err)
		}
	}
	updateProgress(35)

	// Step 2: re-fetch monitored site_pages so AI uses fresh HTML
	var sitePages []ai.SitePageForPrompt
	pageRows, err := db.DB.Query("SELECT id, name, url FROM site_pages WHERE site_id = ?", siteID)
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
			if _, err := db.DB.Exec(`UPDATE site_pages SET last_html=?, last_fetched_at=?, status_code=?, ttfb_ms=? WHERE id=?`,
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
			"SELECT clicks, impressions, ctr, position, keywords_json, pages_json FROM gsc_data WHERE site_id = ? ORDER BY date DESC LIMIT 1",
			siteID,
		).Scan(&clicks, &impressions, &ctr, &position, &kwJSON, &pgJSON); err == nil {
			gscData = &ai.GSCDataForPrompt{Clicks: clicks, Impressions: impressions, CTR: ctr, Position: position, Keywords: kwJSON, Pages: pgJSON}
		}
	}
	{
		var sessions, users, pv int
		var channels string
		if err := db.DB.QueryRow(
			"SELECT sessions, users, pageviews, channels_json FROM ga_data WHERE site_id = ? ORDER BY date DESC LIMIT 1",
			siteID,
		).Scan(&sessions, &users, &pv, &channels); err == nil {
			gaData = &ai.GADataForPrompt{Sessions: sessions, Users: users, Pageviews: pv, Channels: channels}
		}
	}
	{
		var idx int
		var errs string
		if err := db.DB.QueryRow(
			"SELECT index_count, errors_json FROM yandex_data WHERE site_id = ? ORDER BY date DESC LIMIT 1",
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
	batchTotal := len(ai.TechnicalBatches)
	for i, batch := range ai.TechnicalBatches {
		log.Printf("[audit %d] scheduler 批次 %d/%d：%s", taskID, i+1, batchTotal, batch.Name)
		bp := ai.BuildTechPromptBatch(batch, config.Domain, sitePages, pages, gscData, gaData, yandexData)
		chs, raw, model, bErr := ai.CallAIChapters(bp)
		lastModel = model
		if raw != "" {
			lastRaw = raw
		}
		if bErr != nil {
			log.Printf("[audit %d] scheduler 批次 %d 失败: %v", taskID, i+1, bErr)
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
			 VALUES (?, '[]', '[]', '[]', '[]', '', '', ?, '', ?)`,
			taskID, lastModel, snippet,
		); err != nil {
			log.Printf("[audit %d] scheduler insert empty results failed: %v", taskID, err)
		}
		if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg='所有技术审计批次均失败', finished_at=? WHERE id=?",
			time.Now(), taskID); err != nil {
			log.Printf("[audit %d] scheduler mark-failed failed: %v", taskID, err)
		}
		telegram.NotifyUser(ownerUserID, "巡检失败", siteName+": 所有技术审计批次均失败")
		return
	}

	// Step 4b: strategy chapter (independent call, failure not fatal)
	var strategyChapter *ai.V6Chapter
	stratPrompt := ai.BuildStrategyPrompt(config.Domain, sitePages, pages, gscData, gaData, yandexData)
	if strategyCh, _, sErr := ai.CallAIStrategy(stratPrompt); sErr == nil && strategyCh != nil {
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
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			"数据库写入失败: "+txErr.Error(), time.Now(), taskID)
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model, v6_report)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, string(seoJSON), string(codeJSON), string(secJSON), string(perfJSON),
		analysisResult.Suggestions, analysisResult.Summary, lastModel, analysisResult.V6Report,
	); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Printf("[audit %d] scheduler rollback failed: %v", taskID, rbErr)
		}
		log.Printf("[audit %d] scheduler insert results failed: %v", taskID, err)
		if _, err := db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			"写入失败: "+err.Error(), time.Now(), taskID); err != nil {
			log.Printf("[audit %d] scheduler mark-failed failed: %v", taskID, err)
		}
		return
	}
	if _, err := tx.Exec("UPDATE audit_tasks SET status='done', progress=100, finished_at=? WHERE id=?", time.Now(), taskID); err != nil {
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
