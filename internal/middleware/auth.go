package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" || len(secret) < 16 {
		log.Fatal("JWT_SECRET environment variable must be set and at least 16 characters long. " +
			"Generate a strong random string, e.g. `openssl rand -hex 32`")
	}
	jwtSecret = []byte(secret)
}

func GetJWTSecret() []byte {
	return jwtSecret
}

// GenerateCSRFToken creates a random CSRF token for double-submit cookie pattern.
func GenerateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Printf("crypto/rand failed: %v", err)
		return hex.EncodeToString(b)
	}
	return hex.EncodeToString(b)
}

func cookieSecure() bool {
	return os.Getenv("COOKIE_SECURE") == "true"
}

// SetAuthCookies sets the httpOnly JWT cookie and the readable CSRF cookie.
func SetAuthCookies(c *gin.Context, token, csrfToken string) {
	secure := cookieSecure()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "jwt",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookies removes the auth cookies on logout.
func ClearAuthCookies(c *gin.Context) {
	secure := cookieSecure()
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "jwt", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "csrf_token", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int64, email, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenStr string
		viaCookie := false

		// Try httpOnly cookie first, fall back to Authorization header.
		if cookie, err := c.Cookie("jwt"); err == nil && cookie != "" {
			tokenStr = cookie
			viaCookie = true
		} else {
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
				c.Abort()
				return
			}
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			// Enforce HMAC signing method to prevent alg=none / RSA confusion attacks
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

		// CSRF check for state-changing requests when authenticated via cookie.
		if viaCookie && c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
			csrfHeader := c.GetHeader("X-CSRF-Token")
			csrfCookie, _ := c.Cookie("csrf_token")
			if csrfHeader == "" || csrfCookie == "" || csrfHeader != csrfCookie {
				c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "CSRF token 无效"})
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)

		// Track online presence
		RecordOnline(claims.UserID, claims.Email, claims.Role, c.ClientIP())

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

// Online user tracking — records each authenticated request's user + IP.
type OnlineEntry struct {
	UserID   int64
	Email    string
	Role     string
	IP       string
	LastSeen time.Time
}

var onlineUsers sync.Map // key: "userID:IP" → *OnlineEntry

func RecordOnline(userID int64, email, role, ip string) {
	key := fmt.Sprintf("%d:%s", userID, ip)
	onlineUsers.Store(key, &OnlineEntry{
		UserID: userID, Email: email, Role: role, IP: ip, LastSeen: time.Now(),
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
// Must be placed after AuthRequired so user_id is available.
func UserRateLimit(maxPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		if userID == 0 {
			c.Next()
			return
		}

		heavyLimiter.mu.Lock()
		defer heavyLimiter.mu.Unlock()

		now := time.Now()
		window := now.Add(-1 * time.Minute)

		valid := make([]time.Time, 0, len(heavyLimiter.requests[userID]))
		for _, t := range heavyLimiter.requests[userID] {
			if t.After(window) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= maxPerMinute {
			c.JSON(http.StatusTooManyRequests, gin.H{"code": -1, "error": "操作过于频繁，请稍后再试"})
			c.Abort()
			return
		}

		heavyLimiter.requests[userID] = append(valid, now)
		c.Next()
	}
}

// ─── Per-IP login rate limiter ───────────────────────────────────────────────

// Bounded map; janitor goroutine prunes stale IPs every 10 minutes.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

var loginLimiter = &rateLimiter{
	attempts: make(map[string][]time.Time),
}

func init() {
	// Periodic cleanup to prevent unbounded growth.
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
		defer loginLimiter.mu.Unlock()

		now := time.Now()
		window := now.Add(-1 * time.Minute)

		valid := make([]time.Time, 0, len(loginLimiter.attempts[ip]))
		for _, t := range loginLimiter.attempts[ip] {
			if t.After(window) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= 5 {
			c.JSON(http.StatusTooManyRequests, gin.H{"code": -1, "error": "登录尝试过于频繁，请1分钟后重试"})
			c.Abort()
			return
		}

		loginLimiter.attempts[ip] = append(valid, now)
		c.Next()
	}
}
