package middleware

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOrigins caches the parsed ALLOWED_ORIGINS whitelist.
var allowedOrigins []string

// devCORS is true only when GIN_MODE != "release" AND ALLOWED_ORIGINS is empty.
// In release mode with no whitelist, CORS is refused (fail-closed).
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
		if os.Getenv("GIN_MODE") == "release" {
			log.Println("[cors] WARNING: ALLOWED_ORIGINS is empty in release mode — all cross-origin requests will be rejected.")
		} else {
			devCORS = true
			log.Println("[cors] dev mode: reflecting Origin with credentials. Set ALLOWED_ORIGINS before deploying.")
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
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			if allowed {
				c.AbortWithStatus(204)
			} else {
				c.AbortWithStatus(403)
			}
			return
		}
		c.Next()
	}
}
