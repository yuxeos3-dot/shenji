package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

// MustInitJWT loads JWT_SECRET from the environment and validates it. Single-
// user mode still uses a JWT in an httpOnly cookie as the session — the secret
// signs that cookie so a forged value can't bypass auth.
func MustInitJWT() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" || len(secret) < 16 {
		log.Fatal("JWT_SECRET environment variable must be set and at least 16 characters long. " +
			"Generate a strong random string, e.g. `openssl rand -hex 32`")
	}
	jwtSecret = []byte(secret)
}

func SetJWTSecretForTesting(secret []byte) { jwtSecret = secret }
func GetJWTSecret() []byte                 { return jwtSecret }

// Claims is intentionally minimal in single-user mode — there is exactly one
// principal (user_id=1, role=admin) and the token just proves "this browser
// completed the password challenge".
type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

const (
	SingleUserID   int64  = 1
	SingleUserRole string = "admin"
)

// GenerateToken issues a 24h cookie for the singleton user. Kept named
// GenerateToken so the api.Login handler doesn't need a special signature.
func GenerateToken() (string, error) {
	jtiBytes := make([]byte, 16)
	rand.Read(jtiBytes)
	jti := base64.RawURLEncoding.EncodeToString(jtiBytes)

	claims := Claims{
		UserID: SingleUserID,
		Role:   SingleUserRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

const AuthCookieName = "auth_token"

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

		c.Set("user_id", SingleUserID)
		c.Set("role", SingleUserRole)
		c.Next()
	}
}

// ─── Per-user (singleton) rate limiter for heavy endpoints ─────────────────

type userRateLimiter struct {
	mu       sync.Mutex
	requests map[int64][]time.Time
}

var heavyLimiter = &userRateLimiter{requests: make(map[int64][]time.Time)}

func StartBackgroundCleanups(ctx context.Context) {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				heavyLimiter.cleanup()
			}
		}
	}()
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				loginLimiter.cleanup()
			}
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

func UserRateLimit(maxPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		if userID <= 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
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

// ─── Per-IP login rate limiter ─────────────────────────────────────────────

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

var loginLimiter = &rateLimiter{attempts: make(map[string][]time.Time)}

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
