package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerAdminRoutes handles admin-only settings, cache, spider, and analytics.
func registerAdminRoutes(v1 *gin.RouterGroup) {
	// Admin-only group
	admin := v1.Group("/admin", middleware.AdminRequired())
	{
		admin.GET("/global-data", GetGlobalDataOverview)
		admin.GET("/metrics", Metrics)
		admin.GET("/analytics/snapshot", GetAnalyticsSnapshot)
		admin.POST("/analytics/ai-advise", RunAnalyticsAdvise)
	}

	// Settings
	settings := v1.Group("/settings", middleware.AdminRequired())
	{
		settings.GET("", RequirePermission("settings.view"), GetSettings)
		settings.PUT("", RequirePermission("settings.edit"), SaveSettings)
		settings.POST("/test-ai", RequirePermission("settings.test_ai"), TestAI)
		settings.POST("/test-telegram", RequirePermission("settings.test_tg"), TestTelegram)
		settings.GET("/my-ip", GetClientIP)
	}

	// Audit logs
	v1.GET("/audit-logs", RequirePermission("audit_log.view"), ListAuditLogs)

	// Cache management
	cache := v1.Group("/cache", middleware.AdminRequired())
	{
		cache.GET("", RequirePermission("cache.manage"), ListCaches)
		cache.DELETE("/:key", RequirePermission("cache.manage"), ClearCache)
	}

	// Spider analytics
	v1.GET("/spider/sites", RequirePermission("spider.view"), SpiderSiteSummary)
	v1.GET("/spider/overview", RequirePermission("spider.view"), SpiderOverview)
	v1.GET("/spider/trend", RequirePermission("spider.view"), SpiderDailyTrend)
	v1.GET("/spider/logs", RequirePermission("spider.view"), SpiderLogs)
}
