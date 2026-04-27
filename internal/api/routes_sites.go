package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

func registerSiteRoutes(v1 *gin.RouterGroup) {
	v1.GET("/sites", ListSites)
	v1.POST("/sites", CreateSite)
	v1.GET("/sites/:id", GetSite)
	v1.PUT("/sites/:id", UpdateSite)
	v1.DELETE("/sites/:id", DeleteSite)

	v1.GET("/site-bundle/export", ExportSites)
	v1.POST("/site-bundle/import", ImportSites)

	v1.GET("/sites/:id/history", GetSiteHistory)
	v1.POST("/sites/:id/incidents", CreateSiteIncident)
	v1.DELETE("/sites/:id/incidents/:incident_id", DeleteSiteIncident)

	// Audit
	v1.POST("/sites/:id/audit", middleware.UserRateLimit(5), TriggerAudit)
	v1.GET("/sites/:id/audits", ListAudits)
	v1.GET("/audits/:task_id", GetAudit)
	v1.DELETE("/audits/:task_id", DeleteAudit)
	v1.GET("/audits/:task_id/pages", GetAuditPages)
	v1.GET("/audits/:task_id/pages/:type", GetAuditPageByType)

	// Site pages
	v1.GET("/sites/:id/pages", ListSitePages)
	v1.POST("/sites/:id/pages", CreateSitePage)
	v1.DELETE("/sites/:id/pages/:page_id", DeleteSitePage)
	v1.POST("/sites/:id/pages/:page_id/fetch", FetchSitePage)
	v1.GET("/sites/:id/pages/:page_id/source", GetSitePageSource)

	// Dead links
	v1.GET("/sites/:id/deadlinks", GetDeadLinks)
	v1.GET("/sites/:id/deadlinks/progress", GetDeadLinkProgress)
	v1.POST("/sites/:id/deadlinks/check", middleware.UserRateLimit(5), StartDeadLinkCheck)
	v1.POST("/sites/:id/deadlinks/retry", RetryDeadLinks)
}
