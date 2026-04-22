package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"seo-system/internal/api"
	"seo-system/internal/db"
	"seo-system/internal/middleware"
	"seo-system/internal/scheduler"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	godotenv.Load()

	// Validate JWT_SECRET up front. Previously this happened in
	// middleware/auth.go init(), which tripped `go test` in any package
	// that imported the middleware. Calling it here keeps the fail-fast
	// semantic in production while leaving test binaries free to seed a
	// synthetic secret via SetJWTSecretForTesting.
	middleware.MustInitJWT()

	// Initialize database
	db.Init()

	// Create the DI-aware server. New handlers should be methods on srv
	// (e.g. auth.GET("/me", srv.GetMe)). Existing package-level handlers
	// continue to work unchanged via the db.DB global.
	_ = api.DefaultServer()

	// One-time migration of permission codes from flat v1 codes
	// (reports.view/write/polish/export) to granular v2 codes
	// (reports.daily.*, reports.weekly.*, reports.monthly.*, plus
	// new geo.workflow, coord.pre_audit, coord.reject, coord.test_pages).
	// Idempotent — writes a flag into settings when done and no-ops
	// on every subsequent boot.
	api.MigrateLegacyPermissions()

	// One-shot backfill: every existing coordination_tasks row gets its
	// issue_scope column populated by parsing the stored issue_snapshot
	// JSON. New rows are filled inline by CreateCoordTask. Idempotent
	// via settings flag coord_scope_backfilled_v1.
	api.BackfillCoordIssueScope()

	// One-time migration of ALL global platform credentials (Telegram
	// bot, Bing key, FRW apikey, OAuth client + access tokens) onto a
	// designated per-user record (沈万三, fallback first-admin).
	// Idempotent via `perm_migrated_personal_v1` flag.
	//
	// CRITICAL: run in a background goroutine, not synchronously in
	// main(). SQLite with MaxOpenConns=1 plus long-running migrations
	// is a recipe for boot-time deadlock — if the migration blocks,
	// main() never reaches ListenAndServe and login breaks for
	// everybody. Kicking it off async means the HTTP server comes up
	// within milliseconds and the migration finishes whenever it
	// finishes; the user-visible effect is just that per-user credentials
	// might take a few more seconds to appear in the admin monitoring
	// view on first boot after deploy.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[boot] MigrateGlobalToPersonalConfig panic: %v", r)
			}
		}()
		api.MigrateGlobalToPersonalConfig()
	}()

	// Warm the 4-platform data cache right after the DB is ready so first
	// request hits a primed cache. Done in a goroutine — the HTTP server
	// comes up immediately; if an early read loses the race it'll lazy-load
	// the single site on demand.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[boot] WarmDataCache panic: %v", r)
			}
		}()
		api.WarmDataCache()
	}()

	// One-time backfill: flatten all historical audit_results into
	// the new audit_findings table for analysis queries.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[boot] BackfillAuditFindings panic: %v", r)
			}
		}()
		api.BackfillAuditFindings()
	}()

	// Seed initial analysis rules if not present
	api.SeedInitialRules()

	// Start scheduler
	scheduler.Start()
	defer scheduler.Stop()

	// Setup Gin
	// Use gin.New() instead of gin.Default() so we can attach a custom
	// logger that redacts query parameters on OAuth callback paths
	// (security audit #65). Recovery is handled by our RecoverLogger.
	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		path := param.Path
		// Redact query string on OAuth callback paths to avoid logging
		// tokens/codes/state values.
		if strings.Contains(path, "/oauth/") {
			if idx := strings.Index(path, "?"); idx >= 0 {
				path = path[:idx] + "?[REDACTED]"
			}
		}
		return fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %-7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	}))
	r.Use(gin.Recovery())

	// Tell gin which proxy addresses can set X-Forwarded-For / X-Real-IP,
	// otherwise c.ClientIP() is spoofable and IP whitelist + login rate limit
	// can be bypassed. Defaults to loopback only; override with TRUSTED_PROXIES.
	trusted := []string{"127.0.0.1", "::1"}
	if raw := os.Getenv("TRUSTED_PROXIES"); raw != "" {
		trusted = trusted[:0]
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				trusted = append(trusted, p)
			}
		}
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		log.Fatalf("SetTrustedProxies 失败: %v", err)
	}

	// Global middleware
	r.Use(middleware.CORS())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.IPWhitelist())
	r.Use(middleware.RecoverLogger())
	// Route registration — organized by domain in internal/api/routes_*.go
	api.RegisterRoutes(r, middleware.AuthRequired(), middleware.CSRFProtect())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: on SIGINT/SIGTERM, stop accepting new requests and
	// let in-flight ones finish. Running audits/deadlink checks detach into
	// their own goroutines and may outlive the HTTP server.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Start background sweepers with shutdown context so they exit cleanly.
	api.StartLockoutSweeper(shutdownCtx)
	api.StartOAuthStateSweeper(shutdownCtx)
	api.StartResolverCacheSweeper(shutdownCtx)
	middleware.StartBackgroundCleanups(shutdownCtx)
	defer stopSignals()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("SEO巡站系统启动，端口: %s (trusted_proxies=%v)", port, trusted)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("启动失败: %v", err)
		}
	case <-shutdownCtx.Done():
		log.Println("收到关停信号，等待在途请求完成 (最多 30s)...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("HTTP shutdown 失败: %v", err)
		}
		log.Println("已优雅关停")
	}
}
