package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/ai"
	"seo-system/internal/db"
	"seo-system/internal/middleware"
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

var sensitiveKeys = map[string]bool{
	"ai_api_key":           true,
	"telegram_bot_token":   true,
	"gsc_client_secret":    true,
	"yandex_client_secret": true,
	"bing_api_key":         true,
}

// perUserOnlyKeys lists setting names that no longer live in the global
// settings table — every user owns their own copy via user_oauth_config /
// user_telegram / user_bing / users.frw_apikey. GetSettings filters them
// out of the admin response; SaveSettings silently drops them on write.
// Keeps the old shared-platform fields from sneaking back in via a stale
// frontend payload after migration.
var perUserOnlyKeys = map[string]bool{
	"gsc_client_id":        true,
	"gsc_client_secret":    true,
	"gsc_redirect_uri":     true,
	"yandex_client_id":     true,
	"yandex_client_secret": true,
	"yandex_redirect_uri":  true,
	"bing_api_key":         true,
	"telegram_bot_token":   true,
	"telegram_chat_id":     true,
	"frw_apikey":           true,
	"ai_api_key":           true,
	"ai_api_url":           true,
	"ai_model":             true,
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
		// Per-user keys never belong in the global admin view anymore;
		// drop even if something left residue in the table.
		if perUserOnlyKeys[key] {
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

		// Reject per-user-only keys: these moved to user_oauth_config /
		// user_telegram / user_bing / users.frw_apikey and must not be
		// writable via the admin settings page.
		if perUserOnlyKeys[key] {
			continue
		}
		// Skip masked values for sensitive keys (user didn't change them).
		if sensitiveKeys[key] && strings.HasPrefix(value, "****") {
			continue
		}
		// Validate auto_audit_cron: must be parseable and interval >= 1 hour.
		if key == "auto_audit_cron" && value != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			sched, err := parser.Parse(value)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "cron 表达式无效: " + err.Error()})
				return
			}
			// Reject intervals shorter than 1 hour.
			now := time.Now()
			next1 := sched.Next(now)
			next2 := sched.Next(next1)
			if next2.Sub(next1) < time.Hour {
				c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "巡检间隔不能小于 1 小时"})
				return
			}
		}
		if err := db.SetSetting(key, value); err != nil {
			// Don't fail the whole request — a single rotten legacy key
			// (e.g. orphaned `ai_model` / `ai_api_url` from the pre-per-user
			// AI config days) shouldn't block saving something simple like
			// `system_title`. Log the failure and move on.
			log.Printf("[settings] SetSetting(%q) failed: %v", key, err)
			failed = append(failed, key)
		}
		// Eagerly invalidate the in-process IP whitelist cache whenever the
		// setting is saved so the new list takes effect on the next request
		// instead of waiting for the TTL to expire.
		if key == "ip_whitelist" {
			middleware.InvalidateIPWhitelistCache()
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

// TestTelegram exercises the CALLER'S own bot — each user has their own
// under the per-user model. The admin-only legacy /settings/test-telegram
// endpoint therefore tests the admin's Telegram config.
func TestTelegram(c *gin.Context) {
	botToken, chatID, err := LoadUserTelegram(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "error": "Telegram未配置"})
		return
	}
	if err := telegram.Test(botToken, chatID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "Telegram推送测试成功"})
}
