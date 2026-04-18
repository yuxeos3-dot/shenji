package db

import (
	"database/sql"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var DB *sql.DB

const schema = `
-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    email      TEXT UNIQUE NOT NULL,
    password   TEXT NOT NULL,
    role       TEXT DEFAULT 'member',
    telegram_id TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_login DATETIME
);

-- 站点表
CREATE TABLE IF NOT EXISTS sites (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    domain          TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    gsc_site_url    TEXT DEFAULT '',
    ga_property_id  TEXT DEFAULT '',
    yandex_host_id  TEXT DEFAULT '',
    bing_site_url   TEXT DEFAULT '',
    url_category    TEXT DEFAULT '',
    url_detail      TEXT DEFAULT '',
    url_news_list   TEXT DEFAULT '',
    url_news_detail TEXT DEFAULT '',
    url_search      TEXT DEFAULT '',
    url_static      TEXT DEFAULT '',
    status          TEXT DEFAULT 'active',
    audit_schedule_enabled TEXT DEFAULT 'false',
    audit_interval_hours   INTEGER DEFAULT 0,
    audit_daily_hour       INTEGER DEFAULT -1,
    created_by      INTEGER REFERENCES users(id),
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 审计任务表
CREATE TABLE IF NOT EXISTS audit_tasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    triggered_by INTEGER REFERENCES users(id),
    status      TEXT DEFAULT 'pending',
    progress    INTEGER DEFAULT 0,
    error_msg   TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME
);

-- 抓取页面表
CREATE TABLE IF NOT EXISTS audit_pages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES audit_tasks(id),
    page_type   TEXT NOT NULL,
    url         TEXT NOT NULL,
    status_code INTEGER DEFAULT 0,
    ttfb_ms     INTEGER DEFAULT 0,
    html        TEXT DEFAULT '',
    html_size   INTEGER DEFAULT 0,
    error_msg   TEXT DEFAULT '',
    fetched_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- AI分析结果表
CREATE TABLE IF NOT EXISTS audit_results (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES audit_tasks(id),
    seo_issues      TEXT DEFAULT '',
    code_issues     TEXT DEFAULT '',
    security_issues TEXT DEFAULT '',
    perf_issues     TEXT DEFAULT '',
    suggestions     TEXT DEFAULT '',
    summary         TEXT DEFAULT '',
    ai_model        TEXT DEFAULT '',
    v6_report       TEXT DEFAULT '',
    raw_ai_response TEXT DEFAULT '',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- GSC数据表
CREATE TABLE IF NOT EXISTS gsc_data (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    date        TEXT NOT NULL,
    clicks      INTEGER DEFAULT 0,
    impressions INTEGER DEFAULT 0,
    ctr         REAL DEFAULT 0,
    position    REAL DEFAULT 0,
    keywords_json TEXT DEFAULT '[]',
    pages_json    TEXT DEFAULT '[]',
    devices_json  TEXT DEFAULT '{}',
    countries_json TEXT DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, date)
);

-- GA数据表
CREATE TABLE IF NOT EXISTS ga_data (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    date        TEXT NOT NULL,
    sessions    INTEGER DEFAULT 0,
    users       INTEGER DEFAULT 0,
    pageviews   INTEGER DEFAULT 0,
    bounce_rate REAL DEFAULT 0,
    avg_duration REAL DEFAULT 0,
    channels_json TEXT DEFAULT '{}',
    pages_json    TEXT DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, date)
);

-- Yandex数据表
CREATE TABLE IF NOT EXISTS yandex_data (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    date        TEXT NOT NULL,
    clicks      INTEGER DEFAULT 0,
    impressions INTEGER DEFAULT 0,
    index_count INTEGER DEFAULT 0,
    keywords_json TEXT DEFAULT '[]',
    errors_json   TEXT DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, date)
);

-- 站点自定义页面表
CREATE TABLE IF NOT EXISTS site_pages (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id        INTEGER NOT NULL REFERENCES sites(id),
    name           TEXT NOT NULL,
    url            TEXT NOT NULL,
    last_html      TEXT DEFAULT '',
    last_fetched_at DATETIME,
    status_code    INTEGER DEFAULT 0,
    ttfb_ms        INTEGER DEFAULT 0,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 死链检测结果表
CREATE TABLE IF NOT EXISTS dead_link_results (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id     INTEGER NOT NULL,
    url         TEXT NOT NULL,
    source_page TEXT DEFAULT '',
    status_code INTEGER DEFAULT 0,
    ttfb_ms     INTEGER DEFAULT 0,
    checked_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 外链管理表
CREATE TABLE IF NOT EXISTS external_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id     INTEGER NOT NULL,
    url         TEXT NOT NULL,
    anchor_text TEXT DEFAULT '',
    source_url  TEXT DEFAULT '',
    rel_type    TEXT DEFAULT 'dofollow',
    category    TEXT DEFAULT '',
    note        TEXT DEFAULT '',
    status_code INTEGER DEFAULT 0,
    checked_at  DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Bing数据表
CREATE TABLE IF NOT EXISTS bing_data (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id       INTEGER NOT NULL REFERENCES sites(id),
    date          TEXT NOT NULL,
    clicks        INTEGER DEFAULT 0,
    impressions   INTEGER DEFAULT 0,
    ctr           REAL DEFAULT 0,
    avg_position  REAL DEFAULT 0,
    index_count   INTEGER DEFAULT 0,
    queries_json  TEXT DEFAULT 'null',
    pages_json    TEXT DEFAULT 'null',
    crawl_json    TEXT DEFAULT 'null',
    errors_json   TEXT DEFAULT 'null',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, date)
);

-- 每日汇报表
CREATE TABLE IF NOT EXISTS daily_reports (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    report_date TEXT NOT NULL,
    content     TEXT NOT NULL,
    tasks_done  TEXT DEFAULT '[]',
    tasks_todo  TEXT DEFAULT '[]',
    issues      TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, report_date)
);

-- 系统设置表
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- OAuth Token存储
CREATE TABLE IF NOT EXISTS oauth_tokens (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    service      TEXT NOT NULL,
    access_token  TEXT NOT NULL,
    refresh_token TEXT DEFAULT '',
    expires_at   DATETIME,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 审计日志表
CREATE TABLE IF NOT EXISTS audit_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL,
    user_email  TEXT NOT NULL,
    action      TEXT NOT NULL,
    target_type TEXT DEFAULT '',
    target_id   TEXT DEFAULT '',
    detail      TEXT DEFAULT '',
    ip          TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for frequently queried columns
CREATE INDEX IF NOT EXISTS idx_audit_logs_user       ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action     ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created    ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_tasks_site_id    ON audit_tasks(site_id);
CREATE INDEX IF NOT EXISTS idx_audit_tasks_status     ON audit_tasks(status);
CREATE INDEX IF NOT EXISTS idx_audit_tasks_created_at ON audit_tasks(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_pages_task_id    ON audit_pages(task_id);
CREATE INDEX IF NOT EXISTS idx_audit_results_task_id  ON audit_results(task_id);
CREATE INDEX IF NOT EXISTS idx_site_pages_site_id     ON site_pages(site_id);
CREATE INDEX IF NOT EXISTS idx_gsc_data_site_date     ON gsc_data(site_id, date);
CREATE INDEX IF NOT EXISTS idx_ga_data_site_date      ON ga_data(site_id, date);
CREATE INDEX IF NOT EXISTS idx_yandex_data_site_date  ON yandex_data(site_id, date);
CREATE INDEX IF NOT EXISTS idx_dead_link_site_id      ON dead_link_results(site_id);
CREATE INDEX IF NOT EXISTS idx_ext_links_site_id      ON external_links(site_id);
CREATE INDEX IF NOT EXISTS idx_daily_reports_user     ON daily_reports(user_id, report_date);
`

func Init() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./seo.db"
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=on")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// SQLite caveat: with modernc.org/sqlite, opening more than one raw
	// connection to the same DB file results in all-but-one being treated
	// as read-only (unless the DSN explicitly opts into shared-cache
	// read/write mode). An earlier attempt to raise this to 10 to allow
	// concurrent reads actually broke every write migration at startup
	// with "attempt to write a readonly database (8)".
	//
	// For this app's traffic profile (single-process, low QPS, SQLite
	// backend) one connection is plenty — SQLite itself serialises
	// writes, so extra pooled connections only buy us trouble. If read
	// throughput ever becomes a real bottleneck, the right fix is to
	// open a second DB handle in read-only mode for SELECTs, NOT to
	// raise this value.
	DB.SetMaxOpenConns(1)

	// Additional PRAGMAs (idempotent). These reinforce the DSN params
	// and ensure WAL mode, synchronous=NORMAL, busy_timeout=5000 are
	// active even if the driver ignores underscore-prefixed DSN hints.
	for _, p := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA temp_store=MEMORY;",
	} {
		if _, err := DB.Exec(p); err != nil {
			log.Printf("PRAGMA failed (%s): %v", p, err)
		}
	}
	// Verify WAL mode is active (some drivers silently ignore DSN params).
	var journalMode string
	if err := DB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err == nil {
		log.Printf("SQLite journal_mode=%s", journalMode)
	}

	if _, err := DB.Exec(schema); err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	// Migrate existing databases.
	migrations := []string{
		"ALTER TABLE sites ADD COLUMN audit_schedule_enabled TEXT DEFAULT 'false'",
		"ALTER TABLE sites ADD COLUMN audit_interval_hours INTEGER DEFAULT 0",
		"ALTER TABLE sites ADD COLUMN audit_daily_hour INTEGER DEFAULT -1",
		"ALTER TABLE sites ADD COLUMN url_pages_json TEXT DEFAULT '[]'",
		`CREATE TABLE IF NOT EXISTS site_pages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			last_html TEXT DEFAULT '',
			last_fetched_at DATETIME,
			status_code INTEGER DEFAULT 0,
			ttfb_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		"ALTER TABLE sites ADD COLUMN bing_site_url TEXT DEFAULT ''",
		"ALTER TABLE audit_results ADD COLUMN v6_report TEXT DEFAULT ''",
		"ALTER TABLE audit_results ADD COLUMN raw_ai_response TEXT DEFAULT ''",

		// GEO Content Production Module
		`CREATE TABLE IF NOT EXISTS geo_title_libs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			title_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS geo_titles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			lib_id INTEGER NOT NULL REFERENCES geo_title_libs(id),
			title TEXT NOT NULL,
			keyword TEXT DEFAULT '',
			is_used INTEGER DEFAULT 0,
			used_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_titles_lib ON geo_titles(lib_id, is_used)`,
		`CREATE TABLE IF NOT EXISTS geo_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT DEFAULT 'content',
			content TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS geo_knowledge (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			content TEXT NOT NULL,
			char_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS geo_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			title_lib_id INTEGER REFERENCES geo_title_libs(id),
			prompt_id INTEGER REFERENCES geo_prompts(id),
			knowledge_id INTEGER,
			status TEXT DEFAULT 'paused',
			draft_limit INTEGER DEFAULT 20,
			publish_interval INTEGER DEFAULT 86400,
			need_review INTEGER DEFAULT 1,
			is_loop INTEGER DEFAULT 0,
			auto_keywords INTEGER DEFAULT 1,
			auto_description INTEGER DEFAULT 1,
			cron_expr TEXT DEFAULT '',
			created_count INTEGER DEFAULT 0,
			last_run_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS geo_articles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER REFERENCES geo_tasks(id),
			title_id INTEGER REFERENCES geo_titles(id),
			title TEXT NOT NULL,
			slug TEXT DEFAULT '',
			content TEXT NOT NULL,
			excerpt TEXT DEFAULT '',
			keywords TEXT DEFAULT '',
			meta_description TEXT DEFAULT '',
			category TEXT DEFAULT '',
			status TEXT DEFAULT 'draft',
			review_status TEXT DEFAULT 'pending',
			reviewer_note TEXT DEFAULT '',
			ai_model TEXT DEFAULT '',
			word_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			published_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_task ON geo_articles(task_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_review ON geo_articles(review_status)`,
		"ALTER TABLE geo_title_libs ADD COLUMN content_type TEXT DEFAULT 'geo'",
		"ALTER TABLE geo_tasks ADD COLUMN content_type TEXT DEFAULT 'geo'",
		"ALTER TABLE geo_articles ADD COLUMN content_type TEXT DEFAULT 'geo'",
		"ALTER TABLE geo_prompts ADD COLUMN content_type TEXT DEFAULT 'geo'",
		"ALTER TABLE geo_knowledge ADD COLUMN content_type TEXT DEFAULT 'geo'",
		// Make sure all existing rows have content_type='geo' (not NULL)
		"UPDATE geo_title_libs SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_tasks SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_articles SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_prompts SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_knowledge SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		`CREATE INDEX IF NOT EXISTS idx_geo_title_libs_type ON geo_title_libs(content_type)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_tasks_type ON geo_tasks(content_type)`,
		"ALTER TABLE geo_title_libs ADD COLUMN site_id INTEGER DEFAULT 0",
		"ALTER TABLE geo_tasks ADD COLUMN site_id INTEGER DEFAULT 0",
		"ALTER TABLE geo_articles ADD COLUMN site_id INTEGER DEFAULT 0",
		`CREATE INDEX IF NOT EXISTS idx_geo_title_libs_site ON geo_title_libs(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_tasks_site ON geo_tasks(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_site ON geo_articles(site_id)`,
		"ALTER TABLE users ADD COLUMN permissions TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN ai_configs TEXT DEFAULT ''",
		`CREATE TABLE IF NOT EXISTS extlink_pool (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			name TEXT DEFAULT '',
			created_by INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			user_email TEXT NOT NULL,
			action TEXT NOT NULL,
			target_type TEXT DEFAULT '',
			target_id TEXT DEFAULT '',
			detail TEXT DEFAULT '',
			ip TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at)`,

		// ─── Spider (search engine crawler) hit logs ────────────────────────
		`CREATE TABLE IF NOT EXISTS spider_logs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id      INTEGER NOT NULL,
			spider       TEXT NOT NULL,
			ip           TEXT DEFAULT '',
			user_agent   TEXT DEFAULT '',
			url          TEXT DEFAULT '',
			status_code  INTEGER DEFAULT 0,
			ttfb_ms      INTEGER DEFAULT 0,
			referrer     TEXT DEFAULT '',
			visited_at   DATETIME NOT NULL,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_site_time ON spider_logs(site_id, visited_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_site_name_time ON spider_logs(site_id, spider, visited_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_visited_at ON spider_logs(visited_at)`,

		// ─── Coordination workflow ──────────────────────────────────────────
		// Audit reports surface a lot of issues; not all of them are acted
		// on. The coordination feature lets an admin pick the ones that
		// need to be sent to a developer and track the fix through to
		// re-audit. Timeline events live in a sibling table so history is
		// append-only and every state transition is auditable.
		`CREATE TABLE IF NOT EXISTS coordination_tasks (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id         INTEGER NOT NULL REFERENCES sites(id),
			audit_task_id   INTEGER NOT NULL,
			issue_id        TEXT DEFAULT '',           -- id from the v6 report (may repeat across audits)
			issue_title     TEXT NOT NULL,
			issue_severity  TEXT DEFAULT '',
			issue_module    TEXT DEFAULT '',
			issue_chapter   TEXT DEFAULT '',
			issue_snapshot  TEXT DEFAULT '',           -- full JSON of the issue at the moment it was added
			note            TEXT DEFAULT '',           -- free-form description
			status          TEXT NOT NULL DEFAULT 'pending',  -- pending / in_progress / under_review / done
			created_by      INTEGER NOT NULL,
			assignee_id     INTEGER DEFAULT 0,         -- the developer handling it
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at      DATETIME,                  -- set when status moves to in_progress
			submitted_at    DATETIME,                  -- set when developer marks fix done
			verified_at     DATETIME                   -- set when admin closes the task
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_site ON coordination_tasks(site_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_assignee ON coordination_tasks(assignee_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_created_by ON coordination_tasks(created_by)`,

		// Append-only timeline. Every meaningful action (create/assign/
		// start/comment/submit/verify) gets a row so the detail page can
		// render a full history with actor, time, and optional note.
		`CREATE TABLE IF NOT EXISTS coordination_events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     INTEGER NOT NULL REFERENCES coordination_tasks(id),
			event_type  TEXT NOT NULL,                 -- create/assign/start/comment/submit/verify
			from_status TEXT DEFAULT '',
			to_status   TEXT DEFAULT '',
			actor_id    INTEGER NOT NULL,
			actor_name  TEXT DEFAULT '',
			note        TEXT DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_events_task ON coordination_events(task_id, created_at)`,

		// Screenshot attachments. Bound to a task; optionally also to an
		// event (so we can render "2 screenshots uploaded with this
		// comment" style entries on the timeline).
		`CREATE TABLE IF NOT EXISTS coordination_attachments (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     INTEGER NOT NULL REFERENCES coordination_tasks(id),
			event_id    INTEGER DEFAULT 0,
			filename    TEXT NOT NULL,                 -- display name
			filepath    TEXT NOT NULL,                 -- relative path under the uploads root
			size        INTEGER DEFAULT 0,
			mime        TEXT DEFAULT '',
			uploaded_by INTEGER NOT NULL,
			uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_attach_task ON coordination_attachments(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_attach_event ON coordination_attachments(event_id)`,

		// ─── Organization structure (部门 / 组) ─────────────────────────────
		// Two-tier: departments contain groups, groups contain users. Both
		// names are stored on the row — the users table gains FK-ish
		// integer columns that reference these tables. We keep them as
		// plain integers (not REAL foreign keys) so SQLite doesn't
		// complain when a department is deleted (we re-point orphans
		// to 0 in the delete handler).
		`CREATE TABLE IF NOT EXISTS departments (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS dept_groups (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			department_id INTEGER NOT NULL,
			name          TEXT NOT NULL,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by    INTEGER DEFAULT 0,
			UNIQUE(department_id, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dept_groups_dept ON dept_groups(department_id)`,

		// Existing users rows get 0 = unassigned. IF NOT EXISTS isn't
		// supported by SQLite for ALTER TABLE ADD COLUMN, so the
		// "duplicate column" error filter in the migration loop below
		// makes this idempotent.
		`ALTER TABLE users ADD COLUMN department_id INTEGER DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN group_id INTEGER DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_users_dept_group ON users(department_id, group_id)`,

		// ─── Weekly reports ─────────────────────────────────────────────────
		// Aggregate view of a user's week: AI summary of their 5-7 daily
		// reports for that window. Content is free-form markdown that the
		// user polishes in the editor before submitting. Once submitted
		// the row is locked (status='submitted') — anyone trying to edit
		// gets a 409.
		`CREATE TABLE IF NOT EXISTS weekly_reports (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id          INTEGER NOT NULL REFERENCES users(id),
			week_start       TEXT NOT NULL,   -- YYYY-MM-DD inclusive
			week_end         TEXT NOT NULL,   -- YYYY-MM-DD inclusive
			content          TEXT DEFAULT '',
			source_daily_ids TEXT DEFAULT '[]',  -- JSON array of daily_reports.id used as source
			status           TEXT DEFAULT 'draft', -- draft / submitted
			submitted_at     DATETIME,
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, week_start, week_end)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_weekly_user_range ON weekly_reports(user_id, week_start DESC)`,

		// ─── Monthly reports ────────────────────────────────────────────────
		// One row per (user, year, month). Source = submitted weekly
		// reports whose week_start falls in that month. Same state
		// machine as weekly: draft → submitted (locked).
		`CREATE TABLE IF NOT EXISTS monthly_reports (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id           INTEGER NOT NULL REFERENCES users(id),
			year              INTEGER NOT NULL,
			month             INTEGER NOT NULL,   -- 1..12
			content           TEXT DEFAULT '',
			source_weekly_ids TEXT DEFAULT '[]',
			status            TEXT DEFAULT 'draft',
			submitted_at      DATETIME,
			created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, year, month)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_user_ym ON monthly_reports(user_id, year DESC, month DESC)`,

		// ─── Report evaluations (汇报评价) ─────────────────────────────────
		// Polymorphic: one row can point to a daily, weekly, or monthly
		// report. Admins write these; authors read them on their own
		// detail page. UNIQUE(report_type, report_id, evaluator_id)
		// enforces "one evaluator has at most one evaluation per report"
		// — the POST handler upserts instead of creating duplicates.
		`CREATE TABLE IF NOT EXISTS report_evaluations (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			report_type    TEXT NOT NULL,       -- 'daily' | 'weekly' | 'monthly'
			report_id      INTEGER NOT NULL,
			evaluator_id   INTEGER NOT NULL REFERENCES users(id),
			evaluator_name TEXT DEFAULT '',
			content        TEXT NOT NULL,
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(report_type, report_id, evaluator_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_eval_target ON report_evaluations(report_type, report_id)`,
		`CREATE INDEX IF NOT EXISTS idx_eval_by ON report_evaluations(evaluator_id)`,

		// ─── GEO / ExtLink async workflow jobs ──────────────────────────────
		// Tracks 2-step AI pipeline state for per-keyword article writing:
		//   Step 1: long-form draft with anti-AI-detection humanization
		//   Step 2: TDK (meta_title, meta_desc) + FAQ as JSON
		// Frontend polls this table via GET /geo/jobs/:id every ~2s.
		`CREATE TABLE IF NOT EXISTS geo_workflow_jobs (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id           INTEGER NOT NULL,
			site_id           INTEGER NOT NULL,
			content_type      TEXT DEFAULT 'geo',   -- 'geo' | 'extlink'
			title_id          INTEGER DEFAULT 0,    -- source geo_titles.id (0 = ad-hoc)
			keyword           TEXT NOT NULL,
			title_text        TEXT DEFAULT '',
			word_count        INTEGER DEFAULT 1500,
			writing_angle     TEXT DEFAULT '',      -- optional persona hint
			knowledge_id      INTEGER DEFAULT 0,
			status            TEXT DEFAULT 'pending',   -- pending | running | done | failed
			current_step      INTEGER DEFAULT 0,
			step_label        TEXT DEFAULT '',
			draft             TEXT DEFAULT '',          -- step-1 output
			final_content     TEXT DEFAULT '',
			final_meta_title  TEXT DEFAULT '',
			final_meta_desc   TEXT DEFAULT '',
			final_faq         TEXT DEFAULT '[]',
			ai_model          TEXT DEFAULT '',
			error_msg         TEXT DEFAULT '',
			article_id        INTEGER DEFAULT 0,        -- filled on apply
			created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_jobs_user ON geo_workflow_jobs(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_jobs_site ON geo_workflow_jobs(site_id, status)`,

		// ─── Coord test-server pages + pre-audit ───────────────────────────
		// When a coordination task hits under_review (待验收), the reviewer
		// attaches one or more test-server URLs that were modified to fix
		// the issue. We crawl those pages, store their HTML, then ask AI
		// to compare the captured HTML against the original issue snapshot
		// and produce a verdict: fixed / not_fixed / unclear.
		// Reviewer then manually accepts (→ done) or rejects (→ in_progress
		// with required note that goes back to the developer).
		`CREATE TABLE IF NOT EXISTS coord_test_pages (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id         INTEGER NOT NULL,
			name            TEXT NOT NULL,
			url             TEXT NOT NULL,
			last_html       TEXT DEFAULT '',
			last_fetched_at DATETIME,
			status_code     INTEGER DEFAULT 0,
			ttfb_ms         INTEGER DEFAULT 0,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by      INTEGER DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_test_pages_task ON coord_test_pages(task_id)`,

		// Pre-audit output sticks to the task row so the review panel
		// shows the last verdict even after refreshing. verdict:
		// '' | 'fixed' | 'not_fixed' | 'unclear'. reasoning is free text.
		`ALTER TABLE coordination_tasks ADD COLUMN pre_audit_verdict TEXT DEFAULT ''`,
		`ALTER TABLE coordination_tasks ADD COLUMN pre_audit_result TEXT DEFAULT ''`,
		`ALTER TABLE coordination_tasks ADD COLUMN pre_audit_at DATETIME`,

		// ─── GEO wizard extensions ────────────────────────────────────────
		// The per-keyword "AI 生成" flow is being turned into an explicit
		// 4-step wizard. Instead of a separate drafts table, we extend the
		// existing geo_workflow_jobs row with wizard-supplied parameters.
		// When custom_prompt is non-empty we run it verbatim (with
		// {{keyword}}/{{wordcount}}/… variable substitution); empty means
		// "use the server default", which lets us keep existing jobs
		// working unchanged.
		`ALTER TABLE geo_workflow_jobs ADD COLUMN detail_type TEXT DEFAULT 'article'`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN language TEXT DEFAULT 'zh'`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN style TEXT DEFAULT 'professional'`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN output_format TEXT DEFAULT 'markdown'`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN paragraph_struct TEXT DEFAULT ''`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN extra_requirements TEXT DEFAULT ''`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN reference_url TEXT DEFAULT ''`,
		`ALTER TABLE geo_workflow_jobs ADD COLUMN custom_prompt TEXT DEFAULT ''`,

		// ─── Deprecated tables (4-step wizard superseded them) ────────────
		// The 每关键词 AI 生成 向导 replaces the old Prompt 模板 / 知识库 /
		// 生成任务 tabs. Prompts are now per-generation (custom_prompt on
		// geo_workflow_jobs). Knowledge bases and batch tasks are gone —
		// users manage keywords and click AI generate row-by-row.
		// Drop these tables outright; the article rows keep their
		// orphaned task_id column but it's now just historical and
		// unreferenced (no FK enforcement in SQLite).
		`DROP TABLE IF EXISTS geo_tasks`,
		`DROP TABLE IF EXISTS geo_prompts`,
		`DROP TABLE IF EXISTS geo_knowledge`,

		// ─── FRW (外部图/视频生成) ────────────────────────────────────────
		// Per-user FRW credentials. apikey is optional (falls back to
		// the global "frw_apikey" setting if empty). bearer_token and
		// user_id are obtained by the user logging into frw.aiaiartist
		// in their browser and copy-pasting from dev-tools. Token
		// expires; when submit returns 401 the UI asks user to refresh.
		`ALTER TABLE users ADD COLUMN frw_config TEXT DEFAULT ''`,

		// ai_generations: a single table for both image and video tasks.
		// "kind" distinguishes them; "mode" is the FRW template
		// (text2image / img2img / text2video / img2video). status
		// mirrors FRW: 0 pending / 1 running / 2 done / -1 failed / 4 timeout.
		// result_url: the FRW-hosted output; local_path: set when the
		// user clicks "发布" and we mirror it to /data/frw/...
		`CREATE TABLE IF NOT EXISTS ai_generations (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id      INTEGER NOT NULL,
			kind         TEXT NOT NULL,
			mode         TEXT NOT NULL,
			prompt       TEXT NOT NULL DEFAULT '',
			source_url   TEXT DEFAULT '',
			frw_task_id  TEXT DEFAULT '',
			status       INTEGER DEFAULT 0,
			progress     REAL DEFAULT 0,
			result_url   TEXT DEFAULT '',
			local_path   TEXT DEFAULT '',
			error_msg    TEXT DEFAULT '',
			extra_json   TEXT DEFAULT '{}',
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_gen_user_kind ON ai_generations(user_id, kind, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_gen_status ON ai_generations(status)`,
		// ─── Pre-launch performance + wizard-round-trip fixes ─────────────
		// - meta_title: the wizard lets users edit both Meta Title and Meta
		//   Description, but geo_articles was missing the column so edits to
		//   Meta Title silently vanished. Add the column and backfill from
		//   the matching workflow job on apply.
		// - Composite indexes: stats / list endpoints hit these hot columns
		//   without a covering index, causing full-table scans on a growing
		//   articles table.
		`ALTER TABLE geo_articles ADD COLUMN meta_title TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_content_type ON geo_articles(content_type, site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_status ON geo_articles(status, content_type)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_review_ct ON geo_articles(review_status, content_type)`,
		`CREATE INDEX IF NOT EXISTS idx_bing_data_site_date ON bing_data(site_id, date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_dead_link_results_site_status ON dead_link_results(site_id, status_code)`,

		// ─── Per-user platform credentials (pre-launch privacy refactor) ──
		// Before this change, OAuth client credentials, Telegram bot tokens,
		// Bing API key, and FRW apikey lived in the global `settings` table
		// which was readable by any admin. Users also shared one OAuth token
		// per provider, meaning whichever user last clicked "Authorize Google"
		// effectively logged everyone else's dashboards into their own GSC
		// account. The launch model is "every user is their own system"
		// (每一个用户相当于自己一个单独的系统), so all of this moves to
		// per-user storage. Admin keeps monitoring-only visibility via
		// /users/:id/platform-status.

		// OAuth application credentials (client_id / client_secret /
		// redirect_uri) — each user registers their own app in Google Cloud
		// Console / Yandex Console. Values are stored encrypted.
		`CREATE TABLE IF NOT EXISTS user_oauth_config (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id       INTEGER NOT NULL,
			service       TEXT NOT NULL,
			client_id     TEXT DEFAULT '',
			client_secret TEXT DEFAULT '',
			redirect_uri  TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, service)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_oauth_config_user ON user_oauth_config(user_id)`,

		// OAuth access + refresh tokens, per user per service. Replaces
		// the old global oauth_tokens table (which we keep around as a
		// migration source but stop reading from once the migration flag
		// is set).
		`CREATE TABLE IF NOT EXISTS user_oauth_tokens (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id       INTEGER NOT NULL,
			service       TEXT NOT NULL,
			access_token  TEXT NOT NULL,
			refresh_token TEXT DEFAULT '',
			expires_at    DATETIME,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, service)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_oauth_tokens_user ON user_oauth_tokens(user_id)`,

		// Telegram: each user has their own bot + chat. Audit completion
		// notifications and rejected-fix pings are sent via the triggering
		// user's bot (see telegram.SendNotificationFor(userID, ...)).
		`CREATE TABLE IF NOT EXISTS user_telegram (
			user_id    INTEGER PRIMARY KEY,
			bot_token  TEXT DEFAULT '',
			chat_id    TEXT DEFAULT '',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Bing uses a plain API key (not OAuth). One per user.
		`CREATE TABLE IF NOT EXISTS user_bing (
			user_id    INTEGER PRIMARY KEY,
			api_key    TEXT DEFAULT '',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// FRW apikey: added per-user. Drop the global fallback once we
		// read this column everywhere.
		`ALTER TABLE users ADD COLUMN frw_apikey TEXT DEFAULT ''`,
	}
	for _, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			msg := err.Error()
			// Ignorable: migrations are idempotent, and re-running
			// CREATE/ADD COLUMN on an already-migrated DB is expected.
			if strings.Contains(msg, "duplicate") ||
				strings.Contains(msg, "already exists") {
				continue
			}
			// Idempotent data-backfills (UPDATE ... SET default_value
			// WHERE IS NULL) are only there as a safety net for DBs that
			// predate the column's ALTER TABLE ... DEFAULT clause. On
			// some SQLite driver versions these spuriously report
			// "attempt to write a readonly database" during the startup
			// transaction even though the file is writable and the
			// subsequent DML succeeds. Since the DEFAULT clause in the
			// paired ALTER already backfills new rows, swallowing this
			// specific class of error is safe — it keeps the log clean
			// so real problems (like a CREATE TABLE failing) remain
			// visible.
			trimmed := strings.TrimSpace(m)
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "UPDATE ") &&
				strings.Contains(msg, "readonly") {
				continue
			}
			log.Printf("[migration] %.80q: %v", m, err)
		}
	}

	seedDefaultAdmin()
	seedDefaultSettings()

	log.Println("Database initialized successfully")
}

// seedDefaultAdmin creates an admin account on first run. Both
// ADMIN_INIT_EMAIL and ADMIN_INIT_PASSWORD must be set via environment
// variables; the server refuses to start if either is missing.
// The password is never printed to logs.
func seedDefaultAdmin() {
	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		log.Fatalf("Failed to check users: %v", err)
	}
	if count > 0 {
		return
	}

	email := os.Getenv("ADMIN_INIT_EMAIL")
	password := os.Getenv("ADMIN_INIT_PASSWORD")
	if email == "" || password == "" {
		log.Fatal("First run requires ADMIN_INIT_EMAIL and ADMIN_INIT_PASSWORD environment variables. " +
			"Example: ADMIN_INIT_EMAIL=you@example.com ADMIN_INIT_PASSWORD=secret ./server")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	_, err = DB.Exec(
		"INSERT INTO users (name, email, password, role) VALUES (?, ?, ?, ?)",
		"Admin", email, string(hash), "admin",
	)
	if err != nil {
		log.Fatalf("Failed to create default admin: %v", err)
	}

	log.Printf("Default admin created (email: %s)", email)
}

func seedDefaultSettings() {
	defaults := map[string]string{
		"site_title":                "SEO巡站管理系统",
		"auto_audit_enabled":        "false",
		"auto_audit_cron":           "0 9 * * *",
		"auto_audit_interval_hours": "0",
		"max_concurrent_audit":      "3",
		"ip_whitelist":              "",
		"spider_ingest_api_key":     "", // Set via admin UI; empty disables /api/spider/ingest
	}

	for k, v := range defaults {
		DB.Exec("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)", k, v)
	}
}

func GetSetting(key string) string {
	var val string
	err := DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

func SetSetting(key, value string) error {
	_, err := DB.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}
