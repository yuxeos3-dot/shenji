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
<<<<<<< HEAD
		origin := c.Request.Header.Get("Origin")

		// Allow specific origins: configured domain + localhost for dev
		allowed := false
		allowedOrigins := []string{"http://localhost:5173", "http://localhost:8080", "http://localhost:3000"}

		// Add production domain from env
		if domain := os.Getenv("ALLOWED_ORIGIN"); domain != "" {
			allowedOrigins = append(allowedOrigins, domain)
		}
		// Fallback: allow any https origin if not configured (safe default)
		if len(allowedOrigins) == 2 || strings.HasPrefix(origin, "https://") {
			allowed = true
		}
		for _, o := range allowedOrigins {
			if o == origin {
				allowed = true
				break
			}
		}

		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}

=======
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
>>>>>>> 8543e622ba5aebfb08d6ab5cdfcc71e30d89c895
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
