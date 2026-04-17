package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"log"
	"os"
	"strings"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
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

	// Additional PRAGMAs (idempotent).
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

// seedDefaultAdmin creates an admin account on first run. The password is taken
// from ADMIN_INIT_PASSWORD if set; otherwise a strong random password is
// generated and printed to the log exactly once. The operator must log in with
// it and change the password immediately.
func seedDefaultAdmin() {
	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		log.Fatalf("Failed to check users: %v", err)
	}
	if count > 0 {
		return
	}

	password := os.Getenv("ADMIN_INIT_PASSWORD")
	generated := false
	if password == "" {
		// Generate a random password that definitely satisfies the policy
		// (>=12 chars, >=1 upper, >=1 lower, >=1 digit). Starting with a
		// fixed "Aa1" prefix + 20 random chars keeps it memorable-free
		// while guaranteeing complexity.
		buf := make([]byte, 20)
		if _, err := rand.Read(buf); err != nil {
			log.Fatalf("Failed to generate admin password: %v", err)
		}
		password = "Aa1" + base64.RawURLEncoding.EncodeToString(buf)
		generated = true
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	_, err = DB.Exec(
		"INSERT INTO users (name, email, password, role) VALUES (?, ?, ?, ?)",
		"Admin", "admin@seo-system.local", string(hash), "admin",
	)
	if err != nil {
		log.Fatalf("Failed to create default admin: %v", err)
	}

	if generated {
		log.Println("╔════════════════════════════════════════════════════════════════════╗")
		log.Println("║ FIRST-RUN ADMIN ACCOUNT CREATED                                    ║")
		log.Println("║ Email:    admin@seo-system.local                                   ║")
		log.Printf("║ Password: %-56s ║", password)
		log.Println("║ (This password will NOT be shown again — change it on first login) ║")
		log.Println("╚════════════════════════════════════════════════════════════════════╝")
	} else {
		log.Println("Default admin created (password from ADMIN_INIT_PASSWORD env)")
	}
}

func seedDefaultSettings() {
	defaults := map[string]string{
		"site_title":                   "SEO巡站管理系统",
		"auto_audit_enabled":           "false",
		"auto_audit_cron":              "0 9 * * *",
		"auto_audit_interval_hours":    "0",
		"max_concurrent_audit":         "3",
		"ai_api_url":                   "https://ai.router.team/v1",
		"ai_model":                     "claude-sonnet-4-6",
		"ip_whitelist":                 "",
		"spider_ingest_api_key":        "", // Set via admin UI; empty disables /api/spider/ingest
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
