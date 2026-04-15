package api

import (
	"net/http"
	"strings"

	"seo-system/internal/ai"
	"seo-system/internal/db"
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
)

var sensitiveKeys = map[string]bool{
	"ai_api_key":           true,
	"telegram_bot_token":   true,
	"gsc_client_secret":    true,
	"yandex_client_secret": true,
}

func maskValue(key, value string) string {
	if !sensitiveKeys[key] || value == "" {
		return value
	}
	if len(value) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

func GetSettings(c *gin.Context) {
	rows, err := db.DB.Query("SELECT key, value FROM settings")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		settings[key] = maskValue(key, value)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": settings})
}

func SaveSettings(c *gin.Context) {
	var settings map[string]string
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	for key, value := range settings {
		// Skip masked values for sensitive keys (user didn't change them)
		if sensitiveKeys[key] && strings.HasPrefix(value, "****") {
			continue
		}
		if err := db.SetSetting(key, value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败: " + key})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "保存成功"})
}

func TestAI(c *gin.Context) {
	if err := ai.TestAIConnection(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "AI接口连接成功"})
}

func TestTelegram(c *gin.Context) {
	if err := telegram.TestConnection(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "Telegram推送测试成功"})
}
