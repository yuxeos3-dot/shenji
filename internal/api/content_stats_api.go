package api

import (
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetContentStats returns content health statistics from page_content_snapshots.
func GetContentStats(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	var total, thin, noH1, noSchema, noMeta, withAuthor, withBreadcrumb int
	db.DB.QueryRow(`SELECT
		COUNT(*),
		COUNT(*) FILTER (WHERE word_count > 0 AND word_count < 300),
		COUNT(*) FILTER (WHERE h1_count = 0),
		COUNT(*) FILTER (WHERE schema_types_json = '[]' OR schema_types_json IS NULL),
		COUNT(*) FILTER (WHERE meta_description = '' OR meta_description IS NULL),
		COUNT(*) FILTER (WHERE has_author = TRUE),
		COUNT(*) FILTER (WHERE has_breadcrumb = TRUE)
		FROM page_content_snapshots WHERE site_id = $1
		AND snapshot_date = (SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id = $1)`,
		siteID).Scan(&total, &thin, &noH1, &noSchema, &noMeta, &withAuthor, &withBreadcrumb)

	var orphans int
	db.DB.QueryRow(`SELECT COUNT(*) FROM internal_link_stats WHERE site_id=$1 AND is_orphan=TRUE
		AND computed_date=(SELECT MAX(computed_date) FROM internal_link_stats WHERE site_id=$1)`, siteID).Scan(&orphans)

	// Top pages sorted by word_count ascending (worst first)
	var pages []map[string]interface{}
	scanMapRows(&pages, `SELECT url, word_count, h1, image_count, internal_link_count, external_link_count,
		has_schema_article, has_author, has_breadcrumb, content_hash
		FROM page_content_snapshots WHERE site_id=$1
		AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1)
		ORDER BY word_count ASC LIMIT 100`, siteID)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"total": total, "thin": thin, "no_h1": noH1, "no_schema": noSchema,
		"no_meta": noMeta, "with_author": withAuthor, "with_breadcrumb": withBreadcrumb,
		"orphans": orphans, "pages": pages,
	}})
}

// GetRuleFindings returns rule_findings for a site (separate from audit_findings).
func GetRuleFindings(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	var findings []map[string]interface{}
	scanMapRows(&findings, `SELECT id, rule_id, scope_type, target_url, target_query, severity,
		priority_score, confidence, suggestion_text, status, detected_at::text
		FROM rule_findings WHERE site_id=$1
		ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,
		priority_score DESC LIMIT 200`, siteID)

	// Summary by rule
	var summary []map[string]interface{}
	scanMapRows(&summary, `SELECT rule_id, severity, COUNT(*) as count
		FROM rule_findings WHERE site_id=$1 AND status='open'
		GROUP BY rule_id, severity ORDER BY count DESC`, siteID)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"findings": findings, "summary": summary,
	}})
}

// GetBacklinkProfile returns backlink profile from backlinks_all + referring_domains.
func GetBacklinkProfile(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	// Profile from backlink_profile_daily (latest)
	var profile map[string]interface{}
	rows := []map[string]interface{}{}
	scanMapRows(&rows, `SELECT total_backlinks, total_referring_domains, dofollow_count, nofollow_count,
		new_links_24h, lost_links_24h, suspicious_domain_count, date::text
		FROM backlink_profile_daily WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID)
	if len(rows) > 0 {
		profile = rows[0]
	}

	// If no aggregated profile, compute from raw tables
	if profile == nil {
		var totalBL, totalRD, dofollow, suspicious int
		// Try backlinks_all first
		db.DB.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT source_domain), COUNT(*) FILTER (WHERE rel_attr='dofollow'), COUNT(*) FILTER (WHERE is_suspicious)
			FROM backlinks_all WHERE site_id=$1`, siteID).Scan(&totalBL, &totalRD, &dofollow, &suspicious)
		if totalBL == 0 {
			// Fallback to bing_inbound_links + yandex_external_links
			db.DB.QueryRow(`SELECT COUNT(*) FROM bing_inbound_links WHERE site_id=$1`, siteID).Scan(&totalBL)
			var yCount int
			db.DB.QueryRow(`SELECT COUNT(*) FROM yandex_external_links WHERE site_id=$1`, siteID).Scan(&yCount)
			totalBL += yCount
			dofollow = totalBL // assume all dofollow from platform APIs
		}
		profile = map[string]interface{}{
			"total_backlinks": totalBL, "total_referring_domains": totalRD,
			"dofollow_count": dofollow, "suspicious_domain_count": suspicious,
		}
	}

	// Top referring domains
	var domains []map[string]interface{}
	scanMapRows(&domains, `SELECT domain, backlink_count, dofollow_count, tld, is_toxic, first_seen::text, last_seen::text
		FROM referring_domains WHERE site_id=$1 ORDER BY backlink_count DESC LIMIT 50`, siteID)

	// If no referring_domains data, fall back to bing_inbound_links
	if len(domains) == 0 {
		scanMapRows(&domains, `SELECT source_url as domain, COUNT(*) as backlink_count
			FROM bing_inbound_links WHERE site_id=$1
			GROUP BY source_url ORDER BY COUNT(*) DESC LIMIT 50`, siteID)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"profile": profile, "domains": domains,
	}})
}
