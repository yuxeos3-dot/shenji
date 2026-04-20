package api

import (
	"seo-system/internal/db"
)

// Server holds shared dependencies for HTTP handlers. New handlers should
// be methods on Server rather than package-level functions. Existing
// handlers can be migrated incrementally.
//
// Usage in main.go:
//
//	srv := api.NewServer(db.DefaultStore())
//	auth.GET("/me", srv.GetMe)
type Server struct {
	Store *db.Store
}

// NewServer creates a Server with the given dependencies.
func NewServer(store *db.Store) *Server {
	return &Server{Store: store}
}

// DefaultServer creates a Server using the package-level db.DB global.
// This is the migration bridge — new code should use explicit NewServer.
func DefaultServer() *Server {
	return NewServer(db.DefaultStore())
}
