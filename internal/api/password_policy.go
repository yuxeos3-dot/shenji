package api

import (
	"errors"
	"unicode"
)

// ValidatePassword enforces a minimum strength policy: at least 12 chars,
// at least one lowercase, one uppercase, and one digit. Symbols are allowed
// but not required so legitimate mnemonic passphrases still work.
// Returns nil when valid, a user-facing error otherwise.
func ValidatePassword(pw string) error {
	if len(pw) < 12 {
		return errors.New("密码至少 12 位")
	}
	var hasLower, hasUpper, hasDigit bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLower || !hasUpper || !hasDigit {
		return errors.New("密码需同时包含大写字母、小写字母和数字")
	}
	return nil
}
