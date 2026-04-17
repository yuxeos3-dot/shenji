package main

import (
	"context"
	"errors"
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

	// Initialize database
	db.Init()

	// One-time migration of permission codes from flat v1 codes
	// (reports.view/write/polish/export) to granular v2 codes
	// (reports.daily.*, reports.weekly.*, reports.monthly.*, plus
	// new geo.workflow, coord.pre_audit, coord.reject, coord.test_pages).
	// Idempotent — writes a flag into settings when done and no-ops
	// on every subsequent boot.
	api.MigrateLegacyPermissions()

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
	go api.MigrateGlobalToPersonalConfig()

	// Warm the 4-platform data cache right after the DB is ready so first
	// request hits a primed cache. Done in a goroutine — the HTTP server
	// comes up immediately; if an early read loses the race it'll lazy-load
	// the single site on demand.
	go api.WarmDataCache()

	// Start scheduler
	scheduler.Start()
	defer scheduler.Stop()

	// Setup Gin
	r := gin.Default()

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
	r.Use(middleware.IPWhitelist())
	r.Use(middleware.RecoverLogger())
	r.Use(api.MetricsCounter())

	// Public health endpoint — used by load balancers / monitoring. Reports
	// DB reachability. Deliberately outside the auth group. Lives under
	// /api/ so reverse proxies that only forward /api/* still hit it.
	r.GET("/api/healthz", api.Healthz)

	// Prometheus-style metrics endpoint (no-auth by default; wrap with reverse
	// proxy ACL if the host is public).
	r.GET("/api/metrics", api.Metrics)

	// Auth routes (no auth required)
	r.POST("/api/auth/login", middleware.LoginRateLimit(), api.Login)

	// Public branding endpoint — exposes a tiny allow-list of settings
	// (site_title) so the login page can render the configured system name
	// without a session. See publicSettingsKeys in internal/api/settings.go.
	r.GET("/api/v1/settings/public", api.GetPublicSettings)

	// Spider hit ingest — authenticated by X-API-Key (set in system settings).
	// Public so external log shippers (nginx parser, CF worker) can POST to it
	// without holding a JWT cookie.
	r.POST("/api/spider/ingest", api.IngestSpiderLog)

	// OAuth callbacks. MUST be outside the authenticated v1 group: after the
	// user consents on Google/Yandex, the provider redirects the browser back
	// here via a plain 302. Third-party-initiated navigation does not always
	// forward our session cookie (especially in a new tab or cross-site
	// context), so requiring auth here would break every OAuth flow.
	//
	// CSRF-safe because each callback self-validates a server-generated
	// single-use state token (see internal/api/oauth_state.go): the token
	// was issued when the authenticated admin clicked "Authorize", is bound
	// to their user id, has a 10-minute TTL, and is deleted on first use.
	// An attacker who doesn't know the state cannot trigger the handler.
	r.GET("/api/v1/oauth/gsc/callback", api.GSCCallback)
	r.GET("/api/v1/oauth/ga/callback", api.GACallback)
	r.GET("/api/v1/oauth/yandex/callback", api.YandexCallback)

	// FRW published assets (downloaded images/videos) — served from local
	// disk. Paths stored in ai_generations.local_path as "/data/frw/...",
	// so the URL directly maps to the file. No auth: if an attacker can
	// guess /data/frw/u42_gen17.jpg they're welcome to it.
	r.Static("/data/frw", "./data/frw")

	// Authenticated routes
	// CSRF runs AFTER auth: it's cheap to skip for non-cookie clients, and
	// needs to know whether the request is cookie-based.
	v1 := r.Group("/api/v1")
	v1.Use(middleware.AuthRequired(), middleware.CSRFProtect())
	{
		// Auth
		v1.POST("/auth/logout", api.Logout)
		v1.POST("/auth/change-password", api.ChangePassword)
		v1.GET("/auth/me", api.GetMe)
		v1.GET("/auth/csrf", api.GetCSRFToken)
		// Online presence shows every user's email + IP — route it through
		// the ACL so the frontend menu "在线用户" can be gated per-user.
		v1.GET("/auth/online", api.RequirePermission("dashboard.online"), api.GetOnlineUsersAPI)
		v1.GET("/auth/my-permissions", api.GetMyPermissions)
		v1.GET("/auth/my-ai-config", api.GetMyAIConfig)
		v1.PUT("/auth/my-ai-config", api.SaveMyAIConfig)
		v1.POST("/auth/test-my-ai", api.TestMyAI)

		// Personal platform credentials (every logged-in user can
		// manage their own). No permission gate — "my own keys" is
		// the point of the per-user model.
		v1.GET("/auth/my-oauth-config", api.GetMyOAuthConfigs)
		v1.PUT("/auth/my-oauth-config", api.PutMyOAuthConfig)
		v1.DELETE("/auth/my-oauth-token", api.UnbindMyOAuthToken)
		v1.GET("/auth/my-telegram", api.GetMyTelegram)
		v1.PUT("/auth/my-telegram", api.PutMyTelegram)
		v1.POST("/auth/my-telegram/test", api.TestTelegram)
		v1.GET("/auth/my-bing", api.GetMyBing)
		v1.PUT("/auth/my-bing", api.PutMyBing)
		v1.GET("/auth/my-frw-apikey", api.GetMyFrw)
		v1.PUT("/auth/my-frw-apikey", api.PutMyFrw)

		// Users (admin only)
		users := v1.Group("/users")
		users.Use(middleware.AdminRequired())
		{
			users.GET("", api.ListUsers)
			users.POST("", api.CreateUser)
			users.PUT("/:id", api.UpdateUser)
			users.DELETE("/:id", api.DeleteUser)
			users.GET("/:id/permissions", api.GetUserPermissions)
			users.PUT("/:id/permissions", api.SaveUserPermissions)
			users.GET("/permission-tree", api.GetPermissionTree)
			// Admin monitoring: see which platforms a user has configured
			// without exposing the plaintext secrets themselves.
			users.GET("/:id/platform-status", api.GetUserPlatformStatus)
		}

		// Organization structure — admin only. Matches the /users pattern
		// so the settings page can list/create/update/delete departments
		// and groups in the same admin-only section.
		orgs := v1.Group("/org")
		orgs.Use(middleware.AdminRequired())
		{
			orgs.GET("/departments", api.ListDepartments)
			orgs.POST("/departments", api.CreateDepartment)
			orgs.PUT("/departments/:id", api.UpdateDepartment)
			orgs.DELETE("/departments/:id", api.DeleteDepartment)
			orgs.GET("/groups", api.ListGroups)
			orgs.POST("/groups", api.CreateGroup)
			orgs.PUT("/groups/:id", api.UpdateGroup)
			orgs.DELETE("/groups/:id", api.DeleteGroup)
		}

		// Sites
		v1.GET("/sites", api.RequirePermission("sites.view"), api.ListSites)
		v1.POST("/sites", api.RequirePermission("sites.create"), api.CreateSite)
		v1.GET("/sites/:id", api.RequirePermission("sites.view"), api.GetSite)
		v1.PUT("/sites/:id", api.RequirePermission("sites.edit"), api.UpdateSite)
		v1.DELETE("/sites/:id", api.RequirePermission("sites.delete"), api.DeleteSite)

		// Audit (rate-limited: 5/min per user)
		v1.POST("/sites/:id/audit", api.RequirePermission("sites.audit"), middleware.UserRateLimit(5), api.TriggerAudit)
		v1.GET("/sites/:id/audits", api.RequirePermission("sites.audit_view"), api.ListAudits)
		v1.GET("/audits/:task_id", api.RequirePermission("sites.audit_view"), api.GetAudit)
		v1.DELETE("/audits/:task_id", api.RequirePermission("sites.audit_delete"), api.DeleteAudit)
		v1.GET("/audits/:task_id/pages", api.RequirePermission("sites.audit_view"), api.GetAuditPages)
		v1.GET("/audits/:task_id/pages/:type", api.RequirePermission("sites.audit_view"), api.GetAuditPageByType)
		v1.GET("/audits/:task_id/report/download", api.RequirePermission("sites.audit_download"), api.DownloadAuditReport)

		// Site Pages (custom pages management)
		v1.GET("/sites/:id/pages", api.RequirePermission("sites.pages"), api.ListSitePages)
		v1.POST("/sites/:id/pages", api.RequirePermission("sites.pages"), api.CreateSitePage)
		v1.DELETE("/sites/:id/pages/:page_id", api.RequirePermission("sites.pages"), api.DeleteSitePage)
		v1.POST("/sites/:id/pages/:page_id/fetch", api.RequirePermission("sites.pages"), api.FetchSitePage)
		v1.GET("/sites/:id/pages/:page_id/source", api.RequirePermission("sites.pages"), api.GetSitePageSource)

		// Dead Link Check (rate-limited: 5/min per user)
		v1.GET("/sites/:id/deadlinks", api.RequirePermission("deadlinks.view"), api.GetDeadLinks)
		v1.GET("/sites/:id/deadlinks/progress", api.RequirePermission("deadlinks.view"), api.GetDeadLinkProgress)
		v1.POST("/sites/:id/deadlinks/check", api.RequirePermission("deadlinks.check"), middleware.UserRateLimit(5), api.StartDeadLinkCheck)
		v1.POST("/sites/:id/deadlinks/retry", api.RequirePermission("deadlinks.retry"), api.RetryDeadLinks)

		// Content Rewrite (AI, rate-limited: 10/min per user)
		v1.POST("/rewrite", api.RequirePermission("rewrite.use"), middleware.UserRateLimit(10), api.RewriteContent)

		// Shared Backlink Pool
		v1.GET("/extlink-pool", api.RequirePermission("extlinks.view"), api.ListExtlinkPool)
		v1.POST("/extlink-pool/import", api.RequirePermission("extlinks.create"), api.ImportExtlinkPool)
		v1.DELETE("/extlink-pool/:id", api.RequirePermission("extlinks.delete"), api.DeleteExtlinkPoolItem)

		// External Links
		v1.GET("/sites/:id/extlinks", api.RequirePermission("extlinks.view"), api.GetExternalLinks)
		v1.POST("/extlinks", api.RequirePermission("extlinks.create"), api.CreateExternalLink)
		v1.PUT("/extlinks/:id", api.RequirePermission("extlinks.edit"), api.UpdateExternalLink)
		v1.DELETE("/extlinks/:id", api.RequirePermission("extlinks.delete"), api.DeleteExternalLink)
		v1.POST("/extlinks/:id/check", api.RequirePermission("extlinks.check"), api.CheckExternalLink)

		// GSC
		v1.GET("/oauth/gsc", api.GetGSCAuthURL)
		v1.POST("/oauth/gsc/token", api.SaveGSCToken)
		v1.POST("/gsc/store-keywords", api.StoreSiteKeywords) // push keywords from browser
		v1.POST("/sites/:id/gsc/sync", api.RequirePermission("data.sync_gsc"), api.SyncGSCData)
		v1.GET("/sites/:id/gsc", api.RequirePermission("data.view"), api.GetGSCData)

		// GSC Dashboard (account-level overview + detail, date-range aware).
		// Doesn't write to gsc_data — these endpoints are a live view of
		// what the Google account can see, keyed by GSC property URL
		// rather than our local site id.
		v1.GET("/gsc/sites", api.RequirePermission("data.view"), api.GSCListSites)
		v1.GET("/gsc/overview", api.RequirePermission("data.view"), api.GSCOverview)
		v1.GET("/gsc/site-detail", api.RequirePermission("data.view"), api.GSCSiteDetail)

		// GA
		v1.GET("/oauth/ga", api.GetGAAuthURL)
		v1.POST("/sites/:id/ga/sync", api.RequirePermission("data.sync_ga"), api.SyncGAData)
		v1.GET("/sites/:id/ga", api.RequirePermission("data.view"), api.GetGAData)

		// GA4 Dashboard (account-level, date-range). Parallel to GSC dashboard.
		v1.GET("/ga4/properties", api.RequirePermission("data.view"), api.GAListProperties)
		v1.GET("/ga4/overview", api.RequirePermission("data.view"), api.GAOverview)
		v1.GET("/ga4/property-detail", api.RequirePermission("data.view"), api.GAPropertyDetail)

		// Yandex
		v1.GET("/oauth/yandex", api.GetYandexAuthURL)
		v1.POST("/sites/:id/yandex/sync", api.RequirePermission("data.sync_yandex"), api.SyncYandexData)
		v1.GET("/sites/:id/yandex", api.RequirePermission("data.view"), api.GetYandexData)

		// Yandex Dashboard
		v1.GET("/yandex/hosts", api.RequirePermission("data.view"), api.YandexListHosts)
		v1.GET("/yandex/overview", api.RequirePermission("data.view"), api.YandexOverview)
		v1.GET("/yandex/host-detail", api.RequirePermission("data.view"), api.YandexHostDetail)

		// Reports (daily — 日报)
		v1.GET("/reports", api.RequirePermission("reports.daily.view"), api.ListReports)
		v1.POST("/reports", api.RequirePermission("reports.daily.write"), api.SubmitReport)
		v1.POST("/reports/polish", api.RequirePermission("reports.daily.polish"), api.PolishReport)
		v1.GET("/reports/export", api.RequirePermission("reports.daily.export"), api.ExportReports) // must be before /:date

		// Weekly reports (周汇报). MUST be registered before /:date or
		// else gin routes /reports/weekly to the daily GetReportByDate
		// handler with :date="weekly" — the weekly sub-paths like
		// /reports/weekly/123 would also misroute.
		weekly := v1.Group("/reports/weekly")
		{
			weekly.GET("", api.RequirePermission("reports.weekly.view"), api.ListWeeklyReports)
			weekly.GET("/all", api.RequirePermission("reports.weekly.view"), api.AdminListAllWeeklyReports)
			weekly.POST("/generate", api.RequirePermission("reports.weekly.write"), api.GenerateWeeklyReport)
			weekly.GET("/:id", api.RequirePermission("reports.weekly.view"), api.GetWeeklyReport)
			weekly.PUT("/:id", api.RequirePermission("reports.weekly.write"), api.UpdateWeeklyReport)
			weekly.POST("/:id/submit", api.RequirePermission("reports.weekly.write"), api.SubmitWeeklyReport)
			weekly.POST("/:id/polish", api.RequirePermission("reports.weekly.polish"), api.PolishWeeklyReport)
			weekly.DELETE("/:id", api.RequirePermission("reports.weekly.write"), api.DeleteWeeklyReport)
		}

		// Monthly reports (月汇报). Same caveat as weekly: register
		// before /:date. Sources its content from submitted weekly
		// reports whose week_start falls in the given (year, month).
		monthly := v1.Group("/reports/monthly")
		{
			monthly.GET("", api.RequirePermission("reports.monthly.view"), api.ListMonthlyReports)
			monthly.GET("/all", api.RequirePermission("reports.monthly.view"), api.AdminListAllMonthlyReports)
			monthly.POST("/generate", api.RequirePermission("reports.monthly.write"), api.GenerateMonthlyReport)
			monthly.GET("/:id", api.RequirePermission("reports.monthly.view"), api.GetMonthlyReport)
			monthly.PUT("/:id", api.RequirePermission("reports.monthly.write"), api.UpdateMonthlyReport)
			monthly.POST("/:id/submit", api.RequirePermission("reports.monthly.write"), api.SubmitMonthlyReport)
			monthly.POST("/:id/polish", api.RequirePermission("reports.monthly.polish"), api.PolishMonthlyReport)
			monthly.DELETE("/:id", api.RequirePermission("reports.monthly.write"), api.DeleteMonthlyReport)
		}

		v1.GET("/reports/:date", api.RequirePermission("reports.daily.view"), api.GetReportByDate)

		// Report evaluations. GET is visible to anyone who can read
		// at least daily reports (the handler itself further gates by
		// owner vs admin). POST/DELETE stay admin-only via middleware;
		// reports.evaluate in the permission tree is informational —
		// a non-admin cannot currently evaluate no matter the flag.
		v1.GET("/evaluations", api.RequirePermission("reports.daily.view"), api.ListEvaluations)
		v1.POST("/evaluations", middleware.AdminRequired(), api.CreateEvaluation)
		v1.POST("/evaluations/polish", middleware.AdminRequired(), api.PolishEvaluation)
		v1.DELETE("/evaluations/:id", middleware.AdminRequired(), api.DeleteEvaluation)

		// Bing Webmaster
		v1.POST("/sites/:id/bing/sync", api.RequirePermission("data.sync_gsc"), api.SyncBingData)
		v1.GET("/sites/:id/bing", api.RequirePermission("data.view"), api.GetBingData)

		// Bing Dashboard
		v1.GET("/bing/sites", api.RequirePermission("data.view"), api.BingListSites)
		v1.GET("/bing/overview", api.RequirePermission("data.view"), api.BingOverview)
		v1.GET("/bing/site-detail", api.RequirePermission("data.view"), api.BingSiteDetail)

		// Settings (admin only)
		settings := v1.Group("/settings")
		settings.Use(middleware.AdminRequired())
		{
			settings.GET("", api.GetSettings)
			settings.PUT("", api.SaveSettings)
			settings.POST("/test-ai", api.TestAI)
			settings.POST("/test-telegram", api.TestTelegram)
			settings.GET("/my-ip", api.GetClientIP)
		}

		// Audit Logs (admin only)
		v1.GET("/audit-logs", middleware.AdminRequired(), api.ListAuditLogs)

		// Spider analytics (read-only for now; ingest is a public route above).
		// All endpoints are scoped by site; /sites returns a lightweight summary
		// of every registered site so the UI can render the selector.
		v1.GET("/spider/sites", api.RequirePermission("spider.view"), api.SpiderSiteSummary)
		v1.GET("/spider/overview", api.RequirePermission("spider.view"), api.SpiderOverview)
		v1.GET("/spider/trend", api.RequirePermission("spider.view"), api.SpiderDailyTrend)
		v1.GET("/spider/logs", api.RequirePermission("spider.view"), api.SpiderLogs)

		// Coordination workflow (协同管理): turn audit findings into tracked
		// fix tickets that flow through pending → in_progress → under_review
		// → done. Per-permission gating so devs can only move items they're
		// assigned, while admins can do everything.
		coord := v1.Group("/coord")
		{
			coord.GET("/stats", api.RequirePermission("coord.view"), api.CoordStats)
			coord.GET("/tasks", api.RequirePermission("coord.view"), api.ListCoordTasks)
			coord.POST("/tasks", api.RequirePermission("coord.create"), api.CreateCoordTask)
			coord.GET("/tasks/:id", api.RequirePermission("coord.view"), api.GetCoordTask)
			coord.PUT("/tasks/:id", api.RequirePermission("coord.edit"), api.UpdateCoordTask)
			coord.DELETE("/tasks/:id", api.RequirePermission("coord.edit"), api.DeleteCoordTask)
			coord.POST("/tasks/:id/assign", api.RequirePermission("coord.assign"), api.AssignCoordTask)
			coord.POST("/tasks/:id/submit", api.RequirePermission("coord.submit"), api.SubmitCoordTask)
			coord.POST("/tasks/:id/verify", api.RequirePermission("coord.verify"), api.VerifyCoordTask)
			coord.POST("/tasks/:id/comments", api.RequirePermission("coord.comment"), api.AddCoordComment)
			// Developer raises a question → creator answers → back to dev.
			// Both require coord.comment since they're conversational actions;
			// dev doesn't need elevated privileges to ask, admin doesn't
			// need elevated privileges to answer.
			coord.POST("/tasks/:id/ask", api.RequirePermission("coord.comment"), api.AskCoordTask)
			coord.POST("/tasks/:id/answer", api.RequirePermission("coord.comment"), api.AnswerCoordTask)
			// Attachment upload is tied to the task id (so we know what
			// it belongs to). Viewing a single file is addressed by
			// attachment id — same coord.view gate.
			coord.POST("/tasks/:id/attachments", api.RequirePermission("coord.comment"), api.UploadCoordAttachment)
			coord.GET("/attachments/:id/file", api.RequirePermission("coord.view"), api.ServeCoordAttachment)
			coord.DELETE("/attachments/:id", api.RequirePermission("coord.comment"), api.DeleteCoordAttachment)

			// Pre-audit (待验收). Separate permission codes for each
			// distinct admin action: managing the test-page list,
			// running AI audit, accepting (verify), and rejecting.
			// A reviewer can be limited to "only read test pages" by
			// granting coord.view alone.
			coord.GET("/tasks/:id/test-pages", api.RequirePermission("coord.view"), api.ListCoordTestPages)
			coord.POST("/tasks/:id/test-pages", api.RequirePermission("coord.test_pages"), api.CreateCoordTestPage)
			coord.DELETE("/tasks/:id/test-pages/:page_id", api.RequirePermission("coord.test_pages"), api.DeleteCoordTestPage)
			coord.POST("/tasks/:id/test-pages/:page_id/fetch", api.RequirePermission("coord.test_pages"), api.FetchCoordTestPage)
			coord.GET("/tasks/:id/test-pages/:page_id/source", api.RequirePermission("coord.view"), api.GetCoordTestPageSource)
			coord.POST("/tasks/:id/pre-audit", api.RequirePermission("coord.pre_audit"), api.PreAuditCoordTask)
			coord.POST("/tasks/:id/reject", api.RequirePermission("coord.reject"), api.RejectCoordTask)
		}

		// Cache management (admin only)
		cache := v1.Group("/cache")
		cache.Use(middleware.AdminRequired())
		{
			cache.GET("", api.ListCaches)
			cache.DELETE("/:key", api.ClearCache)
		}

		// GEO Content Production
		geo := v1.Group("/geo")
		{
			geo.GET("/stats", api.RequirePermission("geo.view"), api.GeoStats)

			geo.GET("/title-libs", api.RequirePermission("geo.view"), api.GeoListTitleLibs)
			geo.POST("/title-libs", api.RequirePermission("geo.keywords"), api.GeoCreateTitleLib)
			geo.DELETE("/title-libs/:id", api.RequirePermission("geo.keywords"), api.GeoDeleteTitleLib)
			geo.GET("/title-libs/:id/titles", api.RequirePermission("geo.view"), api.GeoListTitles)
			geo.POST("/title-libs/:id/titles", api.RequirePermission("geo.keywords"), api.GeoAddTitles)
			geo.POST("/title-libs/:id/import", api.RequirePermission("geo.keywords"), api.GeoImportTitles)
			geo.POST("/title-libs/:id/import-gsc", api.RequirePermission("geo.keywords"), api.GeoImportFromGSC)
			geo.DELETE("/titles/:id", api.RequirePermission("geo.keywords"), api.GeoDeleteTitle)

			// NOTE: /prompts /knowledge /tasks endpoints were removed in
			// the wizard refactor. Prompts are now per-generation
			// (custom_prompt on geo_workflow_jobs). Knowledge bases and
			// batch tasks are gone — users manage keywords and click AI
			// generate row-by-row via the 4-step wizard.

			geo.GET("/articles", api.RequirePermission("geo.articles"), api.GeoListArticles)
			geo.GET("/articles/:id", api.RequirePermission("geo.articles"), api.GeoGetArticle)
			geo.PUT("/articles/:id", api.RequirePermission("geo.articles"), api.GeoUpdateArticle)
			geo.DELETE("/articles/:id", api.RequirePermission("geo.articles_delete"), api.GeoDeleteArticle)
			geo.POST("/articles/:id/review", api.RequirePermission("geo.articles_review"), api.GeoReviewArticle)
			geo.POST("/articles/:id/publish", api.RequirePermission("geo.articles_publish"), api.GeoPublishArticle)

			// Per-keyword async AI writing workflow (both GEO + ExtLink
			// share this backend; differentiate via content_type in the
			// request body). Returns job id immediately, frontend polls.
			geo.POST("/keyword/workflow/start", api.RequirePermission("geo.workflow"), api.GeoStartKeywordWorkflow)
			geo.GET("/keyword/workflow/jobs", api.RequirePermission("geo.view"), api.GeoListWorkflowJobs)
			geo.GET("/keyword/workflow/:id", api.RequirePermission("geo.view"), api.GeoGetWorkflowJob)
			geo.POST("/keyword/workflow/:id/apply", api.RequirePermission("geo.workflow"), api.GeoApplyWorkflowJob)
			geo.DELETE("/keyword/workflow/:id", api.RequirePermission("geo.workflow"), api.GeoDiscardWorkflowJob)

			// Wizard defaults: built-in prompt template + recommended params.
			// Viewer permission is enough — doesn't expose anything sensitive,
			// just lets step-3 of the wizard pre-fill the editor.
			geo.GET("/defaults", api.RequirePermission("geo.view"), api.GeoGetDefaults)
		}

		// FRW AI image/video generation. Each user brings their own Bearer
		// token + user_id copied from the frw.aiaiartist dashboard; apikey
		// is either set per-user or falls back to the global "frw_apikey"
		// setting. Gated by the new ai_gen.* permissions.
		frwG := v1.Group("/frw")
		{
			// Config is viewable/saveable by any user who can use either
			// image or video gen — the same endpoint serves both flows.
			frwG.GET("/config", api.GetMyFRWConfig)
			frwG.PUT("/config", api.SaveMyFRWConfig)

			// Generation endpoints. Permission check happens per-request
			// because a single user may have only image or only video.
			frwG.GET("/generations", api.RequirePermission("ai_gen.view"), api.ListFRWGenerations)
			frwG.POST("/generations", api.RequirePermission("ai_gen.submit"), api.SubmitFRWGeneration)
			frwG.GET("/generations/:id", api.RequirePermission("ai_gen.view"), api.GetFRWGeneration)
			frwG.POST("/generations/:id/publish", api.RequirePermission("ai_gen.publish"), api.PublishFRWGeneration)
			frwG.DELETE("/generations/:id", api.RequirePermission("ai_gen.submit"), api.DeleteFRWGeneration)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: audit/report downloads can be slow; Gin and the
		// reverse proxy already cap per-request.
		IdleTimeout: 120 * time.Second,
	}

	// Graceful shutdown: on SIGINT/SIGTERM, stop accepting new requests and
	// let in-flight ones finish. Running audits/deadlink checks detach into
	// their own goroutines and may outlive the HTTP server.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
