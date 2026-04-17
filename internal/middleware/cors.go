package middleware

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOrigins caches the parsed ALLOWED_ORIGINS whitelist.
var allowedOrigins []string

// devCORS is true only when ALLOW_DEV_CORS=1 is explicitly set AND
// ALLOWED_ORIGINS is empty. Previously this defaulted to "on" whenever
// GIN_MODE != release, which meant forgetting to set GIN_MODE=release in
// production silently reflected any Origin + credentials.
var devCORS bool

func init() {
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}
	if len(allowedOrigins) == 0 {
		if os.Getenv("ALLOW_DEV_CORS") == "1" {
			devCORS = true
			log.Println("[cors] ALLOW_DEV_CORS=1 — reflecting any Origin with credentials. DO NOT use in production.")
		} else {
			log.Println("[cors] ALLOWED_ORIGINS is empty — all cross-origin requests rejected (fail-closed). Set ALLOWED_ORIGINS=https://example.com or ALLOW_DEV_CORS=1 for local dev.")
		}
	}
}

func isOriginAllowed(origin string) bool {
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := origin != "" && (isOriginAllowed(origin) || devCORS)
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		// If origin is not allowed, no ACAO header → browser blocks the response.

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// X-CSRF-Token must be listed so browsers let the double-submit
		// header through on cross-origin preflight. Without it, every
		// cross-origin write from the SPA is rejected.
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-CSRF-Token, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			// Always return 204 for preflight; disallowed origins simply
			// won't receive ACAO headers, so the browser blocks the
			// response. Returning 403 leaks whitelist membership.
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
