package api

import (
	"strings"
	"sync"
	"time"
)

// accountLockoutWindow is the period over which failures are counted.
// accountLockoutThreshold triggers a lockout once reached.
// accountLockoutDuration is how long the account stays locked.
const (
	accountLockoutWindow    = 15 * time.Minute
	accountLockoutThreshold = 5
	accountLockoutDuration  = 15 * time.Minute
)

type lockoutEntry struct {
	failures    []time.Time
	lockedUntil time.Time
}

var (
	lockoutMu sync.Mutex
	lockouts  = map[string]*lockoutEntry{}
)

// Start a background sweeper so lockouts don't accumulate forever when an
// attacker cycles through many email addresses. Entries are removed once
// they've been unlocked and have no fresh failures inside the rolling
// window.
func init() {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			sweepLockouts()
		}
	}()
}

func sweepLockouts() {
	now := time.Now()
	cutoff := now.Add(-accountLockoutWindow)
	lockoutMu.Lock()
	defer lockoutMu.Unlock()
	for key, entry := range lockouts {
		// Still locked → keep.
		if now.Before(entry.lockedUntil) {
			continue
		}
		// Prune stale failure timestamps.
		kept := entry.failures[:0]
		for _, t := range entry.failures {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		entry.failures = kept
		// No pending lock and no recent failures → drop the entry.
		if len(entry.failures) == 0 {
			delete(lockouts, key)
		}
	}
}

// normalizeEmail lowercases + trims to avoid case-sensitivity sidestepping
// the lockout counter. We key on email because an attacker can rotate IPs
// but needs to guess a stable account.
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
	lockoutMu.Lock()
	defer lockoutMu.Unlock()
	entry := lockouts[key]
	if entry == nil {
		return false, 0
	}
	if now := time.Now(); now.Before(entry.lockedUntil) {
		return true, entry.lockedUntil.Sub(now)
	}
	return false, 0
}

// RecordLoginFailure bumps the counter for the given email. Returns true if
// this failure just triggered a lockout.
func RecordLoginFailure(email string) (locked bool) {
	key := normalizeEmail(email)
	if key == "" {
		return false
	}
	now := time.Now()
	lockoutMu.Lock()
	defer lockoutMu.Unlock()
	entry := lockouts[key]
	if entry == nil {
		entry = &lockoutEntry{}
		lockouts[key] = entry
	}
	// Prune old failures outside the rolling window.
	cutoff := now.Add(-accountLockoutWindow)
	kept := entry.failures[:0]
	for _, t := range entry.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	entry.failures = kept
	if len(entry.failures) >= accountLockoutThreshold && now.After(entry.lockedUntil) {
		entry.lockedUntil = now.Add(accountLockoutDuration)
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
	lockoutMu.Lock()
	defer lockoutMu.Unlock()
	delete(lockouts, key)
}

// LockoutStats returns (tracked_accounts, currently_locked) for the cache UI.
func LockoutStats() (tracked, locked int) {
	now := time.Now()
	lockoutMu.Lock()
	defer lockoutMu.Unlock()
	tracked = len(lockouts)
	for _, e := range lockouts {
		if now.Before(e.lockedUntil) {
			locked++
		}
	}
	return
}

// ClearLockouts wipes all failure counters and unlocks every account.
func ClearLockouts() {
	lockoutMu.Lock()
	defer lockoutMu.Unlock()
	lockouts = map[string]*lockoutEntry{}
}
