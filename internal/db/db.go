package db

import (
	"database/sql"
	"log"
	"os"

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
    url_category    TEXT DEFAULT '',
    url_detail      TEXT DEFAULT '',
    url_news_list   TEXT DEFAULT '',
    url_news_detail TEXT DEFAULT '',
    url_search      TEXT DEFAULT '',
    url_static      TEXT DEFAULT '',
    status          TEXT DEFAULT 'active',
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
`

func Init() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./seo.db"
	}

	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	DB.SetMaxOpenConns(1)

	if _, err := DB.Exec(schema); err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	seedDefaultAdmin()
	seedDefaultSettings()

	log.Println("Database initialized successfully")
}

func seedDefaultAdmin() {
	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		log.Fatalf("Failed to check users: %v", err)
	}
	if count > 0 {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("Admin@123456"), 12)
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
	log.Println("Default admin created: admin@seo-system.local / Admin@123456")
}

func seedDefaultSettings() {
	defaults := map[string]string{
		"site_title":           "SEO巡站管理系统",
		"auto_audit_enabled":   "false",
		"auto_audit_cron":      "0 9 * * *",
		"max_concurrent_audit": "3",
		"ai_api_url":           "https://ai.router.team/v1",
		"ai_model":             "claude-sonnet-4-6",
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
