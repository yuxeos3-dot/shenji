package frw

// JWT decoding helpers used by the FRW single-field onboarding flow.
//
// Background: the "对接 KEY" FRW hands out is a JWT whose payload carries
// a user_id (== sub) field. That token doubles as:
//   - the `apikey` query param + header for the image/video submit API
//   - the `Authorization: Bearer` token for the same call
// So pasting one JWT is enough to drive the whole call. This file turns
// that JWT into (apikey, token, user_id) by reading the payload.
//
// We DO NOT verify the signature — signature verification is the upstream
// FRW service's job. If someone forges a user_id in the payload, the very
// next upstream call will 401 because the Bearer signature won't match
// that fabricated user_id.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// DecodeJWTUserID extracts the user id from a JWT's payload. Tries, in
// order: user_id (string), user_id (number), sub (string). Returns an
// empty string for anything that isn't a well-formed JWT with one of
// those fields — never panics, never errors.
//
// Base64 URL decoding is attempted first (RawURLEncoding — the proper
// JWT encoding). A few implementations drop padding inconsistently, so
// we retry with padded URLEncoding and finally StdEncoding before
// giving up.
func DecodeJWTUserID(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ""
	}
	payload, err := decodeBase64Lenient(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	// user_id (preferred): string form.
	if v, ok := claims["user_id"].(string); ok {
		s := strings.TrimSpace(v)
		if s != "" {
			return s
		}
	}
	// user_id as number — JSON unmarshal gives us float64; stringify
	// without scientific notation and without trailing zeros.
	if v, ok := claims["user_id"].(float64); ok {
		return formatJSONNumber(v)
	}
	if v, ok := claims["user_id"].(json.Number); ok {
		s := strings.TrimSpace(string(v))
		if s != "" {
			return s
		}
	}
	// sub fallback — OAuth / JWT standard subject claim.
	if v, ok := claims["sub"].(string); ok {
		s := strings.TrimSpace(v)
		if s != "" {
			return s
		}
	}
	if v, ok := claims["sub"].(float64); ok {
		return formatJSONNumber(v)
	}
	return ""
}

// LooksLikeJWT is a cheap structural check — 3 non-empty segments
// separated by dots. Doesn't decode, so it's safe to call on every
// request. Callers use this to decide whether to attempt DecodeJWTUserID
// or bail out early when the pasted credential clearly isn't a JWT.
func LooksLikeJWT(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}

// decodeBase64Lenient decodes JWT-style base64 with three fallback
// strategies:
//  1. RawURLEncoding (the proper JWT encoding, no padding)
//  2. URLEncoding with '=' padding added to multiple-of-4 length
//  3. StdEncoding (some sloppy producers emit standard alphabet)
//
// Only the first successful decode is used. Returns error only when
// all three fail.
func decodeBase64Lenient(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	padded := s
	if m := len(padded) % 4; m != 0 {
		padded += strings.Repeat("=", 4-m)
	}
	if b, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("not base64")
}

// formatJSONNumber turns a float64 decoded from JSON (which is how
// encoding/json types all numbers by default) into a decimal string
// suitable for user_id, avoiding scientific notation for large ids
// like "3477210393028333568".
func formatJSONNumber(f float64) string {
	// Integer case — the usual one for FRW user ids.
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
