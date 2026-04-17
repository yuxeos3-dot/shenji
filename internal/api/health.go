package api

import (
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

var startTime = time.Now()

// Healthz returns 200 if the DB is reachable, 503 otherwise. Use this as the
// upstream health check target (nginx, k8s readinessProbe, uptime monitor).
func Healthz(c *gin.Context) {
	if err := db.DB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unreachable", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"uptime_sec":  int64(time.Since(startTime).Seconds()),
		"go_version":  runtime.Version(),
		"goroutines":  runtime.NumGoroutine(),
	})
}

// ─── Lightweight counters ────────────────────────────────────────────────────

var (
	httpRequestsTotal  atomic.Uint64
	httpErrorsTotal    atomic.Uint64
	loginFailuresTotal atomic.Uint64
	loginLockoutsTotal atomic.Uint64
	auditRunsTotal     atomic.Uint64
	deadlinkRunsTotal  atomic.Uint64
	aiCallsTotal       atomic.Uint64
)

// IncHTTPRequests / IncHTTPErrors / ... are cheap atomic increments called from
// middleware and key handlers. Reading them is done by Metrics below.
func IncHTTPRequests()      { httpRequestsTotal.Add(1) }
func IncHTTPErrors()        { httpErrorsTotal.Add(1) }
func IncLoginFailure()      { loginFailuresTotal.Add(1) }
func IncLoginLockout()      { loginLockoutsTotal.Add(1) }
func IncAuditRun()          { auditRunsTotal.Add(1) }
func IncDeadlinkRun()       { deadlinkRunsTotal.Add(1) }
func IncAICall()            { aiCallsTotal.Add(1) }

// MetricsCounter is global middleware that bumps the request/error counters
// so /metrics has data. Kept in this package to avoid an import cycle with
// middleware (which imports api for CSRF/auth helpers elsewhere).
func MetricsCounter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		IncHTTPRequests()
		if c.Writer.Status() >= 400 {
			IncHTTPErrors()
		}
	}
}

// Metrics exposes a minimal Prometheus-text-format feed. No external deps so we
// don't drag in the full client_golang library.
func Metrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	c.Header("Content-Type", "text/plain; version=0.0.4")
	c.String(http.StatusOK,
		"# HELP seo_http_requests_total Total HTTP requests.\n"+
			"# TYPE seo_http_requests_total counter\n"+
			"seo_http_requests_total %d\n"+
			"# HELP seo_http_errors_total HTTP responses with status >= 400.\n"+
			"# TYPE seo_http_errors_total counter\n"+
			"seo_http_errors_total %d\n"+
			"# HELP seo_login_failures_total Failed login attempts (bad password / unknown email).\n"+
			"# TYPE seo_login_failures_total counter\n"+
			"seo_login_failures_total %d\n"+
			"# HELP seo_login_lockouts_total Accounts temporarily locked after repeated failures.\n"+
			"# TYPE seo_login_lockouts_total counter\n"+
			"seo_login_lockouts_total %d\n"+
			"# HELP seo_audit_runs_total Audit tasks started.\n"+
			"# TYPE seo_audit_runs_total counter\n"+
			"seo_audit_runs_total %d\n"+
			"# HELP seo_deadlink_runs_total Deadlink checks started.\n"+
			"# TYPE seo_deadlink_runs_total counter\n"+
			"seo_deadlink_runs_total %d\n"+
			"# HELP seo_ai_calls_total AI API invocations.\n"+
			"# TYPE seo_ai_calls_total counter\n"+
			"seo_ai_calls_total %d\n"+
			"# HELP seo_goroutines Current number of goroutines.\n"+
			"# TYPE seo_goroutines gauge\n"+
			"seo_goroutines %d\n"+
			"# HELP seo_memstats_alloc_bytes Bytes of allocated heap objects.\n"+
			"# TYPE seo_memstats_alloc_bytes gauge\n"+
			"seo_memstats_alloc_bytes %d\n"+
			"# HELP seo_uptime_seconds Process uptime in seconds.\n"+
			"# TYPE seo_uptime_seconds gauge\n"+
			"seo_uptime_seconds %d\n",
		httpRequestsTotal.Load(),
		httpErrorsTotal.Load(),
		loginFailuresTotal.Load(),
		loginLockoutsTotal.Load(),
		auditRunsTotal.Load(),
		deadlinkRunsTotal.Load(),
		aiCallsTotal.Load(),
		runtime.NumGoroutine(),
		m.Alloc,
		int64(time.Since(startTime).Seconds()),
	)
}
