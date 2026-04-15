package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS middleware. Allowed origins are configured via the ALLOWED_ORIGINS env var
// (comma-separated). If ALLOWED_ORIGINS is unset or empty, the middleware falls
// back to allowing all origins with the wildcard (safe for single-machine dev),
// but credentials will NOT be sent because we do not set Allow-Credentials.
// For production, always set ALLOWED_ORIGINS to an explicit whitelist.
func CORS() gin.HandlerFunc {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	var allowed []string
	if raw != "" {
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowed = append(allowed, o)
			}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if len(allowed) == 0 {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			for _, a := range allowed {
				if a == origin || a == "*" {
					c.Header("Access-Control-Allow-Origin", origin)
					c.Header("Vary", "Origin")
					break
				}
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
