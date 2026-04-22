package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"seo-system/internal/ai"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GenerateSiteAnalysisReport creates a comprehensive SEO analysis report for a site.
// Uses health score + findings + anomalies + platform data + AI to produce a 10-block report.
func GenerateSiteAnalysisReport(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	sid, _ := strconv.ParseInt(siteID, 10, 64)
	reportType := c.DefaultQuery("type", "monthly")

	var domain, siteName string
	db.DB.QueryRow("SELECT domain, name FROM sites WHERE id=$1", sid).Scan(&domain, &siteName)

	// Determine period
	now := time.Now()
	var periodStart, periodEnd string
	switch reportType {
	case "weekly":
		periodStart = now.AddDate(0, 0, -7).Format("2006-01-02")
		periodEnd = now.AddDate(0, 0, -1).Format("2006-01-02")
	case "daily":
		periodStart = now.AddDate(0, 0, -1).Format("2006-01-02")
		periodEnd = periodStart
	default: // monthly
		periodStart = now.AddDate(0, -1, 0).Format("2006-01-02")
		periodEnd = now.AddDate(0, 0, -1).Format("2006-01-02")
	}

	// Create report record
	var reportID int64
	err := db.DB.QueryRow(`INSERT INTO site_analysis_reports (site_id, report_type, period_start, period_end, status)
		VALUES ($1, $2, $3, $4, 'generating') RETURNING id`,
		sid, reportType, periodStart, periodEnd).Scan(&reportID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建报告失败"})
		return
	}

	// Generate in background
	go func() {
		generateReportBlocks(reportID, sid, domain, siteName, periodStart, periodEnd, reportType)
	}()

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"report_id": reportID,
		"message":   "报告正在生成中，请稍后查看",
	}})
}

func generateReportBlocks(reportID, siteID int64, domain, siteName, periodStart, periodEnd, reportType string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[report] panic generating report %d: %v", reportID, r)
			db.DB.Exec("UPDATE site_analysis_reports SET status='failed' WHERE id=$1", reportID)
		}
	}()

	// Collect all data for the prompt
	var data reportData
	data.Domain = domain
	data.SiteName = siteName
	data.Period = fmt.Sprintf("%s ~ %s", periodStart, periodEnd)

	// Health score
	db.DB.QueryRow(`SELECT total_score, grade, delta, sub_scores_json::text
		FROM site_health_scores WHERE site_id=$1 ORDER BY date DESC LIMIT 1`, siteID).Scan(
		&data.HealthScore, &data.Grade, &data.Delta, &data.SubScoresJSON)

	// KPIs from gsc_data
	db.DB.QueryRow(`SELECT COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0)
		FROM gsc_data WHERE site_id=$1 AND date >= $2 AND date <= $3`,
		siteID, periodStart, periodEnd).Scan(&data.Clicks, &data.Impressions)

	// Top findings
	rows, _ := db.DB.Query(`SELECT severity, module, title, page_url FROM audit_findings
		WHERE site_id=$1 AND status='open' ORDER BY
		CASE severity WHEN '致命' THEN 1 WHEN 'critical' THEN 1 WHEN '严重' THEN 2 WHEN 'high' THEN 2 ELSE 3 END
		LIMIT 15`, siteID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var f finding
			rows.Scan(&f.Severity, &f.Module, &f.Title, &f.PageURL)
			data.Findings = append(data.Findings, f)
		}
	}

	// Anomalies
	rows2, _ := db.DB.Query(`SELECT metric_name, direction, deviation_sigma, severity
		FROM anomaly_events WHERE site_id=$1 AND event_date >= $2
		ORDER BY abs(deviation_sigma) DESC LIMIT 10`, siteID, periodStart)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var a anomalyInfo
			rows2.Scan(&a.Metric, &a.Direction, &a.Sigma, &a.Severity)
			data.Anomalies = append(data.Anomalies, a)
		}
	}

	// Rank distribution
	var rankDist string
	db.DB.QueryRow(`SELECT string_agg(bucket || ':' || query_count || '词/' || total_clicks || '点击', ', ' ORDER BY
		CASE bucket WHEN '1-3' THEN 1 WHEN '4-10' THEN 2 WHEN '11-20' THEN 3 ELSE 4 END)
		FROM gsc_rank_buckets_daily WHERE site_id=$1
		AND date=(SELECT MAX(date) FROM gsc_rank_buckets_daily WHERE site_id=$1)`, siteID).Scan(&rankDist)
	data.RankDist = rankDist

	// Opportunities count
	db.DB.QueryRow(`SELECT COUNT(*) FROM gsc_opportunities_weekly WHERE site_id=$1`, siteID).Scan(&data.OpportunityCount)

	// Cannibalization count
	db.DB.QueryRow(`SELECT COUNT(*) FROM gsc_cannibalization_monthly WHERE site_id=$1`, siteID).Scan(&data.CannibalCount)

	// Dead links
	db.DB.QueryRow(`SELECT COUNT(*) FROM dead_link_results WHERE site_id=$1 AND status_code >= 400`, siteID).Scan(&data.DeadLinks)

	// Build AI prompt
	prompt := buildReportPrompt(data)

	// Call AI
	ownerID := int64(0)
	db.DB.QueryRow("SELECT COALESCE(created_by,0) FROM sites WHERE id=$1", siteID).Scan(&ownerID)
	aiURL, aiKey, aiModel := ai.ResolveUserAI(ownerID)

	if aiKey == "" {
		// No AI config — generate a template-based report
		markdown := generateTemplateReport(data)
		saveReport(reportID, siteID, data, markdown, "template")
		return
	}

	// AI generation
	aiResp, err := ai.CallSimple(aiURL, aiKey, aiModel, prompt, 4096)
	if err != nil {
		log.Printf("[report] AI call failed for report %d: %v", reportID, err)
		// Fallback to template
		markdown := generateTemplateReport(data)
		saveReport(reportID, siteID, data, markdown, "template_fallback")
		return
	}

	saveReport(reportID, siteID, data, aiResp, aiModel)
}

type reportData struct {
	Domain           string
	SiteName         string
	Period           string
	HealthScore      int
	Grade            string
	Delta            int
	SubScoresJSON    string
	Clicks           int
	Impressions      int
	Findings         []finding
	Anomalies        []anomalyInfo
	RankDist         string
	OpportunityCount int
	CannibalCount    int
	DeadLinks        int
}

type finding struct {
	Severity string
	Module   string
	Title    string
	PageURL  string
}

type anomalyInfo struct {
	Metric    string
	Direction string
	Sigma     float64
	Severity  string
}

func buildReportPrompt(d reportData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`你是一位资深 SEO 分析师。请为站点「%s」(%s) 生成一份 %s 的 SEO 分析报告。

## 数据摘要

- 健康评分: %d/100 (等级: %s, 变化: %+d)
- 子维度: %s
- 总点击: %d, 总曝光: %d
- 排名分布: %s
- 优化机会: %d 个
- 自蚕食关键词: %d 个
- 死链: %d 个

`, d.SiteName, d.Domain, d.Period,
		d.HealthScore, d.Grade, d.Delta,
		d.SubScoresJSON, d.Clicks, d.Impressions,
		d.RankDist, d.OpportunityCount, d.CannibalCount, d.DeadLinks))

	if len(d.Findings) > 0 {
		sb.WriteString("## 待处理问题 TOP 15\n\n")
		for i, f := range d.Findings {
			sb.WriteString(fmt.Sprintf("%d. [%s] %s — %s", i+1, f.Severity, f.Title, f.Module))
			if f.PageURL != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", f.PageURL))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(d.Anomalies) > 0 {
		sb.WriteString("## 异常事件\n\n")
		for _, a := range d.Anomalies {
			sb.WriteString(fmt.Sprintf("- %s %s (偏差 %.1fσ, %s)\n", a.Metric, a.Direction, a.Sigma, a.Severity))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`## 请输出以下 10 个板块的分析报告（Markdown 格式）

1. **摘要**: 一段话总结本期 SEO 状态
2. **核心指标**: 点击/曝光/CTR/排名的变化分析
3. **排名分布**: 各排名段的变化和趋势
4. **CTR 诊断**: CTR 是否偏低及原因
5. **收录诊断**: 收录状态分析
6. **技术健康**: 技术问题汇总和修复建议
7. **内容健康**: 内容质量/新鲜度/重复度
8. **外链分析**: 反链健康度
9. **竞品差异**: 与竞品的差距（如有数据）
10. **策略建议**: 未来 30 天优先行动清单（5-8 项，按优先级排序）

要求：
- 用数据说话，引用具体数字
- 问题要给出具体的修复建议
- 策略要可执行，不要空泛
- 每个板块 100-200 字
`)

	return sb.String()
}

func generateTemplateReport(d reportData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s SEO 分析报告\n\n", d.SiteName))
	sb.WriteString(fmt.Sprintf("**周期**: %s\n\n", d.Period))

	sb.WriteString("## 1. 摘要\n\n")
	sb.WriteString(fmt.Sprintf("站点健康评分 **%d/100** (等级 %s, 较上期 %+d)。", d.HealthScore, d.Grade, d.Delta))
	sb.WriteString(fmt.Sprintf("本期总点击 %d，总曝光 %d。", d.Clicks, d.Impressions))
	if len(d.Findings) > 0 {
		sb.WriteString(fmt.Sprintf("发现 %d 个待处理问题。", len(d.Findings)))
	}
	if d.OpportunityCount > 0 {
		sb.WriteString(fmt.Sprintf("识别 %d 个优化机会。", d.OpportunityCount))
	}
	sb.WriteString("\n\n")

	sb.WriteString("## 2. 核心指标\n\n")
	sb.WriteString(fmt.Sprintf("- 总点击: %d\n- 总曝光: %d\n\n", d.Clicks, d.Impressions))

	sb.WriteString("## 3. 排名分布\n\n")
	if d.RankDist != "" {
		sb.WriteString(d.RankDist + "\n\n")
	} else {
		sb.WriteString("暂无排名分布数据\n\n")
	}

	sb.WriteString("## 4-5. CTR & 收录\n\n")
	sb.WriteString(fmt.Sprintf("待深度同步后完善。\n\n"))

	sb.WriteString("## 6. 技术问题\n\n")
	if len(d.Findings) > 0 {
		for i, f := range d.Findings {
			if i >= 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", f.Severity, f.Title, f.Module))
		}
	} else {
		sb.WriteString("暂无审计问题数据\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## 7-8. 内容 & 外链\n\n")
	sb.WriteString(fmt.Sprintf("死链数: %d\n\n", d.DeadLinks))

	sb.WriteString("## 9. 竞品\n\n暂无竞品数据\n\n")

	sb.WriteString("## 10. 策略建议\n\n")
	sb.WriteString("1. 修复所有致命/严重技术问题\n")
	sb.WriteString("2. 清理死链\n")
	if d.CannibalCount > 0 {
		sb.WriteString(fmt.Sprintf("3. 处理 %d 个自蚕食关键词\n", d.CannibalCount))
	}
	if d.OpportunityCount > 0 {
		sb.WriteString(fmt.Sprintf("4. 优化 %d 个 CTR 机会词\n", d.OpportunityCount))
	}

	return sb.String()
}

func saveReport(reportID, siteID int64, data reportData, markdown, aiModel string) {
	// Parse markdown into blocks (simple heuristic: split by ## headers)
	blocks := parseMarkdownBlocks(markdown)

	summaryJSON, _ := json.Marshal(blocks["摘要"])
	metricsJSON, _ := json.Marshal(blocks["核心指标"])
	rankingJSON, _ := json.Marshal(blocks["排名分布"])
	ctrJSON, _ := json.Marshal(blocks["CTR"])
	idxJSON, _ := json.Marshal(blocks["收录"])
	techJSON, _ := json.Marshal(blocks["技术"])
	contentJSON, _ := json.Marshal(blocks["内容"])
	blJSON, _ := json.Marshal(blocks["外链"])
	compJSON, _ := json.Marshal(blocks["竞品"])
	stratJSON, _ := json.Marshal(blocks["策略"])

	db.DB.Exec(`UPDATE site_analysis_reports SET
		status='ready', total_score=$2, score_delta=$3,
		critical_findings=$4, opportunity_count=$5,
		block_summary=$6, block_metrics=$7, block_ranking=$8,
		block_ctr=$9, block_indexation=$10, block_technical=$11,
		block_content=$12, block_backlinks=$13, block_competitors=$14,
		block_strategy=$15, markdown_text=$16, ai_model=$17, generated_at=NOW()
		WHERE id=$1`,
		reportID, data.HealthScore, data.Delta,
		len(data.Findings), data.OpportunityCount,
		string(summaryJSON), string(metricsJSON), string(rankingJSON),
		string(ctrJSON), string(idxJSON), string(techJSON),
		string(contentJSON), string(blJSON), string(compJSON),
		string(stratJSON), markdown, aiModel)

	log.Printf("[report] Report %d saved (model=%s, len=%d)", reportID, aiModel, len(markdown))
}

func parseMarkdownBlocks(md string) map[string]string {
	blocks := make(map[string]string)
	lines := strings.Split(md, "\n")
	currentKey := ""
	var currentContent strings.Builder

	keywords := map[string]string{
		"摘要": "摘要", "核心指标": "核心指标", "排名": "排名分布",
		"CTR": "CTR", "收录": "收录", "技术": "技术",
		"内容": "内容", "外链": "外链", "竞品": "竞品", "策略": "策略",
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			if currentKey != "" {
				blocks[currentKey] = strings.TrimSpace(currentContent.String())
			}
			currentKey = ""
			currentContent.Reset()
			header := strings.TrimLeft(line, "# ")
			for kw, key := range keywords {
				if strings.Contains(header, kw) {
					currentKey = key
					break
				}
			}
		} else if currentKey != "" {
			currentContent.WriteString(line + "\n")
		}
	}
	if currentKey != "" {
		blocks[currentKey] = strings.TrimSpace(currentContent.String())
	}
	return blocks
}

// ── API Endpoints ──

// ListSiteReports returns all reports for a site
func ListSiteReports(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}
	var reports []map[string]interface{}
	scanMapRows(&reports, `SELECT id, report_type, period_start::text, period_end::text,
		total_score, score_delta, status, generated_at::text, ai_model
		FROM site_analysis_reports WHERE site_id=$1 ORDER BY generated_at DESC LIMIT 50`, siteID)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": reports})
}

// GetSiteReport returns a specific report
func GetSiteReport(c *gin.Context) {
	reportID := c.Param("reportId")
	var report struct {
		ID             int64  `json:"id"`
		SiteID         int64  `json:"site_id"`
		ReportType     string `json:"report_type"`
		PeriodStart    string `json:"period_start"`
		PeriodEnd      string `json:"period_end"`
		TotalScore     int    `json:"total_score"`
		ScoreDelta     int    `json:"score_delta"`
		Status         string `json:"status"`
		MarkdownText   string `json:"markdown_text"`
		AIModel        string `json:"ai_model"`
		GeneratedAt    string `json:"generated_at"`
	}
	err := db.DB.QueryRow(`SELECT id, site_id, report_type, period_start::text, period_end::text,
		total_score, score_delta, status, markdown_text, ai_model, generated_at::text
		FROM site_analysis_reports WHERE id=$1`, reportID).Scan(
		&report.ID, &report.SiteID, &report.ReportType, &report.PeriodStart, &report.PeriodEnd,
		&report.TotalScore, &report.ScoreDelta, &report.Status, &report.MarkdownText,
		&report.AIModel, &report.GeneratedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "报告不存在"})
		return
	}
	if !canAccessSite(c, strconv.FormatInt(report.SiteID, 10)) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": report})
}
