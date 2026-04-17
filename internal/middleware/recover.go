package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// RecoverLogger catches panics that slip past gin.Default's internal recovery,
// logs a stack trace (not to the client), and returns a generic 500. Pair this
// with structured logging in production for easier triage.
func RecoverLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic] %s %s %v\n%s",
					c.Request.Method, c.Request.URL.Path, r, debug.Stack())
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError,
						gin.H{"code": -1, "error": "internal server error"})
				}
			}
		}()
		c.Next()
	}
}
