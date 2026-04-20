package apiutil

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OK sends a standard success response: {"code": 0, "data": ...}
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

// OKMsg sends a success response with a message: {"code": 0, "message": ...}
func OKMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": msg})
}

// Err sends an error response: {"code": -1, "error": ...}
func Err(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": -1, "error": msg})
}

// ErrBadRequest sends a 400 error response.
func ErrBadRequest(c *gin.Context, msg string) {
	Err(c, http.StatusBadRequest, msg)
}

// ErrForbidden sends a 403 error response.
func ErrForbidden(c *gin.Context, msg string) {
	Err(c, http.StatusForbidden, msg)
}

// ErrNotFound sends a 404 error response.
func ErrNotFound(c *gin.Context, msg string) {
	Err(c, http.StatusNotFound, msg)
}

// ErrInternal sends a 500 error response.
func ErrInternal(c *gin.Context, msg string) {
	Err(c, http.StatusInternalServerError, msg)
}

// GetUserID extracts the user_id int64 set by auth middleware.
func GetUserID(c *gin.Context) int64 {
	return c.GetInt64("user_id")
}

// GetUserRole returns the role string set by auth middleware.
func GetUserRole(c *gin.Context) string {
	role, _ := c.Get("role")
	if s, ok := role.(string); ok {
		return s
	}
	return ""
}

// IsAdmin checks if the current user has admin role.
func IsAdmin(c *gin.Context) bool {
	return GetUserRole(c) == "admin"
}
