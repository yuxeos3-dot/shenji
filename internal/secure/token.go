// Package secure holds crypto helpers shared across api, telegram, and
// scheduler. Originally lived in internal/api/secure_token.go but grew
// out of that package once the scheduler needed to decrypt per-user
// Telegram bot tokens without importing api (which would cycle back
// through telegram).
package secure

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

// OAuth refresh_tokens, Telegram bot tokens, Bing API keys, and FRW
// apikeys are all highly sensitive — anyone with the DB file can
// resume API access to the provider. We encrypt them at rest using
// AES-256-GCM with a key derived from JWT_SECRET. Legacy plaintext
// values are transparently accepted on read and re-encrypted on next
// write.

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
		// AES-256 wants 32 bytes; SHA-256 fits perfectly. Prefix so
		// rotating JWT_SECRET invalidates old ciphertext predictably.
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

// Encrypt turns a plaintext token into `enc:v1:<base64(nonce||ct)>`.
// Empty input yields empty output.
func Encrypt(plain string) (string, error) {
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

// Decrypt reverses Encrypt. Legacy plaintext (no prefix) is returned
// unchanged so gradual migration works; callers should re-write encrypted.
func Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, encryptedPrefix) {
		return stored, nil
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

// MustEncrypt logs and returns empty string on failure — for write paths
// where a cipher error shouldn't crash the whole request.
func MustEncrypt(plain string) string {
	enc, err := Encrypt(plain)
	if err != nil {
		log.Printf("[secure] encrypt failed: %v", err)
		return ""
	}
	return enc
}
