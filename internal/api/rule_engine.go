package api

import (
	"log"

	"seo-system/internal/db"
)

// SeedInitialRules inserts the first batch of analysis rules if not present.
func SeedInitialRules() {
	rules := []struct {
		ID, Category, Scope, Severity, Title, SQL string
		Priority                                  int
	}{
		{"URG_001", "urgent", "page", "critical", "页面返回 404/5xx",
			`SELECT url as target_url, status_code as evidence FROM site_pages WHERE site_id=$1 AND status_code >= 400`, 90},
		{"URG_002", "urgent", "page", "high", "页面缺少 H1 标签",
			`SELECT url as target_url, h1_count as evidence FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND h1_count=0`, 80},
		{"URG_003", "urgent", "page", "high", "页面 H1 标签重复（>1个）",
			`SELECT url as target_url, h1_count as evidence FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND h1_count>1`, 70},
		{"URG_004", "urgent", "page", "critical", "页面缺少 Title",
			`SELECT url as target_url FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND (title='' OR title IS NULL)`, 90},
		{"URG_005", "urgent", "page", "high", "页面缺少 Meta Description",
			`SELECT url as target_url FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND (meta_description='' OR meta_description IS NULL)`, 75},
		{"OPP_001", "opportunity", "query", "medium", "高曝光低排名关键词（位置 11-20，曝光>100）",
			`SELECT query as target_query, SUM(impressions) as evidence FROM gsc_query_page_daily WHERE site_id=$1 AND date>=CURRENT_DATE-30 AND position>=10 AND position<20 AND search_type='web' GROUP BY query HAVING SUM(impressions)>100 ORDER BY SUM(impressions) DESC LIMIT 50`, 60},
		{"OPP_002", "opportunity", "query", "medium", "CTR 远低于基线的关键词",
			`SELECT query as target_query FROM gsc_opportunities_weekly WHERE site_id=$1 AND ctr_gap_ratio>0.5 AND impressions>100 ORDER BY opportunity_score DESC LIMIT 30`, 65},
		{"OPT_001", "optimization", "page", "medium", "薄内容页面（<300词）",
			`SELECT url as target_url, word_count as evidence FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND word_count>0 AND word_count<300`, 50},
		{"OPT_002", "optimization", "page", "low", "缺少结构化数据的页面",
			`SELECT url as target_url FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND schema_types_json='[]'`, 40},
		{"OPT_003", "optimization", "page", "medium", "孤岛页面（0内部反链）",
			`SELECT url as target_url, inbound_links as evidence FROM internal_link_stats WHERE site_id=$1 AND computed_date=(SELECT MAX(computed_date) FROM internal_link_stats WHERE site_id=$1) AND is_orphan=TRUE`, 55},
		{"RSK_001", "risk", "query", "high", "自蚕食关键词（多页面争抢同一词）",
			`SELECT query as target_query, page_count as evidence FROM gsc_cannibalization_monthly WHERE site_id=$1 AND severity IN ('high','medium') ORDER BY total_impressions DESC LIMIT 30`, 70},
		{"RSK_002", "risk", "site", "medium", "TLS 证书即将到期（<30天）",
			`SELECT hostname as target_url, days_until_expiry as evidence FROM tls_checks_daily WHERE site_id=$1 AND date=(SELECT MAX(date) FROM tls_checks_daily WHERE site_id=$1) AND days_until_expiry<30 AND days_until_expiry>0`, 80},
		{"RSK_003", "risk", "site", "high", "缺少 SPF 或 DMARC",
			`SELECT hostname as target_url FROM dns_checks_daily WHERE site_id=$1 AND date=(SELECT MAX(date) FROM dns_checks_daily WHERE site_id=$1) AND (has_spf=FALSE OR has_dmarc=FALSE)`, 60},
	}

	for _, r := range rules {
		db.DB.Exec(`INSERT INTO analysis_rules (rule_id, category, scope, severity, priority_base, title, trigger_sql, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,TRUE) ON CONFLICT (rule_id) DO NOTHING`,
			r.ID, r.Category, r.Scope, r.Severity, r.Priority, r.Title, r.SQL)
	}
}

// RunRuleEngine executes all enabled rules for a site and creates findings.
func RunRuleEngine(siteID int64) {
	rows, err := db.DB.Query(`SELECT rule_id, severity, priority_base, title, trigger_sql, suggestion_template
		FROM analysis_rules WHERE enabled=TRUE AND trigger_sql!=''
		ORDER BY priority_base DESC`)
	if err != nil {
		log.Printf("[rules] query rules: %v", err)
		return
	}
	defer rows.Close()

	totalFindings := 0
	for rows.Next() {
		var ruleID, severity, title, sql, suggestion string
		var priority int
		rows.Scan(&ruleID, &severity, &priority, &title, &sql, &suggestion)

		// Check if rule is suppressed for this site
		var suppressed int
		db.DB.QueryRow(`SELECT 1 FROM rule_suppressions WHERE (site_id=$1 OR site_id=0) AND rule_id=$2`, siteID, ruleID).Scan(&suppressed)
		if suppressed > 0 {
			continue
		}

		// Create run record
		var runID int64
		db.DB.QueryRow(`INSERT INTO rule_runs (site_id, rule_id, status) VALUES ($1,$2,'running') RETURNING id`,
			siteID, ruleID).Scan(&runID)

		// Execute the rule SQL
		rRows, err := db.DB.Query(sql, siteID)
		if err != nil {
			db.DB.Exec(`UPDATE rule_runs SET status='failed', error_msg=$1, finished_at=NOW() WHERE id=$2`, err.Error(), runID)
			continue
		}

		cols, _ := rRows.Columns()
		count := 0
		for rRows.Next() {
			vals := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if rRows.Scan(ptrs...) != nil {
				continue
			}

			targetURL := ""
			targetQuery := ""
			for i, col := range cols {
				v := ""
				switch tv := vals[i].(type) {
				case string:
					v = tv
				case []byte:
					v = string(tv)
				}
				if col == "target_url" {
					targetURL = v
				}
				if col == "target_query" {
					targetQuery = v
				}
			}

			// Dedup check: same rule+site+target in last 7 days
			var existing int
			db.DB.QueryRow(`SELECT 1 FROM rule_findings WHERE site_id=$1 AND rule_id=$2 AND target_url=$3 AND target_query=$4
				AND detected_at >= CURRENT_DATE - INTERVAL '7 days' AND status!='resolved'`,
				siteID, ruleID, targetURL, targetQuery).Scan(&existing)
			if existing > 0 {
				continue
			}

			db.DB.Exec(`INSERT INTO rule_findings (rule_id, site_id, run_id, scope_type, target_url, target_query, severity, priority_score, suggestion_text)
				VALUES ($1,$2,$3,'page',$4,$5,$6,$7,$8)`,
				ruleID, siteID, runID, targetURL, targetQuery, severity, float64(priority), suggestion)
			count++
		}
		rRows.Close()

		db.DB.Exec(`UPDATE rule_runs SET status='success', findings_count=$1, finished_at=NOW() WHERE id=$2`, count, runID)
		totalFindings += count
	}

	log.Printf("[rules] site %d: %d new findings from rule engine", siteID, totalFindings)
}
