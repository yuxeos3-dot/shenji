package api

import (
	"context"
	"strings"
	"time"

	"seo-system/internal/db"
)

// accountLockoutWindow is the period over which failures are counted.
// accountLockoutThreshold triggers a lockout once reached.
// accountLockoutDuration is how long the account stays locked.
const (
	accountLockoutWindow    = 15 * time.Minute
	accountLockoutThreshold = 5
	accountLockoutDuration  = 15 * time.Minute
)

// StartLockoutSweeper runs a background goroutine that periodically cleans
// up expired lockout records. It respects the given context for graceful
// shutdown — when ctx is cancelled, the goroutine exits cleanly.
// Call this from main() instead of relying on init() goroutines.
func StartLockoutSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sweepLockouts()
			}
		}
	}()
}

func sweepLockouts() {
	db.DB.Exec("DELETE FROM account_lockouts WHERE locked_until < NOW() - interval '1 hour'")
}

// normalizeEmail lowercases + trims to avoid case-sensitivity sidestepping
// the lockout counter.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// IsAccountLocked returns (locked, retryAfter). If locked it should NOT be
// counted as another failure.
func IsAccountLocked(email string) (bool, time.Duration) {
	key := normalizeEmail(email)
	if key == "" {
		return false, 0
	}
	var lockedUntil time.Time
	err := db.DB.QueryRow(
		"SELECT locked_until FROM account_lockouts WHERE email = $1 AND locked_until > NOW()",
		key,
	).Scan(&lockedUntil)
	if err != nil {
		return false, 0
	}
	return true, time.Until(lockedUntil)
}

// RecordLoginFailure bumps the counter for the given email. Returns true if
// this failure just triggered a lockout.
func RecordLoginFailure(email string) (locked bool) {
	key := normalizeEmail(email)
	if key == "" {
		return false
	}

	// UPSERT: increment failure_count, set locked_until if threshold reached.
	// Reset the counter if the last failure was outside the rolling window.
	var count int
	err := db.DB.QueryRow(`
		INSERT INTO account_lockouts (email, failure_count, last_failure_at, updated_at)
		VALUES ($1, 1, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET
			failure_count = CASE
				WHEN account_lockouts.last_failure_at < NOW() - $2::interval
				THEN 1
				ELSE account_lockouts.failure_count + 1
			END,
			last_failure_at = NOW(),
			updated_at = NOW()
		RETURNING failure_count`,
		key, accountLockoutWindow.String(),
	).Scan(&count)
	if err != nil {
		return false
	}

	if count >= accountLockoutThreshold {
		db.DB.Exec(
			"UPDATE account_lockouts SET locked_until = NOW() + $1::interval WHERE email = $2 AND (locked_until IS NULL OR locked_until < NOW())",
			accountLockoutDuration.String(), key,
		)
		return true
	}
	return false
}

// RecordLoginSuccess clears any pending failures for an account so a
// successful login immediately resets the counter.
func RecordLoginSuccess(email string) {
	key := normalizeEmail(email)
	if key == "" {
		return
	}
	db.DB.Exec("DELETE FROM account_lockouts WHERE email = $1", key)
}

// LockoutStats returns (tracked_accounts, currently_locked) for the cache UI.
func LockoutStats() (tracked, locked int) {
	db.DB.QueryRow("SELECT COUNT(*) FROM account_lockouts").Scan(&tracked)
	db.DB.QueryRow("SELECT COUNT(*) FROM account_lockouts WHERE locked_until > NOW()").Scan(&locked)
	return
}

// ClearLockouts wipes all failure counters and unlocks every account.
func ClearLockouts() {
	db.DB.Exec("DELETE FROM account_lockouts")
}
