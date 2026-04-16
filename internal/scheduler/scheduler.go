package scheduler

import (
	"log"
	"strconv"
	"sync"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"
	"seo-system/internal/ai"
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

	c.Start()
	log.Println("调度器已启动")
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
		ID                   int64
		Name                 string
		Config               crawler.SiteConfig
		ScheduleEnabled      string
		IntervalHours        int
		DailyHour            int
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
		}
	}()

	result, err := db.DB.Exec(
		"INSERT INTO audit_tasks (site_id, triggered_by, status) VALUES (?, 0, 'running')", siteID,
	)
	if err != nil {
		log.Printf("创建巡检任务失败 站点%d: %v", siteID, err)
		return
	}
	taskID, _ := result.LastInsertId()

	// Step 1: Crawl standard pages
	pages := crawler.CrawlAllPages(config)
	for _, p := range pages {
		db.DB.Exec(
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		)
	}

	// Step 2: Auto re-fetch all site_pages to ensure fresh HTML
	pageRows, err := db.DB.Query("SELECT id, name, url FROM site_pages WHERE site_id = ?", siteID)
	var sitePagesList []ai.SitePageForPrompt
	if err == nil {
		type pgInfo struct{ id int64; name, url string }
		var pgList []pgInfo
		for pageRows.Next() {
			var p pgInfo
			if pageRows.Scan(&p.id, &p.name, &p.url) == nil {
				pgList = append(pgList, p)
			}
		}
		pageRows.Close()
		for _, p := range pgList {
			r := crawler.FetchPage(crawler.URLSpec{PageType: p.name, URL: p.url}, 30*time.Second)
			now := time.Now().Format("2006-01-02 15:04:05")
			db.DB.Exec(`UPDATE site_pages SET last_html=?, last_fetched_at=?, status_code=?, ttfb_ms=? WHERE id=?`,
				r.HTML, now, r.StatusCode, r.TTFB, p.id)
		}
		// Reload fresh site_pages for prompt
		freshRows, _ := db.DB.Query("SELECT name, url, last_html, last_fetched_at, status_code, ttfb_ms FROM site_pages WHERE site_id = ? AND last_html != ''", siteID)
		if freshRows != nil {
			for freshRows.Next() {
				var sp ai.SitePageForPrompt
				freshRows.Scan(&sp.Name, &sp.URL, &sp.HTML, &sp.LastFetchedAt, &sp.StatusCode, &sp.TTFB)
				sitePagesList = append(sitePagesList, sp)
			}
			freshRows.Close()
		}
	}

	// Step 3: Build prompt and call AI
	prompt := ai.BuildV6AuditPrompt(config.Domain, sitePagesList, pages, nil, nil, nil)
	analysisResult, model, err := ai.CallAI(prompt)
	if err != nil {
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			err.Error(), time.Now(), taskID)
		telegram.SendNotification("巡检失败", siteName+": "+err.Error())
		return
	}

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
	telegram.SendNotification("巡检完成", siteName+": "+analysisResult.Summary)
}
