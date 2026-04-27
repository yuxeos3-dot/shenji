-- migrate_to_single_user.sql
--
-- Optional one-shot cleanup that drops every table belonging to a feature
-- removed in the single-user slim refactor. Run this AFTER the new binary
-- is deployed and you've confirmed everything works against the trimmed
-- schema. It is destructive — rows in these tables are gone forever.
--
--   psql "$DATABASE_URL" -f deploy/migrate_to_single_user.sql
--
-- The application code does NOT depend on these tables — they're left
-- behind by previous versions and would otherwise sit empty forever.

BEGIN;

-- ── Coordination / GEO / FRW / reports / extlinks / push ─────────────
DROP TABLE IF EXISTS coordination_attachments CASCADE;
DROP TABLE IF EXISTS coordination_events CASCADE;
DROP TABLE IF EXISTS coord_test_pages CASCADE;
DROP TABLE IF EXISTS coord_task_reviewers CASCADE;
DROP TABLE IF EXISTS coordination_tasks CASCADE;
DROP TABLE IF EXISTS geo_articles CASCADE;
DROP TABLE IF EXISTS geo_workflow_jobs CASCADE;
DROP TABLE IF EXISTS geo_tasks CASCADE;
DROP TABLE IF EXISTS geo_knowledge CASCADE;
DROP TABLE IF EXISTS geo_prompts CASCADE;
DROP TABLE IF EXISTS geo_titles CASCADE;
DROP TABLE IF EXISTS geo_title_libs CASCADE;
DROP TABLE IF EXISTS ai_generations CASCADE;
DROP TABLE IF EXISTS daily_reports CASCADE;
DROP TABLE IF EXISTS weekly_reports CASCADE;
DROP TABLE IF EXISTS monthly_reports CASCADE;
DROP TABLE IF EXISTS report_evaluations CASCADE;
DROP TABLE IF EXISTS extlink_pool CASCADE;
DROP TABLE IF EXISTS external_links CASCADE;
DROP TABLE IF EXISTS push_logs CASCADE;
DROP TABLE IF EXISTS push_schedules CASCADE;

-- ── Spider / RUM ingest ──────────────────────────────────────────────
DROP TABLE IF EXISTS spider_logs CASCADE;
DROP TABLE IF EXISTS rum_beacons CASCADE;
DROP TABLE IF EXISTS rum_events CASCADE;

-- ── Multi-user organizational tables ─────────────────────────────────
DROP TABLE IF EXISTS departments CASCADE;
DROP TABLE IF EXISTS dept_groups CASCADE;
DROP TABLE IF EXISTS user_platform_hides CASCADE;
DROP TABLE IF EXISTS user_telegram CASCADE;
DROP TABLE IF EXISTS account_lockouts CASCADE;
DROP TABLE IF EXISTS audit_logs CASCADE;

-- ── Deep-data analysis surface (rules / anomalies / health / reports) ─
DROP TABLE IF EXISTS rule_finding_history CASCADE;
DROP TABLE IF EXISTS rule_findings CASCADE;
DROP TABLE IF EXISTS rule_runs CASCADE;
DROP TABLE IF EXISTS rule_suppressions CASCADE;
DROP TABLE IF EXISTS analysis_rules CASCADE;
DROP TABLE IF EXISTS metric_baselines CASCADE;
DROP TABLE IF EXISTS metric_snapshots_daily CASCADE;
DROP TABLE IF EXISTS dimension_attributions CASCADE;
DROP TABLE IF EXISTS anomaly_events CASCADE;
DROP TABLE IF EXISTS site_health_scores CASCADE;
DROP TABLE IF EXISTS health_score_components CASCADE;
DROP TABLE IF EXISTS site_analysis_reports CASCADE;
DROP TABLE IF EXISTS report_templates CASCADE;
DROP TABLE IF EXISTS report_subscriptions CASCADE;
DROP TABLE IF EXISTS timeline_events CASCADE;
DROP TABLE IF EXISTS event_correlations CASCADE;
DROP TABLE IF EXISTS aio_observations CASCADE;
DROP TABLE IF EXISTS ai_crawler_logs CASCADE;
DROP TABLE IF EXISTS ai_crawler_daily CASCADE;
DROP TABLE IF EXISTS llms_txt_snapshots CASCADE;
DROP TABLE IF EXISTS competitor_differentiators CASCADE;
DROP TABLE IF EXISTS competitor_backlink_intersection CASCADE;
DROP TABLE IF EXISTS competitor_serp_rankings CASCADE;
DROP TABLE IF EXISTS competitor_content_snapshots CASCADE;
DROP TABLE IF EXISTS competitor_tdk_snapshots CASCADE;
DROP TABLE IF EXISTS competitors CASCADE;
DROP TABLE IF EXISTS tracked_queries CASCADE;
DROP TABLE IF EXISTS serp_position_daily CASCADE;
DROP TABLE IF EXISTS serp_observations CASCADE;
DROP TABLE IF EXISTS serp_features_daily CASCADE;
DROP TABLE IF EXISTS web_vitals_daily CASCADE;
DROP TABLE IF EXISTS lighthouse_audits CASCADE;
DROP TABLE IF EXISTS resource_timing_daily CASCADE;
DROP TABLE IF EXISTS backlinks_all CASCADE;
DROP TABLE IF EXISTS referring_domains CASCADE;
DROP TABLE IF EXISTS backlink_profile_daily CASCADE;
DROP TABLE IF EXISTS anchor_distribution_daily CASCADE;
DROP TABLE IF EXISTS suspicious_links CASCADE;
DROP TABLE IF EXISTS disavow_records CASCADE;
DROP TABLE IF EXISTS internal_link_edges CASCADE;
DROP TABLE IF EXISTS internal_link_stats CASCADE;
DROP TABLE IF EXISTS site_link_graph_metrics CASCADE;
DROP TABLE IF EXISTS page_content_snapshots CASCADE;
DROP TABLE IF EXISTS content_duplicate_clusters CASCADE;
DROP TABLE IF EXISTS content_freshness_stats CASCADE;
DROP TABLE IF EXISTS content_topic_clusters CASCADE;
DROP TABLE IF EXISTS structured_data_daily CASCADE;
DROP TABLE IF EXISTS structured_data_issues CASCADE;
DROP TABLE IF EXISTS hreflang_map CASCADE;
DROP TABLE IF EXISTS hreflang_errors CASCADE;
DROP TABLE IF EXISTS eeat_signals_daily CASCADE;
DROP TABLE IF EXISTS author_pages CASCADE;
DROP TABLE IF EXISTS crawl_budget_daily CASCADE;
DROP TABLE IF EXISTS crawl_waste_pages CASCADE;
DROP TABLE IF EXISTS priority_underserved_pages CASCADE;
DROP TABLE IF EXISTS site_groups CASCADE;
DROP TABLE IF EXISTS site_group_members CASCADE;
DROP TABLE IF EXISTS site_relations CASCADE;
DROP TABLE IF EXISTS network_risk_events CASCADE;
DROP TABLE IF EXISTS tls_checks_daily CASCADE;
DROP TABLE IF EXISTS dns_checks_daily CASCADE;
DROP TABLE IF EXISTS security_headers_daily CASCADE;
DROP TABLE IF EXISTS algorithm_updates CASCADE;
DROP TABLE IF EXISTS site_algorithm_impact CASCADE;
DROP TABLE IF EXISTS seo_experiments CASCADE;
DROP TABLE IF EXISTS seo_experiment_results CASCADE;
DROP TABLE IF EXISTS ux_sessions_daily CASCADE;
DROP TABLE IF EXISTS ux_friction_signals CASCADE;
DROP TABLE IF EXISTS brand_mentions CASCADE;
DROP TABLE IF EXISTS ua_cloaking_checks CASCADE;
DROP TABLE IF EXISTS ctr_baseline CASCADE;
DROP TABLE IF EXISTS query_enrichment CASCADE;
DROP TABLE IF EXISTS gsc_opportunities_weekly CASCADE;
DROP TABLE IF EXISTS gsc_cannibalization_monthly CASCADE;
DROP TABLE IF EXISTS gsc_query_diff_weekly CASCADE;

-- Per-platform deep-detail daily tables that powered the deep-data
-- dashboards. The basic *_data tables (gsc_data, ga_data, bing_data,
-- yandex_data) are kept — they back the data-management overview cards.
DROP TABLE IF EXISTS gsc_query_page_daily CASCADE;
DROP TABLE IF EXISTS gsc_rank_buckets_daily CASCADE;
DROP TABLE IF EXISTS gsc_url_inspection CASCADE;
DROP TABLE IF EXISTS gsc_sitemaps CASCADE;
DROP TABLE IF EXISTS gsc_appearance_daily CASCADE;
DROP TABLE IF EXISTS bing_query_page_daily CASCADE;
DROP TABLE IF EXISTS bing_query_daily CASCADE;
DROP TABLE IF EXISTS bing_page_daily CASCADE;
DROP TABLE IF EXISTS bing_inbound_links CASCADE;
DROP TABLE IF EXISTS bing_url_status CASCADE;
DROP TABLE IF EXISTS bing_keyword_research CASCADE;
DROP TABLE IF EXISTS bing_sitemaps CASCADE;
DROP TABLE IF EXISTS yandex_query_daily CASCADE;
DROP TABLE IF EXISTS yandex_index_events_daily CASCADE;
DROP TABLE IF EXISTS yandex_http_status_daily CASCADE;
DROP TABLE IF EXISTS yandex_indexed_urls CASCADE;
DROP TABLE IF EXISTS yandex_external_links CASCADE;
DROP TABLE IF EXISTS ga_daily_extended CASCADE;
DROP TABLE IF EXISTS ga_landing_pages_daily CASCADE;
DROP TABLE IF EXISTS ga_source_medium_daily CASCADE;
DROP TABLE IF EXISTS ga_events_daily CASCADE;
DROP TABLE IF EXISTS ga_tech_daily CASCADE;
DROP TABLE IF EXISTS ga_geo_daily CASCADE;
DROP TABLE IF EXISTS ga_hourly CASCADE;
DROP TABLE IF EXISTS ga_scroll_depth_daily CASCADE;
DROP TABLE IF EXISTS ga_outbound_clicks_daily CASCADE;
DROP TABLE IF EXISTS ga_conversions_daily CASCADE;
DROP TABLE IF EXISTS ga_day_of_week CASCADE;

-- ── Drop multi-user / org columns from users ──────────────────────────
-- Single-user mode has exactly one row at id=1; these columns are inert.
ALTER TABLE users DROP COLUMN IF EXISTS department_id CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS group_id CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS register_status CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS token_invalidated_after CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS permissions CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS frw_apikey CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS frw_config CASCADE;

COMMIT;
