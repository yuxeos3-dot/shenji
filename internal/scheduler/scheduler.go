package scheduler

import (
	"log"
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

	// Check if auto audit is enabled
	enabled := db.GetSetting("auto_audit_enabled")
	if enabled == "true" {
		cronExpr := db.GetSetting("auto_audit_cron")
		if cronExpr == "" {
			cronExpr = "0 9 * * *"
		}

		_, err := c.AddFunc(cronExpr, runScheduledAudit)
		if err != nil {
			log.Printf("Failed to schedule auto audit: %v", err)
		} else {
			log.Printf("Auto audit scheduled: %s", cronExpr)
		}
	}

	c.Start()
	log.Println("Scheduler started")
}

func Stop() {
	if c != nil {
		c.Stop()
	}
}

func runScheduledAudit() {
	log.Println("Running scheduled audit for all active sites...")

	rows, err := db.DB.Query(
		"SELECT id, domain, name, url_category, url_detail, url_news_list, url_news_detail, url_search, url_static FROM sites WHERE status = 'active'",
	)
	if err != nil {
		log.Printf("Failed to query sites: %v", err)
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
		rows.Scan(&id, &domain, &name, &urlCat, &urlDet, &urlNL, &urlND, &urlSearch, &urlStatic)
		sites = append(sites, siteInfo{
			ID:   id,
			Name: name,
			Config: crawler.SiteConfig{
				Domain:       domain,
				URLCategory:  urlCat,
				URLDetail:    urlDet,
				URLNewsList:  urlNL,
				URLNewsDetail: urlND,
				URLSearch:    urlSearch,
				URLStatic:   urlStatic,
			},
		})
	}

	for _, site := range sites {
		go runSiteAudit(site.ID, site.Name, site.Config)
	}
}

func runSiteAudit(siteID int64, siteName string, config crawler.SiteConfig) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Scheduled audit for site %d panicked: %v", siteID, r)
		}
	}()

	result, err := db.DB.Exec(
		"INSERT INTO audit_tasks (site_id, triggered_by, status) VALUES (?, 0, 'running')", siteID,
	)
	if err != nil {
		log.Printf("Failed to create audit task for site %d: %v", siteID, err)
		return
	}
	taskID, _ := result.LastInsertId()

	// Crawl
	pages := crawler.CrawlAllPages(config)

	// Save pages
	for _, p := range pages {
		db.DB.Exec(
			"INSERT INTO audit_pages (task_id, page_type, url, status_code, ttfb_ms, html, html_size, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			taskID, p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTML, p.HTMLSize, p.Error,
		)
	}

	// Build prompt and call AI
	prompt := ai.BuildAnalysisPrompt(config.Domain, pages, nil, nil, nil)
	analysisResult, model, err := ai.CallAI(prompt)
	if err != nil {
		db.DB.Exec("UPDATE audit_tasks SET status='error', error_msg=?, finished_at=? WHERE id=?",
			err.Error(), time.Now(), taskID)
		telegram.SendNotification("定时巡检失败", siteName+": "+err.Error())
		return
	}

	// Save results
	seoJSON, _ := json.Marshal(analysisResult.SEOIssues)
	codeJSON, _ := json.Marshal(analysisResult.CodeIssues)
	secJSON, _ := json.Marshal(analysisResult.SecurityIssues)
	perfJSON, _ := json.Marshal(analysisResult.PerfIssues)

	db.DB.Exec(
		`INSERT INTO audit_results (task_id, seo_issues, code_issues, security_issues, perf_issues, suggestions, summary, ai_model)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, string(seoJSON), string(codeJSON), string(secJSON), string(perfJSON),
		analysisResult.Suggestions, analysisResult.Summary, model,
	)

	db.DB.Exec("UPDATE audit_tasks SET status='done', progress=100, finished_at=? WHERE id=?", time.Now(), taskID)

	telegram.SendNotification("定时巡检完成", siteName+": "+analysisResult.Summary)
}
