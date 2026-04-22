package api

import "github.com/gin-gonic/gin"

// registerDeepDataRoutes wires up the "Platform Data Deep" endpoints.
// These supplement (not replace) the existing data routes.
func registerDeepDataRoutes(v1 *gin.RouterGroup) {
	// ── Unified data (single endpoint for all platform data) ─────
	v1.GET("/sites/:id/platform-data", RequirePermission("data.view"), GetUnifiedPlatformData)
	v1.POST("/sites/:id/sync-and-load", RequirePermission("data.view"), SyncAndLoad)

	// ── Site Analysis (全站分析) ──────────────────────────────────
	v1.GET("/sites/:id/health-score", RequirePermission("data.view"), GetSiteHealthScore)
	v1.POST("/sites/:id/health-score/compute", RequirePermission("data.view"), ComputeAndReturnHealth)
	v1.GET("/sites/:id/analysis-summary", RequirePermission("data.view"), GetSiteAnalysisSummary)
	v1.POST("/sites/:id/run-analysis", RequirePermission("data.view"), RunAnalysisPipelineHandler)

	// ── Site ID resolution ────────────────────────────────────────
	v1.GET("/resolve-site-id", RequirePermission("data.view"), ResolveSiteID)

	// ── GSC Deep Sync ─────────────────────────────────────────────
	v1.POST("/sites/:id/gsc/deep-sync", RequirePermission("data.sync_gsc"), SyncGSCDeep)
	v1.POST("/sites/:id/gsc/url-inspection", RequirePermission("data.sync_gsc"), SyncGSCURLInspection)

	// ── GSC Deep Query ────────────────────────────────────────────
	v1.GET("/sites/:id/deep/gsc/overview", RequirePermission("data.view"), GetGSCOverviewDeep)
	v1.GET("/sites/:id/deep/gsc/buckets", RequirePermission("data.view"), GetGSCRankBuckets)
	v1.GET("/sites/:id/deep/gsc/keywords", RequirePermission("data.view"), GetGSCKeywords)
	v1.GET("/sites/:id/deep/gsc/keyword-detail", RequirePermission("data.view"), GetGSCKeywordDetail)
	v1.GET("/sites/:id/deep/gsc/pages", RequirePermission("data.view"), GetGSCPages)
	v1.GET("/sites/:id/deep/gsc/appearance", RequirePermission("data.view"), GetGSCAppearance)
	v1.GET("/sites/:id/deep/gsc/url-inspection", RequirePermission("data.view"), GetGSCUrlInspection)
	v1.GET("/sites/:id/deep/gsc/sitemaps", RequirePermission("data.view"), GetGSCSitemapsData)
	v1.GET("/sites/:id/deep/gsc/opportunities", RequirePermission("data.view"), GetGSCOpportunities)
	v1.GET("/sites/:id/deep/gsc/cannibalization", RequirePermission("data.view"), GetGSCCannibalization)
	v1.GET("/sites/:id/deep/gsc/query-diff", RequirePermission("data.view"), GetGSCQueryDiff)
	v1.GET("/sites/:id/deep/gsc/keywords/export", RequirePermission("data.view"), GetGSCKeywordsExport)

	// ── GA4 Deep Sync ─────────────────────────────────────────────
	v1.POST("/sites/:id/ga/deep-sync", RequirePermission("data.sync_ga"), SyncGA4Deep)

	// ── GA4 Deep Query ────────────────────────────────────────────
	v1.GET("/sites/:id/deep/ga/extended", RequirePermission("data.view"), GetGA4Extended)
	v1.GET("/sites/:id/deep/ga/landing-pages", RequirePermission("data.view"), GetGA4LandingPages)
	v1.GET("/sites/:id/deep/ga/source-medium", RequirePermission("data.view"), GetGA4SourceMedium)
	v1.GET("/sites/:id/deep/ga/events", RequirePermission("data.view"), GetGA4Events)
	v1.GET("/sites/:id/deep/ga/tech", RequirePermission("data.view"), GetGA4Tech)
	v1.GET("/sites/:id/deep/ga/geo", RequirePermission("data.view"), GetGA4Geo)
	v1.GET("/sites/:id/deep/ga/hourly", RequirePermission("data.view"), GetGA4Hourly)
	v1.GET("/sites/:id/deep/ga/day-of-week", RequirePermission("data.view"), GetGA4DayOfWeek)
	v1.GET("/sites/:id/deep/ga/scroll-depth", RequirePermission("data.view"), GetGA4ScrollDepth)
	v1.GET("/sites/:id/deep/ga/outbound-clicks", RequirePermission("data.view"), GetGA4OutboundClicks)
	v1.GET("/sites/:id/deep/ga/conversions", RequirePermission("data.view"), GetGA4ConversionsDaily)

	// ── Bing Deep Sync ────────────────────────────────────────────
	v1.POST("/sites/:id/bing/deep-sync", RequirePermission("data.sync_bing"), SyncBingDeep)

	// ── Bing Deep Query ───────────────────────────────────────────
	v1.GET("/sites/:id/deep/bing/keywords", RequirePermission("data.view"), GetBingKeywords)
	v1.GET("/sites/:id/deep/bing/pages", RequirePermission("data.view"), GetBingPages)
	v1.GET("/sites/:id/deep/bing/inbound-links", RequirePermission("data.view"), GetBingInboundLinks)
	v1.GET("/sites/:id/deep/bing/sitemaps", RequirePermission("data.view"), GetBingSitemaps)

	// ── Yandex Deep Sync ──────────────────────────────────────────
	v1.POST("/sites/:id/yandex/deep-sync", RequirePermission("data.sync_yandex"), SyncYandexDeep)

	// ── Yandex Deep Query ─────────────────────────────────────────
	v1.GET("/sites/:id/deep/yandex/keywords", RequirePermission("data.view"), GetYandexKeywords)
	v1.GET("/sites/:id/deep/yandex/index-events", RequirePermission("data.view"), GetYandexIndexEvents)
	v1.GET("/sites/:id/deep/yandex/http-status", RequirePermission("data.view"), GetYandexHTTPStatus)
	v1.GET("/sites/:id/deep/yandex/external-links", RequirePermission("data.view"), GetYandexExternalLinks)
	v1.GET("/sites/:id/deep/yandex/indexed-urls", RequirePermission("data.view"), GetYandexIndexedUrls)
}
