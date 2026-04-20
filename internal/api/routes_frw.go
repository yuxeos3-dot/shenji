package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerFRWRoutes handles FRW image/video generation endpoints.
func registerFRWRoutes(v1 *gin.RouterGroup) {
	// Static file serving for generated content
	v1.GET("/data/frw/:kind/:filename", ServeFRWFile)

	frw := v1.Group("/frw")
	{
		frw.GET("/config", GetMyFRWConfig)
		frw.PUT("/config", SaveMyFRWConfig)
		frw.GET("/generations", RequirePermission("ai_gen.view"), ListFRWGenerations)
		frw.POST("/generations", RequirePermission("ai_gen.submit"), middleware.UserRateLimit(10), SubmitFRWGeneration)
		frw.GET("/generations/:id", RequirePermission("ai_gen.view"), GetFRWGeneration)
		frw.POST("/generations/:id/publish", RequirePermission("ai_gen.publish"), PublishFRWGeneration)
		frw.DELETE("/generations/:id", RequirePermission("ai_gen.delete"), DeleteFRWGeneration)
	}
}
