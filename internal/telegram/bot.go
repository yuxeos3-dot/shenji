package telegram

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/secure"
)

// escapeMD escapes Telegram Markdown V1 special characters.
func escapeMD(s string) string {
	for _, ch := range []string{"_", "*", "`", "["} {
		s = strings.ReplaceAll(s, ch, "\\"+ch)
	}
	return s
}

// Send delivers a formatted notification via the supplied bot token and
// chat id. The caller is responsible for resolving credentials.
func Send(botToken, chatID, title, message string) {
	if botToken == "" || chatID == "" {
		log.Printf("Telegram not configured, skipping notification: %s", title)
		return
	}
	text := fmt.Sprintf("*%s*\n%s", escapeMD(title), escapeMD(message))
	sendMessage(botToken, chatID, text)
}

// NotifyUser looks up the given user's per-user bot token + chat id from
// user_telegram and delivers the notification via their own bot. Used
// from non-request contexts like the audit scheduler where we don't
// have a gin.Context but still want notifications routed correctly.
// No-op when the user hasn't configured Telegram.
func NotifyUser(userID int64, title, message string) {
	if userID <= 0 {
		return
	}
	var enc, chatID string
	err := db.DB.QueryRow(
		`SELECT COALESCE(bot_token,''), COALESCE(chat_id,'') FROM user_telegram WHERE user_id = ?`,
		userID,
	).Scan(&enc, &chatID)
	if err != nil {
		return
	}
	if enc == "" || chatID == "" {
		return
	}
	botToken, err := secure.Decrypt(enc)
	if err != nil {
		log.Printf("[telegram] decrypt bot_token for user %d failed: %v", userID, err)
		return
	}
	Send(botToken, chatID, title, message)
}

// redactToken replaces the bot token in a string with a placeholder.
// net/http wraps errors with the full URL (including our embedded bot
// token) — naïvely logging those errors would spill the secret.
func redactToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[REDACTED]")
}

func sendMessage(botToken, chatID, text string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", text)
	params.Set("parse_mode", "Markdown")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(apiURL, params)
	if err != nil {
		log.Printf("Telegram send failed: %s", redactToken(err.Error(), botToken))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		log.Printf("Telegram API error (%d): %s", resp.StatusCode, redactToken(string(body), botToken))
	}
}

// Test verifies the given bot token + chat id by actually posting a
// tiny message. Returns nil on HTTP 200. Used by the PersonalCenter UI
// so each user can sanity-check their own bot before relying on it.
func Test(botToken, chatID string) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("Telegram未配置")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", "SEO巡站系统测试通知 - 连接成功!")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(apiURL, params)
	if err != nil {
		return fmt.Errorf("发送失败: %s", redactToken(err.Error(), botToken))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return fmt.Errorf("API错误 (%d): %s", resp.StatusCode, redactToken(string(body), botToken))
	}

	return nil
}
