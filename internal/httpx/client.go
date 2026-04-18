package httpx

import (
	"net/http"
	"time"

	"seo-system/internal/crawler"
)

// Outbound returns an HTTP client with the SSRF-safe transport.
// Use for all user-triggered outbound calls (AI, OAuth, Telegram, etc.)
func Outbound(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: crawler.SharedTransport(),
	}
}
