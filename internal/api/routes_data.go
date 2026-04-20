package api

import "github.com/gin-gonic/gin"

// registerDataRoutes handles OAuth URLs and platform dashboard endpoints.
func registerDataRoutes(v1 *gin.RouterGroup) {
	// GSC
	v1.GET("/oauth/gsc", GetGSCAuthURL)
	v1.POST("/oauth/gsc/token", SaveGSCToken)
	v1.POST("/gsc/store-keywords", RequirePermission("data.sync_gsc"), StoreSiteKeywords)
	v1.POST("/sites/:id/gsc/sync", RequirePermission("data.sync_gsc"), SyncGSCData)
	v1.GET("/sites/:id/gsc", RequirePermission("data.view"), GetGSCData)
	v1.GET("/gsc/sites", RequirePermission("data.view"), GSCListSites)
	v1.GET("/gsc/overview", RequirePermission("data.view"), GSCOverview)
	v1.GET("/gsc/site-detail", RequirePermission("data.view"), GSCSiteDetail)

	// GA4
	v1.GET("/oauth/ga", GetGAAuthURL)
	v1.POST("/sites/:id/ga/sync", RequirePermission("data.sync_ga"), SyncGAData)
	v1.GET("/sites/:id/ga", RequirePermission("data.view"), GetGAData)
	v1.GET("/ga4/properties", RequirePermission("data.view"), GAListProperties)
	v1.GET("/ga4/overview", RequirePermission("data.view"), GAOverview)
	v1.GET("/ga4/property-detail", RequirePermission("data.view"), GAPropertyDetail)

	// Yandex
	v1.GET("/oauth/yandex", GetYandexAuthURL)
	v1.POST("/sites/:id/yandex/sync", RequirePermission("data.sync_yandex"), SyncYandexData)
	v1.GET("/sites/:id/yandex", RequirePermission("data.view"), GetYandexData)
	v1.GET("/yandex/hosts", RequirePermission("data.view"), YandexListHosts)
	v1.GET("/yandex/overview", RequirePermission("data.view"), YandexOverview)
	v1.GET("/yandex/host-detail", RequirePermission("data.view"), YandexHostDetail)

	// Bing
	v1.POST("/sites/:id/bing/sync", RequirePermission("data.sync_bing"), SyncBingData)
	v1.GET("/sites/:id/bing", RequirePermission("data.view"), GetBingData)
	v1.GET("/bing/sites", RequirePermission("data.view"), BingListSites)
	v1.GET("/bing/overview", RequirePermission("data.view"), BingOverview)
	v1.GET("/bing/site-detail", RequirePermission("data.view"), BingSiteDetail)
}
