package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
)

// OAuth refresh_tokens are highly sensitive — anyone with the DB file can
// resume API access to GSC / GA / Yandex. We encrypt them at rest using
// AES-256-GCM with a key derived from JWT_SECRET. Legacy plaintext values
// are transparently accepted on read and re-encrypted on next write.

const encryptedPrefix = "enc:v1:"

var (
	gcmOnce sync.Once
	gcmAEAD cipher.AEAD
	gcmErr  error
)

func gcmCipher() (cipher.AEAD, error) {
	gcmOnce.Do(func() {
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			gcmErr = errors.New("JWT_SECRET not set — cannot derive token encryption key")
			return
		}
		// AES-256 wants 32 bytes; SHA-256 fits perfectly. Prefix so rotating
		// JWT_SECRET invalidates old ciphertext predictably.
		sum := sha256.Sum256([]byte("oauth-token-v1:" + secret))
		block, err := aes.NewCipher(sum[:])
		if err != nil {
			gcmErr = err
			return
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			gcmErr = err
			return
		}
		gcmAEAD = aead
	})
	return gcmAEAD, gcmErr
}

// EncryptToken turns a plaintext token into `enc:v1:<base64(nonce||ct)>`.
// Empty input yields empty output so DB rows without a token stay represented
// as "" rather than picking up an encrypted empty-string blob.
func EncryptToken(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	aead, err := gcmCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(plain), nil)
	blob := append(nonce, sealed...)
	return encryptedPrefix + base64.RawURLEncoding.EncodeToString(blob), nil
}

// DecryptToken reverses EncryptToken. If the input lacks the encrypted
// prefix it's assumed to be legacy plaintext (pre-migration rows) and
// returned unchanged; callers should re-write encrypted.
func DecryptToken(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, encryptedPrefix) {
		return stored, nil // legacy plaintext
	}
	aead, err := gcmCipher()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, encryptedPrefix))
	if err != nil {
		return "", err
	}
	ns := aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// MustEncryptToken is a convenience wrapper that logs and returns empty
// string on failure. Useful where a bad cipher setup shouldn't crash a
// single DB write but should still be surfaced.
func MustEncryptToken(plain string) string {
	enc, err := EncryptToken(plain)
	if err != nil {
		log.Printf("[secure_token] encrypt failed: %v", err)
		return ""
	}
	return enc
}
