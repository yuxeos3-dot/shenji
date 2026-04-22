package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes wires all API handlers onto the given gin.Engine.
// This replaces the monolithic route block previously in main.go,
// organizing endpoints by domain for maintainability.
func RegisterRoutes(r *gin.Engine, authMW, csrfMW gin.HandlerFunc) {
	r.Use(MetricsCounter())

	// Public routes (no auth required)
	registerPublicRoutes(r)

	// Authenticated + CSRF-protected routes
	v1 := r.Group("/api/v1", authMW, csrfMW)
	registerAuthRoutes(v1)
	registerUserRoutes(v1)
	registerSiteRoutes(v1)
	registerDataRoutes(v1)
	registerReportRoutes(v1)
	registerCoordRoutes(v1)
	registerGeoRoutes(v1)
	registerFRWRoutes(v1)
	registerAdminRoutes(v1)
	registerDeepDataRoutes(v1)

	// RUM beacon endpoints (public, no auth — used by client-side JS)
	r.POST("/api/rum/beacon", RUMBeaconIngest)
	r.POST("/api/rum/event", RUMEventIngest)
}

// registerPublicRoutes handles unauthenticated endpoints.
func registerPublicRoutes(r *gin.Engine) {
	r.GET("/api/healthz", Healthz)
	r.POST("/api/auth/login", middleware.LoginRateLimit(), Login)
	r.POST("/api/auth/register", middleware.LoginRateLimit(), Register)
	r.GET("/api/auth/departments-public", PublicListDepartments)
	r.GET("/api/auth/groups-public", PublicListGroups)
	r.GET("/api/v1/settings/public", GetPublicSettings)
	r.POST("/api/spider/ingest", IngestSpiderLog)

	// OAuth callbacks — public because the browser lands here after the
	// OAuth provider redirects; the state param validates the session.
	r.GET("/api/v1/oauth/gsc/callback", GSCCallback)
	r.GET("/api/v1/oauth/ga/callback", GACallback)
	r.GET("/api/v1/oauth/yandex/callback", YandexCallback)
}
