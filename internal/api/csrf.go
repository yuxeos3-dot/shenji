package api

import (
	"net/http"
	"strings"

	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// GetCSRFToken returns the current CSRF token, issuing a new one if absent
// or invalid. Frontend calls this once after login / on app boot and then
// mirrors the value into the `X-CSRF-Token` header on state-changing calls.
func GetCSRFToken(c *gin.Context) {
	existing, _ := c.Cookie(middleware.CSRFCookieName)
	if existing != "" && middleware.ValidateCSRFToken(existing) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"token": existing}})
		return
	}
	token := middleware.IssueCSRFToken(c, isHTTPS(c))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"token": token}})
}

// isHTTPS best-effort detects whether the original request came over TLS so
// we can flag cookies Secure. Reverse-proxy friendly: checks TLS field and
// `X-Forwarded-Proto` header.
func isHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}
