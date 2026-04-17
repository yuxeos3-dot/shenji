package api

// Per-user platform credentials.
//
// Design goal: "every user is their own system" — no user can see another
// user's API keys, OAuth tokens, Telegram bot, or FRW credentials. The
// admin keeps global monitoring visibility via /users/:id/platform-status,
// but admin can't read plaintext secrets either.
//
// Storage:
//   - OAuth app credentials (client_id / client_secret / redirect_uri) →
//     user_oauth_config, one row per (user_id, service).
//   - OAuth access / refresh tokens → user_oauth_tokens, one row per
//     (user_id, service). Replaces the old global oauth_tokens table.
//   - Telegram bot + chat → user_telegram, one row per user.
//   - Bing API key → user_bing, one row per user.
//   - FRW apikey → users.frw_apikey column (FRW bearer / user_id still
//     live in users.frw_config because they're semi-transient browser
//     credentials the user re-pastes every few hours).
//
// All string secrets are encrypted at rest via EncryptToken.

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Types ─────────────────────────────────────────────────────────────────

type oauthAppConfig struct {
	Service      string `json:"service"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"` // returned masked on GET, written raw on PUT
	RedirectURI  string `json:"redirect_uri"`
	Bound        bool   `json:"bound"` // true when there's also a saved access token
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// ─── Loaders used by other backend code ────────────────────────────────────

// LoadUserOAuthClient returns the user's app credentials for a service.
// Secrets come back decrypted so the caller can use them directly on the
// outbound token endpoint. Error is sql.ErrNoRows if the user never saved
// anything — the calling handler should turn that into a clear 400 message
// like "请先在个人中心填写 GSC 的 client_id / client_secret".
func LoadUserOAuthClient(userID int64, service string) (clientID, clientSecret, redirectURI string, err error) {
	var cidStr, csStr string
	err = db.DB.QueryRow(
		`SELECT COALESCE(client_id,''), COALESCE(client_secret,''), COALESCE(redirect_uri,'')
		 FROM user_oauth_config WHERE user_id = ? AND service = ?`,
		userID, service,
	).Scan(&cidStr, &csStr, &redirectURI)
	if err != nil {
		return "", "", "", err
	}
	clientID = cidStr
	if csStr != "" {
		if dec, dErr := DecryptToken(csStr); dErr == nil {
			clientSecret = dec
		} else {
			// secret was stored before encryption was enabled, treat as plaintext
			clientSecret = csStr
		}
	}
	return
}

// LoadUserBingKey returns the user's Bing API key (decrypted).
func LoadUserBingKey(userID int64) (string, error) {
	var enc string
	err := db.DB.QueryRow(`SELECT COALESCE(api_key,'') FROM user_bing WHERE user_id = ?`, userID).Scan(&enc)
	if err != nil {
		return "", err
	}
	if enc == "" {
		return "", nil
	}
	if dec, dErr := DecryptToken(enc); dErr == nil {
		return dec, nil
	}
	return enc, nil
}

// LoadUserTelegram returns the user's bot token + chat id (token decrypted).
func LoadUserTelegram(userID int64) (botToken, chatID string, err error) {
	var enc string
	err = db.DB.QueryRow(
		`SELECT COALESCE(bot_token,''), COALESCE(chat_id,'') FROM user_telegram WHERE user_id = ?`,
		userID,
	).Scan(&enc, &chatID)
	if err != nil {
		return "", "", err
	}
	if enc == "" {
		return "", chatID, nil
	}
	if dec, dErr := DecryptToken(enc); dErr == nil {
		botToken = dec
	} else {
		botToken = enc
	}
	return
}

// LoadUserFrwApiKey returns the user's FRW apikey (decrypted). Falls back to
// the empty string when the user hasn't configured one — the caller should
// refuse to call FRW in that case rather than falling back to a global.
func LoadUserFrwApiKey(userID int64) string {
	var enc string
	if err := db.DB.QueryRow(`SELECT COALESCE(frw_apikey,'') FROM users WHERE id = ?`, userID).Scan(&enc); err != nil {
		return ""
	}
	if enc == "" {
		return ""
	}
	if dec, err := DecryptToken(enc); err == nil {
		return dec
	}
	return enc
}

// ─── GET handlers ──────────────────────────────────────────────────────────

// GetMyOAuthConfigs GET /api/v1/auth/my-oauth-config
//
// Returns all three OAuth app configs for the caller in one payload so
// the personal-center tab can render them together. Secrets are replaced
// with a fixed masked placeholder; the frontend shows it as ●●●●●●, and
// only overwrites when the user types a new value (empty on submit =
// "leave as-is").
func GetMyOAuthConfigs(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	out := []oauthAppConfig{}
	for _, svc := range []string{"gsc", "ga", "yandex"} {
		cfg := oauthAppConfig{Service: svc}
		var clientID, clientSecretEnc, redirectURI string
		err := db.DB.QueryRow(
			`SELECT COALESCE(client_id,''), COALESCE(client_secret,''), COALESCE(redirect_uri,'')
			 FROM user_oauth_config WHERE user_id = ? AND service = ?`,
			uid, svc,
		).Scan(&clientID, &clientSecretEnc, &redirectURI)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("[personal-config] load %s for user %d: %v", svc, uid, err)
		}
		cfg.ClientID = clientID
		cfg.RedirectURI = redirectURI
		if clientSecretEnc != "" {
			cfg.ClientSecret = "●●●●●●"
		}
		// Bound status: presence of an access token.
		var expiresAt sql.NullString
		if err := db.DB.QueryRow(
			`SELECT COALESCE(expires_at,'') FROM user_oauth_tokens WHERE user_id = ? AND service = ?`,
			uid, svc,
		).Scan(&expiresAt); err == nil {
			cfg.Bound = true
			if expiresAt.Valid {
				cfg.ExpiresAt = expiresAt.String
			}
		}
		out = append(out, cfg)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// PutMyOAuthConfig PUT /api/v1/auth/my-oauth-config
// Body: { service, client_id, client_secret, redirect_uri }
// Empty client_secret means "don't overwrite".
func PutMyOAuthConfig(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var req struct {
		Service      string `json:"service" binding:"required"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURI  string `json:"redirect_uri"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if req.Service != "gsc" && req.Service != "ga" && req.Service != "yandex" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "service 必须是 gsc / ga / yandex"})
		return
	}
	if len(req.ClientID) > 512 || len(req.ClientSecret) > 512 || len(req.RedirectURI) > 1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "字段过长"})
		return
	}

	// Build the upsert. If ClientSecret is empty we want to preserve the
	// existing value, so we branch: an empty secret falls back to the
	// existing encrypted blob via COALESCE.
	var secretEnc string
	if req.ClientSecret != "" {
		enc, err := EncryptToken(req.ClientSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "加密失败"})
			return
		}
		secretEnc = enc
	}

	if secretEnc != "" {
		_, err := db.DB.Exec(
			`INSERT INTO user_oauth_config (user_id, service, client_id, client_secret, redirect_uri)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, service) DO UPDATE SET
			   client_id = excluded.client_id,
			   client_secret = excluded.client_secret,
			   redirect_uri = excluded.redirect_uri,
			   updated_at = CURRENT_TIMESTAMP`,
			uid, req.Service, req.ClientID, secretEnc, req.RedirectURI,
		)
		if err != nil {
			log.Printf("[personal-config] upsert %s secret for user %d: %v", req.Service, uid, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
			return
		}
	} else {
		// Preserve existing client_secret.
		_, err := db.DB.Exec(
			`INSERT INTO user_oauth_config (user_id, service, client_id, client_secret, redirect_uri)
			 VALUES (?, ?, ?, '', ?)
			 ON CONFLICT(user_id, service) DO UPDATE SET
			   client_id = excluded.client_id,
			   redirect_uri = excluded.redirect_uri,
			   updated_at = CURRENT_TIMESTAMP`,
			uid, req.Service, req.ClientID, req.RedirectURI,
		)
		if err != nil {
			log.Printf("[personal-config] upsert %s (no secret) for user %d: %v", req.Service, uid, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
			return
		}
	}
	WriteAuditLog(c, "oauth_config_update", "user_oauth_config", req.Service, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存"})
}

// UnbindMyOAuthToken DELETE /api/v1/auth/my-oauth-token?service=gsc
// Deletes the access+refresh token row so the user can re-authorize.
// Leaves the client_id/client_secret app credentials in place.
func UnbindMyOAuthToken(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	svc := c.Query("service")
	if svc != "gsc" && svc != "ga" && svc != "yandex" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "service 非法"})
		return
	}
	if _, err := db.DB.Exec(`DELETE FROM user_oauth_tokens WHERE user_id = ? AND service = ?`, uid, svc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解绑失败"})
		return
	}
	WriteAuditLog(c, "oauth_unbind", "user_oauth_tokens", svc, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已解绑"})
}

// ─── Telegram / Bing / FRW per-user endpoints ──────────────────────────────

func GetMyTelegram(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var enc, chatID string
	err := db.DB.QueryRow(
		`SELECT COALESCE(bot_token,''), COALESCE(chat_id,'') FROM user_telegram WHERE user_id = ?`,
		uid,
	).Scan(&enc, &chatID)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[personal-config] GetMyTelegram uid=%d: %v", uid, err)
	}
	masked := ""
	if enc != "" {
		masked = "●●●●●●"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"bot_token": masked,
		"chat_id":   chatID,
		"bound":     enc != "" && chatID != "",
	}})
}

func PutMyTelegram(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var req struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if len(req.BotToken) > 256 || len(req.ChatID) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "字段过长"})
		return
	}
	if req.BotToken != "" {
		enc, err := EncryptToken(req.BotToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "加密失败"})
			return
		}
		_, err = db.DB.Exec(
			`INSERT INTO user_telegram (user_id, bot_token, chat_id) VALUES (?, ?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET
			   bot_token = excluded.bot_token,
			   chat_id = excluded.chat_id,
			   updated_at = CURRENT_TIMESTAMP`,
			uid, enc, req.ChatID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
			return
		}
	} else {
		// Update only chat_id; keep token as-is.
		_, err := db.DB.Exec(
			`INSERT INTO user_telegram (user_id, bot_token, chat_id) VALUES (?, '', ?)
			 ON CONFLICT(user_id) DO UPDATE SET
			   chat_id = excluded.chat_id,
			   updated_at = CURRENT_TIMESTAMP`,
			uid, req.ChatID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
			return
		}
	}
	WriteAuditLog(c, "telegram_config_update", "user_telegram", fmt.Sprintf("%d", uid), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存"})
}

func GetMyBing(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var enc string
	if err := db.DB.QueryRow(`SELECT COALESCE(api_key,'') FROM user_bing WHERE user_id = ?`, uid).Scan(&enc); err != nil && err != sql.ErrNoRows {
		log.Printf("[personal-config] GetMyBing uid=%d: %v", uid, err)
	}
	masked := ""
	if enc != "" {
		masked = "●●●●●●"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"api_key": masked,
		"bound":   enc != "",
	}})
}

func PutMyBing(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if len(req.APIKey) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "api_key 过长"})
		return
	}
	if strings.TrimSpace(req.APIKey) == "" {
		// empty = unbind
		if _, err := db.DB.Exec(`DELETE FROM user_bing WHERE user_id = ?`, uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "清除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已清除"})
		return
	}
	enc, err := EncryptToken(req.APIKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "加密失败"})
		return
	}
	if _, err := db.DB.Exec(
		`INSERT INTO user_bing (user_id, api_key) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   api_key = excluded.api_key,
		   updated_at = CURRENT_TIMESTAMP`,
		uid, enc,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	WriteAuditLog(c, "bing_config_update", "user_bing", fmt.Sprintf("%d", uid), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存"})
}

func GetMyFrw(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var enc string
	if err := db.DB.QueryRow(`SELECT COALESCE(frw_apikey,'') FROM users WHERE id = ?`, uid).Scan(&enc); err != nil && err != sql.ErrNoRows {
		log.Printf("[personal-config] GetMyFrw uid=%d: %v", uid, err)
	}
	masked := ""
	if enc != "" {
		masked = "●●●●●●"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"api_key": masked,
		"bound":   enc != "",
	}})
}

func PutMyFrw(c *gin.Context) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if len(req.APIKey) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "api_key 过长"})
		return
	}
	if strings.TrimSpace(req.APIKey) == "" {
		if _, err := db.DB.Exec(`UPDATE users SET frw_apikey = '' WHERE id = ?`, uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "清除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已清除"})
		return
	}
	enc, err := EncryptToken(req.APIKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "加密失败"})
		return
	}
	if _, err := db.DB.Exec(`UPDATE users SET frw_apikey = ? WHERE id = ?`, enc, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	WriteAuditLog(c, "frw_config_update", "users.frw_apikey", fmt.Sprintf("%d", uid), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存"})
}

// ─── Admin monitoring ──────────────────────────────────────────────────────

// GetUserPlatformStatus GET /api/v1/users/:id/platform-status  (admin only)
// Returns a summary of which platforms the target user has configured.
// Does NOT return plaintext secrets — just boolean presence + OAuth
// token expiry timestamps. Used by the Users page to let admins see who
// still needs to finish onboarding.
func GetUserPlatformStatus(c *gin.Context) {
	var uid int64
	if _, err := fmtSscanInt64(c.Param("id"), &uid); err != nil || uid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "用户 ID 非法"})
		return
	}
	ps := platformsSummary(uid)
	ps["user_id"] = uid
	ps["sites"] = userSiteCount(uid)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": ps})
}

func oauthStatus(uid int64, svc string) gin.H {
	var cid, secret string
	db.DB.QueryRow(
		`SELECT COALESCE(client_id,''), COALESCE(client_secret,'') FROM user_oauth_config WHERE user_id = ? AND service = ?`,
		uid, svc,
	).Scan(&cid, &secret)
	var expiresAt sql.NullString
	db.DB.QueryRow(
		`SELECT COALESCE(expires_at,'') FROM user_oauth_tokens WHERE user_id = ? AND service = ?`,
		uid, svc,
	).Scan(&expiresAt)
	h := gin.H{
		"has_client_id":     cid != "",
		"has_client_secret": secret != "",
		"bound":             false,
	}
	if expiresAt.Valid && expiresAt.String != "" {
		h["bound"] = true
		h["expires_at"] = expiresAt.String
	}
	return h
}

func simpleBound(uid int64, sqlStmt string) gin.H {
	var v string
	db.DB.QueryRow(sqlStmt, uid).Scan(&v)
	return gin.H{"bound": v != ""}
}

func telegramBound(uid int64) gin.H {
	var tok, chat string
	db.DB.QueryRow(`SELECT COALESCE(bot_token,''), COALESCE(chat_id,'') FROM user_telegram WHERE user_id = ?`, uid).Scan(&tok, &chat)
	return gin.H{"bound": tok != "" && chat != ""}
}

func aiBound(uid int64) gin.H {
	cfg := loadUserAIConfig(uid)
	filled := 0
	for _, s := range cfg.Slots {
		if s.APIURL != "" && s.APIKey != "" {
			filled++
		}
	}
	return gin.H{"bound": filled > 0, "slots": filled}
}

func userSiteCount(uid int64) int {
	var n int
	db.DB.QueryRow(`SELECT COUNT(*) FROM sites WHERE created_by = ?`, uid).Scan(&n)
	return n
}

// GetUserOverview GET /api/v1/users/:id/overview (admin only)
// High-level snapshot used by the 用户监控 page header: basic profile,
// platform binding booleans, site count by status, coord task count
// by status. Counts only — detail drills use the existing sites /
// coord endpoints with ?user_id / ?involved_user_id filters.
func GetUserOverview(c *gin.Context) {
	var uid int64
	if _, err := fmtSscanInt64(c.Param("id"), &uid); err != nil || uid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "用户 ID 非法"})
		return
	}

	// Basic profile. Email + role help admin distinguish members with
	// the same display name; department + group show org placement.
	var name, email, role, deptName, groupName string
	var lastLogin sql.NullString
	err := db.DB.QueryRow(`
		SELECT u.name, u.email, u.role, COALESCE(u.last_login,''),
		       COALESCE(d.name,''), COALESCE(g.name,'')
		FROM users u
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE u.id = ?`, uid,
	).Scan(&name, &email, &role, &lastLogin, &deptName, &groupName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
		return
	}

	// Site counts by audit status.
	var sitesTotal, sitesHealthy, sitesPending, sitesFailed int
	db.DB.QueryRow(`SELECT COUNT(*) FROM sites WHERE created_by = ?`, uid).Scan(&sitesTotal)
	db.DB.QueryRow(`
		SELECT COUNT(*) FROM sites s
		WHERE s.created_by = ?
		  AND (SELECT status FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) = 'done'`,
		uid).Scan(&sitesHealthy)
	db.DB.QueryRow(`
		SELECT COUNT(*) FROM sites s
		WHERE s.created_by = ?
		  AND (SELECT status FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) IS NULL`,
		uid).Scan(&sitesPending)
	db.DB.QueryRow(`
		SELECT COUNT(*) FROM sites s
		WHERE s.created_by = ?
		  AND (SELECT status FROM audit_tasks WHERE site_id = s.id ORDER BY id DESC LIMIT 1) = 'error'`,
		uid).Scan(&sitesFailed)

	// Coord task counts by status for tasks the user is involved in
	// (created OR assigned). A single query with a CASE/SUM avoids
	// hitting the DB five times.
	var coordTotal, coordPending, coordInProgress, coordUnderReview, coordDone int
	db.DB.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN status='in_progress' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN status='under_review' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END),0)
		FROM coordination_tasks
		WHERE created_by = ? OR assignee_id = ?`,
		uid, uid,
	).Scan(&coordTotal, &coordPending, &coordInProgress, &coordUnderReview, &coordDone)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"user_id":     uid,
		"name":        name,
		"email":       email,
		"role":        role,
		"last_login":  lastLogin.String,
		"department":  deptName,
		"group":       groupName,
		"platforms":   platformsSummary(uid),
		"sites": gin.H{
			"total":     sitesTotal,
			"healthy":   sitesHealthy,
			"pending":   sitesPending,
			"failed":    sitesFailed,
		},
		"coord": gin.H{
			"total":        coordTotal,
			"pending":      coordPending,
			"in_progress":  coordInProgress,
			"under_review": coordUnderReview,
			"done":         coordDone,
		},
	}})
}

// platformsSummary is the inner helper used by both the overview endpoint
// and the existing /users/:id/platform-status — factored out so they
// stay consistent as new platforms get added.
func platformsSummary(uid int64) gin.H {
	return gin.H{
		"gsc":      oauthStatus(uid, "gsc"),
		"ga":       oauthStatus(uid, "ga"),
		"yandex":   oauthStatus(uid, "yandex"),
		"bing":     simpleBound(uid, "SELECT COALESCE(api_key,'') FROM user_bing WHERE user_id = ?"),
		"telegram": telegramBound(uid),
		"frw":      simpleBound(uid, "SELECT COALESCE(frw_apikey,'') FROM users WHERE id = ?"),
		"ai":       aiBound(uid),
	}
}

// fmtSscanInt64 is a tiny helper so we don't pull in fmt.Sscanf at call sites.
func fmtSscanInt64(s string, out *int64) (int, error) {
	var n int
	var err error
	*out, err = parseInt64(s)
	if err == nil {
		n = 1
	}
	return n, err
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}
