package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes wires all API handlers onto the given gin.Engine.
// Single-user / single-password mode — RequirePermission is gone, the
// authMW middleware is the only gate (in addition to CSRF).
func RegisterRoutes(r *gin.Engine, authMW, csrfMW gin.HandlerFunc) {
	registerPublicRoutes(r)

	v1 := r.Group("/api/v1", authMW, csrfMW)
	registerAuthRoutes(v1)
	registerSiteRoutes(v1)
	registerDataRoutes(v1)
	registerAdminRoutes(v1)
}

func registerPublicRoutes(r *gin.Engine) {
	r.GET("/api/healthz", Healthz)
	r.POST("/api/auth/login", middleware.LoginRateLimit(), Login)
	r.GET("/api/v1/settings/public", GetPublicSettings)

	// OAuth callbacks — public because the browser lands here after the
	// OAuth provider redirects; the state param validates the session.
	r.GET("/api/v1/oauth/gsc/callback", GSCCallback)
	r.GET("/api/v1/oauth/ga/callback", GACallback)
	r.GET("/api/v1/oauth/yandex/callback", YandexCallback)
}
