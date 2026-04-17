package api

// Per-user notification helpers.
//
// Wraps telegram.NotifyUser so callers in the api package have a
// uniform helper next to the other per-user API operations. Scheduler
// and other non-request contexts call telegram.NotifyUser directly.

import "seo-system/internal/telegram"

// NotifyUser sends a Telegram notification to the given user via their
// configured bot. No-op when the user doesn't have Telegram set up.
func NotifyUser(userID int64, title, message string) {
	telegram.NotifyUser(userID, title, message)
}
