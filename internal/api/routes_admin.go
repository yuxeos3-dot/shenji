package api

import "github.com/gin-gonic/gin"

func registerAdminRoutes(v1 *gin.RouterGroup) {
	settings := v1.Group("/settings")
	{
		settings.GET("", GetSettings)
		settings.PUT("", SaveSettings)
		settings.POST("/test-ai", TestAI)
	}
}
