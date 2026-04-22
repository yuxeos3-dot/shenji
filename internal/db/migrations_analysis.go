package db

// AnalysisMigrations contains all tables for the SEO Deep Analysis system.
// Sprint 1-5: 71 new tables + ALTER existing tables.
var AnalysisMigrations = []string{

	// ═══════════════════════════════════════════════════════════════
	// A · 规则引擎层 (P0) — 5 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS analysis_rules (
		id              SERIAL PRIMARY KEY,
		rule_id         TEXT UNIQUE NOT NULL,
		version         TEXT NOT NULL DEFAULT 'v1',
		category        TEXT NOT NULL,
		scope           TEXT NOT NULL,
		severity        TEXT NOT NULL DEFAULT 'medium',
		priority_base   INT NOT NULL DEFAULT 50,
		title           TEXT NOT NULL,
		description     TEXT DEFAULT '',
		data_deps_json  JSONB DEFAULT '[]',
		sample_threshold_json JSONB DEFAULT '{}',
		trigger_dsl     TEXT NOT NULL DEFAULT '',
		trigger_sql     TEXT DEFAULT '',
		confidence_formula TEXT DEFAULT '',
		impact_formula  TEXT DEFAULT '',
		suggestion_template TEXT DEFAULT '',
		acceptance_criteria TEXT DEFAULT '',
		dedup_window_days INT DEFAULT 7,
		enabled         BOOLEAN DEFAULT TRUE,
		created_at      TIMESTAMPTZ DEFAULT NOW(),
		updated_at      TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rules_cat ON analysis_rules(category, enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_rules_scope ON analysis_rules(scope, enabled)`,

	`CREATE TABLE IF NOT EXISTS rule_findings (
		id              BIGSERIAL PRIMARY KEY,
		rule_id         TEXT NOT NULL,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		run_id          BIGINT DEFAULT 0,
		scope_type      TEXT DEFAULT 'site',
		target_url      TEXT DEFAULT '',
		target_query    TEXT DEFAULT '',
		severity        TEXT NOT NULL,
		priority_score  DOUBLE PRECISION DEFAULT 0,
		confidence      DOUBLE PRECISION DEFAULT 0,
		impact_estimate DOUBLE PRECISION DEFAULT 0,
		evidence_json   JSONB DEFAULT '{}',
		suggestion_text TEXT DEFAULT '',
		status          TEXT NOT NULL DEFAULT 'open',
		ack_by          INT DEFAULT 0,
		ack_at          TIMESTAMPTZ,
		resolved_by     INT DEFAULT 0,
		resolved_at     TIMESTAMPTZ,
		resolution_note TEXT DEFAULT '',
		coord_task_id   INT DEFAULT 0,
		detected_at     TIMESTAMPTZ DEFAULT NOW(),
		expires_at      TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_findings_site_open ON rule_findings(site_id, status, priority_score DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_findings_rule ON rule_findings(rule_id, detected_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_findings_severity ON rule_findings(site_id, severity, detected_at DESC)`,

	`CREATE TABLE IF NOT EXISTS rule_runs (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		rule_id         TEXT NOT NULL,
		started_at      TIMESTAMPTZ DEFAULT NOW(),
		finished_at     TIMESTAMPTZ,
		status          TEXT DEFAULT 'running',
		rows_scanned    BIGINT DEFAULT 0,
		findings_count  INT DEFAULT 0,
		error_msg       TEXT DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rule_runs_site ON rule_runs(site_id, rule_id, started_at DESC)`,

	`CREATE TABLE IF NOT EXISTS rule_finding_history (
		id           BIGSERIAL PRIMARY KEY,
		finding_id   BIGINT NOT NULL,
		event_type   TEXT NOT NULL,
		from_status  TEXT DEFAULT '',
		to_status    TEXT DEFAULT '',
		actor_id     INT DEFAULT 0,
		note         TEXT DEFAULT '',
		created_at   TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_finding_hist ON rule_finding_history(finding_id, created_at)`,

	`CREATE TABLE IF NOT EXISTS rule_suppressions (
		id          SERIAL PRIMARY KEY,
		site_id     INT DEFAULT 0,
		rule_id     TEXT NOT NULL,
		mode        TEXT NOT NULL DEFAULT 'disable',
		override_json JSONB DEFAULT '{}',
		reason      TEXT DEFAULT '',
		created_by  INT NOT NULL DEFAULT 0,
		created_at  TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(site_id, rule_id)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// B · 异常检测 / 基线 (P0) — 4 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS metric_baselines (
		id            BIGSERIAL PRIMARY KEY,
		site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		scope_type    TEXT NOT NULL DEFAULT 'site',
		scope_key     TEXT DEFAULT '',
		metric_name   TEXT NOT NULL,
		platform      TEXT DEFAULT 'all',
		period        TEXT NOT NULL DEFAULT 'daily',
		window_days   INT NOT NULL DEFAULT 28,
		baseline_date DATE NOT NULL,
		mean          DOUBLE PRECISION DEFAULT 0,
		stddev        DOUBLE PRECISION DEFAULT 0,
		median        DOUBLE PRECISION DEFAULT 0,
		p10           DOUBLE PRECISION DEFAULT 0,
		p25           DOUBLE PRECISION DEFAULT 0,
		p75           DOUBLE PRECISION DEFAULT 0,
		p90           DOUBLE PRECISION DEFAULT 0,
		p95           DOUBLE PRECISION DEFAULT 0,
		trend_slope   DOUBLE PRECISION DEFAULT 0,
		seasonality_json JSONB DEFAULT '{}',
		sample_size   INT DEFAULT 0,
		UNIQUE(site_id, scope_type, scope_key, metric_name, platform, period, baseline_date)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_baseline_site ON metric_baselines(site_id, metric_name, baseline_date DESC)`,

	`CREATE TABLE IF NOT EXISTS anomaly_events (
		id            BIGSERIAL PRIMARY KEY,
		site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		scope_type    TEXT DEFAULT 'site',
		scope_key     TEXT DEFAULT '',
		metric_name   TEXT NOT NULL,
		platform      TEXT DEFAULT 'all',
		method        TEXT NOT NULL,
		direction     TEXT DEFAULT 'both',
		detected_at   TIMESTAMPTZ DEFAULT NOW(),
		event_date    DATE NOT NULL,
		observed_value DOUBLE PRECISION,
		expected_value DOUBLE PRECISION,
		lower_bound   DOUBLE PRECISION,
		upper_bound   DOUBLE PRECISION,
		deviation_sigma DOUBLE PRECISION,
		severity      TEXT NOT NULL DEFAULT 'medium',
		is_change_point BOOLEAN DEFAULT FALSE,
		explanation_json JSONB DEFAULT '{}',
		correlated_events_json JSONB DEFAULT '[]',
		status        TEXT DEFAULT 'open',
		investigated_by INT DEFAULT 0,
		resolution_note TEXT DEFAULT '',
		finding_id    BIGINT DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_anomaly_site_date ON anomaly_events(site_id, event_date DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_anomaly_open ON anomaly_events(site_id, status, severity)`,

	`CREATE TABLE IF NOT EXISTS dimension_attributions (
		id            BIGSERIAL PRIMARY KEY,
		anomaly_id    BIGINT NOT NULL,
		dimension     TEXT NOT NULL,
		dimension_value TEXT NOT NULL,
		contribution_pct DOUBLE PRECISION DEFAULT 0,
		prev_value    DOUBLE PRECISION DEFAULT 0,
		curr_value    DOUBLE PRECISION DEFAULT 0,
		delta         DOUBLE PRECISION DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_attr_anomaly ON dimension_attributions(anomaly_id, contribution_pct DESC)`,

	`CREATE TABLE IF NOT EXISTS metric_snapshots_daily (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date           DATE NOT NULL,
		metric_name    TEXT NOT NULL,
		platform       TEXT DEFAULT 'all',
		value          DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, date, metric_name, platform)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_snap_site_metric ON metric_snapshots_daily(site_id, metric_name, date DESC)`,

	// ═══════════════════════════════════════════════════════════════
	// C · 站点健康评分 (P0) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS site_health_scores (
		id                   BIGSERIAL PRIMARY KEY,
		site_id              INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                 DATE NOT NULL,
		period               TEXT DEFAULT 'daily',
		total_score          INT DEFAULT 0,
		indexation_score     INT DEFAULT 0,
		ranking_score        INT DEFAULT 0,
		traffic_score        INT DEFAULT 0,
		ctr_score            INT DEFAULT 0,
		technical_score      INT DEFAULT 0,
		content_score        INT DEFAULT 0,
		backlink_score       INT DEFAULT 0,
		ux_score             INT DEFAULT 0,
		security_score       INT DEFAULT 0,
		stability_score      INT DEFAULT 0,
		brand_score          INT DEFAULT 0,
		eeat_score           INT DEFAULT 0,
		aio_visibility_score INT DEFAULT 0,
		prev_period_score    INT DEFAULT 0,
		delta                INT DEFAULT 0,
		trend_7d             DOUBLE PRECISION DEFAULT 0,
		trend_28d            DOUBLE PRECISION DEFAULT 0,
		grade                TEXT DEFAULT '',
		warning_count        INT DEFAULT 0,
		critical_count       INT DEFAULT 0,
		sub_scores_json      JSONB DEFAULT '{}',
		computed_at          TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(site_id, date, period)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_health_site ON site_health_scores(site_id, date DESC)`,

	`CREATE TABLE IF NOT EXISTS health_score_components (
		id             BIGSERIAL PRIMARY KEY,
		health_id      BIGINT NOT NULL,
		component_code TEXT NOT NULL,
		component_name TEXT NOT NULL,
		dimension      TEXT NOT NULL,
		weight         DOUBLE PRECISION DEFAULT 0,
		raw_value      DOUBLE PRECISION DEFAULT 0,
		score          DOUBLE PRECISION DEFAULT 0,
		formula        TEXT DEFAULT '',
		evidence_json  JSONB DEFAULT '{}'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_hsc_health ON health_score_components(health_id)`,

	// ═══════════════════════════════════════════════════════════════
	// D · 站点分析报告 (P0) — 3 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS site_analysis_reports (
		id                  BIGSERIAL PRIMARY KEY,
		site_id             INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		report_type         TEXT NOT NULL,
		report_scope        TEXT NOT NULL DEFAULT 'single_site',
		site_group_id       INT DEFAULT 0,
		period_start        DATE NOT NULL,
		period_end          DATE NOT NULL,
		generated_at        TIMESTAMPTZ DEFAULT NOW(),
		generated_by        INT DEFAULT 0,
		version             INT DEFAULT 1,
		status              TEXT DEFAULT 'generating',
		total_score         INT DEFAULT 0,
		score_delta         INT DEFAULT 0,
		critical_findings   INT DEFAULT 0,
		high_findings       INT DEFAULT 0,
		medium_findings     INT DEFAULT 0,
		opportunity_count   INT DEFAULT 0,
		block_summary       JSONB DEFAULT '{}',
		block_metrics       JSONB DEFAULT '{}',
		block_ranking       JSONB DEFAULT '{}',
		block_ctr           JSONB DEFAULT '{}',
		block_indexation    JSONB DEFAULT '{}',
		block_technical     JSONB DEFAULT '{}',
		block_content       JSONB DEFAULT '{}',
		block_backlinks     JSONB DEFAULT '{}',
		block_competitors   JSONB DEFAULT '{}',
		block_strategy      JSONB DEFAULT '{}',
		markdown_text       TEXT DEFAULT '',
		html_text           TEXT DEFAULT '',
		docx_path           TEXT DEFAULT '',
		ai_model            TEXT DEFAULT '',
		ai_tokens_used      INT DEFAULT 0,
		UNIQUE(site_id, report_type, period_start, period_end, version)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sar_site ON site_analysis_reports(site_id, period_end DESC)`,

	`CREATE TABLE IF NOT EXISTS report_templates (
		id             SERIAL PRIMARY KEY,
		code           TEXT UNIQUE NOT NULL,
		name           TEXT NOT NULL,
		scope          TEXT DEFAULT 'single_site',
		blocks_json    JSONB NOT NULL DEFAULT '[]',
		ai_prompt_template TEXT DEFAULT '',
		output_formats TEXT DEFAULT 'md,html,docx',
		enabled        BOOLEAN DEFAULT TRUE,
		created_by     INT DEFAULT 0,
		created_at     TIMESTAMPTZ DEFAULT NOW(),
		updated_at     TIMESTAMPTZ DEFAULT NOW()
	)`,

	`CREATE TABLE IF NOT EXISTS report_subscriptions (
		id              SERIAL PRIMARY KEY,
		user_id         INT NOT NULL,
		site_id         INT DEFAULT 0,
		site_group_id   INT DEFAULT 0,
		template_code   TEXT NOT NULL,
		cron_expr       TEXT NOT NULL,
		delivery        TEXT DEFAULT 'web',
		delivery_target TEXT DEFAULT '',
		enabled         BOOLEAN DEFAULT TRUE,
		last_run_at     TIMESTAMPTZ,
		created_at      TIMESTAMPTZ DEFAULT NOW()
	)`,

	// ═══════════════════════════════════════════════════════════════
	// E · audit_findings flatten (P0) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS audit_findings (
		id                BIGSERIAL PRIMARY KEY,
		task_id           INT NOT NULL,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		chapter           TEXT NOT NULL,
		module            TEXT NOT NULL,
		issue_code        TEXT DEFAULT '',
		title             TEXT NOT NULL,
		severity          TEXT NOT NULL,
		confidence        TEXT DEFAULT 'high',
		page_type         TEXT DEFAULT '',
		page_url          TEXT DEFAULT '',
		element_selector  TEXT DEFAULT '',
		evidence_json     JSONB DEFAULT '{}',
		fix_suggestion    TEXT DEFAULT '',
		fix_type          TEXT DEFAULT '',
		estimated_effort  TEXT DEFAULT '',
		detected_at       TIMESTAMPTZ DEFAULT NOW(),
		coord_task_id     INT DEFAULT 0,
		finding_id        BIGINT DEFAULT 0,
		status            TEXT DEFAULT 'open',
		first_seen_task_id INT DEFAULT 0,
		recurrence_count  INT DEFAULT 1
	)`,
	`CREATE INDEX IF NOT EXISTS idx_af_task ON audit_findings(task_id)`,
	`CREATE INDEX IF NOT EXISTS idx_af_site_sev ON audit_findings(site_id, severity, detected_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_af_module ON audit_findings(site_id, module, detected_at DESC)`,

	`CREATE TABLE IF NOT EXISTS audit_finding_dedup (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		dedup_key       TEXT NOT NULL,
		first_task_id   INT NOT NULL,
		last_task_id    INT NOT NULL,
		first_seen_at   TIMESTAMPTZ,
		last_seen_at    TIMESTAMPTZ,
		occurrences     INT DEFAULT 1,
		current_status  TEXT DEFAULT 'open',
		latest_finding_id BIGINT DEFAULT 0,
		UNIQUE(site_id, dedup_key)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// F · 时间线与事件 (P0) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS timeline_events (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT DEFAULT 0,
		event_category TEXT NOT NULL,
		event_type     TEXT NOT NULL,
		title          TEXT NOT NULL,
		description    TEXT DEFAULT '',
		severity       TEXT DEFAULT 'info',
		impact_scope   TEXT DEFAULT '',
		occurred_at    TIMESTAMPTZ NOT NULL,
		ends_at        TIMESTAMPTZ,
		source_type    TEXT DEFAULT 'manual',
		source_id      BIGINT DEFAULT 0,
		related_incident_id INT DEFAULT 0,
		related_anomaly_id BIGINT DEFAULT 0,
		related_finding_id BIGINT DEFAULT 0,
		extra_json     JSONB DEFAULT '{}',
		created_by     INT DEFAULT 0,
		created_at     TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_timeline_site ON timeline_events(site_id, occurred_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_timeline_category ON timeline_events(event_category, occurred_at DESC)`,

	`CREATE TABLE IF NOT EXISTS event_correlations (
		id            BIGSERIAL PRIMARY KEY,
		event_id      BIGINT NOT NULL,
		anomaly_id    BIGINT DEFAULT 0,
		finding_id    BIGINT DEFAULT 0,
		related_event_id BIGINT DEFAULT 0,
		correlation_type TEXT DEFAULT 'temporal',
		correlation_score DOUBLE PRECISION DEFAULT 0,
		time_lag_hours INT DEFAULT 0,
		method        TEXT DEFAULT '',
		explanation   TEXT DEFAULT '',
		created_at    TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_evcorr_event ON event_correlations(event_id)`,

	// ═══════════════════════════════════════════════════════════════
	// G · AIO / AI 搜索可见性 (P1) — 4 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS aio_observations (
		id                BIGSERIAL PRIMARY KEY,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		query             TEXT NOT NULL,
		observed_at       TIMESTAMPTZ DEFAULT NOW(),
		observer_id       INT DEFAULT 0,
		country           TEXT DEFAULT '',
		language          TEXT DEFAULT '',
		device            TEXT DEFAULT 'desktop',
		search_engine     TEXT DEFAULT 'google',
		has_aio           BOOLEAN DEFAULT FALSE,
		aio_position      INT DEFAULT 0,
		aio_text          TEXT DEFAULT '',
		cited_sources_json JSONB DEFAULT '[]',
		our_domain_cited  BOOLEAN DEFAULT FALSE,
		our_cited_urls_json JSONB DEFAULT '[]',
		screenshot_path   TEXT DEFAULT '',
		notes             TEXT DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_aio_obs_site ON aio_observations(site_id, observed_at DESC)`,

	`CREATE TABLE IF NOT EXISTS ai_crawler_logs (
		id               BIGSERIAL PRIMARY KEY,
		site_id          INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		crawler_name     TEXT NOT NULL,
		crawler_company  TEXT DEFAULT '',
		ip               TEXT DEFAULT '',
		ip_verified      BOOLEAN DEFAULT FALSE,
		user_agent       TEXT DEFAULT '',
		url              TEXT DEFAULT '',
		status_code      INT DEFAULT 0,
		bytes_sent       INT DEFAULT 0,
		visited_at       TIMESTAMPTZ NOT NULL,
		robots_allowed   BOOLEAN DEFAULT TRUE,
		created_at       TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_aicrawl_site_time ON ai_crawler_logs(site_id, visited_at DESC)`,

	`CREATE TABLE IF NOT EXISTS ai_crawler_daily (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date            DATE NOT NULL,
		crawler_name    TEXT NOT NULL,
		total_requests  INT DEFAULT 0,
		unique_urls     INT DEFAULT 0,
		bytes_total     BIGINT DEFAULT 0,
		status_2xx      INT DEFAULT 0,
		status_4xx      INT DEFAULT 0,
		status_5xx      INT DEFAULT 0,
		blocked_by_robots INT DEFAULT 0,
		UNIQUE(site_id, date, crawler_name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_aicd_site ON ai_crawler_daily(site_id, date DESC)`,

	`CREATE TABLE IF NOT EXISTS llms_txt_snapshots (
		id              BIGSERIAL PRIMARY KEY,
		domain          TEXT NOT NULL,
		site_id         INT DEFAULT 0,
		checked_at      TIMESTAMPTZ DEFAULT NOW(),
		exists_flag     BOOLEAN DEFAULT FALSE,
		content         TEXT DEFAULT '',
		size_bytes      INT DEFAULT 0,
		sections_json   JSONB DEFAULT '{}'
	)`,

	// ═══════════════════════════════════════════════════════════════
	// H · 竞品分析 (P1) — 6 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS competitors (
		id            SERIAL PRIMARY KEY,
		site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		domain        TEXT NOT NULL,
		name          TEXT DEFAULT '',
		tier          TEXT DEFAULT 'direct',
		notes         TEXT DEFAULT '',
		monitor_enabled BOOLEAN DEFAULT TRUE,
		created_by    INT DEFAULT 0,
		created_at    TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(site_id, domain)
	)`,

	`CREATE TABLE IF NOT EXISTS competitor_tdk_snapshots (
		id              BIGSERIAL PRIMARY KEY,
		competitor_id   INT NOT NULL,
		url             TEXT NOT NULL,
		snapshot_date   DATE NOT NULL,
		title           TEXT DEFAULT '',
		description     TEXT DEFAULT '',
		h1              TEXT DEFAULT '',
		canonical       TEXT DEFAULT '',
		schema_types_json JSONB DEFAULT '[]',
		word_count      INT DEFAULT 0,
		content_hash    TEXT DEFAULT '',
		changed_from_prev BOOLEAN DEFAULT FALSE,
		UNIQUE(competitor_id, url, snapshot_date)
	)`,

	`CREATE TABLE IF NOT EXISTS competitor_content_snapshots (
		id              BIGSERIAL PRIMARY KEY,
		competitor_id   INT NOT NULL,
		url             TEXT NOT NULL,
		snapshot_date   DATE NOT NULL,
		word_count      INT DEFAULT 0,
		image_count     INT DEFAULT 0,
		heading_structure_json JSONB DEFAULT '[]',
		internal_links_count INT DEFAULT 0,
		external_links_count INT DEFAULT 0,
		content_hash    TEXT DEFAULT '',
		UNIQUE(competitor_id, url, snapshot_date)
	)`,

	`CREATE TABLE IF NOT EXISTS competitor_serp_rankings (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		competitor_id  INT NOT NULL,
		query          TEXT NOT NULL,
		checked_at     TIMESTAMPTZ DEFAULT NOW(),
		engine         TEXT DEFAULT 'google',
		competitor_position INT DEFAULT 0,
		our_position   INT DEFAULT 0,
		position_delta INT DEFAULT 0,
		competitor_url TEXT DEFAULT '',
		our_url        TEXT DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_csr_site ON competitor_serp_rankings(site_id, checked_at DESC)`,

	`CREATE TABLE IF NOT EXISTS competitor_backlink_intersection (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		competitor_id  INT NOT NULL,
		referring_domain TEXT NOT NULL,
		source_url     TEXT DEFAULT '',
		anchor_text    TEXT DEFAULT '',
		links_to_us    BOOLEAN DEFAULT FALSE,
		first_seen     DATE,
		last_seen      DATE
	)`,

	`CREATE TABLE IF NOT EXISTS competitor_differentiators (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		competitor_id   INT DEFAULT 0,
		analyzed_at     TIMESTAMPTZ DEFAULT NOW(),
		category        TEXT NOT NULL,
		gap_type        TEXT NOT NULL,
		title           TEXT NOT NULL,
		description     TEXT DEFAULT '',
		our_data_json   JSONB DEFAULT '{}',
		their_data_json JSONB DEFAULT '{}',
		suggestion      TEXT DEFAULT '',
		priority        INT DEFAULT 50
	)`,

	// ═══════════════════════════════════════════════════════════════
	// I · SERP 真实位置追踪 (P1) — 4 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS tracked_queries (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		query           TEXT NOT NULL,
		target_url      TEXT DEFAULT '',
		priority        INT DEFAULT 50,
		engines         TEXT DEFAULT 'google,bing,yandex',
		country         TEXT DEFAULT '',
		language        TEXT DEFAULT '',
		devices         TEXT DEFAULT 'desktop,mobile',
		check_frequency TEXT DEFAULT 'weekly',
		enabled         BOOLEAN DEFAULT TRUE,
		source          TEXT DEFAULT 'manual',
		tags_json       JSONB DEFAULT '[]',
		last_observed_at TIMESTAMPTZ,
		created_by      INT DEFAULT 0,
		created_at      TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tq_site ON tracked_queries(site_id, enabled)`,

	`CREATE TABLE IF NOT EXISTS serp_position_daily (
		id                     BIGSERIAL PRIMARY KEY,
		site_id                INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                   DATE NOT NULL,
		query                  TEXT NOT NULL,
		engine                 TEXT NOT NULL,
		device                 TEXT DEFAULT 'all',
		country                TEXT DEFAULT 'all',
		avg_position           DOUBLE PRECISION,
		impressions            INT DEFAULT 0,
		clicks                 INT DEFAULT 0,
		ctr                    DOUBLE PRECISION DEFAULT 0,
		top_page_url           TEXT DEFAULT '',
		page_count             INT DEFAULT 0,
		sample_confidence      TEXT DEFAULT 'low',
		UNIQUE(site_id, date, query, engine, device, country)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_spd_site_date ON serp_position_daily(site_id, date DESC)`,

	`CREATE TABLE IF NOT EXISTS serp_observations (
		id                BIGSERIAL PRIMARY KEY,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		query             TEXT NOT NULL,
		observer_id       INT DEFAULT 0,
		observed_at       TIMESTAMPTZ DEFAULT NOW(),
		engine            TEXT NOT NULL,
		country           TEXT DEFAULT '',
		device            TEXT DEFAULT 'desktop',
		our_position      INT DEFAULT 0,
		our_url           TEXT DEFAULT '',
		our_title_shown   TEXT DEFAULT '',
		our_desc_shown    TEXT DEFAULT '',
		our_rich_results_json JSONB DEFAULT '[]',
		top10_json        JSONB DEFAULT '[]',
		serp_features_json JSONB DEFAULT '[]',
		paid_ads_count    INT DEFAULT 0,
		screenshot_path   TEXT DEFAULT '',
		notes             TEXT DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_so_site ON serp_observations(site_id, observed_at DESC)`,

	`CREATE TABLE IF NOT EXISTS serp_features_daily (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date           DATE NOT NULL,
		engine         TEXT NOT NULL,
		feature        TEXT NOT NULL,
		observation_count INT DEFAULT 0,
		our_presence   INT DEFAULT 0,
		data_source    TEXT DEFAULT 'observation',
		UNIQUE(site_id, date, engine, feature, data_source)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// J · Web Vitals & 性能 (P1) — 4 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS web_vitals_daily (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date            DATE NOT NULL,
		url             TEXT NOT NULL,
		device          TEXT DEFAULT 'mobile',
		data_source     TEXT DEFAULT 'rum',
		lcp_p75_ms      INT DEFAULT 0,
		inp_p75_ms      INT DEFAULT 0,
		cls_p75         DOUBLE PRECISION DEFAULT 0,
		fcp_p75_ms      INT DEFAULT 0,
		ttfb_p75_ms     INT DEFAULT 0,
		sample_size     BIGINT DEFAULT 0,
		cwv_pass        BOOLEAN DEFAULT FALSE,
		UNIQUE(site_id, date, url, device, data_source)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_wvd_site_date ON web_vitals_daily(site_id, date DESC)`,

	`CREATE TABLE IF NOT EXISTS rum_beacons (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url             TEXT NOT NULL,
		metric_name     TEXT NOT NULL,
		metric_value    DOUBLE PRECISION NOT NULL,
		metric_rating   TEXT DEFAULT '',
		device_type     TEXT DEFAULT '',
		connection_type TEXT DEFAULT '',
		country         TEXT DEFAULT '',
		session_id      TEXT DEFAULT '',
		element_selector TEXT DEFAULT '',
		entries_json    JSONB DEFAULT '[]',
		reported_at     TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rum_site_time ON rum_beacons(site_id, reported_at DESC)`,

	`CREATE TABLE IF NOT EXISTS lighthouse_audits (
		id                 BIGSERIAL PRIMARY KEY,
		site_id            INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url                TEXT NOT NULL,
		audited_at         TIMESTAMPTZ DEFAULT NOW(),
		device             TEXT DEFAULT 'mobile',
		performance_score  INT DEFAULT 0,
		accessibility_score INT DEFAULT 0,
		best_practices_score INT DEFAULT 0,
		seo_score          INT DEFAULT 0,
		lcp_ms             INT DEFAULT 0,
		fcp_ms             INT DEFAULT 0,
		tbt_ms             INT DEFAULT 0,
		cls                DOUBLE PRECISION DEFAULT 0,
		si_ms              INT DEFAULT 0,
		opportunities_json JSONB DEFAULT '[]',
		diagnostics_json   JSONB DEFAULT '[]',
		raw_report_path    TEXT DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_la_site ON lighthouse_audits(site_id, audited_at DESC)`,

	`CREATE TABLE IF NOT EXISTS resource_timing_daily (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date           DATE NOT NULL,
		page_url       TEXT NOT NULL,
		resource_type  TEXT NOT NULL,
		request_count  INT DEFAULT 0,
		total_size_kb  INT DEFAULT 0,
		avg_duration_ms INT DEFAULT 0,
		p75_duration_ms INT DEFAULT 0,
		third_party_pct DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, date, page_url, resource_type)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// K · 反链深度 (P2) — 6 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS backlinks_all (
		id               BIGSERIAL PRIMARY KEY,
		site_id          INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		source_url       TEXT NOT NULL,
		source_domain    TEXT NOT NULL,
		target_url       TEXT NOT NULL,
		anchor_text      TEXT DEFAULT '',
		anchor_type      TEXT DEFAULT '',
		rel_attr         TEXT DEFAULT 'dofollow',
		data_source      TEXT NOT NULL,
		first_seen_at    TIMESTAMPTZ,
		last_seen_at     TIMESTAMPTZ,
		lost_at          TIMESTAMPTZ,
		our_authority_hint INT DEFAULT 0,
		source_http_status INT DEFAULT 0,
		source_tld       TEXT DEFAULT '',
		is_suspicious    BOOLEAN DEFAULT FALSE,
		suspicious_reasons_json JSONB DEFAULT '[]',
		UNIQUE(site_id, source_url, target_url)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_bkl_site ON backlinks_all(site_id, last_seen_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_bkl_domain ON backlinks_all(site_id, source_domain)`,

	`CREATE TABLE IF NOT EXISTS referring_domains (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		domain          TEXT NOT NULL,
		first_seen      DATE,
		last_seen       DATE,
		backlink_count  INT DEFAULT 0,
		dofollow_count  INT DEFAULT 0,
		authority_hint  INT DEFAULT 0,
		country         TEXT DEFAULT '',
		tld             TEXT DEFAULT '',
		is_toxic        BOOLEAN DEFAULT FALSE,
		is_relevant     BOOLEAN DEFAULT TRUE,
		UNIQUE(site_id, domain)
	)`,

	`CREATE TABLE IF NOT EXISTS backlink_profile_daily (
		id                    BIGSERIAL PRIMARY KEY,
		site_id               INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                  DATE NOT NULL,
		total_backlinks       BIGINT DEFAULT 0,
		total_referring_domains INT DEFAULT 0,
		dofollow_count        BIGINT DEFAULT 0,
		nofollow_count        BIGINT DEFAULT 0,
		new_links_24h         INT DEFAULT 0,
		lost_links_24h        INT DEFAULT 0,
		new_domains_24h       INT DEFAULT 0,
		lost_domains_24h      INT DEFAULT 0,
		suspicious_domain_count INT DEFAULT 0,
		tld_distribution_json JSONB DEFAULT '{}',
		UNIQUE(site_id, date)
	)`,

	`CREATE TABLE IF NOT EXISTS anchor_distribution_daily (
		id          BIGSERIAL PRIMARY KEY,
		site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date        DATE NOT NULL,
		anchor_text TEXT NOT NULL,
		anchor_type TEXT DEFAULT '',
		link_count  INT DEFAULT 0,
		domain_count INT DEFAULT 0,
		UNIQUE(site_id, date, anchor_text)
	)`,

	`CREATE TABLE IF NOT EXISTS suspicious_links (
		id            BIGSERIAL PRIMARY KEY,
		site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		backlink_id   BIGINT NOT NULL,
		risk_score    INT DEFAULT 0,
		signals_json  JSONB DEFAULT '[]',
		reviewer_action TEXT DEFAULT '',
		reviewed_by   INT DEFAULT 0,
		reviewed_at   TIMESTAMPTZ,
		created_at    TIMESTAMPTZ DEFAULT NOW()
	)`,

	`CREATE TABLE IF NOT EXISTS disavow_records (
		id            BIGSERIAL PRIMARY KEY,
		site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		scope         TEXT NOT NULL,
		target        TEXT NOT NULL,
		reason        TEXT DEFAULT '',
		submitted_to  TEXT DEFAULT 'google',
		submitted_at  TIMESTAMPTZ,
		status        TEXT DEFAULT 'pending',
		submitted_by  INT DEFAULT 0,
		file_path     TEXT DEFAULT '',
		UNIQUE(site_id, scope, target)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// L · 内链图谱 (P2) — 3 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS internal_link_edges (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		source_url     TEXT NOT NULL,
		target_url     TEXT NOT NULL,
		anchor_text    TEXT DEFAULT '',
		rel_attr       TEXT DEFAULT 'dofollow',
		position_on_page TEXT DEFAULT '',
		discovered_at  TIMESTAMPTZ DEFAULT NOW(),
		last_seen_at   TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ile_src ON internal_link_edges(site_id, source_url)`,
	`CREATE INDEX IF NOT EXISTS idx_ile_tgt ON internal_link_edges(site_id, target_url)`,

	`CREATE TABLE IF NOT EXISTS internal_link_stats (
		id                  BIGSERIAL PRIMARY KEY,
		site_id             INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url                 TEXT NOT NULL,
		computed_date       DATE NOT NULL,
		inbound_links       INT DEFAULT 0,
		outbound_links      INT DEFAULT 0,
		depth_from_home     INT DEFAULT 0,
		pagerank            DOUBLE PRECISION DEFAULT 0,
		is_orphan           BOOLEAN DEFAULT FALSE,
		in_sitemap          BOOLEAN DEFAULT FALSE,
		in_nav              BOOLEAN DEFAULT FALSE,
		UNIQUE(site_id, url, computed_date)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ils_site ON internal_link_stats(site_id, computed_date DESC)`,

	`CREATE TABLE IF NOT EXISTS site_link_graph_metrics (
		id                    BIGSERIAL PRIMARY KEY,
		site_id               INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		computed_date         DATE NOT NULL,
		total_urls            INT DEFAULT 0,
		total_edges           INT DEFAULT 0,
		avg_inbound           DOUBLE PRECISION DEFAULT 0,
		orphan_count          INT DEFAULT 0,
		max_depth             INT DEFAULT 0,
		avg_depth             DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, computed_date)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// M · 页面内容快照 / 重复内容 (P2) — 4 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS page_content_snapshots (
		id                    BIGSERIAL PRIMARY KEY,
		site_id               INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url                   TEXT NOT NULL,
		snapshot_date         DATE NOT NULL,
		status_code           INT DEFAULT 0,
		title                 TEXT DEFAULT '',
		meta_description      TEXT DEFAULT '',
		h1                    TEXT DEFAULT '',
		h1_count              INT DEFAULT 0,
		canonical             TEXT DEFAULT '',
		robots_meta           TEXT DEFAULT '',
		word_count            INT DEFAULT 0,
		image_count           INT DEFAULT 0,
		images_with_alt_count INT DEFAULT 0,
		external_link_count   INT DEFAULT 0,
		internal_link_count   INT DEFAULT 0,
		schema_types_json     JSONB DEFAULT '[]',
		content_hash          TEXT DEFAULT '',
		shingle_minhash       TEXT DEFAULT '',
		duplicate_cluster_id  INT DEFAULT 0,
		has_author            BOOLEAN DEFAULT FALSE,
		has_published_date    BOOLEAN DEFAULT FALSE,
		has_schema_article    BOOLEAN DEFAULT FALSE,
		has_breadcrumb        BOOLEAN DEFAULT FALSE,
		UNIQUE(site_id, url, snapshot_date)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_pcs_site ON page_content_snapshots(site_id, snapshot_date DESC)`,

	`CREATE TABLE IF NOT EXISTS content_duplicate_clusters (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		cluster_method  TEXT DEFAULT 'minhash',
		similarity_threshold DOUBLE PRECISION DEFAULT 0.85,
		member_count    INT DEFAULT 0,
		representative_url TEXT DEFAULT '',
		avg_similarity  DOUBLE PRECISION DEFAULT 0,
		severity        TEXT DEFAULT 'low',
		suggested_action TEXT DEFAULT '',
		first_detected  TIMESTAMPTZ DEFAULT NOW()
	)`,

	`CREATE TABLE IF NOT EXISTS content_freshness_stats (
		id                 BIGSERIAL PRIMARY KEY,
		site_id            INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		computed_date      DATE NOT NULL,
		page_type          TEXT DEFAULT '',
		total_pages        INT DEFAULT 0,
		modified_last_7d   INT DEFAULT 0,
		modified_last_30d  INT DEFAULT 0,
		stale_over_180d    INT DEFAULT 0,
		stale_over_365d    INT DEFAULT 0,
		avg_age_days       DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, computed_date, page_type)
	)`,

	`CREATE TABLE IF NOT EXISTS content_topic_clusters (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		cluster_label   TEXT NOT NULL,
		parent_id       INT DEFAULT 0,
		query_count     INT DEFAULT 0,
		page_count      INT DEFAULT 0,
		total_impressions BIGINT DEFAULT 0,
		total_clicks    BIGINT DEFAULT 0,
		avg_position    DOUBLE PRECISION DEFAULT 0,
		hub_url         TEXT DEFAULT '',
		completeness_score DOUBLE PRECISION DEFAULT 0,
		created_at      TIMESTAMPTZ DEFAULT NOW()
	)`,

	// ═══════════════════════════════════════════════════════════════
	// N · 结构化数据 (P2) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS structured_data_daily (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date           DATE NOT NULL,
		schema_type    TEXT NOT NULL,
		url_count      INT DEFAULT 0,
		valid_count    INT DEFAULT 0,
		warning_count  INT DEFAULT 0,
		error_count    INT DEFAULT 0,
		UNIQUE(site_id, date, schema_type)
	)`,

	`CREATE TABLE IF NOT EXISTS structured_data_issues (
		id           BIGSERIAL PRIMARY KEY,
		site_id      INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url          TEXT NOT NULL,
		schema_type  TEXT NOT NULL,
		issue_type   TEXT NOT NULL,
		property     TEXT DEFAULT '',
		severity     TEXT DEFAULT 'medium',
		detail       TEXT DEFAULT '',
		detected_at  TIMESTAMPTZ DEFAULT NOW(),
		resolved_at  TIMESTAMPTZ
	)`,

	// ═══════════════════════════════════════════════════════════════
	// O · hreflang (P2) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS hreflang_map (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		source_url      TEXT NOT NULL,
		hreflang        TEXT NOT NULL,
		target_url      TEXT NOT NULL,
		declared_in     TEXT DEFAULT 'link',
		discovered_at   TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(site_id, source_url, hreflang)
	)`,

	`CREATE TABLE IF NOT EXISTS hreflang_errors (
		id             BIGSERIAL PRIMARY KEY,
		site_id        INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		error_type     TEXT NOT NULL,
		url_a          TEXT DEFAULT '',
		url_b          TEXT DEFAULT '',
		detail         TEXT DEFAULT '',
		severity       TEXT DEFAULT 'medium',
		detected_at    TIMESTAMPTZ DEFAULT NOW(),
		resolved_at    TIMESTAMPTZ
	)`,

	// ═══════════════════════════════════════════════════════════════
	// P · E-E-A-T 信号 (P2) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS eeat_signals_daily (
		id                  BIGSERIAL PRIMARY KEY,
		site_id             INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                DATE NOT NULL,
		has_about_page      BOOLEAN DEFAULT FALSE,
		has_contact_page    BOOLEAN DEFAULT FALSE,
		has_privacy_policy  BOOLEAN DEFAULT FALSE,
		has_author_pages    BOOLEAN DEFAULT FALSE,
		author_page_count   INT DEFAULT 0,
		schema_organization BOOLEAN DEFAULT FALSE,
		schema_breadcrumb_coverage_pct DOUBLE PRECISION DEFAULT 0,
		pages_with_author_pct DOUBLE PRECISION DEFAULT 0,
		pages_with_date_pct DOUBLE PRECISION DEFAULT 0,
		brand_queries_volume INT DEFAULT 0,
		original_content_pct DOUBLE PRECISION DEFAULT 0,
		duplicate_content_pct DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, date)
	)`,

	`CREATE TABLE IF NOT EXISTS author_pages (
		id                BIGSERIAL PRIMARY KEY,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		author_name       TEXT NOT NULL,
		author_url        TEXT DEFAULT '',
		bio_length        INT DEFAULT 0,
		has_photo         BOOLEAN DEFAULT FALSE,
		has_social_links  BOOLEAN DEFAULT FALSE,
		has_schema_person BOOLEAN DEFAULT FALSE,
		published_count   INT DEFAULT 0,
		UNIQUE(site_id, author_name)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// Q · 抓取预算 (P2) — 3 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS crawl_budget_daily (
		id                    BIGSERIAL PRIMARY KEY,
		site_id               INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                  DATE NOT NULL,
		crawler               TEXT NOT NULL,
		total_requests        INT DEFAULT 0,
		unique_urls           INT DEFAULT 0,
		avg_response_ms       INT DEFAULT 0,
		status_2xx_count      INT DEFAULT 0,
		status_3xx_count      INT DEFAULT 0,
		status_4xx_count      INT DEFAULT 0,
		status_5xx_count      INT DEFAULT 0,
		total_bytes           BIGINT DEFAULT 0,
		waste_rate            DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, date, crawler)
	)`,

	`CREATE TABLE IF NOT EXISTS crawl_waste_pages (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		computed_date   DATE NOT NULL,
		url             TEXT NOT NULL,
		waste_reason    TEXT NOT NULL,
		crawl_count_30d INT DEFAULT 0,
		last_crawled    TIMESTAMPTZ,
		suggested_action TEXT DEFAULT ''
	)`,

	`CREATE TABLE IF NOT EXISTS priority_underserved_pages (
		id                BIGSERIAL PRIMARY KEY,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		computed_date     DATE NOT NULL,
		url               TEXT NOT NULL,
		priority_score    DOUBLE PRECISION DEFAULT 0,
		impressions_30d   INT DEFAULT 0,
		crawl_count_30d   INT DEFAULT 0,
		last_crawled      TIMESTAMPTZ
	)`,

	// ═══════════════════════════════════════════════════════════════
	// R · 站群关系 (P3) — 4 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS site_groups (
		id              SERIAL PRIMARY KEY,
		name            TEXT NOT NULL UNIQUE,
		description     TEXT DEFAULT '',
		group_type      TEXT DEFAULT 'portfolio',
		risk_profile    TEXT DEFAULT 'low',
		created_by      INT DEFAULT 0,
		created_at      TIMESTAMPTZ DEFAULT NOW()
	)`,

	`CREATE TABLE IF NOT EXISTS site_group_members (
		id          SERIAL PRIMARY KEY,
		group_id    INT NOT NULL REFERENCES site_groups(id) ON DELETE CASCADE,
		site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		role        TEXT DEFAULT 'member',
		language    TEXT DEFAULT '',
		country     TEXT DEFAULT '',
		UNIQUE(group_id, site_id)
	)`,

	`CREATE TABLE IF NOT EXISTS site_relations (
		id             BIGSERIAL PRIMARY KEY,
		site_a_id      INT NOT NULL,
		site_b_id      INT NOT NULL,
		relation_type  TEXT NOT NULL,
		edge_count     INT DEFAULT 0,
		last_observed  TIMESTAMPTZ,
		UNIQUE(site_a_id, site_b_id, relation_type)
	)`,

	`CREATE TABLE IF NOT EXISTS network_risk_events (
		id              BIGSERIAL PRIMARY KEY,
		group_id        INT DEFAULT 0,
		trigger_site_id INT NOT NULL,
		affected_sites_json JSONB DEFAULT '[]',
		risk_type       TEXT NOT NULL,
		severity        TEXT DEFAULT 'medium',
		detected_at     TIMESTAMPTZ DEFAULT NOW(),
		resolved_at     TIMESTAMPTZ,
		description     TEXT DEFAULT ''
	)`,

	// ═══════════════════════════════════════════════════════════════
	// S · TLS / DNS / 安全头 (P3) — 3 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS tls_checks_daily (
		id                  BIGSERIAL PRIMARY KEY,
		site_id             INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                DATE NOT NULL,
		hostname            TEXT NOT NULL,
		cert_valid          BOOLEAN DEFAULT FALSE,
		cert_issuer         TEXT DEFAULT '',
		valid_from          TIMESTAMPTZ,
		valid_to            TIMESTAMPTZ,
		days_until_expiry   INT DEFAULT 0,
		tls_version         TEXT DEFAULT '',
		hsts_present        BOOLEAN DEFAULT FALSE,
		UNIQUE(site_id, date, hostname)
	)`,

	`CREATE TABLE IF NOT EXISTS dns_checks_daily (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date            DATE NOT NULL,
		hostname        TEXT NOT NULL,
		a_records_json  JSONB DEFAULT '[]',
		ns_json         JSONB DEFAULT '[]',
		resolution_time_ms INT DEFAULT 0,
		has_spf         BOOLEAN DEFAULT FALSE,
		has_dmarc       BOOLEAN DEFAULT FALSE,
		UNIQUE(site_id, date, hostname)
	)`,

	`CREATE TABLE IF NOT EXISTS security_headers_daily (
		id                BIGSERIAL PRIMARY KEY,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date              DATE NOT NULL,
		url               TEXT NOT NULL,
		hsts              TEXT DEFAULT '',
		csp               TEXT DEFAULT '',
		x_frame_options   TEXT DEFAULT '',
		referrer_policy   TEXT DEFAULT '',
		mixed_content     BOOLEAN DEFAULT FALSE,
		score             INT DEFAULT 0,
		UNIQUE(site_id, date, url)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// T · 算法更新追踪 (P3) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS algorithm_updates (
		id              SERIAL PRIMARY KEY,
		engine          TEXT NOT NULL,
		update_name     TEXT NOT NULL,
		update_type     TEXT DEFAULT '',
		announced_at    TIMESTAMPTZ,
		started_at      TIMESTAMPTZ,
		ended_at        TIMESTAMPTZ,
		official        BOOLEAN DEFAULT FALSE,
		volatility_score DOUBLE PRECISION DEFAULT 0,
		source_type     TEXT DEFAULT 'manual',
		source_url      TEXT DEFAULT '',
		summary         TEXT DEFAULT '',
		logged_by       INT DEFAULT 0,
		UNIQUE(engine, update_name)
	)`,

	`CREATE TABLE IF NOT EXISTS site_algorithm_impact (
		id                BIGSERIAL PRIMARY KEY,
		site_id           INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		update_id         INT NOT NULL,
		impact_direction  TEXT DEFAULT 'neutral',
		impact_magnitude  DOUBLE PRECISION DEFAULT 0,
		clicks_change_pct DOUBLE PRECISION DEFAULT 0,
		position_change   DOUBLE PRECISION DEFAULT 0,
		affected_pages    INT DEFAULT 0,
		analyzed_at       TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(site_id, update_id)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// U · 实验 / A/B 测试 (P3) — 2 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS seo_experiments (
		id              SERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		experiment_name TEXT NOT NULL,
		hypothesis      TEXT NOT NULL,
		experiment_type TEXT NOT NULL,
		url_pattern     TEXT NOT NULL,
		metric_target   TEXT NOT NULL,
		planned_duration_days INT DEFAULT 14,
		started_at      TIMESTAMPTZ,
		ended_at        TIMESTAMPTZ,
		status          TEXT DEFAULT 'draft',
		created_by      INT DEFAULT 0,
		created_at      TIMESTAMPTZ DEFAULT NOW()
	)`,

	`CREATE TABLE IF NOT EXISTS seo_experiment_results (
		id                BIGSERIAL PRIMARY KEY,
		experiment_id     INT NOT NULL,
		date              DATE NOT NULL,
		arm               TEXT NOT NULL,
		url               TEXT DEFAULT '',
		clicks            INT DEFAULT 0,
		impressions       INT DEFAULT 0,
		ctr               DOUBLE PRECISION DEFAULT 0,
		position          DOUBLE PRECISION DEFAULT 0,
		p_value           DOUBLE PRECISION,
		effect_size       DOUBLE PRECISION
	)`,

	// ═══════════════════════════════════════════════════════════════
	// V · UX 阻力信号 (P3) — 3 tables
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS rum_events (
		id          BIGSERIAL PRIMARY KEY,
		site_id     INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url         TEXT NOT NULL,
		event_type  TEXT NOT NULL,
		event_value DOUBLE PRECISION DEFAULT 0,
		element_selector TEXT DEFAULT '',
		device_type TEXT DEFAULT '',
		session_id  TEXT DEFAULT '',
		extra_json  JSONB DEFAULT '{}',
		reported_at TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rume_site ON rum_events(site_id, reported_at DESC)`,

	`CREATE TABLE IF NOT EXISTS ux_sessions_daily (
		id                    BIGSERIAL PRIMARY KEY,
		site_id               INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		date                  DATE NOT NULL,
		sessions              INT DEFAULT 0,
		pages_per_session     DOUBLE PRECISION DEFAULT 0,
		avg_scroll_depth_pct  DOUBLE PRECISION DEFAULT 0,
		rage_click_rate       DOUBLE PRECISION DEFAULT 0,
		dead_click_rate       DOUBLE PRECISION DEFAULT 0,
		quick_back_rate       DOUBLE PRECISION DEFAULT 0,
		js_error_rate         DOUBLE PRECISION DEFAULT 0,
		UNIQUE(site_id, date)
	)`,

	`CREATE TABLE IF NOT EXISTS ux_friction_signals (
		id           BIGSERIAL PRIMARY KEY,
		site_id      INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url          TEXT NOT NULL,
		date         DATE NOT NULL,
		signal_type  TEXT NOT NULL,
		severity     TEXT DEFAULT 'medium',
		metric_value DOUBLE PRECISION DEFAULT 0,
		baseline     DOUBLE PRECISION DEFAULT 0,
		sample_size  INT DEFAULT 0,
		UNIQUE(site_id, url, date, signal_type)
	)`,

	// ═══════════════════════════════════════════════════════════════
	// 附加表
	// ═══════════════════════════════════════════════════════════════

	`CREATE TABLE IF NOT EXISTS brand_mentions (
		id              BIGSERIAL PRIMARY KEY,
		site_id         INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		brand_term      TEXT NOT NULL,
		source_url      TEXT NOT NULL,
		source_domain   TEXT NOT NULL,
		source_type     TEXT DEFAULT '',
		snippet         TEXT DEFAULT '',
		sentiment       TEXT DEFAULT 'neutral',
		discovered_at   TIMESTAMPTZ DEFAULT NOW(),
		discovered_by   TEXT DEFAULT 'crawler'
	)`,

	`CREATE TABLE IF NOT EXISTS ua_cloaking_checks (
		id            BIGSERIAL PRIMARY KEY,
		site_id       INT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		url           TEXT NOT NULL,
		checked_at    TIMESTAMPTZ DEFAULT NOW(),
		ua_googlebot_hash TEXT DEFAULT '',
		ua_mobile_hash TEXT DEFAULT '',
		ua_desktop_hash TEXT DEFAULT '',
		has_divergence BOOLEAN DEFAULT FALSE,
		divergence_severity TEXT DEFAULT 'low'
	)`,

	// ═══════════════════════════════════════════════════════════════
	// ALTER 现有表 — 补分析元数据
	// ═══════════════════════════════════════════════════════════════

	// sites 表补字段
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN tier TEXT DEFAULT 'mid'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN maturity TEXT DEFAULT 'mature'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN content_category TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN monetization TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN target_countries JSONB DEFAULT '[]'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN target_languages JSONB DEFAULT '[]'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN primary_device TEXT DEFAULT 'mobile'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN est_monthly_sessions INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN est_total_urls INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN brand_terms_json JSONB DEFAULT '[]'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN competitor_domains_json JSONB DEFAULT '[]'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN baseline_noise_threshold DOUBLE PRECISION DEFAULT 0.15; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN analysis_enabled BOOLEAN DEFAULT TRUE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN cdn_provider TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN hosting_provider TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN cms_platform TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE sites ADD COLUMN site_group_id INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

	// audit_results 补字段
	"DO $$ BEGIN ALTER TABLE audit_results ADD COLUMN critical_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE audit_results ADD COLUMN serious_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE audit_results ADD COLUMN medium_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE audit_results ADD COLUMN low_count INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE audit_results ADD COLUMN total_findings INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE audit_results ADD COLUMN findings_flattened BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

	// site_pages 补字段
	"DO $$ BEGIN ALTER TABLE site_pages ADD COLUMN page_type TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE site_pages ADD COLUMN in_sitemap BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE site_pages ADD COLUMN depth_from_home INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE site_pages ADD COLUMN content_hash TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE site_pages ADD COLUMN is_priority BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

	// spider_logs 补字段
	"DO $$ BEGIN ALTER TABLE spider_logs ADD COLUMN response_size_bytes INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE spider_logs ADD COLUMN is_waste BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE spider_logs ADD COLUMN waste_reason TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

	// coordination_tasks 补字段
	"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN source_type TEXT DEFAULT 'manual'; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN source_id BIGINT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE coordination_tasks ADD COLUMN auto_created BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",

	// query_enrichment 补字段
	"DO $$ BEGIN ALTER TABLE query_enrichment ADD COLUMN cluster_id INT DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE query_enrichment ADD COLUMN primary_page_url TEXT DEFAULT ''; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE query_enrichment ADD COLUMN is_tracked BOOLEAN DEFAULT FALSE; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
	"DO $$ BEGIN ALTER TABLE query_enrichment ADD COLUMN priority_score DOUBLE PRECISION DEFAULT 0; EXCEPTION WHEN duplicate_column THEN NULL; END $$",
}
