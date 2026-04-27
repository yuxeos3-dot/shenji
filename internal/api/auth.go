package api

import (
	"crypto/subtle"
	"net/http"
	"os"
	"sync"

	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// LoginRequest carries the single-password challenge body. There is no
// username field — single-user mode has exactly one principal.
type LoginRequest struct {
	Password string `json:"password" binding:"required"`
}

// auditBatchOnce guards the env-var read so a missing APP_PASSWORD only logs
// once at first request rather than on every login attempt.
var (
	appPasswordOnce sync.Once
	appPasswordVal  string
)

func appPassword() string {
	appPasswordOnce.Do(func() {
		appPasswordVal = os.Getenv("APP_PASSWORD")
	})
	return appPasswordVal
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	expected := appPassword()
	if expected == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "服务器未配置 APP_PASSWORD"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(expected)) != 1 {
		IncLoginFailure()
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "密码错误"})
		return
	}

	token, err := middleware.GenerateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "生成Token失败"})
		return
	}

	secure := isHTTPS(c)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(middleware.AuthCookieName, token, 24*60*60, "/", "", secure, true /* httpOnly */)
	csrf := middleware.IssueCSRFToken(c, secure)

	go WarmDataCache()

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"csrf_token": csrf,
			"user": gin.H{
				"id":   middleware.SingleUserID,
				"name": "admin",
			},
		},
	})
}

func Logout(c *gin.Context) {
	secure := isHTTPS(c)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(middleware.AuthCookieName, "", -1, "/", "", secure, true)
	c.SetCookie(middleware.CSRFCookieName, "", -1, "/", "", secure, false)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已注销"})
}

func GetMe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"id":   middleware.SingleUserID,
			"name": "admin",
		},
	})
}
