package api

import (
	"os"
	"testing"

	"seo-system/internal/middleware"
)

// TestMain seeds a deterministic JWT secret so the middleware package
// doesn't trip its length check when imported transitively. We use a
// fixed 32-byte string rather than reading the real env var — tests
// should never accidentally rely on the operator's production secret.
func TestMain(m *testing.M) {
	middleware.SetJWTSecretForTesting([]byte("test-secret-for-unit-tests-only!!"))
	os.Exit(m.Run())
}
