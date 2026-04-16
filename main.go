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
		v1.GET("/sites", api.ListSites)
		v1.POST("/sites", api.CreateSite)
		v1.GET("/sites/:id", api.GetSite)
		v1.PUT("/sites/:id", api.UpdateSite)
		v1.DELETE("/sites/:id", api.DeleteSite)

		// Audit
		v1.POST("/sites/:id/audit", api.TriggerAudit)
		v1.GET("/sites/:id/audits", api.ListAudits)
		v1.GET("/audits/:task_id", api.GetAudit)
		v1.DELETE("/audits/:task_id", api.DeleteAudit)
		v1.GET("/audits/:task_id/pages", api.GetAuditPages)
		v1.GET("/audits/:task_id/pages/:type", api.GetAuditPageByType)
		v1.GET("/audits/:task_id/report/download", api.DownloadAuditReport)

		// Site Pages (custom pages management)
		v1.GET("/sites/:id/pages", api.ListSitePages)
		v1.POST("/sites/:id/pages", api.CreateSitePage)
		v1.DELETE("/sites/:id/pages/:page_id", api.DeleteSitePage)
		v1.POST("/sites/:id/pages/:page_id/fetch", api.FetchSitePage)
		v1.GET("/sites/:id/pages/:page_id/source", api.GetSitePageSource)

		// Dead Link Check
		v1.GET("/sites/:id/deadlinks", api.GetDeadLinks)
		v1.GET("/sites/:id/deadlinks/progress", api.GetDeadLinkProgress)
		v1.POST("/sites/:id/deadlinks/check", api.StartDeadLinkCheck)
		v1.POST("/sites/:id/deadlinks/retry", api.RetryDeadLinks)

		// Content Rewrite (AI)
		v1.POST("/rewrite", api.RewriteContent)

		// Shared Backlink Pool
		v1.GET("/extlink-pool", api.ListExtlinkPool)
		v1.POST("/extlink-pool/import", api.ImportExtlinkPool)
		v1.DELETE("/extlink-pool/:id", api.DeleteExtlinkPoolItem)

		// External Links
		v1.GET("/sites/:id/extlinks", api.GetExternalLinks)
		v1.POST("/extlinks", api.CreateExternalLink)
		v1.PUT("/extlinks/:id", api.UpdateExternalLink)
		v1.DELETE("/extlinks/:id", api.DeleteExternalLink)
		v1.POST("/extlinks/:id/check", api.CheckExternalLink)

		// GSC
		v1.GET("/oauth/gsc", api.GetGSCAuthURL)
		v1.GET("/oauth/gsc/callback", api.GSCCallback)
		v1.POST("/oauth/gsc/token", api.SaveGSCToken) // receive token from browser-side OAuth
		v1.POST("/sites/:id/gsc/sync", api.SyncGSCData)
		v1.GET("/sites/:id/gsc", api.GetGSCData)

		// GA
		v1.GET("/oauth/ga", api.GetGAAuthURL)
		v1.GET("/oauth/ga/callback", api.GACallback)
		v1.POST("/sites/:id/ga/sync", api.SyncGAData)
		v1.GET("/sites/:id/ga", api.GetGAData)

		// Yandex
		v1.GET("/oauth/yandex", api.GetYandexAuthURL)
		v1.GET("/oauth/yandex/callback", api.YandexCallback)
		v1.POST("/sites/:id/yandex/sync", api.SyncYandexData)
		v1.GET("/sites/:id/yandex", api.GetYandexData)

		// Reports
		v1.GET("/reports", api.ListReports)
		v1.POST("/reports", api.SubmitReport)
		v1.POST("/reports/polish", api.PolishReport)
		v1.GET("/reports/export", api.ExportReports)  // must be before /:date
		v1.GET("/reports/:date", api.GetReportByDate)

		// Bing Webmaster
		v1.POST("/sites/:id/bing/sync", api.SyncBingData)
		v1.GET("/sites/:id/bing", api.GetBingData)

		// Settings (admin only)
		settings := v1.Group("/settings")
		settings.Use(middleware.AdminRequired())
		{
			settings.GET("", api.GetSettings)
			settings.PUT("", api.SaveSettings)
			settings.POST("/test-ai", api.TestAI)
			settings.POST("/test-telegram", api.TestTelegram)
		}

		// GEO Content Production
		geo := v1.Group("/geo")
		{
			geo.GET("/stats", api.GeoStats)

			geo.GET("/title-libs", api.GeoListTitleLibs)
			geo.POST("/title-libs", api.GeoCreateTitleLib)
			geo.DELETE("/title-libs/:id", api.GeoDeleteTitleLib)
			geo.GET("/title-libs/:id/titles", api.GeoListTitles)
			geo.POST("/title-libs/:id/titles", api.GeoAddTitles)
			geo.POST("/title-libs/:id/import", api.GeoImportTitles)
			geo.POST("/title-libs/:id/import-gsc", api.GeoImportFromGSC)
			geo.DELETE("/titles/:id", api.GeoDeleteTitle)

			geo.GET("/prompts", api.GeoListPrompts)
			geo.POST("/prompts", api.GeoCreatePrompt)
			geo.PUT("/prompts/:id", api.GeoUpdatePrompt)
			geo.DELETE("/prompts/:id", api.GeoDeletePrompt)

			geo.GET("/knowledge", api.GeoListKnowledge)
			geo.POST("/knowledge", api.GeoCreateKnowledge)
			geo.PUT("/knowledge/:id", api.GeoUpdateKnowledge)
			geo.DELETE("/knowledge/:id", api.GeoDeleteKnowledge)

			geo.GET("/tasks", api.GeoListTasks)
			geo.POST("/tasks", api.GeoCreateTask)
			geo.PUT("/tasks/:id", api.GeoUpdateTask)
			geo.DELETE("/tasks/:id", api.GeoDeleteTask)
			geo.POST("/tasks/:id/start", api.GeoStartTask)
			geo.POST("/tasks/:id/stop", api.GeoStopTask)
			geo.POST("/tasks/:id/run", api.GeoRunTask)

			geo.GET("/articles", api.GeoListArticles)
			geo.GET("/articles/:id", api.GeoGetArticle)
			geo.PUT("/articles/:id", api.GeoUpdateArticle)
			geo.DELETE("/articles/:id", api.GeoDeleteArticle)
			geo.POST("/articles/:id/review", api.GeoReviewArticle)
			geo.POST("/articles/:id/publish", api.GeoPublishArticle)
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
