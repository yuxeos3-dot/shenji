package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerAuthRoutes handles authentication, personal config, and team endpoints.
func registerAuthRoutes(v1 *gin.RouterGroup) {
	// Team directory (member-safe)
	v1.GET("/team/members", ListTeamMembers)

	// Platform hides (per-user)
	v1.GET("/platform-hides", ListPlatformHides)
	v1.POST("/platform-hides", AddPlatformHide)
	v1.DELETE("/platform-hides", RemovePlatformHide)

	// Auth actions
	v1.POST("/auth/logout", Logout)
	v1.POST("/auth/change-password", middleware.UserRateLimit(10), ChangePassword)
	v1.GET("/auth/me", GetMe)
	v1.GET("/auth/csrf", GetCSRFToken)
	v1.GET("/auth/online", middleware.AdminRequired(), GetOnlineUsersAPI)
	v1.GET("/auth/my-permissions", GetMyPermissions)

	// Personal AI config
	v1.GET("/auth/my-ai-config", GetMyAIConfig)
	v1.PUT("/auth/my-ai-config", SaveMyAIConfig)
	v1.POST("/auth/test-my-ai", TestMyAI)

	// Personal OAuth config
	v1.GET("/auth/my-oauth-config", GetMyOAuthConfigs)
	v1.PUT("/auth/my-oauth-config", PutMyOAuthConfig)
	v1.DELETE("/auth/my-oauth-token", UnbindMyOAuthToken)

	// Personal Telegram
	v1.GET("/auth/my-telegram", GetMyTelegram)
	v1.PUT("/auth/my-telegram", PutMyTelegram)
	v1.POST("/auth/my-telegram/test", TestTelegram)

	// Personal Bing / FRW
	v1.GET("/auth/my-bing", GetMyBing)
	v1.PUT("/auth/my-bing", PutMyBing)
	v1.GET("/auth/my-frw-apikey", GetMyFrw)
	v1.PUT("/auth/my-frw-apikey", PutMyFrw)
}
