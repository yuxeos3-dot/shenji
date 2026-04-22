package api

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ComputeSiteHealthScore calculates a 0-100 health score for a site.
// It reads from all available data sources and produces a composite score.
func ComputeSiteHealthScore(siteID int64) {
	date := time.Now().Format("2006-01-02")
	var s healthScoreBuilder
	s.siteID = siteID
	s.date = date

	// ── 1. Traffic score (from gsc_data / ga_data) ──
	var gscClicks, gscImpressions int
	var gscCTR, gscPosition float64
	db.DB.QueryRow(`SELECT COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
		CASE WHEN SUM(impressions)>0 THEN SUM(clicks)::float/SUM(impressions) ELSE 0 END,
		CASE WHEN SUM(impressions)>0 THEN AVG(position) ELSE 0 END
		FROM gsc_data WHERE site_id=$1 AND date >= CURRENT_DATE - INTERVAL '30 days'`, siteID).Scan(
		&gscClicks, &gscImpressions, &gscCTR, &gscPosition)

	// Also try deep table
	var deepClicks int
	db.DB.QueryRow(`SELECT COALESCE(SUM(clicks),0) FROM gsc_query_page_daily
		WHERE site_id=$1 AND date >= CURRENT_DATE - INTERVAL '30 days' AND search_type='web'`, siteID).Scan(&deepClicks)
	if deepClicks > gscClicks {
		gscClicks = deepClicks
	}

	s.trafficScore = clampScore(scoreFromTraffic(gscClicks))

	// ── 2. Ranking score (from rank buckets or gsc position) ──
	var top3, top10, top20 int
	db.DB.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN bucket='1-3' THEN query_count ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN bucket IN ('1-3','4-10') THEN query_count ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN bucket IN ('1-3','4-10','11-20') THEN query_count ELSE 0 END),0)
		FROM gsc_rank_buckets_daily WHERE site_id=$1
		AND date=(SELECT MAX(date) FROM gsc_rank_buckets_daily WHERE site_id=$1)`, siteID).Scan(&top3, &top10, &top20)

	if top20 > 0 {
		s.rankingScore = clampScore(int(float64(top10) / float64(top20) * 100))
	} else if gscPosition > 0 {
		s.rankingScore = clampScore(int(math.Max(0, 100-gscPosition*3)))
	}

	// ── 3. CTR score ──
	if gscCTR > 0 {
		s.ctrScore = clampScore(int(gscCTR * 100 * 10)) // 10% CTR = 100分
	}

	// ── 4. Technical score (from audit_results) ──
	var criticalCount, seriousCount, totalFindings int
	db.DB.QueryRow(`SELECT
		COALESCE(critical_count,0), COALESCE(serious_count,0), COALESCE(total_findings,0)
		FROM audit_results WHERE site_id IN (SELECT id FROM audit_tasks WHERE site_id=$1)
		ORDER BY created_at DESC LIMIT 1`, siteID).Scan(&criticalCount, &seriousCount, &totalFindings)

	// Also count from audit_findings if available
	var afCritical, afSerious int
	db.DB.QueryRow(`SELECT
		COUNT(*) FILTER (WHERE severity IN ('致命','critical')),
		COUNT(*) FILTER (WHERE severity IN ('严重','serious','high'))
		FROM audit_findings WHERE site_id=$1 AND status='open'`, siteID).Scan(&afCritical, &afSerious)
	if afCritical > criticalCount {
		criticalCount = afCritical
	}
	if afSerious > seriousCount {
		seriousCount = afSerious
	}

	techPenalty := criticalCount*20 + seriousCount*10
	s.technicalScore = clampScore(100 - techPenalty)

	// ── 5. Indexation score (from gsc_url_inspection / yandex) ──
	var totalInspected, passCount int
	db.DB.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE verdict='PASS')
		FROM gsc_url_inspection WHERE site_id=$1`, siteID).Scan(&totalInspected, &passCount)

	if totalInspected > 0 {
		s.indexationScore = clampScore(int(float64(passCount) / float64(totalInspected) * 100))
	} else {
		s.indexationScore = 50 // Unknown
	}

	// ── 6. Content score (pages with word_count, freshness) ──
	var pagesCount, lowContent int
	db.DB.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE word_count < 300)
		FROM page_content_snapshots WHERE site_id=$1
		AND snapshot_date=(SELECT MAX(snapshot_date) FROM page_content_snapshots WHERE site_id=$1)`,
		siteID).Scan(&pagesCount, &lowContent)
	if pagesCount > 0 {
		s.contentScore = clampScore(int((1 - float64(lowContent)/float64(pagesCount)) * 100))
	} else {
		s.contentScore = 50
	}

	// ── 7. Backlink score ──
	var totalBacklinks, suspiciousCount int
	db.DB.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE is_suspicious)
		FROM backlinks_all WHERE site_id=$1`, siteID).Scan(&totalBacklinks, &suspiciousCount)
	if totalBacklinks > 0 {
		healthyPct := float64(totalBacklinks-suspiciousCount) / float64(totalBacklinks)
		s.backlinkScore = clampScore(int(healthyPct * 100))
	} else {
		// Check bing/yandex external links
		var extCount int
		db.DB.QueryRow(`SELECT COUNT(*) FROM bing_inbound_links WHERE site_id=$1`, siteID).Scan(&extCount)
		if extCount > 0 {
			s.backlinkScore = 60
		} else {
			s.backlinkScore = 30
		}
	}

	// ── 8. Security score (dead links, TLS) ──
	var deadLinks int
	db.DB.QueryRow(`SELECT COUNT(*) FROM dead_link_results WHERE site_id=$1 AND status_code >= 400`, siteID).Scan(&deadLinks)
	s.securityScore = clampScore(100 - deadLinks*5)

	// ── 9. UX score (from GA4 bounce rate / engagement) ──
	var bounceRate float64
	db.DB.QueryRow(`SELECT COALESCE(bounce_rate,0) FROM ga_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(&bounceRate)
	if bounceRate > 0 {
		s.uxScore = clampScore(int((1 - bounceRate) * 100))
	} else {
		s.uxScore = 50
	}

	// ── 10. Stability score (anomaly count in last 30 days) ──
	var anomalyCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM anomaly_events WHERE site_id=$1
		AND event_date >= CURRENT_DATE - INTERVAL '30 days' AND severity IN ('critical','high')`,
		siteID).Scan(&anomalyCount)
	s.stabilityScore = clampScore(100 - anomalyCount*15)

	// ── Compute total ──
	weights := map[string]float64{
		"traffic": 0.15, "ranking": 0.15, "ctr": 0.10, "technical": 0.15,
		"indexation": 0.10, "content": 0.10, "backlink": 0.08, "security": 0.05,
		"ux": 0.07, "stability": 0.05,
	}
	total := weights["traffic"]*float64(s.trafficScore) +
		weights["ranking"]*float64(s.rankingScore) +
		weights["ctr"]*float64(s.ctrScore) +
		weights["technical"]*float64(s.technicalScore) +
		weights["indexation"]*float64(s.indexationScore) +
		weights["content"]*float64(s.contentScore) +
		weights["backlink"]*float64(s.backlinkScore) +
		weights["security"]*float64(s.securityScore) +
		weights["ux"]*float64(s.uxScore) +
		weights["stability"]*float64(s.stabilityScore)
	s.totalScore = clampScore(int(total))

	// Grade
	switch {
	case s.totalScore >= 90:
		s.grade = "S"
	case s.totalScore >= 80:
		s.grade = "A"
	case s.totalScore >= 70:
		s.grade = "B"
	case s.totalScore >= 60:
		s.grade = "C"
	case s.totalScore >= 40:
		s.grade = "D"
	default:
		s.grade = "F"
	}

	// Previous score for delta
	var prevScore int
	db.DB.QueryRow(`SELECT total_score FROM site_health_scores WHERE site_id=$1 AND date < $2 ORDER BY date DESC LIMIT 1`,
		siteID, date).Scan(&prevScore)
	s.delta = s.totalScore - prevScore

	// Save
	subJSON, _ := json.Marshal(map[string]int{
		"traffic": s.trafficScore, "ranking": s.rankingScore, "ctr": s.ctrScore,
		"technical": s.technicalScore, "indexation": s.indexationScore, "content": s.contentScore,
		"backlink": s.backlinkScore, "security": s.securityScore, "ux": s.uxScore, "stability": s.stabilityScore,
	})

	db.DB.Exec(`INSERT INTO site_health_scores (site_id, date, total_score, indexation_score, ranking_score,
		traffic_score, ctr_score, technical_score, content_score, backlink_score, ux_score, security_score,
		stability_score, prev_period_score, delta, grade, warning_count, critical_count, sub_scores_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (site_id, date, period) DO UPDATE SET
		total_score=EXCLUDED.total_score, indexation_score=EXCLUDED.indexation_score,
		ranking_score=EXCLUDED.ranking_score, traffic_score=EXCLUDED.traffic_score,
		ctr_score=EXCLUDED.ctr_score, technical_score=EXCLUDED.technical_score,
		content_score=EXCLUDED.content_score, backlink_score=EXCLUDED.backlink_score,
		ux_score=EXCLUDED.ux_score, security_score=EXCLUDED.security_score,
		stability_score=EXCLUDED.stability_score, prev_period_score=EXCLUDED.prev_period_score,
		delta=EXCLUDED.delta, grade=EXCLUDED.grade, warning_count=EXCLUDED.warning_count,
		critical_count=EXCLUDED.critical_count, sub_scores_json=EXCLUDED.sub_scores_json,
		computed_at=CURRENT_TIMESTAMP`,
		siteID, date, s.totalScore, s.indexationScore, s.rankingScore,
		s.trafficScore, s.ctrScore, s.technicalScore, s.contentScore, s.backlinkScore,
		s.uxScore, s.securityScore, s.stabilityScore, prevScore, s.delta, s.grade,
		seriousCount, criticalCount, string(subJSON))

	// Also snapshot key metrics
	snapshotMetric(siteID, date, "clicks", "gsc", float64(gscClicks))
	snapshotMetric(siteID, date, "impressions", "gsc", float64(gscImpressions))
	snapshotMetric(siteID, date, "ctr", "gsc", gscCTR)
	snapshotMetric(siteID, date, "position", "gsc", gscPosition)

	log.Printf("[health] site %d: score=%d grade=%s delta=%+d", siteID, s.totalScore, s.grade, s.delta)
}

func snapshotMetric(siteID int64, date, metric, platform string, value float64) {
	db.DB.Exec(`INSERT INTO metric_snapshots_daily (site_id, date, metric_name, platform, value)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (site_id, date, metric_name, platform) DO UPDATE SET value=EXCLUDED.value`,
		siteID, date, metric, platform, value)
}

type healthScoreBuilder struct {
	siteID                                                                          int64
	date                                                                            string
	totalScore, trafficScore, rankingScore, ctrScore, technicalScore                int
	indexationScore, contentScore, backlinkScore, uxScore, securityScore, stabilityScore int
	delta                                                                           int
	grade                                                                           string
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func scoreFromTraffic(clicks int) int {
	if clicks <= 0 {
		return 0
	}
	// Log scale: 10 clicks = 20, 100 = 40, 1000 = 60, 10000 = 80, 100000 = 100
	return clampScore(int(math.Log10(float64(clicks)) * 20))
}

// ── API endpoints ──

func GetSiteHealthScore(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	var score struct {
		TotalScore      int    `json:"total_score"`
		Grade           string `json:"grade"`
		Delta           int    `json:"delta"`
		IndexationScore int    `json:"indexation_score"`
		RankingScore    int    `json:"ranking_score"`
		TrafficScore    int    `json:"traffic_score"`
		CTRScore        int    `json:"ctr_score"`
		TechnicalScore  int    `json:"technical_score"`
		ContentScore    int    `json:"content_score"`
		BacklinkScore   int    `json:"backlink_score"`
		UXScore         int    `json:"ux_score"`
		SecurityScore   int    `json:"security_score"`
		StabilityScore  int    `json:"stability_score"`
		WarningCount    int    `json:"warning_count"`
		CriticalCount   int    `json:"critical_count"`
		Date            string `json:"date"`
		ComputedAt      string `json:"computed_at"`
	}

	err := db.DB.QueryRow(`SELECT total_score, grade, delta, indexation_score, ranking_score,
		traffic_score, ctr_score, technical_score, content_score, backlink_score,
		ux_score, security_score, stability_score, warning_count, critical_count,
		date::text, computed_at::text
		FROM site_health_scores WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
		&score.TotalScore, &score.Grade, &score.Delta, &score.IndexationScore, &score.RankingScore,
		&score.TrafficScore, &score.CTRScore, &score.TechnicalScore, &score.ContentScore,
		&score.BacklinkScore, &score.UXScore, &score.SecurityScore, &score.StabilityScore,
		&score.WarningCount, &score.CriticalCount, &score.Date, &score.ComputedAt)
	if err != nil {
		// No score yet — compute one now
		sid, _ := strconv.ParseInt(siteID, 10, 64)
		ComputeSiteHealthScore(sid)
		// Re-query
		db.DB.QueryRow(`SELECT total_score, grade, delta, indexation_score, ranking_score,
			traffic_score, ctr_score, technical_score, content_score, backlink_score,
			ux_score, security_score, stability_score, warning_count, critical_count,
			date::text, computed_at::text
			FROM site_health_scores WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
			&score.TotalScore, &score.Grade, &score.Delta, &score.IndexationScore, &score.RankingScore,
			&score.TrafficScore, &score.CTRScore, &score.TechnicalScore, &score.ContentScore,
			&score.BacklinkScore, &score.UXScore, &score.SecurityScore, &score.StabilityScore,
			&score.WarningCount, &score.CriticalCount, &score.Date, &score.ComputedAt)
	}

	// History trend (last 30 days)
	var history []map[string]interface{}
	rows, _ := db.DB.Query(`SELECT date::text, total_score, grade FROM site_health_scores
		WHERE site_id=$1 ORDER BY date DESC LIMIT 30`, siteID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var d, g string
			var s int
			rows.Scan(&d, &s, &g)
			history = append(history, map[string]interface{}{"date": d, "score": s, "grade": g})
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"score": score, "history": history,
	}})
}

// ComputeAndReturnHealth computes fresh health score then returns it
func ComputeAndReturnHealth(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	ComputeSiteHealthScore(sid)
	GetSiteHealthScore(c)
}

// GetSiteAnalysisSummary returns a comprehensive analysis summary for a site
// combining health score + top findings + anomalies + timeline
func GetSiteAnalysisSummary(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	// Health score
	var totalScore int
	var grade string
	var delta int
	var subJSON string
	db.DB.QueryRow(`SELECT total_score, grade, delta, COALESCE(sub_scores_json::text,'{}')
		FROM site_health_scores WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
		&totalScore, &grade, &delta, &subJSON)

	// Top findings (from audit_findings)
	var findings []map[string]interface{}
	scanMapRows(&findings, `SELECT title, severity, module, page_url, status, detected_at::text
		FROM audit_findings WHERE site_id=$1 AND status='open'
		ORDER BY CASE severity WHEN '致命' THEN 1 WHEN 'critical' THEN 1 WHEN '严重' THEN 2 WHEN 'high' THEN 2 ELSE 3 END,
		detected_at DESC LIMIT 20`, siteID)

	// Recent anomalies
	var anomalies []map[string]interface{}
	scanMapRows(&anomalies, `SELECT metric_name, platform, severity, direction, event_date::text, observed_value, expected_value, deviation_sigma, status
		FROM anomaly_events WHERE site_id=$1 ORDER BY event_date DESC LIMIT 10`, siteID)

	// Timeline events
	var timeline []map[string]interface{}
	scanMapRows(&timeline, `SELECT event_category, event_type, title, severity, occurred_at::text
		FROM timeline_events WHERE (site_id=$1 OR site_id=0) ORDER BY occurred_at DESC LIMIT 20`, siteID)

	// Recent audit info
	var lastAuditDate string
	var lastAuditStatus string
	db.DB.QueryRow(`SELECT created_at::text, status FROM audit_tasks WHERE site_id=$1 ORDER BY id DESC LIMIT 1`,
		siteID).Scan(&lastAuditDate, &lastAuditStatus)

	// Dead links count
	var deadLinks int
	db.DB.QueryRow(`SELECT COUNT(*) FROM dead_link_results WHERE site_id=$1 AND status_code >= 400`, siteID).Scan(&deadLinks)

	// Opportunities count
	var oppCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM gsc_opportunities_weekly WHERE site_id=$1`, siteID).Scan(&oppCount)

	// Cannibalization count
	var cannibalCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM gsc_cannibalization_monthly WHERE site_id=$1`, siteID).Scan(&cannibalCount)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"health_score":    totalScore,
		"grade":           grade,
		"delta":           delta,
		"sub_scores":      json.RawMessage(subJSON),
		"findings":        findings,
		"anomalies":       anomalies,
		"timeline":        timeline,
		"last_audit_date": lastAuditDate,
		"last_audit_status": lastAuditStatus,
		"dead_links":      deadLinks,
		"opportunities":   oppCount,
		"cannibalization":  cannibalCount,
	}})
}
