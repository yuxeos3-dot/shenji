package api

import (
	"log"
	"math"
	"time"

	"seo-system/internal/db"
)

// RunAnomalyDetection computes baselines and detects anomalies for a site.
// Uses z-score method on metric_snapshots_daily.
func RunAnomalyDetection(siteID int64) {
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	metrics := []struct{ name, platform string }{
		// GSC
		{"clicks", "gsc"}, {"impressions", "gsc"}, {"ctr", "gsc"}, {"position", "gsc"},
		// GA4
		{"sessions", "ga"}, {"users", "ga"}, {"pageviews", "ga"}, {"bounce_rate", "ga"},
		// Bing
		{"clicks", "bing"}, {"impressions", "bing"},
		// Yandex
		{"clicks", "yandex"}, {"impressions", "yandex"},
		// Cross-platform
		{"dead_links", "all"},
	}

	for _, m := range metrics {
		detectAnomaly(siteID, date, m.name, m.platform)
	}
}

func detectAnomaly(siteID int64, date, metric, platform string) {
	windowDays := 28

	// Get recent values for baseline
	var values []float64
	rows, err := db.DB.Query(`SELECT value FROM metric_snapshots_daily
		WHERE site_id=$1 AND metric_name=$2 AND platform=$3
		AND date >= CURRENT_DATE - ($4+1) * INTERVAL '1 day' AND date < $5::date
		ORDER BY date`, siteID, metric, platform, windowDays, date)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var v float64
		rows.Scan(&v)
		values = append(values, v)
	}

	if len(values) < 7 {
		return // Not enough data for baseline
	}

	// Compute mean and stddev
	var sum, sumSq float64
	for _, v := range values {
		sum += v
		sumSq += v * v
	}
	n := float64(len(values))
	mean := sum / n
	variance := sumSq/n - mean*mean
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)

	// Save baseline
	db.DB.Exec(`INSERT INTO metric_baselines (site_id, scope_type, metric_name, platform, period, window_days, baseline_date, mean, stddev, sample_size)
		VALUES ($1,'site',$2,$3,'daily',$4,$5::date,$6,$7,$8)
		ON CONFLICT (site_id, scope_type, scope_key, metric_name, platform, period, baseline_date) DO UPDATE SET
		mean=EXCLUDED.mean, stddev=EXCLUDED.stddev, sample_size=EXCLUDED.sample_size`,
		siteID, metric, platform, windowDays, date, mean, stddev, len(values))

	// Get today's value
	var currentValue float64
	err = db.DB.QueryRow(`SELECT value FROM metric_snapshots_daily
		WHERE site_id=$1 AND metric_name=$2 AND platform=$3 AND date=$4::date`,
		siteID, metric, platform, date).Scan(&currentValue)
	if err != nil {
		return
	}

	// Z-score detection
	if stddev < 0.001 {
		return // Too stable to detect anomalies
	}
	zscore := (currentValue - mean) / stddev

	if math.Abs(zscore) < 2.0 {
		return // Within normal range
	}

	severity := "medium"
	if math.Abs(zscore) >= 4.0 {
		severity = "critical"
	} else if math.Abs(zscore) >= 3.0 {
		severity = "high"
	}

	direction := "up"
	if zscore < 0 {
		direction = "down"
	}

	lowerBound := mean - 2*stddev
	upperBound := mean + 2*stddev

	db.DB.Exec(`INSERT INTO anomaly_events (site_id, metric_name, platform, method, direction, event_date,
		observed_value, expected_value, lower_bound, upper_bound, deviation_sigma, severity)
		VALUES ($1,$2,$3,'zscore',$4,$5::date,$6,$7,$8,$9,$10,$11)
		ON CONFLICT DO NOTHING`,
		siteID, metric, platform, direction, date,
		currentValue, mean, lowerBound, upperBound, zscore, severity)

	log.Printf("[anomaly] site %d: %s.%s z=%.1f (observed=%.1f, mean=%.1f±%.1f) → %s %s",
		siteID, metric, platform, zscore, currentValue, mean, stddev, severity, direction)
}

// SnapshotMetricsFromPlatformData creates daily metric snapshots from existing data tables.
// Run daily before anomaly detection.
func SnapshotMetricsFromPlatformData(siteID int64) {
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// GSC metrics
	var clicks, impressions int
	var ctr, position float64
	err := db.DB.QueryRow(`SELECT clicks, impressions, ctr, position
		FROM gsc_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(&clicks, &impressions, &ctr, &position)
	if err == nil {
		snapshotMetric(siteID, date, "clicks", "gsc", float64(clicks))
		snapshotMetric(siteID, date, "impressions", "gsc", float64(impressions))
		snapshotMetric(siteID, date, "ctr", "gsc", ctr)
		snapshotMetric(siteID, date, "position", "gsc", position)
	}

	// GA4 metrics
	var sessions, users, pageviews int
	var bounceRate float64
	err = db.DB.QueryRow(`SELECT sessions, users, pageviews, bounce_rate
		FROM ga_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(&sessions, &users, &pageviews, &bounceRate)
	if err == nil {
		snapshotMetric(siteID, date, "sessions", "ga", float64(sessions))
		snapshotMetric(siteID, date, "users", "ga", float64(users))
		snapshotMetric(siteID, date, "pageviews", "ga", float64(pageviews))
		snapshotMetric(siteID, date, "bounce_rate", "ga", bounceRate)
	}

	// Bing metrics
	var bingClicks, bingImpressions int
	err = db.DB.QueryRow(`SELECT clicks, impressions FROM bing_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`,
		siteID).Scan(&bingClicks, &bingImpressions)
	if err == nil {
		snapshotMetric(siteID, date, "clicks", "bing", float64(bingClicks))
		snapshotMetric(siteID, date, "impressions", "bing", float64(bingImpressions))
	}

	// Yandex metrics
	var yClicks, yImpressions int
	err = db.DB.QueryRow(`SELECT clicks, impressions FROM yandex_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`,
		siteID).Scan(&yClicks, &yImpressions)
	if err == nil {
		snapshotMetric(siteID, date, "clicks", "yandex", float64(yClicks))
		snapshotMetric(siteID, date, "impressions", "yandex", float64(yImpressions))
	}

	// Dead links count
	var deadLinks int
	db.DB.QueryRow(`SELECT COUNT(*) FROM dead_link_results WHERE site_id=$1 AND status_code >= 400`, siteID).Scan(&deadLinks)
	snapshotMetric(siteID, date, "dead_links", "all", float64(deadLinks))
}

// AutoGenerateTimelineEvents creates timeline events from various data sources.
func AutoGenerateTimelineEvents(siteID int64) {
	// 1. From anomaly_events → timeline
	db.DB.Exec(`INSERT INTO timeline_events (site_id, event_category, event_type, title, severity, occurred_at, source_type, related_anomaly_id)
		SELECT site_id, 'anomaly', 'metric_anomaly',
			metric_name || ' ' || direction || ' (z=' || ROUND(deviation_sigma::numeric,1) || ')',
			severity, detected_at, 'auto', id
		FROM anomaly_events
		WHERE site_id=$1 AND id NOT IN (SELECT related_anomaly_id FROM timeline_events WHERE site_id=$1 AND related_anomaly_id > 0)
		AND detected_at >= CURRENT_DATE - INTERVAL '30 days'`, siteID)

	// 2. From audit_tasks → timeline (new audits)
	db.DB.Exec(`INSERT INTO timeline_events (site_id, event_category, event_type, title, severity, occurred_at, source_type, source_id)
		SELECT site_id, 'audit', 'audit_completed',
			'站点审计完成 (状态: ' || status || ')',
			CASE WHEN status='done' THEN 'info' ELSE 'medium' END,
			COALESCE(finished_at, created_at), 'auto', id
		FROM audit_tasks
		WHERE site_id=$1
		AND id NOT IN (SELECT source_id FROM timeline_events WHERE site_id=$1 AND event_category='audit' AND source_id > 0)
		AND created_at >= CURRENT_DATE - INTERVAL '30 days'`, siteID)

	// 3. From site_incidents → timeline (manual events)
	db.DB.Exec(`INSERT INTO timeline_events (site_id, event_category, event_type, title, severity, occurred_at, source_type, related_incident_id)
		SELECT site_id, 'manual', 'incident', title, severity, occurred_at, 'manual', id
		FROM site_incidents
		WHERE site_id=$1
		AND id NOT IN (SELECT related_incident_id FROM timeline_events WHERE site_id=$1 AND related_incident_id > 0)`, siteID)

	// 4. From rule_findings (critical/high rules) → timeline
	db.DB.Exec(`INSERT INTO timeline_events (site_id, event_category, event_type, title, severity, occurred_at, source_type, related_finding_id)
		SELECT site_id, 'rule', 'rule_triggered',
			rule_id || ': ' || LEFT(COALESCE(target_url, target_query, ''), 80),
			severity, detected_at, 'auto', id
		FROM rule_findings
		WHERE site_id=$1 AND severity IN ('critical','high')
		AND id NOT IN (SELECT related_finding_id FROM timeline_events WHERE site_id=$1 AND related_finding_id > 0)
		AND detected_at >= CURRENT_DATE - INTERVAL '30 days'`, siteID)

	// 5. From algorithm_updates (global events) → timeline
	db.DB.Exec(`INSERT INTO timeline_events (site_id, event_category, event_type, title, severity, occurred_at, source_type)
		SELECT 0, 'algorithm', engine || '_update', update_name, 'high', COALESCE(started_at, announced_at), 'auto'
		FROM algorithm_updates
		WHERE id NOT IN (SELECT source_id FROM timeline_events WHERE event_category='algorithm' AND source_id > 0)
		AND announced_at >= CURRENT_DATE - INTERVAL '90 days'`)
}

// RunFullAnalysisPipeline runs all analysis steps for a site in order.
func RunFullAnalysisPipeline(siteID int64) {
	log.Printf("[analysis] site %d: starting full pipeline", siteID)
	start := time.Now()

	// Get domain for probes
	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id=$1", siteID).Scan(&domain)

	// Step 1: Snapshot metrics
	SnapshotMetricsFromPlatformData(siteID)

	// Step 2: Anomaly detection
	RunAnomalyDetection(siteID)

	// Step 3: Auto-generate timeline events
	AutoGenerateTimelineEvents(siteID)

	// Step 4: Compute health score
	ComputeSiteHealthScore(siteID)

	// Step 5: Compute derived data (opportunities, cannibalization, diff)
	ComputeOpportunities(siteID)
	ComputeCannibalization(siteID)
	ComputeQueryDiff(siteID)

	// Step 6: Infrastructure probes (TLS/DNS/Security headers)
	if domain != "" {
		RunAllProbes(siteID, domain)
	}

	// Step 7: Rule engine (execute all enabled rules → rule_findings)
	RunRuleEngine(siteID)

	log.Printf("[analysis] site %d: pipeline complete in %s", siteID, time.Since(start).Round(time.Millisecond))
}
