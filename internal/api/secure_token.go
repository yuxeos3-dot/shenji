package api

// Compatibility shim. Real implementation now lives in
// internal/secure/token.go so other subpackages (telegram, scheduler)
// can encrypt / decrypt without importing the api package.

import (
	"seo-system/internal/secure"
)

func EncryptToken(plain string) (string, error) { return secure.Encrypt(plain) }
func DecryptToken(stored string) (string, error) { return secure.Decrypt(stored) }
func MustEncryptToken(plain string) string       { return secure.MustEncrypt(plain) }
