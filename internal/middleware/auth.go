package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

// MustInitJWT loads JWT_SECRET from the environment and validates it, aborting
// the process if the value is missing or too short to be safe for HS256. This
// used to live in package-level init(), which meant `go test` bailed out in
// CI whenever the test runner wasn't handed a real secret. Splitting it out
// lets main.go call it explicitly at startup while tests can inject a known
// secret via SetJWTSecretForTesting.
func MustInitJWT() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" || len(secret) < 16 {
		log.Fatal("JWT_SECRET environment variable must be set and at least 16 characters long. " +
			"Generate a strong random string, e.g. `openssl rand -hex 32`")
	}
	jwtSecret = []byte(secret)
}

// SetJWTSecretForTesting seeds the package-level secret from a test binary.
// Not for production use — main.go should call MustInitJWT instead.
func SetJWTSecretForTesting(secret []byte) {
	jwtSecret = secret
}

func GetJWTSecret() []byte {
	return jwtSecret
}

type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int64, email, role string) (string, error) {
	// Generate random jti (JWT ID) for unique token identification
	jtiBytes := make([]byte, 16)
	rand.Read(jtiBytes)
	jti := base64.RawURLEncoding.EncodeToString(jtiBytes)

	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// AuthCookieName is the name of the httpOnly cookie that carries the JWT.
// Kept exported so handlers (login/logout) can share the constant.
const AuthCookieName = "auth_token"

// extractJWT finds the JWT in either the auth_token cookie (preferred) or the
// Authorization: Bearer header (fallback, for API clients / legacy frontends).
func extractJWT(c *gin.Context) string {
	if ck, err := c.Cookie(AuthCookieName); err == nil && ck != "" {
		return ck
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}


// ─── Token invalidation cache (30s TTL) ────────────────────────────────────

type tokenInvalidationEntry struct {
	invalidatedAfter int64
	fetchedAt        time.Time
}

var (
	tokenInvalidationCache = make(map[int64]*tokenInvalidationEntry)
	tokenInvalidationMu    sync.RWMutex
)

const tokenInvalidationTTL = 30 * time.Second

// getTokenInvalidatedAfter returns the token_invalidated_after value for a user,
// using a 30s TTL cache to avoid hitting the DB on every request.
func getTokenInvalidatedAfter(userID int64) int64 {
	tokenInvalidationMu.RLock()
	if entry, ok := tokenInvalidationCache[userID]; ok && time.Since(entry.fetchedAt) < tokenInvalidationTTL {
		tokenInvalidationMu.RUnlock()
		return entry.invalidatedAfter
	}
	tokenInvalidationMu.RUnlock()

	var val int64
	db.DB.QueryRow("SELECT COALESCE(token_invalidated_after, 0) FROM users WHERE id = $1", userID).Scan(&val)

	tokenInvalidationMu.Lock()
	tokenInvalidationCache[userID] = &tokenInvalidationEntry{
		invalidatedAfter: val,
		fetchedAt:        time.Now(),
	}
	tokenInvalidationMu.Unlock()
	return val
}

// InvalidateTokenCache removes a user's entry from the cache so the next
// request re-reads from DB. Called after logout / password change.
func InvalidateTokenCache(userID int64) {
	tokenInvalidationMu.Lock()
	delete(tokenInvalidationCache, userID)
	tokenInvalidationMu.Unlock()
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractJWT(c)
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "Token无效或已过期"})
			c.Abort()
			return
		}

		// Check token invalidation: if JWT was issued before the user's
		// token_invalidated_after timestamp, reject it.
		if claims.IssuedAt != nil {
			invalidatedAfter := getTokenInvalidatedAfter(claims.UserID)
			if invalidatedAfter > 0 && claims.IssuedAt.Time.Unix() < invalidatedAfter {
				c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "登录已失效，请重新登录"})
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)

		// Track online presence — keyed by (user, JWT jti) so each
		// distinct login session counts once regardless of how many
		// requests it makes or how Cloudflare load-balances them
		// across edges. IP extracted via realClientIP so CF edge
		// addresses don't end up displayed as the user's "login IP".
		RecordOnline(claims.UserID, claims.Email, claims.Role, realClientIP(c), claims.ID)

		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ─── IP Whitelist (DB-backed with cache) ─────────────────────────────────────

type ipWhitelistEntry struct {
	exact string     // empty when cidr is set
	cidr  *net.IPNet // nil when exact is set
}

var (
	cachedEntries []ipWhitelistEntry
	ipCacheMu     sync.RWMutex
	ipCacheExpiry time.Time
)

const ipCacheTTL = 30 * time.Second

// refreshIPCache reads ip_whitelist from DB and caches parsed entries. Invalid
// lines are logged and dropped so a typo can't silently open access.
func refreshIPCache() []ipWhitelistEntry {
	ipCacheMu.Lock()
	defer ipCacheMu.Unlock()

	if time.Now().Before(ipCacheExpiry) {
		return cachedEntries
	}

	raw := db.GetSetting("ip_whitelist")
	entries := parseIPWhitelist(raw)
	cachedEntries = entries
	ipCacheExpiry = time.Now().Add(ipCacheTTL)
	return entries
}

// parseIPWhitelist splits the setting by newlines / commas and validates each
// entry as a plain IP or a CIDR block. Invalid tokens are logged and skipped.
func parseIPWhitelist(raw string) []ipWhitelistEntry {
	var entries []ipWhitelistEntry
	for _, block := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';'
	}) {
		token := strings.TrimSpace(block)
		if token == "" {
			continue
		}
		if strings.Contains(token, "/") {
			_, ipnet, err := net.ParseCIDR(token)
			if err != nil {
				log.Printf("[ip_whitelist] invalid CIDR %q: %v", token, err)
				continue
			}
			entries = append(entries, ipWhitelistEntry{cidr: ipnet})
			continue
		}
		if ip := net.ParseIP(token); ip != nil {
			entries = append(entries, ipWhitelistEntry{exact: ip.String()})
			continue
		}
		log.Printf("[ip_whitelist] invalid IP %q (ignored)", token)
	}
	return entries
}

// IPWhitelist blocks requests from IPs not in the whitelist.
// If ip_whitelist setting is empty, all IPs are allowed.
func IPWhitelist() gin.HandlerFunc {
	return func(c *gin.Context) {
		entries := getIPWhitelist()
		if len(entries) == 0 {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		parsed := net.ParseIP(clientIP)
		for _, e := range entries {
			if e.cidr != nil {
				if parsed != nil && e.cidr.Contains(parsed) {
					c.Next()
					return
				}
				continue
			}
			if e.exact == clientIP {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "IP 不在白名单中"})
		c.Abort()
	}
}

func getIPWhitelist() []ipWhitelistEntry {
	ipCacheMu.RLock()
	if time.Now().Before(ipCacheExpiry) {
		entries := cachedEntries
		ipCacheMu.RUnlock()
		return entries
	}
	ipCacheMu.RUnlock()
	return refreshIPCache()
}

// IPWhitelistCacheStats returns (entries, next_refresh_at) for /cache UI.
func IPWhitelistCacheStats() (int, time.Time) {
	ipCacheMu.RLock()
	defer ipCacheMu.RUnlock()
	return len(cachedEntries), ipCacheExpiry
}

// InvalidateIPWhitelistCache forces a reload on next request.
func InvalidateIPWhitelistCache() {
	ipCacheMu.Lock()
	cachedEntries = nil
	ipCacheExpiry = time.Time{}
	ipCacheMu.Unlock()
}

// realClientIP resolves the true browser-side IP when the request has
// passed through a CDN (Cloudflare) or trusted reverse proxy. Gin's
// c.ClientIP() uses X-Forwarded-For + trusted proxy list, but behind
// Cloudflare X-F-F can contain the edge POP address, not the end user.
// Cloudflare always sets CF-Connecting-IP to the real client; other
// CDNs use X-Real-IP. Fall back to c.ClientIP() as a last resort.
//
// Without this, the "online users" panel accumulates a handful of
// Cloudflare edge IPs per login (one per POP the browser keepalives to)
// and the user appears to have "5 个会话" when they've only logged in once.
func realClientIP(c *gin.Context) string {
	if ip := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if ip := strings.TrimSpace(c.GetHeader("X-Real-IP")); ip != "" {
		return ip
	}
	return c.ClientIP()
}

// Online user tracking — records each authenticated session's user + IP.
// One entry per distinct JWT jti, so page refreshes / CF edge rotation
// don't create phantom sessions. LastSeen and IP are overwritten on
// every request; a session genuinely logged in from two networks
// flip-flops its displayed IP, which matches real behaviour.
type OnlineEntry struct {
	UserID   int64
	Email    string
	Role     string
	IP       string
	SessID   string // JWT jti — opaque to the caller, used only as a dedup key
	LastSeen time.Time
}

// onlineUsers keyed by "userID:jti". Pre-jti rows (empty jti — only
// possible for tokens issued before this change) collapse to
// "userID:" and are overwritten on the session's next request.
var onlineUsers sync.Map

func RecordOnline(userID int64, email, role, ip, sessID string) {
	key := fmt.Sprintf("%d:%s", userID, sessID)
	onlineUsers.Store(key, &OnlineEntry{
		UserID: userID, Email: email, Role: role, IP: ip,
		SessID: sessID, LastSeen: time.Now(),
	})
}

// GetOnlineUsers returns users active in the last 5 minutes.
func GetOnlineUsers() []OnlineEntry {
	cutoff := time.Now().Add(-5 * time.Minute)
	var result []OnlineEntry
	onlineUsers.Range(func(k, v interface{}) bool {
		e := v.(*OnlineEntry)
		if e.LastSeen.After(cutoff) {
			result = append(result, *e)
		} else {
			onlineUsers.Delete(k) // cleanup stale
		}
		return true
	})
	return result
}

// OnlineUsersSize returns the total number of (user, ip) session entries.
func OnlineUsersSize() int {
	n := 0
	onlineUsers.Range(func(_, _ interface{}) bool { n++; return true })
	return n
}

// ClearOnlineUsers drops every cached presence entry.
func ClearOnlineUsers() {
	onlineUsers.Range(func(k, _ interface{}) bool { onlineUsers.Delete(k); return true })
}

// HeavyLimiterStats / ClearHeavyLimiter expose the per-user rate limiter state.
func HeavyLimiterStats() int {
	heavyLimiter.mu.Lock()
	defer heavyLimiter.mu.Unlock()
	return len(heavyLimiter.requests)
}

func ClearHeavyLimiter() {
	heavyLimiter.mu.Lock()
	defer heavyLimiter.mu.Unlock()
	heavyLimiter.requests = make(map[int64][]time.Time)
}

// LoginLimiterStats / ClearLoginLimiter expose the per-IP login-attempt limiter.
func LoginLimiterStats() int {
	loginLimiter.mu.Lock()
	defer loginLimiter.mu.Unlock()
	return len(loginLimiter.attempts)
}

func ClearLoginLimiter() {
	loginLimiter.mu.Lock()
	defer loginLimiter.mu.Unlock()
	loginLimiter.attempts = make(map[string][]time.Time)
}

// ─── Per-user rate limiter for heavy endpoints ───────────────────────────────

type userRateLimiter struct {
	mu       sync.Mutex
	requests map[int64][]time.Time
}

var heavyLimiter = &userRateLimiter{
	requests: make(map[int64][]time.Time),
}

func init() {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			heavyLimiter.cleanup()
		}
	}()
	// Background cleanup for stale online-user entries (prevents memory leak
	// when admin never opens the online-users page).
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			cleanupStaleOnlineUsers()
		}
	}()
}

func cleanupStaleOnlineUsers() {
	cutoff := time.Now().Add(-5 * time.Minute)
	onlineUsers.Range(func(k, v interface{}) bool {
		entry, ok := v.(*OnlineEntry)
		if !ok || entry.LastSeen.Before(cutoff) {
			onlineUsers.Delete(k)
		}
		return true
	})
}

func (r *userRateLimiter) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for uid, ts := range r.requests {
		fresh := ts[:0]
		for _, t := range ts {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) == 0 {
			delete(r.requests, uid)
		} else {
			r.requests[uid] = fresh
		}
	}
}

// UserRateLimit limits per-user requests to maxPerMinute within a 1-minute window.
// If user_id is missing or non-positive (e.g. unauthenticated request sneaking
// past an upstream check, or JWT decoded into an unexpected type), we refuse
// the request outright — otherwise the rate limiter could be bypassed by any
// request that happens to zero out user_id in the gin context.
func UserRateLimit(maxPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		if userID <= 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录或登录已失效"})
			c.Abort()
			return
		}

		heavyLimiter.mu.Lock()

		now := time.Now()
		window := now.Add(-1 * time.Minute)

		valid := make([]time.Time, 0, len(heavyLimiter.requests[userID]))
		for _, t := range heavyLimiter.requests[userID] {
			if t.After(window) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= maxPerMinute {
			heavyLimiter.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"code": -1, "error": "操作过于频繁，请稍后再试"})
			c.Abort()
			return
		}

		heavyLimiter.requests[userID] = append(valid, now)
		heavyLimiter.mu.Unlock()
		c.Next()
	}
}

// ─── Per-IP login rate limiter ───────────────────────────────────────────────

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

var loginLimiter = &rateLimiter{
	attempts: make(map[string][]time.Time),
}

func init() {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			loginLimiter.cleanup()
		}
	}()
}

func (r *rateLimiter) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for ip, ts := range r.attempts {
		fresh := ts[:0]
		for _, t := range ts {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) == 0 {
			delete(r.attempts, ip)
		} else {
			r.attempts[ip] = fresh
		}
	}
}

func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		loginLimiter.mu.Lock()

		now := time.Now()
		window := now.Add(-1 * time.Minute)

		valid := make([]time.Time, 0, len(loginLimiter.attempts[ip]))
		for _, t := range loginLimiter.attempts[ip] {
			if t.After(window) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= 5 {
			loginLimiter.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"code": -1, "error": "登录尝试过于频繁，请1分钟后重试"})
			c.Abort()
			return
		}

		loginLimiter.attempts[ip] = append(valid, now)
		loginLimiter.mu.Unlock()
		c.Next()
	}
}
