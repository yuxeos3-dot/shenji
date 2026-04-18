package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"
	"seo-system/internal/secure"

	"github.com/gin-gonic/gin"
)

// ─── User AI Config (per-user API keys with round-robin) ─────────────────────

type AISlot struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

type UserAIConfig struct {
	Slots []AISlot `json:"slots"` // up to 3 slots
}

// GetMyAIConfig returns the current user's AI configuration.
func GetMyAIConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	cfg := loadUserAIConfig(userID)
	// Mask keys for display
	for i := range cfg.Slots {
		k := cfg.Slots[i].APIKey
		if len(k) > 8 {
			cfg.Slots[i].APIKey = k[:4] + "****" + k[len(k)-4:]
		} else if k != "" {
			cfg.Slots[i].APIKey = "****"
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": cfg})
}

// SaveMyAIConfig saves the current user's AI configuration.
// Masked key handling is explicit: any key that matches the display mask
// (first4 + "****" + last4, or the literal "****") is treated as "keep old".
// Any other value (including empty string) overwrites — an empty string from
// the UI means "clear this slot" rather than "preserve silently".
func SaveMyAIConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var cfg UserAIConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	// Limit to 3 slots
	if len(cfg.Slots) > 3 {
		cfg.Slots = cfg.Slots[:3]
	}
	old := loadUserAIConfig(userID)
	for i := range cfg.Slots {
		if i >= len(old.Slots) {
			continue
		}
		oldKey := old.Slots[i].APIKey
		newKey := cfg.Slots[i].APIKey
		if isMaskedKey(newKey, oldKey) {
			cfg.Slots[i].APIKey = oldKey
		}
	}
	// Encrypt API keys at rest before storing
	for i := range cfg.Slots {
		if cfg.Slots[i].APIKey != "" {
			enc, err := secure.Encrypt(cfg.Slots[i].APIKey)
			if err != nil {
				log.Printf("[user_ai] encrypt api_key for user %d slot %d: %v", userID, i, err)
				c.JSON(500, gin.H{"code": -1, "error": "加密失败"})
				return
			}
			cfg.Slots[i].APIKey = enc
		}
	}
	js, _ := json.Marshal(cfg)
	if _, err := db.DB.Exec("UPDATE users SET ai_configs=? WHERE id=?", string(js), userID); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": "AI 配置已保存"})
}

// isMaskedKey detects whether newKey is the UI's masked rendering of oldKey.
func isMaskedKey(newKey, oldKey string) bool {
	if newKey == "****" {
		return true
	}
	if len(oldKey) > 8 && newKey == oldKey[:4]+"****"+oldKey[len(oldKey)-4:] {
		return true
	}
	return false
}

// TestMyAI tests the user's AI configuration (tries each slot in order).
func TestMyAI(c *gin.Context) {
	userID := c.GetInt64("user_id")
	cfg := loadUserAIConfig(userID)
	if len(cfg.Slots) == 0 {
		c.JSON(200, gin.H{"code": -1, "error": "未配置任何 API，请先在个人设置中添加"})
		return
	}
	results := make([]gin.H, 0, len(cfg.Slots))
	for i, slot := range cfg.Slots {
		if slot.APIURL == "" || slot.APIKey == "" {
			results = append(results, gin.H{"slot": i + 1, "status": "skip", "error": "未填写"})
			continue
		}
		err := testAISlot(slot)
		if err != nil {
			results = append(results, gin.H{"slot": i + 1, "status": "fail", "error": err.Error()})
		} else {
			results = append(results, gin.H{"slot": i + 1, "status": "ok", "model": slot.Model})
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": results})
}

func testAISlot(slot AISlot) error {
	model := slot.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	body := map[string]interface{}{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
		"max_tokens": 5,
	}
	bj, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", slot.APIURL+"/chat/completions", bytes.NewReader(bj))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+slot.APIKey)
	client := httpx.Outbound(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func loadUserAIConfig(userID int64) UserAIConfig {
	var raw string
	db.DB.QueryRow("SELECT COALESCE(ai_configs,'') FROM users WHERE id=?", userID).Scan(&raw)
	var cfg UserAIConfig
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			log.Printf("[user_ai] unmarshal ai_configs for user %d failed: %v", userID, err)
		}
	}
	// Decrypt API keys; legacy plaintext values pass through unchanged.
	for i := range cfg.Slots {
		if cfg.Slots[i].APIKey != "" {
			dec, err := secure.Decrypt(cfg.Slots[i].APIKey)
			if err != nil {
				// Legacy plaintext — keep as-is, will be encrypted on next save.
				log.Printf("[user_ai] decrypt api_key for user %d slot %d: %v (treating as plaintext)", userID, i, err)
			} else {
				cfg.Slots[i].APIKey = dec
			}
		}
	}
	return cfg
}

// ─── Resolve AI config: user first, then system fallback, with round-robin ───

// ResolveAI picks the best available AI slot for a user. It tries user's slots
// in order; if all fail or none configured, falls back to system settings.
// Returns (apiURL, apiKey, model).
func ResolveAI(userID int64) (string, string, string) {
	cfg := loadUserAIConfig(userID)
	for _, slot := range cfg.Slots {
		if slot.APIURL != "" && slot.APIKey != "" {
			model := slot.Model
			if model == "" {
				model = "claude-sonnet-4-6"
			}
			return slot.APIURL, slot.APIKey, model
		}
	}
	// No fallback — user must configure their own
	return "", "", ""
}

// CallAIWithUserConfig calls AI using the user's config with automatic failover
// across their configured slots. Returns (content, model_used, error).
func CallAIWithUserConfig(userID int64, prompt string, maxTokens int) (string, string, error) {
	cfg := loadUserAIConfig(userID)

	// Try user's slots first
	for i, slot := range cfg.Slots {
		if slot.APIURL == "" || slot.APIKey == "" {
			continue
		}
		model := slot.Model
		if model == "" {
			model = "claude-sonnet-4-6"
		}
		content, err := callAISlot(slot.APIURL, slot.APIKey, model, prompt, maxTokens)
		if err != nil {
			log.Printf("[ai-roundrobin] slot %d (%s) failed: %v, trying next", i+1, slot.APIURL, err)
			continue
		}
		return content, model, nil
	}

	// No fallback — each user must configure their own API key
	return "", "", fmt.Errorf("AI 调用失败：请先在「个人 AI 设置」中配置你自己的 API Key（至少填写 1 组）")
}

func callAISlot(apiURL, apiKey, model, prompt string, maxTokens int) (string, error) {
	body := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	}
	bj, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("AI 请求体序列化失败: %w", err)
	}
	req, err := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(bj))
	if err != nil {
		return "", fmt.Errorf("AI 请求构造失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := httpx.Outbound(120 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %.500s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("AI 无返回")
	}
	return parsed.Choices[0].Message.Content, nil
}
