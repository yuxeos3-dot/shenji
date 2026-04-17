package api

import (
	"fmt"
	"log"
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
	"bing_api_key":         true,
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

// publicSettingsKeys is the allow-list of setting keys the frontend needs
// before the user has logged in (used to render the login page title /
// tab title / logo). Everything here MUST be safe to expose anonymously.
var publicSettingsKeys = map[string]bool{
	"site_title": true,
}

// GetPublicSettings is an unauthenticated variant of GetSettings that
// returns only the handful of branding keys the login page needs. Anything
// not in the allow-list is refused — don't add sensitive keys here.
func GetPublicSettings(c *gin.Context) {
	settings := make(map[string]string, len(publicSettingsKeys))
	for key := range publicSettingsKeys {
		settings[key] = db.GetSetting(key)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": settings})
}

func SaveSettings(c *gin.Context) {
	// Accept any JSON values (string / null / number) — the frontend used
	// to occasionally send `null` for settings the user hadn't populated,
	// which broke a strict map[string]string bind and failed the whole
	// save. Coerce every value to string here.
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	var failed []string
	for key, v := range raw {
		// Coerce to string. null → "", numbers → canonical representation.
		value := ""
		switch x := v.(type) {
		case string:
			value = x
		case nil:
			value = ""
		case bool:
			if x {
				value = "true"
			} else {
				value = "false"
			}
		default:
			// numbers, objects: stringify via fmt
			value = strings.TrimSpace(strings.Trim(fmt.Sprintf("%v", x), "[]"))
		}

		// Skip masked values for sensitive keys (user didn't change them).
		if sensitiveKeys[key] && strings.HasPrefix(value, "****") {
			continue
		}
		if err := db.SetSetting(key, value); err != nil {
			// Don't fail the whole request — a single rotten legacy key
			// (e.g. orphaned `ai_model` / `ai_api_url` from the pre-per-user
			// AI config days) shouldn't block saving something simple like
			// `system_title`. Log the failure and move on.
			log.Printf("[settings] SetSetting(%q) failed: %v", key, err)
			failed = append(failed, key)
		}
	}

	WriteAuditLog(c, "save_settings", "settings", "", "批量更新系统设置")

	if len(failed) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":   0,
			"data":   "部分保存成功",
			"failed": failed,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "保存成功"})
}

func GetClientIP(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": c.ClientIP()})
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
