package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRF uses the "double-submit token" pattern: on login we set a readable
// cookie `csrf_token`, and the frontend mirrors its value in the
// `X-CSRF-Token` header on every state-changing request. The token is an
// HMAC over a random nonce so it survives stateless restarts.

const (
	CSRFCookieName = "csrf_token"
	CSRFHeaderName = "X-CSRF-Token"
	csrfTTLSeconds = 24 * 60 * 60
)

func csrfKey() []byte {
	sum := sha256.Sum256(append([]byte("csrf:"), jwtSecret...))
	return sum[:]
}

// IssueCSRFToken creates a fresh HMAC token and sets it as a readable cookie
// (NOT httpOnly — that is the whole point of double-submit). The returned
// string is also surfaced in JSON for clients that can't read cross-site
// cookies.
func IssueCSRFToken(c *gin.Context, secure bool) string {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	mac := hmac.New(sha256.New, csrfKey())
	mac.Write(nonce)
	sig := mac.Sum(nil)
	token := base64.RawURLEncoding.EncodeToString(nonce) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(CSRFCookieName, token, csrfTTLSeconds, "/", "", secure, false /* httpOnly=false */)
	return token
}

// ValidateCSRFToken verifies the token's HMAC. Format: base64(nonce).base64(HMAC(nonce)).
func ValidateCSRFToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(nonce) != 16 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, csrfKey())
	mac.Write(nonce)
	return hmac.Equal(sig, mac.Sum(nil))
}

// CSRFProtect enforces the double-submit cookie pattern on state-changing
// requests. Safe methods (GET/HEAD/OPTIONS) pass through. Requests that
// authenticate via Authorization: Bearer (no cookie) are also exempted since
// they can't be CSRF'd — the browser won't attach headers cross-origin
// without CORS approval and will never set `Authorization` on its own.
//
// Any other write request must include both:
//   - a `csrf_token` cookie
//   - a matching `X-CSRF-Token` header
//
// Both values are HMAC-verified to prevent forgery.
func CSRFProtect() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		// Bearer-only clients (no auth_token cookie) aren't CSRF-vulnerable.
		if _, err := c.Cookie(AuthCookieName); err != nil {
			if strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
				c.Next()
				return
			}
		}

		header := c.GetHeader(CSRFHeaderName)
		cookie, _ := c.Cookie(CSRFCookieName)
		if header == "" || cookie == "" || header != cookie {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"code": -1, "error": "CSRF token 缺失或不匹配"})
			return
		}
		if !ValidateCSRFToken(header) {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"code": -1, "error": "CSRF token 无效"})
			return
		}
		c.Next()
	}
}
