package main

import (
	"log"
	"os"

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

	// Middleware
	r.Use(middleware.CORS())
	r.Use(middleware.IPWhitelist())

	// Auth routes (no auth required)
	r.POST("/api/auth/login", middleware.LoginRateLimit(), api.Login)

	// Authenticated routes
	v1 := r.Group("/api/v1")
	v1.Use(middleware.AuthRequired())
	{
		// Auth
		v1.POST("/auth/logout", api.Logout)
		v1.POST("/auth/change-password", api.ChangePassword)
		v1.GET("/auth/me", api.GetMe)
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
		v1.POST("/oauth/gsc/token", api.SaveGSCToken) // receive token from browser-side OAuth
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
		v1.GET("/reports/export", api.RequirePermission("reports.export"), api.ExportReports)  // must be before /:date
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

	log.Printf("SEO巡站系统启动，端口: %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
