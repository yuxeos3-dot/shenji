package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerSiteRoutes handles sites, audits, pages, dead links, external links,
// content rewrite, and push management.
func registerSiteRoutes(v1 *gin.RouterGroup) {
	// Sites CRUD
	v1.GET("/sites", RequirePermission("sites.view"), ListSites)
	v1.POST("/sites", RequirePermission("sites.create"), CreateSite)
	v1.GET("/sites/:id", RequirePermission("sites.view"), GetSite)
	v1.PUT("/sites/:id", RequirePermission("sites.edit"), UpdateSite)
	v1.DELETE("/sites/:id", RequirePermission("sites.delete"), DeleteSite)

	// Site bundle import/export
	v1.GET("/site-bundle/export", RequirePermission("sites.export"), ExportSites)
	v1.POST("/site-bundle/import", RequirePermission("sites.import"), ImportSites)

	// Site history (incidents)
	v1.GET("/sites/:id/history", RequirePermission("sites.view"), GetSiteHistory)
	v1.POST("/sites/:id/incidents", RequirePermission("sites.edit"), CreateSiteIncident)
	v1.DELETE("/sites/:id/incidents/:incident_id", RequirePermission("sites.edit"), DeleteSiteIncident)

	// Audit
	v1.POST("/sites/:id/audit", RequirePermission("sites.audit"), middleware.UserRateLimit(5), TriggerAudit)
	v1.GET("/sites/:id/audits", RequirePermission("sites.audit_view"), ListAudits)
	v1.GET("/audits/:task_id", RequirePermission("sites.audit_view"), GetAudit)
	v1.DELETE("/audits/:task_id", RequirePermission("sites.audit_delete"), DeleteAudit)
	v1.GET("/audits/:task_id/pages", RequirePermission("sites.audit_view"), GetAuditPages)
	v1.GET("/audits/:task_id/pages/:type", RequirePermission("sites.audit_view"), GetAuditPageByType)
	v1.GET("/audits/:task_id/report/download", RequirePermission("sites.audit_download"), DownloadAuditReport)

	// Site pages
	v1.GET("/sites/:id/pages", RequirePermission("sites.pages"), ListSitePages)
	v1.POST("/sites/:id/pages", RequirePermission("sites.pages"), CreateSitePage)
	v1.DELETE("/sites/:id/pages/:page_id", RequirePermission("sites.pages"), DeleteSitePage)
	v1.POST("/sites/:id/pages/:page_id/fetch", RequirePermission("sites.pages"), FetchSitePage)
	v1.GET("/sites/:id/pages/:page_id/source", RequirePermission("sites.pages"), GetSitePageSource)

	// Dead links
	v1.GET("/sites/:id/deadlinks", RequirePermission("deadlinks.view"), GetDeadLinks)
	v1.GET("/sites/:id/deadlinks/progress", RequirePermission("deadlinks.view"), GetDeadLinkProgress)
	v1.POST("/sites/:id/deadlinks/check", RequirePermission("deadlinks.check"), middleware.UserRateLimit(5), StartDeadLinkCheck)
	v1.POST("/sites/:id/deadlinks/retry", RequirePermission("deadlinks.retry"), RetryDeadLinks)

	// Content rewrite
	v1.POST("/rewrite", RequirePermission("rewrite.use"), middleware.UserRateLimit(10), RewriteContent)

	// External links
	v1.GET("/extlink-pool", RequirePermission("extlinks.view"), ListExtlinkPool)
	v1.POST("/extlink-pool/import", RequirePermission("extlinks.create"), ImportExtlinkPool)
	v1.DELETE("/extlink-pool/:id", RequirePermission("extlinks.delete"), DeleteExtlinkPoolItem)
	v1.GET("/sites/:id/extlinks", RequirePermission("extlinks.view"), GetExternalLinks)
	v1.POST("/extlinks", RequirePermission("extlinks.create"), CreateExternalLink)
	v1.PUT("/extlinks/:id", RequirePermission("extlinks.edit"), UpdateExternalLink)
	v1.DELETE("/extlinks/:id", RequirePermission("extlinks.delete"), DeleteExternalLink)
	v1.POST("/extlinks/:id/check", RequirePermission("extlinks.check"), middleware.UserRateLimit(20), CheckExternalLink)

	// Push management
	v1.GET("/sites/:id/push/logs", RequirePermission("push.view"), ListPushLogs)
	v1.POST("/sites/:id/push/sitemap", RequirePermission("push.submit"), SubmitSitemap)
	v1.POST("/push/sitemap-all", RequirePermission("push.submit_all"), SubmitSitemapAll)
	v1.POST("/sites/:id/push/urls", RequirePermission("push.submit"), middleware.UserRateLimit(10), SubmitURLs)
	v1.GET("/sites/:id/push/schedules", RequirePermission("push.view"), ListPushSchedules)
	v1.POST("/sites/:id/push/schedules", RequirePermission("push.schedule"), SavePushSchedule)
	v1.DELETE("/push/schedules/:id", RequirePermission("push.schedule"), DeletePushSchedule)
	v1.PUT("/push/schedules/:id/toggle", RequirePermission("push.schedule"), TogglePushSchedule)
}
