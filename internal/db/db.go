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
		// GEO 关键词库 — 每用户独立，不是共享资源。新增 user_id 列，
		// 旧行 user_id=0 作为"orphan 管理员专属"处理。配合 handler
		// 里按 user_id 过滤，避免 admin 和 member 互相看到 / 删除。
		"DO $$ BEGIN ALTER TABLE geo_title_libs ADD COLUMN user_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE INDEX IF NOT EXISTS idx_geo_title_libs_user ON geo_title_libs(user_id)`,
		// 协同任务 · 组长 / 经理审核环节。新人创建任务时先指定拟派开
		// 发 (pending_assignee_id)，再勾选一个或多个审核人；任务进入
		// pending_review 状态，所有审核人同意之后系统自动推给开发。
		"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN pending_assignee_id INTEGER DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN pending_assign_note TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		`CREATE TABLE IF NOT EXISTS coord_task_reviewers (
			id            SERIAL PRIMARY KEY,
			task_id       INTEGER NOT NULL REFERENCES coordination_tasks(id) ON DELETE CASCADE,
			reviewer_id   INTEGER NOT NULL REFERENCES users(id),
			decision      TEXT NOT NULL DEFAULT 'pending',
			decision_note TEXT DEFAULT '',
			decided_at    TIMESTAMPTZ,
			created_at    TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(task_id, reviewer_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_reviewers_task ON coord_task_reviewers(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_coord_reviewers_reviewer ON coord_task_reviewers(reviewer_id, decision)`,
		// 数据平台站点隐藏 — 用户在 GSC / GA4 / Bing / Yandex 总览页点
		// × 隐藏不想看的站点，只影响当前用户的视图。site_key 对不同平
		// 台含义不同：GSC 是 site_url，GA4 是 property_id，Bing 是
		// site_url，Yandex 是 host_id。
		`CREATE TABLE IF NOT EXISTS user_platform_hides (
			id         SERIAL PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			platform   TEXT NOT NULL,
			site_key   TEXT NOT NULL,
			hidden_at  TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(user_id, platform, site_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_platform_hides ON user_platform_hides(user_id, platform)`,

		// ─── Phase 2: Platform Data Deep Fields (2026-04) ────────────────
		// ALTER sites — add SEO metadata columns
		"DO $$ BEGIN ALTER TABLE sites ADD COLUMN brand_regex TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE sites ADD COLUMN site_language TEXT DEFAULT 'zh'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE sites ADD COLUMN primary_country TEXT DEFAULT 'chn'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE sites ADD COLUMN site_type TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		// ALTER gsc_data
		"DO $$ BEGIN ALTER TABLE gsc_data ADD COLUMN data_sampled BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		// ALTER bing_data
		"DO $$ BEGIN ALTER TABLE bing_data ADD COLUMN url_submission_quota_daily INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE bing_data ADD COLUMN url_submission_quota_monthly INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE bing_data ADD COLUMN blocked_urls_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE bing_data ADD COLUMN deep_link_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE bing_data ADD COLUMN inbound_links_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		// ALTER yandex_data
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN sqi INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN searchable_pages_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN excluded_pages_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN avg_show_position DOUBLE PRECISION DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN avg_click_position DOUBLE PRECISION DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN unique_users INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN site_problems_json JSONB DEFAULT '{}'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN recrawl_quota_daily INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
		"DO $$ BEGIN ALTER TABLE yandex_data ADD COLUMN recrawl_quota_used INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

		// ─── GSC Deep Tables ─────────────────────────────────────────────
		// 1. GSC query × page × device × country × date (fact table)
		`CREATE TABLE IF NOT EXISTS gsc_query_page_daily (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date              DATE NOT NULL,
			query             TEXT NOT NULL,
			page_url          TEXT NOT NULL,
			country           TEXT DEFAULT 'all',
			device            TEXT DEFAULT 'all',
			search_type       TEXT DEFAULT 'web',
			clicks            INT DEFAULT 0,
			impressions       INT DEFAULT 0,
			ctr               DOUBLE PRECISION DEFAULT 0,
			position          DOUBLE PRECISION DEFAULT 0,
			created_at        TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(site_id, date, query, page_url, country, device, search_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gqpd_site_date ON gsc_query_page_daily(site_id, date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_gqpd_site_position ON gsc_query_page_daily(site_id, position)`,
		`CREATE INDEX IF NOT EXISTS idx_gqpd_query ON gsc_query_page_daily(site_id, query)`,
		`CREATE INDEX IF NOT EXISTS idx_gqpd_page ON gsc_query_page_daily(site_id, page_url)`,
		`CREATE INDEX IF NOT EXISTS idx_gqpd_clicks ON gsc_query_page_daily(site_id, date, clicks DESC)`,

		// 2. GSC rank buckets daily (aggregated from gsc_query_page_daily)
		`CREATE TABLE IF NOT EXISTS gsc_rank_buckets_daily (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date              DATE NOT NULL,
			bucket            TEXT NOT NULL,
			search_type       TEXT DEFAULT 'web',
			country           TEXT DEFAULT 'all',
			device            TEXT DEFAULT 'all',
			query_count       INT DEFAULT 0,
			page_count        INT DEFAULT 0,
			query_page_count  INT DEFAULT 0,
			total_clicks      INT DEFAULT 0,
			total_impressions INT DEFAULT 0,
			avg_ctr           DOUBLE PRECISION DEFAULT 0,
			avg_position      DOUBLE PRECISION DEFAULT 0,
			top_queries_json  JSONB DEFAULT '[]',
			top_pages_json    JSONB DEFAULT '[]',
			UNIQUE(site_id, date, bucket, search_type, country, device)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_grbd_site_date ON gsc_rank_buckets_daily(site_id, date DESC)`,

		// 3. GSC URL Inspection
		`CREATE TABLE IF NOT EXISTS gsc_url_inspection (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			url               TEXT NOT NULL,
			inspected_at      TIMESTAMPTZ DEFAULT NOW(),
			verdict           TEXT DEFAULT '',
			coverage_state    TEXT DEFAULT '',
			robots_txt_state  TEXT DEFAULT '',
			indexing_state    TEXT DEFAULT '',
			page_fetch_state  TEXT DEFAULT '',
			last_crawl_time   TIMESTAMPTZ,
			google_canonical  TEXT DEFAULT '',
			user_canonical    TEXT DEFAULT '',
			crawled_as        TEXT DEFAULT '',
			mobile_usable     BOOLEAN DEFAULT TRUE,
			rich_results_json JSONB DEFAULT '[]',
			referring_urls_json JSONB DEFAULT '[]',
			sitemap_urls_json JSONB DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gui_site_url ON gsc_url_inspection(site_id, url)`,
		`CREATE INDEX IF NOT EXISTS idx_gui_coverage ON gsc_url_inspection(site_id, coverage_state)`,

		// 4. GSC Sitemap status
		`CREATE TABLE IF NOT EXISTS gsc_sitemaps (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			sitemap_path      TEXT NOT NULL,
			snapshot_date     DATE NOT NULL,
			is_sitemaps_index BOOLEAN DEFAULT FALSE,
			is_pending        BOOLEAN DEFAULT FALSE,
			type              TEXT DEFAULT '',
			last_submitted    TIMESTAMPTZ,
			last_downloaded   TIMESTAMPTZ,
			errors            INT DEFAULT 0,
			warnings          INT DEFAULT 0,
			contents_json     JSONB DEFAULT '[]',
			UNIQUE(site_id, sitemap_path, snapshot_date)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gsm_site ON gsc_sitemaps(site_id, snapshot_date DESC)`,

		// 5. GSC searchAppearance daily
		`CREATE TABLE IF NOT EXISTS gsc_appearance_daily (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date              DATE NOT NULL,
			appearance        TEXT NOT NULL,
			clicks            INT DEFAULT 0,
			impressions       INT DEFAULT 0,
			ctr               DOUBLE PRECISION DEFAULT 0,
			position          DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, appearance)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gad_site_date ON gsc_appearance_daily(site_id, date DESC)`,
		"DO $$ BEGIN ALTER TABLE gsc_appearance_daily ADD COLUMN top_queries_json JSONB DEFAULT '[]'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

		// ─── Bing Deep Tables ────────────────────────────────────────────
		// 6. Bing query × page daily
		`CREATE TABLE IF NOT EXISTS bing_query_page_daily (
			id                      BIGSERIAL PRIMARY KEY,
			site_id                 INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date                    DATE NOT NULL,
			query                   TEXT NOT NULL,
			page_url                TEXT NOT NULL,
			clicks                  INT DEFAULT 0,
			impressions             INT DEFAULT 0,
			avg_impression_position DOUBLE PRECISION DEFAULT 0,
			avg_click_position      DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, query, page_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bqpd_site_date ON bing_query_page_daily(site_id, date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_bqpd_query ON bing_query_page_daily(site_id, query)`,
		`CREATE INDEX IF NOT EXISTS idx_bqpd_page ON bing_query_page_daily(site_id, page_url)`,

		// 7. Bing query daily (aggregated)
		`CREATE TABLE IF NOT EXISTS bing_query_daily (
			id                      BIGSERIAL PRIMARY KEY,
			site_id                 INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date                    DATE NOT NULL,
			query                   TEXT NOT NULL,
			clicks                  INT DEFAULT 0,
			impressions             INT DEFAULT 0,
			ctr                     DOUBLE PRECISION DEFAULT 0,
			avg_impression_position DOUBLE PRECISION DEFAULT 0,
			avg_click_position      DOUBLE PRECISION DEFAULT 0,
			page_count              INT DEFAULT 0,
			UNIQUE(site_id, date, query)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bqd_site_date ON bing_query_daily(site_id, date DESC)`,

		// 8. Bing page daily
		`CREATE TABLE IF NOT EXISTS bing_page_daily (
			id                      BIGSERIAL PRIMARY KEY,
			site_id                 INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date                    DATE NOT NULL,
			page_url                TEXT NOT NULL,
			clicks                  INT DEFAULT 0,
			impressions             INT DEFAULT 0,
			ctr                     DOUBLE PRECISION DEFAULT 0,
			avg_impression_position DOUBLE PRECISION DEFAULT 0,
			avg_click_position      DOUBLE PRECISION DEFAULT 0,
			query_count             INT DEFAULT 0,
			UNIQUE(site_id, date, page_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bpd_site_date ON bing_page_daily(site_id, date DESC)`,

		// 9. Bing inbound links
		`CREATE TABLE IF NOT EXISTS bing_inbound_links (
			id          BIGSERIAL PRIMARY KEY,
			site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			target_page TEXT DEFAULT '',
			source_url  TEXT DEFAULT '',
			anchor_text TEXT DEFAULT '',
			first_seen  DATE,
			last_seen   DATE,
			UNIQUE(site_id, target_page, source_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bil_site ON bing_inbound_links(site_id)`,

		// 10. Bing URL status
		`CREATE TABLE IF NOT EXISTS bing_url_status (
			id              BIGSERIAL PRIMARY KEY,
			site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			url             TEXT NOT NULL,
			checked_at      TIMESTAMPTZ,
			is_indexed      BOOLEAN DEFAULT FALSE,
			last_crawled    TIMESTAMPTZ,
			http_status     INT DEFAULT 0,
			robots_state    TEXT DEFAULT '',
			clicks_30d      INT DEFAULT 0,
			impressions_30d INT DEFAULT 0,
			UNIQUE(site_id, url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bus_site ON bing_url_status(site_id)`,

		// 11. Bing keyword research
		`CREATE TABLE IF NOT EXISTS bing_keyword_research (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			seed_query        TEXT NOT NULL,
			country           TEXT DEFAULT '',
			language          TEXT DEFAULT '',
			week_start        DATE NOT NULL,
			keyword           TEXT NOT NULL,
			impressions       BIGINT DEFAULT 0,
			broad_impressions BIGINT DEFAULT 0,
			is_related        BOOLEAN DEFAULT FALSE,
			UNIQUE(site_id, seed_query, keyword, week_start)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bkr_site ON bing_keyword_research(site_id, seed_query)`,

		// 12. Bing sitemaps
		`CREATE TABLE IF NOT EXISTS bing_sitemaps (
			id              SERIAL PRIMARY KEY,
			site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			sitemap_url     TEXT NOT NULL,
			last_submitted  TIMESTAMPTZ,
			last_downloaded TIMESTAMPTZ,
			url_count       INT DEFAULT 0,
			status          TEXT DEFAULT '',
			UNIQUE(site_id, sitemap_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bsm_site ON bing_sitemaps(site_id)`,

		// ─── Yandex Deep Tables ──────────────────────────────────────────
		// 13. Yandex query daily
		`CREATE TABLE IF NOT EXISTS yandex_query_daily (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date              DATE NOT NULL,
			query             TEXT NOT NULL,
			query_id          TEXT DEFAULT '',
			device_type       TEXT DEFAULT 'ALL',
			total_shows       INT DEFAULT 0,
			total_clicks      INT DEFAULT 0,
			total_ctr         DOUBLE PRECISION DEFAULT 0,
			avg_show_position DOUBLE PRECISION DEFAULT 0,
			avg_click_position DOUBLE PRECISION DEFAULT 0,
			unique_users      INT DEFAULT 0,
			UNIQUE(site_id, date, query, device_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_yqd_site_date ON yandex_query_daily(site_id, date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_yqd_query ON yandex_query_daily(site_id, query)`,

		// 14. Yandex index events daily
		`CREATE TABLE IF NOT EXISTS yandex_index_events_daily (
			id         BIGSERIAL PRIMARY KEY,
			site_id    INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date       DATE NOT NULL,
			event_type TEXT NOT NULL,
			count      INT DEFAULT 0,
			UNIQUE(site_id, date, event_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_yied_site ON yandex_index_events_daily(site_id, date DESC)`,

		// 15. Yandex HTTP status daily
		`CREATE TABLE IF NOT EXISTS yandex_http_status_daily (
			id          BIGSERIAL PRIMARY KEY,
			site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date        DATE NOT NULL,
			status_code TEXT NOT NULL,
			count       INT DEFAULT 0,
			UNIQUE(site_id, date, status_code)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_yhsd_site ON yandex_http_status_daily(site_id, date DESC)`,

		// 16. Yandex indexed URLs
		`CREATE TABLE IF NOT EXISTS yandex_indexed_urls (
			id          BIGSERIAL PRIMARY KEY,
			site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			url         TEXT NOT NULL,
			status      TEXT DEFAULT '',
			http_code   INT DEFAULT 0,
			access_date TIMESTAMPTZ,
			fetched_at  TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(site_id, url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_yiu_site ON yandex_indexed_urls(site_id)`,

		// 17. Yandex external links
		`CREATE TABLE IF NOT EXISTS yandex_external_links (
			id         BIGSERIAL PRIMARY KEY,
			site_id    INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			target_url TEXT DEFAULT '',
			source_url TEXT DEFAULT '',
			first_seen DATE,
			last_seen  DATE,
			UNIQUE(site_id, target_url, source_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_yel_site ON yandex_external_links(site_id)`,

		// ─── GA4 Deep Tables ─────────────────────────────────────────────
		// 18. GA4 extended daily
		`CREATE TABLE IF NOT EXISTS ga_daily_extended (
			id                       BIGSERIAL PRIMARY KEY,
			site_id                  INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date                     DATE NOT NULL,
			total_users              INT DEFAULT 0,
			active_users             INT DEFAULT 0,
			new_users                INT DEFAULT 0,
			scrolled_users           INT DEFAULT 0,
			sessions                 INT DEFAULT 0,
			engaged_sessions         INT DEFAULT 0,
			sessions_per_user        DOUBLE PRECISION DEFAULT 0,
			average_session_duration DOUBLE PRECISION DEFAULT 0,
			engagement_rate          DOUBLE PRECISION DEFAULT 0,
			bounce_rate              DOUBLE PRECISION DEFAULT 0,
			user_engagement_duration BIGINT DEFAULT 0,
			dau_per_mau              DOUBLE PRECISION DEFAULT 0,
			wau_per_mau              DOUBLE PRECISION DEFAULT 0,
			dau_per_wau              DOUBLE PRECISION DEFAULT 0,
			screen_page_views        INT DEFAULT 0,
			page_views_per_session   DOUBLE PRECISION DEFAULT 0,
			page_views_per_user      DOUBLE PRECISION DEFAULT 0,
			event_count              INT DEFAULT 0,
			event_count_per_user     DOUBLE PRECISION DEFAULT 0,
			events_per_session       DOUBLE PRECISION DEFAULT 0,
			conversions              INT DEFAULT 0,
			total_revenue            DOUBLE PRECISION DEFAULT 0,
			purchase_revenue         DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gde_site ON ga_daily_extended(site_id, date DESC)`,

		// 19. GA4 landing pages daily
		`CREATE TABLE IF NOT EXISTS ga_landing_pages_daily (
			id                       BIGSERIAL PRIMARY KEY,
			site_id                  INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date                     DATE NOT NULL,
			landing_page             TEXT NOT NULL,
			sessions                 INT DEFAULT 0,
			users                    INT DEFAULT 0,
			new_users                INT DEFAULT 0,
			engaged_sessions         INT DEFAULT 0,
			engagement_rate          DOUBLE PRECISION DEFAULT 0,
			bounce_rate              DOUBLE PRECISION DEFAULT 0,
			average_session_duration DOUBLE PRECISION DEFAULT 0,
			screen_page_views        INT DEFAULT 0,
			conversions              INT DEFAULT 0,
			total_revenue            DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, landing_page)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_glpd_site ON ga_landing_pages_daily(site_id, date DESC)`,

		// 20. GA4 source medium daily
		`CREATE TABLE IF NOT EXISTS ga_source_medium_daily (
			id              BIGSERIAL PRIMARY KEY,
			site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date            DATE NOT NULL,
			source          TEXT DEFAULT '',
			medium          TEXT DEFAULT '',
			channel_group   TEXT DEFAULT '',
			campaign        TEXT DEFAULT '',
			sessions        INT DEFAULT 0,
			users           INT DEFAULT 0,
			new_users       INT DEFAULT 0,
			engaged_sessions INT DEFAULT 0,
			engagement_rate DOUBLE PRECISION DEFAULT 0,
			conversions     INT DEFAULT 0,
			total_revenue   DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, source, medium, campaign)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gsmd_site ON ga_source_medium_daily(site_id, date DESC)`,

		// 21. GA4 events daily
		`CREATE TABLE IF NOT EXISTS ga_events_daily (
			id                   BIGSERIAL PRIMARY KEY,
			site_id              INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date                 DATE NOT NULL,
			event_name           TEXT NOT NULL,
			event_count          INT DEFAULT 0,
			users_with_event     INT DEFAULT 0,
			event_value_sum      DOUBLE PRECISION DEFAULT 0,
			event_count_per_user DOUBLE PRECISION DEFAULT 0,
			is_conversion        BOOLEAN DEFAULT FALSE,
			UNIQUE(site_id, date, event_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ged_site ON ga_events_daily(site_id, date DESC)`,

		// 22. GA4 tech daily (device / os / browser)
		`CREATE TABLE IF NOT EXISTS ga_tech_daily (
			id              BIGSERIAL PRIMARY KEY,
			site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date            DATE NOT NULL,
			dimension_type  TEXT NOT NULL,
			dimension_value TEXT NOT NULL,
			sessions        INT DEFAULT 0,
			users           INT DEFAULT 0,
			bounce_rate     DOUBLE PRECISION DEFAULT 0,
			engagement_rate DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, dimension_type, dimension_value)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gtd_site ON ga_tech_daily(site_id, date DESC)`,

		// 23. GA4 geo daily
		`CREATE TABLE IF NOT EXISTS ga_geo_daily (
			id               BIGSERIAL PRIMARY KEY,
			site_id          INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date             DATE NOT NULL,
			country          TEXT DEFAULT '',
			region           TEXT DEFAULT '',
			city             TEXT DEFAULT '',
			sessions         INT DEFAULT 0,
			users            INT DEFAULT 0,
			new_users        INT DEFAULT 0,
			engaged_sessions INT DEFAULT 0,
			engagement_rate  DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, country, region, city)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ggd_site ON ga_geo_daily(site_id, date DESC)`,

		// 24. GA4 hourly distribution
		`CREATE TABLE IF NOT EXISTS ga_hourly (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date              DATE NOT NULL,
			hour              INT NOT NULL CHECK (hour BETWEEN 0 AND 23),
			sessions          INT DEFAULT 0,
			users             INT DEFAULT 0,
			screen_page_views INT DEFAULT 0,
			UNIQUE(site_id, date, hour)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gah_site ON ga_hourly(site_id, date DESC)`,

		// 25. GA4 scroll depth daily
		`CREATE TABLE IF NOT EXISTS ga_scroll_depth_daily (
			id               BIGSERIAL PRIMARY KEY,
			site_id          INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date             DATE NOT NULL,
			page_path        TEXT DEFAULT '',
			percent_scrolled INT DEFAULT 0,
			users            INT DEFAULT 0,
			event_count      INT DEFAULT 0,
			UNIQUE(site_id, date, page_path, percent_scrolled)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gsdd_site ON ga_scroll_depth_daily(site_id, date DESC)`,

		// 26. GA4 outbound clicks daily
		`CREATE TABLE IF NOT EXISTS ga_outbound_clicks_daily (
			id          BIGSERIAL PRIMARY KEY,
			site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date        DATE NOT NULL,
			link_domain TEXT DEFAULT '',
			link_url    TEXT DEFAULT '',
			event_count INT DEFAULT 0,
			users       INT DEFAULT 0,
			UNIQUE(site_id, date, link_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gocd_site ON ga_outbound_clicks_daily(site_id, date DESC)`,

		// 27. GA4 conversions daily
		`CREATE TABLE IF NOT EXISTS ga_conversions_daily (
			id            BIGSERIAL PRIMARY KEY,
			site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			date          DATE NOT NULL,
			event_name    TEXT NOT NULL,
			source        TEXT DEFAULT '',
			medium        TEXT DEFAULT '',
			conversions   INT DEFAULT 0,
			total_revenue DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, date, event_name, source, medium)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gcd_site ON ga_conversions_daily(site_id, date DESC)`,

		// 27b. GA4 day of week distribution
		`CREATE TABLE IF NOT EXISTS ga_day_of_week (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			year_week         TEXT NOT NULL,
			day_of_week       INT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
			sessions          INT DEFAULT 0,
			users             INT DEFAULT 0,
			screen_page_views INT DEFAULT 0,
			conversions       INT DEFAULT 0,
			UNIQUE(site_id, year_week, day_of_week)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gadow_site ON ga_day_of_week(site_id, year_week DESC)`,

		// ─── Derived / Enrichment Tables ─────────────────────────────────
		// 28. CTR baseline (global)
		`CREATE TABLE IF NOT EXISTS ctr_baseline (
			position   INT PRIMARY KEY,
			ctr        DOUBLE PRECISION NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// 29. Query enrichment (keyword tagging)
		`CREATE TABLE IF NOT EXISTS query_enrichment (
			site_id        INT NOT NULL,
			query          TEXT NOT NULL,
			platform       TEXT NOT NULL DEFAULT 'gsc',
			brand_category TEXT DEFAULT 'unknown',
			intent         TEXT DEFAULT 'unknown',
			length_cat     TEXT DEFAULT 'body',
			word_count     INT DEFAULT 0,
			char_count     INT DEFAULT 0,
			is_question    BOOLEAN DEFAULT FALSE,
			is_comparison  BOOLEAN DEFAULT FALSE,
			first_seen_date DATE,
			last_seen_date  DATE,
			updated_at     TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (site_id, query, platform)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_qe_brand ON query_enrichment(site_id, brand_category)`,
		`CREATE INDEX IF NOT EXISTS idx_qe_intent ON query_enrichment(site_id, intent)`,

		// 30. GSC opportunities weekly
		`CREATE TABLE IF NOT EXISTS gsc_opportunities_weekly (
			id                    BIGSERIAL PRIMARY KEY,
			site_id               INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			week_start            DATE NOT NULL,
			query                 TEXT NOT NULL,
			page_url              TEXT DEFAULT '',
			impressions           INT DEFAULT 0,
			clicks                INT DEFAULT 0,
			actual_ctr            DOUBLE PRECISION DEFAULT 0,
			baseline_ctr          DOUBLE PRECISION DEFAULT 0,
			ctr_gap_ratio         DOUBLE PRECISION DEFAULT 0,
			position              DOUBLE PRECISION DEFAULT 0,
			opportunity_score     DOUBLE PRECISION DEFAULT 0,
			estimated_extra_clicks INT DEFAULT 0,
			opportunity_type      TEXT DEFAULT '',
			UNIQUE(site_id, week_start, query, page_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gow_site ON gsc_opportunities_weekly(site_id, week_start DESC)`,

		// 31. GSC cannibalization monthly
		`CREATE TABLE IF NOT EXISTS gsc_cannibalization_monthly (
			id                BIGSERIAL PRIMARY KEY,
			site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			month             TEXT NOT NULL,
			query             TEXT NOT NULL,
			page_count        INT DEFAULT 0,
			competing_pages   JSONB DEFAULT '[]',
			severity          TEXT DEFAULT 'low',
			total_impressions INT DEFAULT 0,
			total_clicks      INT DEFAULT 0,
			UNIQUE(site_id, month, query)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gcm_site ON gsc_cannibalization_monthly(site_id, month DESC)`,

		// 32. GSC query diff weekly (lost / new / risen / fallen)
		`CREATE TABLE IF NOT EXISTS gsc_query_diff_weekly (
			id             BIGSERIAL PRIMARY KEY,
			site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			week_start     DATE NOT NULL,
			diff_type      TEXT NOT NULL,
			query          TEXT NOT NULL,
			page_url       TEXT DEFAULT '',
			prev_clicks    INT DEFAULT 0,
			curr_clicks    INT DEFAULT 0,
			click_delta    INT DEFAULT 0,
			prev_position  DOUBLE PRECISION DEFAULT 0,
			curr_position  DOUBLE PRECISION DEFAULT 0,
			position_delta DOUBLE PRECISION DEFAULT 0,
			UNIQUE(site_id, week_start, diff_type, query, page_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gqdw_site ON gsc_query_diff_weekly(site_id, week_start DESC)`,

		// Seed CTR baseline data
		`INSERT INTO ctr_baseline (position, ctr) VALUES
			(1, 0.27), (2, 0.15), (3, 0.10), (4, 0.07), (5, 0.06),
			(6, 0.050), (7, 0.042), (8, 0.036), (9, 0.031), (10, 0.028),
			(11, 0.024), (12, 0.022), (13, 0.020), (14, 0.018), (15, 0.017),
			(16, 0.016), (17, 0.015), (18, 0.014), (19, 0.013), (20, 0.012),
			(21, 0.011), (22, 0.011), (23, 0.010), (24, 0.010), (25, 0.009),
			(26, 0.009), (27, 0.008), (28, 0.008), (29, 0.008), (30, 0.007),
			(31, 0.007), (32, 0.007), (33, 0.006), (34, 0.006), (35, 0.006),
			(36, 0.006), (37, 0.006), (38, 0.005), (39, 0.005), (40, 0.005),
			(41, 0.005), (42, 0.005), (43, 0.005), (44, 0.005), (45, 0.005),
			(46, 0.005), (47, 0.005), (48, 0.005), (49, 0.005), (50, 0.005),
			(51, 0.005), (52, 0.004), (53, 0.004), (54, 0.004), (55, 0.004),
			(56, 0.004), (57, 0.004), (58, 0.004), (59, 0.004), (60, 0.004),
			(61, 0.003), (62, 0.003), (63, 0.003), (64, 0.003), (65, 0.003),
			(66, 0.003), (67, 0.003), (68, 0.003), (69, 0.003), (70, 0.003),
			(71, 0.003), (72, 0.003), (73, 0.003), (74, 0.003), (75, 0.003),
			(76, 0.002), (77, 0.002), (78, 0.002), (79, 0.002), (80, 0.002),
			(81, 0.002), (82, 0.002), (83, 0.002), (84, 0.002), (85, 0.002),
			(86, 0.002), (87, 0.002), (88, 0.002), (89, 0.002), (90, 0.002),
			(91, 0.002), (92, 0.002), (93, 0.002), (94, 0.002), (95, 0.002),
			(96, 0.002), (97, 0.002), (98, 0.002), (99, 0.002), (100, 0.002)
		ON CONFLICT DO NOTHING`,
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
