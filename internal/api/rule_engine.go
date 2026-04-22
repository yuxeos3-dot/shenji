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
		// Extended rules (URG_006-010)
		{"URG_006", "urgent", "page", "critical", "Canonical 指向不同域",
			`SELECT url as target_url, google_canonical as evidence FROM gsc_url_inspection WHERE site_id=$1 AND google_canonical!='' AND google_canonical NOT LIKE '%' || (SELECT REPLACE(REPLACE(domain,'https://',''),'http://','') FROM sites WHERE id=$1) || '%'`, 85},
		{"URG_007", "urgent", "page", "high", "Sitemap 有错误",
			`SELECT sitemap_path as target_url, errors as evidence FROM gsc_sitemaps WHERE site_id=$1 AND errors > 0`, 75},
		{"URG_008", "urgent", "page", "high", "Title 与 H1 完全相同",
			`SELECT url as target_url FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND title!='' AND h1!='' AND LOWER(TRIM(title))=LOWER(TRIM(h1))`, 55},
		{"URG_009", "urgent", "page", "medium", "缺少 Canonical 标签",
			`SELECT url as target_url FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND (canonical='' OR canonical IS NULL)`, 60},
		{"URG_010", "urgent", "page", "high", "图片缺少 Alt 属性（>50% 图片无 Alt）",
			`SELECT url as target_url, image_count, images_with_alt_count FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND image_count > 3 AND images_with_alt_count::float/GREATEST(image_count,1) < 0.5`, 55},
		// Extended OPP rules
		{"OPP_003", "opportunity", "query", "medium", "品牌词有曝光但无点击",
			`SELECT query as target_query, SUM(impressions) as evidence FROM gsc_query_page_daily q JOIN query_enrichment e ON e.site_id=q.site_id AND e.query=q.query AND e.platform='gsc' WHERE q.site_id=$1 AND e.brand_category='brand' AND q.date>=CURRENT_DATE-30 GROUP BY q.query HAVING SUM(clicks)=0 AND SUM(impressions)>50`, 50},
		{"OPP_004", "opportunity", "query", "medium", "排名下滑词（WoW 位置变差 > 5 位）",
			`SELECT query as target_query, position_delta as evidence FROM gsc_query_diff_weekly WHERE site_id=$1 AND diff_type='fallen' AND position_delta > 5 ORDER BY ABS(click_delta) DESC LIMIT 30`, 65},
		{"OPP_005", "opportunity", "query", "medium", "丢失词（上周有流量本周没了）",
			`SELECT query as target_query, prev_clicks as evidence FROM gsc_query_diff_weekly WHERE site_id=$1 AND diff_type='lost' AND prev_clicks > 5 ORDER BY prev_clicks DESC LIMIT 30`, 70},
		// Extended OPT rules
		{"OPT_004", "optimization", "page", "medium", "外部链接过多（>50个外链）",
			`SELECT url as target_url, external_link_count as evidence FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND external_link_count > 50`, 40},
		{"OPT_005", "optimization", "page", "low", "内容过于陈旧（>180天未更新）",
			`SELECT url as target_url FROM content_freshness_stats WHERE site_id=$1 AND stale_over_180d > 0`, 35},
		{"OPT_006", "optimization", "page", "medium", "无 Schema Breadcrumb",
			`SELECT url as target_url FROM page_content_snapshots WHERE site_id=$1 AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1) AND has_breadcrumb=FALSE`, 40},
		// Strategy rules
		{"STR_001", "strategy", "site", "info", "内容更新频率低（30天内修改<10%页面）",
			`SELECT computed_date::text as target_url, modified_last_30d, total_pages FROM content_freshness_stats WHERE site_id=$1 AND total_pages > 10 AND modified_last_30d::float/GREATEST(total_pages,1) < 0.1 ORDER BY computed_date DESC LIMIT 1`, 30},
		{"STR_002", "strategy", "site", "info", "反链增长停滞（30天无新反链）",
			`SELECT date::text as target_url, new_links_24h FROM backlink_profile_daily WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, 30},
		// Extended RSK rules
		{"RSK_004", "risk", "site", "medium", "安全头评分低（<50/100）",
			`SELECT url as target_url, score as evidence FROM security_headers_daily WHERE site_id=$1 AND date=(SELECT MAX(date) FROM security_headers_daily WHERE site_id=$1) AND score < 50`, 55},
		{"RSK_005", "risk", "site", "high", "死链数量过多（>20个）",
			`SELECT COUNT(*) as evidence FROM dead_link_results WHERE site_id=$1 AND status_code >= 400 HAVING COUNT(*) > 20`, 65},
		{"RSK_006", "risk", "page", "medium", "抓取浪费率高（>30%）",
			`SELECT crawler as target_url, waste_rate as evidence FROM crawl_budget_daily WHERE site_id=$1 AND date=(SELECT MAX(date) FROM crawl_budget_daily WHERE site_id=$1) AND waste_rate > 0.3`, 50},
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
