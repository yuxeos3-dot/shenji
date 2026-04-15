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

		// Users (admin only)
		users := v1.Group("/users")
		users.Use(middleware.AdminRequired())
		{
			users.GET("", api.ListUsers)
			users.POST("", api.CreateUser)
			users.PUT("/:id", api.UpdateUser)
			users.DELETE("/:id", api.DeleteUser)
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

		// Content Rewrite (AI)
		v1.POST("/rewrite", api.RewriteContent)

		// External Links
		v1.GET("/sites/:id/extlinks", api.GetExternalLinks)
		v1.POST("/extlinks", api.CreateExternalLink)
		v1.PUT("/extlinks/:id", api.UpdateExternalLink)
		v1.DELETE("/extlinks/:id", api.DeleteExternalLink)
		v1.POST("/extlinks/:id/check", api.CheckExternalLink)

		// GSC
		v1.GET("/oauth/gsc", api.GetGSCAuthURL)
		v1.GET("/oauth/gsc/callback", api.GSCCallback)
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
		v1.GET("/reports/:date", api.GetReportByDate)
		v1.GET("/reports/export", api.ExportReports)

		// Settings (admin only)
		settings := v1.Group("/settings")
		settings.Use(middleware.AdminRequired())
		{
			settings.GET("", api.GetSettings)
			settings.PUT("", api.SaveSettings)
			settings.POST("/test-ai", api.TestAI)
			settings.POST("/test-telegram", api.TestTelegram)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("SEO巡站系统启动，端口: %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
