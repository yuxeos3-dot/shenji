package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerGeoRoutes handles GEO content production endpoints.
func registerGeoRoutes(v1 *gin.RouterGroup) {
	geo := v1.Group("/geo")
	{
		geo.GET("/stats", RequirePermission("geo.view"), GeoStats)
		geo.GET("/defaults", RequirePermission("geo.view"), GeoGetDefaults)

		// Title libraries
		geo.GET("/title-libs", RequirePermission("geo.view"), GeoListTitleLibs)
		geo.POST("/title-libs", RequirePermission("geo.keywords"), GeoCreateTitleLib)
		geo.DELETE("/title-libs/:id", RequirePermission("geo.keywords"), GeoDeleteTitleLib)
		geo.GET("/title-libs/:id/titles", RequirePermission("geo.view"), GeoListTitles)
		geo.POST("/title-libs/:id/titles", RequirePermission("geo.keywords"), GeoAddTitles)
		geo.POST("/title-libs/:id/import", RequirePermission("geo.keywords"), GeoImportTitles)
		geo.POST("/title-libs/:id/import-gsc", RequirePermission("geo.keywords"), GeoImportFromGSC)
		geo.DELETE("/titles/:id", RequirePermission("geo.keywords"), GeoDeleteTitle)

		// Articles
		geo.GET("/articles", RequirePermission("geo.articles"), GeoListArticles)
		geo.GET("/articles/:id", RequirePermission("geo.articles"), GeoGetArticle)
		geo.PUT("/articles/:id", RequirePermission("geo.articles"), GeoUpdateArticle)
		geo.DELETE("/articles/:id", RequirePermission("geo.articles_delete"), GeoDeleteArticle)
		geo.POST("/articles/:id/review", RequirePermission("geo.articles_review"), GeoReviewArticle)
		geo.POST("/articles/:id/publish", RequirePermission("geo.articles_publish"), GeoPublishArticle)

		// Keyword workflow
		geo.POST("/keyword/workflow/start", RequirePermission("geo.workflow"), middleware.UserRateLimit(5), GeoStartKeywordWorkflow)
		geo.GET("/keyword/workflow/jobs", RequirePermission("geo.view"), GeoListWorkflowJobs)
		geo.GET("/keyword/workflow/:id", RequirePermission("geo.view"), GeoGetWorkflowJob)
		geo.POST("/keyword/workflow/:id/apply", RequirePermission("geo.workflow"), GeoApplyWorkflowJob)
		geo.DELETE("/keyword/workflow/:id", RequirePermission("geo.workflow"), GeoDiscardWorkflowJob)
	}
}
