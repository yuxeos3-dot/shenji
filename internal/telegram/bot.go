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
)

// escapeMD escapes Telegram Markdown V1 special characters.
func escapeMD(s string) string {
	for _, ch := range []string{"_", "*", "`", "["} {
		s = strings.ReplaceAll(s, ch, "\\"+ch)
	}
	return s
}

func SendNotification(title, message string) {
	botToken := db.GetSetting("telegram_bot_token")
	chatID := db.GetSetting("telegram_chat_id")

	if botToken == "" || chatID == "" {
		log.Printf("Telegram not configured, skipping notification: %s", title)
		return
	}

	text := fmt.Sprintf("*%s*\n%s", escapeMD(title), escapeMD(message))
	sendMessage(botToken, chatID, text)
}

func SendToUser(telegramID, title, message string) {
	botToken := db.GetSetting("telegram_bot_token")
	if botToken == "" || telegramID == "" {
		return
	}

	text := fmt.Sprintf("*%s*\n%s", escapeMD(title), escapeMD(message))
	sendMessage(botToken, telegramID, text)
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

func TestConnection() error {
	botToken := db.GetSetting("telegram_bot_token")
	chatID := db.GetSetting("telegram_chat_id")

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
