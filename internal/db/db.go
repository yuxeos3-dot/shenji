package db

import (
	"database/sql"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var DB *sql.DB

// Each CREATE TABLE is a separate string so we can execute them one by one
// (PostgreSQL does not support multiple CREATE TABLE in a single Exec).
var schemaTables = []string{
	`CREATE TABLE IF NOT EXISTS users (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    email      TEXT UNIQUE NOT NULL,
    password   TEXT NOT NULL,
    role       TEXT DEFAULT 'member',
    telegram_id TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_login TIMESTAMPTZ
)`,
	`CREATE TABLE IF NOT EXISTS sites (
    id              SERIAL PRIMARY KEY,
    domain          TEXT NOT NULL,
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
    url_pages_json  TEXT DEFAULT '[]',
    created_by      INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(domain, created_by)
)`,
	`CREATE TABLE IF NOT EXISTS audit_tasks (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    triggered_by INTEGER REFERENCES users(id),
    status      TEXT DEFAULT 'pending',
    progress    INTEGER DEFAULT 0,
    error_msg   TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    finished_at TIMESTAMPTZ
)`,
	`CREATE TABLE IF NOT EXISTS audit_pages (
    id          SERIAL PRIMARY KEY,
    task_id     INTEGER NOT NULL REFERENCES audit_tasks(id),
    page_type   TEXT NOT NULL,
    url         TEXT NOT NULL,
    status_code INTEGER DEFAULT 0,
    ttfb_ms     INTEGER DEFAULT 0,
    html        TEXT DEFAULT '',
    html_size   INTEGER DEFAULT 0,
    error_msg   TEXT DEFAULT '',
    fetched_at  TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS audit_results (
    id          SERIAL PRIMARY KEY,
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
    created_at      TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS gsc_data (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    date        TEXT NOT NULL,
    clicks      INTEGER DEFAULT 0,
    impressions INTEGER DEFAULT 0,
    ctr         DOUBLE PRECISION DEFAULT 0,
    position    DOUBLE PRECISION DEFAULT 0,
    keywords_json JSONB DEFAULT '[]',
    pages_json    JSONB DEFAULT '[]',
    devices_json  JSONB DEFAULT '{}',
    countries_json JSONB DEFAULT '[]',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(site_id, date)
)`,
	`CREATE TABLE IF NOT EXISTS ga_data (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    date        TEXT NOT NULL,
    sessions    INTEGER DEFAULT 0,
    users       INTEGER DEFAULT 0,
    pageviews   INTEGER DEFAULT 0,
    bounce_rate DOUBLE PRECISION DEFAULT 0,
    avg_duration DOUBLE PRECISION DEFAULT 0,
    channels_json JSONB DEFAULT '{}',
    pages_json    JSONB DEFAULT '[]',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(site_id, date)
)`,
	`CREATE TABLE IF NOT EXISTS yandex_data (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    date        TEXT NOT NULL,
    clicks      INTEGER DEFAULT 0,
    impressions INTEGER DEFAULT 0,
    index_count INTEGER DEFAULT 0,
    keywords_json JSONB DEFAULT '[]',
    errors_json   JSONB DEFAULT '[]',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(site_id, date)
)`,
	`CREATE TABLE IF NOT EXISTS site_pages (
    id             SERIAL PRIMARY KEY,
    site_id        INTEGER NOT NULL REFERENCES sites(id),
    name           TEXT NOT NULL,
    url            TEXT NOT NULL,
    last_html      TEXT DEFAULT '',
    last_fetched_at TIMESTAMPTZ,
    status_code    INTEGER DEFAULT 0,
    ttfb_ms        INTEGER DEFAULT 0,
    created_at     TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS dead_link_results (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL,
    url         TEXT NOT NULL,
    source_page TEXT DEFAULT '',
    status_code INTEGER DEFAULT 0,
    ttfb_ms     INTEGER DEFAULT 0,
    checked_at  TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS external_links (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL,
    url         TEXT NOT NULL,
    anchor_text TEXT DEFAULT '',
    source_url  TEXT DEFAULT '',
    rel_type    TEXT DEFAULT 'dofollow',
    category    TEXT DEFAULT '',
    note        TEXT DEFAULT '',
    status_code INTEGER DEFAULT 0,
    checked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS bing_data (
    id            SERIAL PRIMARY KEY,
    site_id       INTEGER NOT NULL REFERENCES sites(id),
    date          TEXT NOT NULL,
    clicks        INTEGER DEFAULT 0,
    impressions   INTEGER DEFAULT 0,
    ctr           DOUBLE PRECISION DEFAULT 0,
    avg_position  DOUBLE PRECISION DEFAULT 0,
    index_count   INTEGER DEFAULT 0,
    queries_json  JSONB DEFAULT 'null',
    pages_json    JSONB DEFAULT 'null',
    crawl_json    JSONB DEFAULT 'null',
    errors_json   JSONB DEFAULT 'null',
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(site_id, date)
)`,
	`CREATE TABLE IF NOT EXISTS daily_reports (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    report_date TEXT NOT NULL,
    content     TEXT NOT NULL,
    tasks_done  JSONB DEFAULT '[]',
    tasks_todo  JSONB DEFAULT '[]',
    issues      TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, report_date)
)`,
	`CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
)`,
	`CREATE TABLE IF NOT EXISTS oauth_tokens (
    id           SERIAL PRIMARY KEY,
    service      TEXT NOT NULL,
    access_token  TEXT NOT NULL,
    refresh_token TEXT DEFAULT '',
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL,
    user_email  TEXT NOT NULL,
    action      TEXT NOT NULL,
    target_type TEXT DEFAULT '',
    target_id   TEXT DEFAULT '',
    detail      TEXT DEFAULT '',
    ip          TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS push_logs (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    user_id     INTEGER NOT NULL REFERENCES users(id),
    platform    TEXT NOT NULL,
    push_type   TEXT NOT NULL,
    target_url  TEXT NOT NULL,
    status      TEXT DEFAULT 'pending',
    response    TEXT DEFAULT '',
    error_msg   TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW()
)`,
	`CREATE TABLE IF NOT EXISTS push_schedules (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id),
    user_id     INTEGER NOT NULL REFERENCES users(id),
    platform    TEXT NOT NULL,
    push_type   TEXT NOT NULL,
    target_url  TEXT NOT NULL,
    cron_expr   TEXT NOT NULL,
    enabled     BOOLEAN DEFAULT true,
    last_run_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
)`,
}

var schemaIndexes = []string{
	"CREATE INDEX IF NOT EXISTS idx_audit_logs_user       ON audit_logs(user_id)",
	"CREATE INDEX IF NOT EXISTS idx_audit_logs_action     ON audit_logs(action)",
	"CREATE INDEX IF NOT EXISTS idx_audit_logs_created    ON audit_logs(created_at)",
	"CREATE INDEX IF NOT EXISTS idx_audit_tasks_site_id    ON audit_tasks(site_id)",
	"CREATE INDEX IF NOT EXISTS idx_audit_tasks_status     ON audit_tasks(status)",
	"CREATE INDEX IF NOT EXISTS idx_audit_tasks_created_at ON audit_tasks(created_at)",
	"CREATE INDEX IF NOT EXISTS idx_audit_pages_task_id    ON audit_pages(task_id)",
	"CREATE INDEX IF NOT EXISTS idx_audit_results_task_id  ON audit_results(task_id)",
	"CREATE INDEX IF NOT EXISTS idx_site_pages_site_id     ON site_pages(site_id)",
	"CREATE INDEX IF NOT EXISTS idx_gsc_data_site_date     ON gsc_data(site_id, date)",
	"CREATE INDEX IF NOT EXISTS idx_ga_data_site_date      ON ga_data(site_id, date)",
	"CREATE INDEX IF NOT EXISTS idx_yandex_data_site_date  ON yandex_data(site_id, date)",
	"CREATE INDEX IF NOT EXISTS idx_dead_link_site_id      ON dead_link_results(site_id)",
	"CREATE INDEX IF NOT EXISTS idx_ext_links_site_id      ON external_links(site_id)",
	"CREATE INDEX IF NOT EXISTS idx_daily_reports_user     ON daily_reports(user_id, report_date)",
	"CREATE INDEX IF NOT EXISTS idx_push_logs_site_id      ON push_logs(site_id)",
	"CREATE INDEX IF NOT EXISTS idx_push_logs_user_id      ON push_logs(user_id)",
	"CREATE INDEX IF NOT EXISTS idx_push_logs_created_at   ON push_logs(created_at)",
	"CREATE INDEX IF NOT EXISTS idx_push_schedules_site_id ON push_schedules(site_id)",
	"CREATE INDEX IF NOT EXISTS idx_push_schedules_enabled ON push_schedules(enabled)",
}

func Init() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required (e.g. postgres://user:pass@localhost:5432/seo?sslmode=disable)")
	}

	var err error
	DB, err = sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Connection pool settings for ~300 user scale
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(10)
	DB.SetConnMaxLifetime(30 * time.Minute)
	DB.SetConnMaxIdleTime(5 * time.Minute)

	// Verify connectivity
	if err := DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Create schema tables one by one
	for _, ddl := range schemaTables {
		if _, err := DB.Exec(ddl); err != nil {
			log.Fatalf("Failed to create schema table: %v\nDDL: %.200s", err, ddl)
		}
	}
	for _, ddl := range schemaIndexes {
		if _, err := DB.Exec(ddl); err != nil {
			log.Fatalf("Failed to create index: %v\nDDL: %s", err, ddl)
		}
	}

	// Migrate existing databases.
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS geo_title_libs (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			title_count INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS geo_titles (
			id SERIAL PRIMARY KEY,
			lib_id INTEGER NOT NULL REFERENCES geo_title_libs(id),
			title TEXT NOT NULL,
			keyword TEXT DEFAULT '',
			is_used INTEGER DEFAULT 0,
			used_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_titles_lib ON geo_titles(lib_id, is_used)`,
		`CREATE TABLE IF NOT EXISTS geo_prompts (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT DEFAULT 'content',
			content TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS geo_knowledge (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			content TEXT NOT NULL,
			char_count INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS geo_tasks (
			id SERIAL PRIMARY KEY,
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
			last_run_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS geo_articles (
			id SERIAL PRIMARY KEY,
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
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			published_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_task ON geo_articles(task_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_review ON geo_articles(review_status)`,
		"DO $$ BEGIN ALTER TABLE geo_title_libs ADD COLUMN content_type TEXT DEFAULT 'geo'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_tasks ADD COLUMN content_type TEXT DEFAULT 'geo'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_articles ADD COLUMN content_type TEXT DEFAULT 'geo'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_prompts ADD COLUMN content_type TEXT DEFAULT 'geo'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_knowledge ADD COLUMN content_type TEXT DEFAULT 'geo'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"UPDATE geo_title_libs SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_tasks SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_articles SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_prompts SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		"UPDATE geo_knowledge SET content_type='geo' WHERE content_type IS NULL OR content_type=''",
		`CREATE INDEX IF NOT EXISTS idx_geo_title_libs_type ON geo_title_libs(content_type)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_tasks_type ON geo_tasks(content_type)`,
		"DO $$ BEGIN ALTER TABLE geo_title_libs ADD COLUMN site_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_tasks ADD COLUMN site_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_articles ADD COLUMN site_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE INDEX IF NOT EXISTS idx_geo_title_libs_site ON geo_title_libs(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_tasks_site ON geo_tasks(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_site ON geo_articles(site_id)`,
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN permissions TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN ai_configs TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE TABLE IF NOT EXISTS extlink_pool (
			id SERIAL PRIMARY KEY,
			url TEXT NOT NULL,
			name TEXT DEFAULT '',
			created_by INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS spider_logs (
			id           SERIAL PRIMARY KEY,
			site_id      INTEGER NOT NULL,
			spider       TEXT NOT NULL,
			ip           TEXT DEFAULT '',
			user_agent   TEXT DEFAULT '',
			url          TEXT DEFAULT '',
			status_code  INTEGER DEFAULT 0,
			ttfb_ms      INTEGER DEFAULT 0,
			referrer     TEXT DEFAULT '',
			visited_at   TIMESTAMPTZ NOT NULL,
			created_at   TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_site_time ON spider_logs(site_id, visited_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_site_name_time ON spider_logs(site_id, spider, visited_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_visited_at ON spider_logs(visited_at)`,
		`CREATE TABLE IF NOT EXISTS coordination_tasks (
			id              SERIAL PRIMARY KEY,
			site_id         INTEGER NOT NULL REFERENCES sites(id),
			audit_task_id   INTEGER NOT NULL,
			issue_id        TEXT DEFAULT '',
			issue_title     TEXT NOT NULL,
			issue_severity  TEXT DEFAULT '',
			issue_module    TEXT DEFAULT '',
			issue_chapter   TEXT DEFAULT '',
			issue_snapshot  TEXT DEFAULT '',
			note            TEXT DEFAULT '',
			status          TEXT NOT NULL DEFAULT 'pending',
			created_by      INTEGER NOT NULL,
			assignee_id     INTEGER DEFAULT 0,
			created_at      TIMESTAMPTZ DEFAULT NOW(),
			started_at      TIMESTAMPTZ,
			submitted_at    TIMESTAMPTZ,
			verified_at     TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_site ON coordination_tasks(site_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_assignee ON coordination_tasks(assignee_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_created_by ON coordination_tasks(created_by)`,
		`CREATE TABLE IF NOT EXISTS coordination_events (
			id          SERIAL PRIMARY KEY,
			task_id     INTEGER NOT NULL REFERENCES coordination_tasks(id),
			event_type  TEXT NOT NULL,
			from_status TEXT DEFAULT '',
			to_status   TEXT DEFAULT '',
			actor_id    INTEGER NOT NULL,
			actor_name  TEXT DEFAULT '',
			note        TEXT DEFAULT '',
			created_at  TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_events_task ON coordination_events(task_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS coordination_attachments (
			id          SERIAL PRIMARY KEY,
			task_id     INTEGER NOT NULL REFERENCES coordination_tasks(id),
			event_id    INTEGER DEFAULT 0,
			filename    TEXT NOT NULL,
			filepath    TEXT NOT NULL,
			size        INTEGER DEFAULT 0,
			mime        TEXT DEFAULT '',
			uploaded_by INTEGER NOT NULL,
			uploaded_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_attach_task ON coordination_attachments(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_attach_event ON coordination_attachments(event_id)`,
		`CREATE TABLE IF NOT EXISTS departments (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			created_by INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS dept_groups (
			id            SERIAL PRIMARY KEY,
			department_id INTEGER NOT NULL,
			name          TEXT NOT NULL,
			created_at    TIMESTAMPTZ DEFAULT NOW(),
			created_by    INTEGER DEFAULT 0,
			UNIQUE(department_id, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dept_groups_dept ON dept_groups(department_id)`,
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN department_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN group_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE INDEX IF NOT EXISTS idx_users_dept_group ON users(department_id, group_id)`,
		`CREATE TABLE IF NOT EXISTS weekly_reports (
			id               SERIAL PRIMARY KEY,
			user_id          INTEGER NOT NULL REFERENCES users(id),
			week_start       TEXT NOT NULL,
			week_end         TEXT NOT NULL,
			content          TEXT DEFAULT '',
			source_daily_ids JSONB DEFAULT '[]',
			status           TEXT DEFAULT 'draft',
			submitted_at     TIMESTAMPTZ,
			created_at       TIMESTAMPTZ DEFAULT NOW(),
			updated_at       TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(user_id, week_start, week_end)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_weekly_user_range ON weekly_reports(user_id, week_start DESC)`,
		`CREATE TABLE IF NOT EXISTS monthly_reports (
			id                SERIAL PRIMARY KEY,
			user_id           INTEGER NOT NULL REFERENCES users(id),
			year              INTEGER NOT NULL,
			month             INTEGER NOT NULL,
			content           TEXT DEFAULT '',
			source_weekly_ids JSONB DEFAULT '[]',
			status            TEXT DEFAULT 'draft',
			submitted_at      TIMESTAMPTZ,
			created_at        TIMESTAMPTZ DEFAULT NOW(),
			updated_at        TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(user_id, year, month)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_user_ym ON monthly_reports(user_id, year DESC, month DESC)`,
		`CREATE TABLE IF NOT EXISTS report_evaluations (
			id             SERIAL PRIMARY KEY,
			report_type    TEXT NOT NULL,
			report_id      INTEGER NOT NULL,
			evaluator_id   INTEGER NOT NULL REFERENCES users(id),
			evaluator_name TEXT DEFAULT '',
			content        TEXT NOT NULL,
			created_at     TIMESTAMPTZ DEFAULT NOW(),
			updated_at     TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(report_type, report_id, evaluator_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_eval_target ON report_evaluations(report_type, report_id)`,
		`CREATE INDEX IF NOT EXISTS idx_eval_by ON report_evaluations(evaluator_id)`,
		`CREATE TABLE IF NOT EXISTS geo_workflow_jobs (
			id                SERIAL PRIMARY KEY,
			user_id           INTEGER NOT NULL,
			site_id           INTEGER NOT NULL,
			content_type      TEXT DEFAULT 'geo',
			title_id          INTEGER DEFAULT 0,
			keyword           TEXT NOT NULL,
			title_text        TEXT DEFAULT '',
			word_count        INTEGER DEFAULT 1500,
			writing_angle     TEXT DEFAULT '',
			knowledge_id      INTEGER DEFAULT 0,
			status            TEXT DEFAULT 'pending',
			current_step      INTEGER DEFAULT 0,
			step_label        TEXT DEFAULT '',
			draft             TEXT DEFAULT '',
			final_content     TEXT DEFAULT '',
			final_meta_title  TEXT DEFAULT '',
			final_meta_desc   TEXT DEFAULT '',
			final_faq         JSONB DEFAULT '[]',
			ai_model          TEXT DEFAULT '',
			error_msg         TEXT DEFAULT '',
			article_id        INTEGER DEFAULT 0,
			created_at        TIMESTAMPTZ DEFAULT NOW(),
			updated_at        TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_jobs_user ON geo_workflow_jobs(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_jobs_site ON geo_workflow_jobs(site_id, status)`,
		`CREATE TABLE IF NOT EXISTS coord_test_pages (
			id              SERIAL PRIMARY KEY,
			task_id         INTEGER NOT NULL,
			name            TEXT NOT NULL,
			url             TEXT NOT NULL,
			last_html       TEXT DEFAULT '',
			last_fetched_at TIMESTAMPTZ,
			status_code     INTEGER DEFAULT 0,
			ttfb_ms         INTEGER DEFAULT 0,
			created_at      TIMESTAMPTZ DEFAULT NOW(),
			created_by      INTEGER DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_test_pages_task ON coord_test_pages(task_id)`,
		"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN pre_audit_verdict TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN pre_audit_result TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN pre_audit_at TIMESTAMPTZ; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN detail_type TEXT DEFAULT 'article'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN language TEXT DEFAULT 'zh'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN style TEXT DEFAULT 'professional'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN output_format TEXT DEFAULT 'markdown'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN paragraph_struct TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN extra_requirements TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN reference_url TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE geo_workflow_jobs ADD COLUMN custom_prompt TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`DROP TABLE IF EXISTS geo_tasks`,
		`DROP TABLE IF EXISTS geo_prompts`,
		`DROP TABLE IF EXISTS geo_knowledge`,
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN frw_config TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE TABLE IF NOT EXISTS ai_generations (
			id           SERIAL PRIMARY KEY,
			user_id      INTEGER NOT NULL,
			kind         TEXT NOT NULL,
			mode         TEXT NOT NULL,
			prompt       TEXT NOT NULL DEFAULT '',
			source_url   TEXT DEFAULT '',
			frw_task_id  TEXT DEFAULT '',
			status       INTEGER DEFAULT 0,
			progress     DOUBLE PRECISION DEFAULT 0,
			result_url   TEXT DEFAULT '',
			local_path   TEXT DEFAULT '',
			error_msg    TEXT DEFAULT '',
			extra_json   JSONB DEFAULT '{}',
			created_at   TIMESTAMPTZ DEFAULT NOW(),
			updated_at   TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_gen_user_kind ON ai_generations(user_id, kind, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_gen_status ON ai_generations(status)`,
		"DO $$ BEGIN ALTER TABLE geo_articles ADD COLUMN meta_title TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_content_type ON geo_articles(content_type, site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_status ON geo_articles(status, content_type)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_review_ct ON geo_articles(review_status, content_type)`,
		`CREATE INDEX IF NOT EXISTS idx_bing_data_site_date ON bing_data(site_id, date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_dead_link_results_site_status ON dead_link_results(site_id, status_code)`,
		`CREATE TABLE IF NOT EXISTS user_oauth_config (
			id            SERIAL PRIMARY KEY,
			user_id       INTEGER NOT NULL,
			service       TEXT NOT NULL,
			client_id     TEXT DEFAULT '',
			client_secret TEXT DEFAULT '',
			redirect_uri  TEXT DEFAULT '',
			created_at    TIMESTAMPTZ DEFAULT NOW(),
			updated_at    TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(user_id, service)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_oauth_config_user ON user_oauth_config(user_id)`,
		`CREATE TABLE IF NOT EXISTS user_oauth_tokens (
			id            SERIAL PRIMARY KEY,
			user_id       INTEGER NOT NULL,
			service       TEXT NOT NULL,
			access_token  TEXT NOT NULL,
			refresh_token TEXT DEFAULT '',
			expires_at    TIMESTAMPTZ,
			created_at    TIMESTAMPTZ DEFAULT NOW(),
			updated_at    TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(user_id, service)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_oauth_tokens_user ON user_oauth_tokens(user_id)`,
		`CREATE TABLE IF NOT EXISTS user_telegram (
			user_id    INTEGER PRIMARY KEY,
			bot_token  TEXT DEFAULT '',
			chat_id    TEXT DEFAULT '',
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS user_bing (
			user_id    INTEGER PRIMARY KEY,
			api_key    TEXT DEFAULT '',
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN frw_apikey TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"UPDATE users SET email = LOWER(email)",
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_ci ON users(LOWER(email))`,
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN token_invalidated_after INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		// Phase 3: Additional indexes for common query patterns.
		`CREATE INDEX IF NOT EXISTS idx_sites_created_by ON sites(created_by)`,
		`CREATE INDEX IF NOT EXISTS idx_coordination_tasks_site ON coordination_tasks(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_coordination_events_task ON coordination_events(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_site ON geo_articles(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_workflow_jobs_user ON geo_workflow_jobs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_spider_logs_site ON spider_logs(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_users_role ON users(role)`,
		`CREATE INDEX IF NOT EXISTS idx_coordination_tasks_status ON coordination_tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_geo_articles_type_site ON geo_articles(content_type, site_id)`,
		// Phase 4: Persistent state tables (lockout + OAuth state).
		`CREATE TABLE IF NOT EXISTS account_lockouts (
			email TEXT PRIMARY KEY,
			failure_count INTEGER DEFAULT 0,
			last_failure_at TIMESTAMPTZ,
			locked_until TIMESTAMPTZ,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_states (
			state TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		// ─── Self-registration support (2026-04) ─────────────────────────
		// Users self-register via the public POST /api/auth/register form
		// and land in register_status='pending'; an admin reviews from the
		// 用户管理 → 待审核 list and either approves (granting permissions)
		// or rejects (deletes the row). Existing rows default to 'approved'
		// so the migration is non-disruptive.
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN tg_name TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN nickname TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN employee_id TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN invite_code TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE users ADD COLUMN register_status TEXT DEFAULT 'approved'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		// employee_id is the field-level unique identifier for staff —
		// case-insensitive so HF001 and hf001 collide.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_employee_id_ci ON users(LOWER(employee_id)) WHERE employee_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_users_register_status ON users(register_status)`,
		// 协同任务 — 抽取 issue_snapshot 里的 scope 到独立列，避免每次
		// 列表请求都解析 JSON。新建任务时由 CreateCoordTask 写入；老
		// 行通过 perm_migrated_v6 一次性回填。
		"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN issue_scope TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE INDEX IF NOT EXISTS idx_coord_issue_scope ON coordination_tasks(issue_scope) WHERE issue_scope <> ''`,
		// 站点历史事件 — 手动登记的运维 / 故障 / 黑名单事件，配合
		// 协同管理的"已完成任务"一起在站点详情页里做"历史错误汇总"展示。
		`CREATE TABLE IF NOT EXISTS site_incidents (
			id          SERIAL PRIMARY KEY,
			site_id     INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			title       TEXT NOT NULL,
			description TEXT DEFAULT '',
			severity    TEXT DEFAULT 'medium',
			occurred_at TIMESTAMPTZ DEFAULT NOW(),
			created_by  INTEGER NOT NULL,
			created_at  TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_site_incidents_site ON site_incidents(site_id, occurred_at DESC)`,
	}
	for _, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "duplicate") ||
				strings.Contains(msg, "already exists") {
				continue
			}
			trimmed := strings.TrimSpace(m)
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "UPDATE ") {
				continue
			}
			log.Printf("[migration] %.80q: %v", m, err)
		}
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
		"INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, $4)",
		"Admin", email, string(hash), "admin",
	)
	if err != nil {
		log.Fatalf("Failed to create default admin: %v", err)
	}

	log.Printf("Default admin created (email: %s)", email)
}

func seedDefaultSettings() {
	defaults := map[string]string{
		"site_title":                "SEOå·¡ç«ç®¡çç³»ç»",
		"auto_audit_enabled":        "false",
		"auto_audit_cron":           "0 9 * * *",
		"auto_audit_interval_hours": "0",
		"max_concurrent_audit":      "3",
		"ip_whitelist":              "",
		"spider_ingest_api_key":     "",
		// Comma-separated whitelist of single-use invite codes for the
		// public /api/auth/register form. Admin manages via system
		// settings. Codes are consumed (removed) on successful use.
		"invite_codes_csv":          "",
	}

	for k, v := range defaults {
		DB.Exec("INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT DO NOTHING", k, v)
	}
}

func GetSetting(key string) string {
	var val string
	err := DB.QueryRow("SELECT value FROM settings WHERE key = $1", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

func SetSetting(key, value string) error {
	_, err := DB.Exec("INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value", key, value)
	return err
}
