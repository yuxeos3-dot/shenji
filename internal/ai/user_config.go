package ai

import (
	"encoding/json"
	"log"

	"seo-system/internal/db"
	"seo-system/internal/secure"
)

// AISlot represents a single AI API configuration slot.
type AISlot struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

// UserAIConfig holds the per-user AI configuration (up to 3 slots).
type UserAIConfig struct {
	Slots []AISlot `json:"slots"`
}

// ResolveUserAI picks the first valid AI slot from a user's config.
// Returns (apiURL, apiKey, model). All empty if none configured.
func ResolveUserAI(userID int64) (string, string, string) {
	var raw string
	db.DB.QueryRow("SELECT COALESCE(ai_configs,'') FROM users WHERE id=$1", userID).Scan(&raw)
	var cfg UserAIConfig
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			log.Printf("[ai] unmarshal ai_configs for user %d failed: %v", userID, err)
		}
	}
	// Decrypt API keys
	for i := range cfg.Slots {
		if cfg.Slots[i].APIKey != "" {
			dec, err := secure.Decrypt(cfg.Slots[i].APIKey)
			if err != nil {
				log.Printf("[ai] decrypt api_key for user %d slot %d: %v (treating as plaintext)", userID, i, err)
			} else {
				cfg.Slots[i].APIKey = dec
			}
		}
	}
	for _, slot := range cfg.Slots {
		if slot.APIURL != "" && slot.APIKey != "" {
			model := slot.Model
			if model == "" {
				model = "claude-sonnet-4-6"
			}
			return slot.APIURL, slot.APIKey, model
		}
	}
	return "", "", ""
}
