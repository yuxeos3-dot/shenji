package api

import (
	"strings"
	"testing"
)

// Password policy: ≥12 chars, with lower + upper + digit. These tests lock
// in the policy so a future refactor can't silently relax it — weakening
// the policy would shrink the bcrypt input space and undo a large chunk
// of the login brute-force protection.
func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr string // "" means nil error
	}{
		{"too_short", "Aa1bcdef", "密码至少 12 位"},
		{"exact_11_short", "Aa1bcdefghi", "密码至少 12 位"},
		{"missing_upper", "aaaa1bcdefgh", "需同时包含"},
		{"missing_lower", "AAAA1BCDEFGH", "需同时包含"},
		{"missing_digit", "AaBbCcDdEeFf", "需同时包含"},
		{"ok_minimum", "Abcdefghijk1", ""},
		{"ok_with_symbols", "Abcdefghij1!", ""},
		{"ok_long_passphrase", "Correcthorsebattery1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.pw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}
