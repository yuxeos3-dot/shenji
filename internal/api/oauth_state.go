package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

const oauthStateTTL = 10 * time.Minute

// StartOAuthStateSweeper runs a background goroutine that periodically
// cleans up expired OAuth states. Respects context for graceful shutdown.
func StartOAuthStateSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				db.DB.Exec("DELETE FROM oauth_states WHERE expires_at < NOW()")
			}
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

// translateOAuthError maps the cryptic error codes Google / Yandex put
// in the callback URL (?error=access_denied&error_description=...) into
// a human-readable Chinese hint. The previous behaviour was to ignore
// the error param entirely and fall through to "缺少授权码", which
// made it impossible for an operator to tell whether they'd clicked
// cancel, set up the Cloud project wrong, or hit a scope restriction.
func translateOAuthError(code, desc string) string {
	switch code {
	case "access_denied":
		return "授权被拒绝：你在同意页点了「取消」，或 OAuth 同意屏幕未添加 webmasters.readonly / analytics.readonly scope。请到 Google Cloud Console 检查 OAuth 同意屏幕，然后重新授权并点「允许」"
	case "invalid_scope":
		return "Scope 无效：请到 Google Cloud Console → OAuth 同意屏幕 → 添加 webmasters.readonly / analytics.readonly"
	case "admin_policy_enforced":
		return "Google Workspace 管理员策略拒绝：联系域管理员允许第三方 OAuth 应用"
	case "disallowed_useragent":
		return "当前浏览器被 Google 禁止 OAuth：请改用 Chrome / Edge / Firefox"
	case "org_internal":
		return "OAuth 应用设为「内部应用」，但授权的 Google 账号不在同一个 Workspace 组织"
	case "redirect_uri_mismatch":
		return "重定向 URI 不匹配：Google Cloud Console 里的授权重定向 URI 必须和后台填的完全一致"
	case "unauthorized_client":
		return "Client ID 无权调用该 API：请到 Google Cloud Console 启用 Search Console API / Analytics Data API"
	case "invalid_client":
		return "Client ID 或 Secret 错误：请在「个人中心 → OAuth 应用凭据」重新填写"
	}
	if desc != "" {
		return code + "：" + desc
	}
	return "授权失败：" + code
}
