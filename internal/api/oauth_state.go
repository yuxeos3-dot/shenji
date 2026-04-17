package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// oauthStates stores short-lived CSRF tokens keyed by state string.
// A state is bound to the originating user id and an expiry, and is single-use.
type oauthStateEntry struct {
	UserID    int64
	ExpiresAt time.Time
}

var (
	oauthStatesMu sync.Mutex
	oauthStates   = map[string]oauthStateEntry{}
)

const oauthStateTTL = 10 * time.Minute

// newOAuthState generates a random state, stores it for the current user,
// and returns it. Callers must include it in the OAuth authorize URL.
func newOAuthState(userID int64) (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(buf)

	oauthStatesMu.Lock()
	defer oauthStatesMu.Unlock()
	// Opportunistic GC: drop expired entries.
	now := time.Now()
	for k, v := range oauthStates {
		if now.After(v.ExpiresAt) {
			delete(oauthStates, k)
		}
	}
	oauthStates[state] = oauthStateEntry{UserID: userID, ExpiresAt: now.Add(oauthStateTTL)}
	return state, nil
}

// consumeOAuthState validates and removes a state. It returns the bound
// user id if the state is valid and not expired, or ok=false otherwise.
func consumeOAuthState(state string) (userID int64, ok bool) {
	if state == "" {
		return 0, false
	}
	oauthStatesMu.Lock()
	defer oauthStatesMu.Unlock()
	entry, exists := oauthStates[state]
	if !exists {
		return 0, false
	}
	delete(oauthStates, state)
	if time.Now().After(entry.ExpiresAt) {
		return 0, false
	}
	return entry.UserID, true
}

// requireValidState aborts the request with 400 if the state query parameter
// is missing or invalid. Returns true if ok.
func requireValidState(c *gin.Context) bool {
	if _, ok := consumeOAuthState(c.Query("state")); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "OAuth state 无效或已过期，请重新发起授权"})
		return false
	}
	return true
}
