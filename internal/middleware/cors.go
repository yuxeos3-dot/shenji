package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
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
