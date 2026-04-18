package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

const oauthStateTTL = 10 * time.Minute

// Start a background cleanup goroutine for expired OAuth states.
func init() {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			db.DB.Exec("DELETE FROM oauth_states WHERE expires_at < NOW()")
		}
	}()
}

// newOAuthState generates a random state, stores it for the current user,
// and returns it. Callers must include it in the OAuth authorize URL.
func newOAuthState(userID int64) (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(buf)

	_, err := db.DB.Exec(
		"INSERT INTO oauth_states (state, user_id, expires_at) VALUES ($1, $2, $3)",
		state, userID, time.Now().Add(oauthStateTTL),
	)
	if err != nil {
		return "", err
	}
	return state, nil
}

// consumeOAuthState validates and removes a state. It returns the bound
// user id if the state is valid and not expired, or ok=false otherwise.
func consumeOAuthState(state string) (userID int64, ok bool) {
	if state == "" {
		return 0, false
	}
	// SELECT + DELETE atomically: read the row and delete it in one statement.
	err := db.DB.QueryRow(
		"DELETE FROM oauth_states WHERE state = $1 AND expires_at > NOW() RETURNING user_id",
		state,
	).Scan(&userID)
	if err != nil {
		// Also try to clean up expired entry if it existed.
		db.DB.Exec("DELETE FROM oauth_states WHERE state = $1", state)
		return 0, false
	}
	if userID <= 0 {
		return 0, false
	}
	return userID, true
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

// OAuthStateStats returns the number of pending OAuth states.
func OAuthStateStats() int {
	var count int
	db.DB.QueryRow("SELECT COUNT(*) FROM oauth_states WHERE expires_at > NOW()").Scan(&count)
	return count
}

// ClearOAuthStates drops every in-flight state, effectively canceling any
// OAuth authorize redirects currently waiting on a callback.
func ClearOAuthStates() {
	db.DB.Exec("DELETE FROM oauth_states")
}
