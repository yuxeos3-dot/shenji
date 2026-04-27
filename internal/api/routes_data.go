package api

import "github.com/gin-gonic/gin"

func registerDataRoutes(v1 *gin.RouterGroup) {
	// GSC
	v1.GET("/oauth/gsc", GetGSCAuthURL)
	v1.POST("/oauth/gsc/token", SaveGSCToken)
	v1.POST("/sites/:id/gsc/sync", SyncGSCData)
	v1.GET("/sites/:id/gsc", GetGSCData)
	v1.GET("/gsc/sites", GSCListSites)
	v1.GET("/gsc/overview", GSCOverview)
	v1.GET("/gsc/site-detail", GSCSiteDetail)

	// GA4
	v1.GET("/oauth/ga", GetGAAuthURL)
	v1.POST("/sites/:id/ga/sync", SyncGAData)
	v1.GET("/sites/:id/ga", GetGAData)
	v1.GET("/ga4/properties", GAListProperties)
	v1.GET("/ga4/overview", GAOverview)
	v1.GET("/ga4/property-detail", GAPropertyDetail)

	// Yandex
	v1.GET("/oauth/yandex", GetYandexAuthURL)
	v1.POST("/sites/:id/yandex/sync", SyncYandexData)
	v1.GET("/sites/:id/yandex", GetYandexData)
	v1.GET("/yandex/hosts", YandexListHosts)
	v1.GET("/yandex/overview", YandexOverview)
	v1.GET("/yandex/host-detail", YandexHostDetail)

	// Bing
	v1.POST("/sites/:id/bing/sync", SyncBingData)
	v1.GET("/sites/:id/bing", GetBingData)
	v1.GET("/bing/sites", BingListSites)
	v1.GET("/bing/overview", BingOverview)
	v1.GET("/bing/site-detail", BingSiteDetail)
}
