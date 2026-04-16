package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOrigins caches the parsed ALLOWED_ORIGINS whitelist.
var allowedOrigins []string

func init() {
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}
}

func isOriginAllowed(origin string) bool {
	if len(allowedOrigins) == 0 {
		return true // no whitelist configured — allow all (dev mode)
	}
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
		if origin != "" && isOriginAllowed(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		} else if len(allowedOrigins) == 0 {
			// No whitelist: allow all (backwards compat for dev)
			c.Header("Access-Control-Allow-Origin", "*")
		}
		// If origin is not allowed and whitelist is set, no ACAO header → browser blocks

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
