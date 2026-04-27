package api

import "github.com/gin-gonic/gin"

func registerAuthRoutes(v1 *gin.RouterGroup) {
	v1.POST("/auth/logout", Logout)
	v1.GET("/auth/me", GetMe)
	v1.GET("/auth/csrf", GetCSRFToken)

	// Personal AI / OAuth / Bing config — single user, but the per-user
	// table layout is preserved (singleton row at user_id=1) so the
	// existing handlers keep working without a schema migration.
	v1.GET("/auth/my-ai-config", GetMyAIConfig)
	v1.PUT("/auth/my-ai-config", SaveMyAIConfig)
	v1.POST("/auth/test-my-ai", TestMyAI)

	v1.GET("/auth/my-oauth-config", GetMyOAuthConfigs)
	v1.PUT("/auth/my-oauth-config", PutMyOAuthConfig)
	v1.DELETE("/auth/my-oauth-token", UnbindMyOAuthToken)

	v1.GET("/auth/my-bing", GetMyBing)
	v1.PUT("/auth/my-bing", PutMyBing)
}
