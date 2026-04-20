package frw

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// makeJWT builds a fake but structurally-valid JWT with the given
// payload claims. Signature is a dummy — we never verify it.
func makeJWT(claims map[string]interface{}) string {
	header := `{"alg":"HS256","typ":"JWT"}`
	payload, _ := json.Marshal(claims)
	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(header)),
		base64.RawURLEncoding.EncodeToString(payload),
		base64.RawURLEncoding.EncodeToString([]byte("signature")),
	}, ".")
}

func TestDecodeJWTUserID_FRWOnboardingShape(t *testing.T) {
	// Mirrors the real FRW 对接 KEY payload shape: both user_id and
	// sub carry the same decimal string id. Expected user_id matches
	// the one documented in the spec (3477210393028333568).
	tok := makeJWT(map[string]interface{}{
		"user_id":  "3477210393028333568",
		"sub":      "3477210393028333568",
		"role":     "user",
		"platform": 1,
	})
	if got := DecodeJWTUserID(tok); got != "3477210393028333568" {
		t.Fatalf("FRW shape: want 3477210393028333568, got %q", got)
	}
}

func TestDecodeJWTUserID_NumericUserID(t *testing.T) {
	// Some providers emit user_id as a number; stringify without
	// scientific notation.
	tok := makeJWT(map[string]interface{}{
		"user_id": 123456789,
	})
	if got := DecodeJWTUserID(tok); got != "123456789" {
		t.Fatalf("numeric user_id: want 123456789, got %q", got)
	}
}

func TestDecodeJWTUserID_SubFallback(t *testing.T) {
	tok := makeJWT(map[string]interface{}{
		"sub": "abc-xyz-42",
	})
	if got := DecodeJWTUserID(tok); got != "abc-xyz-42" {
		t.Fatalf("sub fallback: want abc-xyz-42, got %q", got)
	}
}

func TestDecodeJWTUserID_PreferUserIDOverSub(t *testing.T) {
	tok := makeJWT(map[string]interface{}{
		"user_id": "primary",
		"sub":     "fallback",
	})
	if got := DecodeJWTUserID(tok); got != "primary" {
		t.Fatalf("user_id should win over sub, got %q", got)
	}
}

func TestDecodeJWTUserID_EmptyValuesSkipped(t *testing.T) {
	// Empty user_id should fall through to sub.
	tok := makeJWT(map[string]interface{}{
		"user_id": "",
		"sub":     "took-over",
	})
	if got := DecodeJWTUserID(tok); got != "took-over" {
		t.Fatalf("empty user_id: want took-over, got %q", got)
	}
}

func TestDecodeJWTUserID_LeadingTrailingWhitespace(t *testing.T) {
	tok := makeJWT(map[string]interface{}{"user_id": "whitespaced"})
	// Simulate user copy/pasting with surrounding spaces / newlines.
	padded := "  \n\t" + tok + "\n "
	if got := DecodeJWTUserID(padded); got != "whitespaced" {
		t.Fatalf("whitespace trim: want whitespaced, got %q", got)
	}
}

func TestDecodeJWTUserID_BadInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"only whitespace", "   \n\t "},
		{"two segments", "aa.bb"},
		{"four segments", "aa.bb.cc.dd"},
		{"empty middle segment", "aa..cc"},
		{"empty first segment", ".bb.cc"},
		{"empty last segment", "aa.bb."},
		{"middle garbage (invalid base64)", "aa.!!!not-base64!!!.cc"},
		{"middle valid base64 but not JSON", "aa." + base64.RawURLEncoding.EncodeToString([]byte("this is not JSON")) + ".cc"},
		{"middle valid JSON but no id field", "aa." + base64.RawURLEncoding.EncodeToString([]byte(`{"role":"user"}`)) + ".cc"},
	}
	for _, tc := range cases {
		if got := DecodeJWTUserID(tc.input); got != "" {
			t.Errorf("%s: want empty string, got %q", tc.name, got)
		}
	}
}

func TestDecodeJWTUserID_PaddedBase64Tolerance(t *testing.T) {
	// Payload that requires 2 bytes of padding in StdEncoding. The
	// lenient decoder should still recover it.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.StdEncoding.EncodeToString([]byte(`{"user_id":"padded"}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	tok := header + "." + payload + "." + sig
	if got := DecodeJWTUserID(tok); got != "padded" {
		t.Fatalf("padded base64 tolerance: got %q", got)
	}
}

func TestLooksLikeJWT(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"a.b.c", true},
		{"abc.def.ghi", true},
		{"  a.b.c  ", true}, // trimmed
		{"", false},
		{"   ", false},
		{"a.b", false},
		{"a.b.c.d", false},
		{"a..c", false},
		{".b.c", false},
		{"a.b.", false},
		{"no dots at all", false},
	}
	for _, tc := range cases {
		if got := LooksLikeJWT(tc.in); got != tc.want {
			t.Errorf("LooksLikeJWT(%q): want %v, got %v", tc.in, tc.want, got)
		}
	}
}
