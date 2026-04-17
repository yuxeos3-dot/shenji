package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"seo-system/internal/api"
	"seo-system/internal/db"
	"seo-system/internal/middleware"
	"seo-system/internal/scheduler"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	godotenv.Load()

	// Initialize database
	db.Init()

	// Start scheduler
	scheduler.Start()
	defer scheduler.Stop()

	// Setup Gin
	r := gin.Default()

	// Tell gin which proxy addresses can set X-Forwarded-For / X-Real-IP,
	// otherwise c.ClientIP() is spoofable and IP whitelist + login rate limit
	// can be bypassed. Defaults to loopback only; override with TRUSTED_PROXIES.
	trusted := []string{"127.0.0.1", "::1"}
	if raw := os.Getenv("TRUSTED_PROXIES"); raw != "" {
		trusted = trusted[:0]
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				trusted = append(trusted, p)
			}
		}
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		log.Fatalf("SetTrustedProxies 失败: %v", err)
	}

	// Global middleware
	r.Use(middleware.CORS())
	r.Use(middleware.IPWhitelist())
	r.Use(middleware.RecoverLogger())
	r.Use(api.MetricsCounter())

	// Public health endpoint — used by load balancers / monitoring. Reports
	// DB reachability. Deliberately outside the auth group. Lives under
	// /api/ so reverse proxies that only forward /api/* still hit it.
	r.GET("/api/healthz", api.Healthz)

	// Prometheus-style metrics endpoint (no-auth by default; wrap with reverse
	// proxy ACL if the host is public).
	r.GET("/api/metrics", api.Metrics)

	// Auth routes (no auth required)
	r.POST("/api/auth/login", middleware.LoginRateLimit(), api.Login)

	// Spider hit ingest — authenticated by X-API-Key (set in system settings).
	// Public so external log shippers (nginx parser, CF worker) can POST to it
	// without holding a JWT cookie.
	r.POST("/api/spider/ingest", api.IngestSpiderLog)

	// Authenticated routes
	// CSRF runs AFTER auth: it's cheap to skip for non-cookie clients, and
	// needs to know whether the request is cookie-based.
	v1 := r.Group("/api/v1")
	v1.Use(middleware.AuthRequired(), middleware.CSRFProtect())
	{
		// Auth
		v1.POST("/auth/logout", api.Logout)
		v1.POST("/auth/change-password", api.ChangePassword)
		v1.GET("/auth/me", api.GetMe)
		v1.GET("/auth/csrf", api.GetCSRFToken)
		v1.GET("/auth/online", api.GetOnlineUsersAPI)
		v1.GET("/auth/my-permissions", api.GetMyPermissions)
		v1.GET("/auth/my-ai-config", api.GetMyAIConfig)
		v1.PUT("/auth/my-ai-config", api.SaveMyAIConfig)
		v1.POST("/auth/test-my-ai", api.TestMyAI)

		// Users (admin only)
		users := v1.Group("/users")
		users.Use(middleware.AdminRequired())
		{
			users.GET("", api.ListUsers)
			users.POST("", api.CreateUser)
			users.PUT("/:id", api.UpdateUser)
			users.DELETE("/:id", api.DeleteUser)
			users.GET("/:id/permissions", api.GetUserPermissions)
			users.PUT("/:id/permissions", api.SaveUserPermissions)
			users.GET("/permission-tree", api.GetPermissionTree)
		}

		// Sites
		v1.GET("/sites", api.RequirePermission("sites.view"), api.ListSites)
		v1.POST("/sites", api.RequirePermission("sites.create"), api.CreateSite)
		v1.GET("/sites/:id", api.RequirePermission("sites.view"), api.GetSite)
		v1.PUT("/sites/:id", api.RequirePermission("sites.edit"), api.UpdateSite)
		v1.DELETE("/sites/:id", api.RequirePermission("sites.delete"), api.DeleteSite)

		// Audit (rate-limited: 5/min per user)
		v1.POST("/sites/:id/audit", api.RequirePermission("sites.audit"), middleware.UserRateLimit(5), api.TriggerAudit)
		v1.GET("/sites/:id/audits", api.RequirePermission("sites.audit_view"), api.ListAudits)
		v1.GET("/audits/:task_id", api.RequirePermission("sites.audit_view"), api.GetAudit)
		v1.DELETE("/audits/:task_id", api.RequirePermission("sites.audit_delete"), api.DeleteAudit)
		v1.GET("/audits/:task_id/pages", api.RequirePermission("sites.audit_view"), api.GetAuditPages)
		v1.GET("/audits/:task_id/pages/:type", api.RequirePermission("sites.audit_view"), api.GetAuditPageByType)
		v1.GET("/audits/:task_id/report/download", api.RequirePermission("sites.audit_download"), api.DownloadAuditReport)

		// Site Pages (custom pages management)
		v1.GET("/sites/:id/pages", api.RequirePermission("sites.pages"), api.ListSitePages)
		v1.POST("/sites/:id/pages", api.RequirePermission("sites.pages"), api.CreateSitePage)
		v1.DELETE("/sites/:id/pages/:page_id", api.RequirePermission("sites.pages"), api.DeleteSitePage)
		v1.POST("/sites/:id/pages/:page_id/fetch", api.RequirePermission("sites.pages"), api.FetchSitePage)
		v1.GET("/sites/:id/pages/:page_id/source", api.RequirePermission("sites.pages"), api.GetSitePageSource)

		// Dead Link Check (rate-limited: 5/min per user)
		v1.GET("/sites/:id/deadlinks", api.RequirePermission("deadlinks.view"), api.GetDeadLinks)
		v1.GET("/sites/:id/deadlinks/progress", api.RequirePermission("deadlinks.view"), api.GetDeadLinkProgress)
		v1.POST("/sites/:id/deadlinks/check", api.RequirePermission("deadlinks.check"), middleware.UserRateLimit(5), api.StartDeadLinkCheck)
		v1.POST("/sites/:id/deadlinks/retry", api.RequirePermission("deadlinks.retry"), api.RetryDeadLinks)

		// Content Rewrite (AI, rate-limited: 10/min per user)
		v1.POST("/rewrite", api.RequirePermission("rewrite.use"), middleware.UserRateLimit(10), api.RewriteContent)

		// Shared Backlink Pool
		v1.GET("/extlink-pool", api.RequirePermission("extlinks.view"), api.ListExtlinkPool)
		v1.POST("/extlink-pool/import", api.RequirePermission("extlinks.create"), api.ImportExtlinkPool)
		v1.DELETE("/extlink-pool/:id", api.RequirePermission("extlinks.delete"), api.DeleteExtlinkPoolItem)

		// External Links
		v1.GET("/sites/:id/extlinks", api.RequirePermission("extlinks.view"), api.GetExternalLinks)
		v1.POST("/extlinks", api.RequirePermission("extlinks.create"), api.CreateExternalLink)
		v1.PUT("/extlinks/:id", api.RequirePermission("extlinks.edit"), api.UpdateExternalLink)
		v1.DELETE("/extlinks/:id", api.RequirePermission("extlinks.delete"), api.DeleteExternalLink)
		v1.POST("/extlinks/:id/check", api.RequirePermission("extlinks.check"), api.CheckExternalLink)

		// GSC
		v1.GET("/oauth/gsc", api.GetGSCAuthURL)
		v1.GET("/oauth/gsc/callback", api.GSCCallback)
		v1.POST("/oauth/gsc/token", api.SaveGSCToken)
		v1.POST("/gsc/store-keywords", api.StoreSiteKeywords) // push keywords from browser
		v1.POST("/sites/:id/gsc/sync", api.RequirePermission("data.sync_gsc"), api.SyncGSCData)
		v1.GET("/sites/:id/gsc", api.RequirePermission("data.view"), api.GetGSCData)

		// GA
		v1.GET("/oauth/ga", api.GetGAAuthURL)
		v1.GET("/oauth/ga/callback", api.GACallback)
		v1.POST("/sites/:id/ga/sync", api.RequirePermission("data.sync_ga"), api.SyncGAData)
		v1.GET("/sites/:id/ga", api.RequirePermission("data.view"), api.GetGAData)

		// Yandex
		v1.GET("/oauth/yandex", api.GetYandexAuthURL)
		v1.GET("/oauth/yandex/callback", api.YandexCallback)
		v1.POST("/sites/:id/yandex/sync", api.RequirePermission("data.sync_yandex"), api.SyncYandexData)
		v1.GET("/sites/:id/yandex", api.RequirePermission("data.view"), api.GetYandexData)

		// Reports
		v1.GET("/reports", api.RequirePermission("reports.view"), api.ListReports)
		v1.POST("/reports", api.RequirePermission("reports.write"), api.SubmitReport)
		v1.POST("/reports/polish", api.RequirePermission("reports.polish"), api.PolishReport)
		v1.GET("/reports/export", api.RequirePermission("reports.export"), api.ExportReports) // must be before /:date
		v1.GET("/reports/:date", api.RequirePermission("reports.view"), api.GetReportByDate)

		// Bing Webmaster
		v1.POST("/sites/:id/bing/sync", api.RequirePermission("data.sync_gsc"), api.SyncBingData)
		v1.GET("/sites/:id/bing", api.RequirePermission("data.view"), api.GetBingData)

		// Settings (admin only)
		settings := v1.Group("/settings")
		settings.Use(middleware.AdminRequired())
		{
			settings.GET("", api.GetSettings)
			settings.PUT("", api.SaveSettings)
			settings.POST("/test-ai", api.TestAI)
			settings.POST("/test-telegram", api.TestTelegram)
			settings.GET("/my-ip", api.GetClientIP)
		}

		// Audit Logs (admin only)
		v1.GET("/audit-logs", middleware.AdminRequired(), api.ListAuditLogs)

		// Spider analytics (read-only for now; ingest is a public route above)
		v1.GET("/spider/overview", api.RequirePermission("spider.view"), api.SpiderOverview)
		v1.GET("/spider/trend", api.RequirePermission("spider.view"), api.SpiderDailyTrend)
		v1.GET("/spider/logs", api.RequirePermission("spider.view"), api.SpiderLogs)

		// Cache management (admin only)
		cache := v1.Group("/cache")
		cache.Use(middleware.AdminRequired())
		{
			cache.GET("", api.ListCaches)
			cache.DELETE("/:key", api.ClearCache)
		}

		// GEO Content Production
		geo := v1.Group("/geo")
		{
			geo.GET("/stats", api.RequirePermission("geo.view"), api.GeoStats)

			geo.GET("/title-libs", api.RequirePermission("geo.view"), api.GeoListTitleLibs)
			geo.POST("/title-libs", api.RequirePermission("geo.keywords"), api.GeoCreateTitleLib)
			geo.DELETE("/title-libs/:id", api.RequirePermission("geo.keywords"), api.GeoDeleteTitleLib)
			geo.GET("/title-libs/:id/titles", api.RequirePermission("geo.view"), api.GeoListTitles)
			geo.POST("/title-libs/:id/titles", api.RequirePermission("geo.keywords"), api.GeoAddTitles)
			geo.POST("/title-libs/:id/import", api.RequirePermission("geo.keywords"), api.GeoImportTitles)
			geo.POST("/title-libs/:id/import-gsc", api.RequirePermission("geo.keywords"), api.GeoImportFromGSC)
			geo.DELETE("/titles/:id", api.RequirePermission("geo.keywords"), api.GeoDeleteTitle)

			geo.GET("/prompts", api.RequirePermission("geo.view"), api.GeoListPrompts)
			geo.POST("/prompts", api.RequirePermission("geo.prompts"), api.GeoCreatePrompt)
			geo.PUT("/prompts/:id", api.RequirePermission("geo.prompts"), api.GeoUpdatePrompt)
			geo.DELETE("/prompts/:id", api.RequirePermission("geo.prompts"), api.GeoDeletePrompt)

			geo.GET("/knowledge", api.RequirePermission("geo.view"), api.GeoListKnowledge)
			geo.POST("/knowledge", api.RequirePermission("geo.knowledge"), api.GeoCreateKnowledge)
			geo.PUT("/knowledge/:id", api.RequirePermission("geo.knowledge"), api.GeoUpdateKnowledge)
			geo.DELETE("/knowledge/:id", api.RequirePermission("geo.knowledge"), api.GeoDeleteKnowledge)

			geo.GET("/tasks", api.RequirePermission("geo.view"), api.GeoListTasks)
			geo.POST("/tasks", api.RequirePermission("geo.tasks"), api.GeoCreateTask)
			geo.PUT("/tasks/:id", api.RequirePermission("geo.tasks"), api.GeoUpdateTask)
			geo.DELETE("/tasks/:id", api.RequirePermission("geo.tasks"), api.GeoDeleteTask)
			geo.POST("/tasks/:id/start", api.RequirePermission("geo.tasks"), api.GeoStartTask)
			geo.POST("/tasks/:id/stop", api.RequirePermission("geo.tasks"), api.GeoStopTask)
			geo.POST("/tasks/:id/run", api.RequirePermission("geo.tasks_run"), api.GeoRunTask)

			geo.GET("/articles", api.RequirePermission("geo.articles"), api.GeoListArticles)
			geo.GET("/articles/:id", api.RequirePermission("geo.articles"), api.GeoGetArticle)
			geo.PUT("/articles/:id", api.RequirePermission("geo.articles"), api.GeoUpdateArticle)
			geo.DELETE("/articles/:id", api.RequirePermission("geo.articles_delete"), api.GeoDeleteArticle)
			geo.POST("/articles/:id/review", api.RequirePermission("geo.articles_review"), api.GeoReviewArticle)
			geo.POST("/articles/:id/publish", api.RequirePermission("geo.articles_publish"), api.GeoPublishArticle)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: audit/report downloads can be slow; Gin and the
		// reverse proxy already cap per-request.
		IdleTimeout: 120 * time.Second,
	}

	// Graceful shutdown: on SIGINT/SIGTERM, stop accepting new requests and
	// let in-flight ones finish. Running audits/deadlink checks detach into
	// their own goroutines and may outlive the HTTP server.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("SEO巡站系统启动，端口: %s (trusted_proxies=%v)", port, trusted)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("启动失败: %v", err)
		}
	case <-shutdownCtx.Done():
		log.Println("收到关停信号，等待在途请求完成 (最多 30s)...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("HTTP shutdown 失败: %v", err)
		}
		log.Println("已优雅关停")
	}
}
