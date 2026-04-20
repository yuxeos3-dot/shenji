package api

// Global Analytics / Strategy Intelligence.
//
// This endpoint is the product's "one screen that pulls everything
// together". It answers: "across every user, every site, every
// platform — what's going well, what's broken, where are the biggest
// opportunities, and what should we do about it?"
//
// The backend is split in two handlers:
//
//   GET  /v1/admin/analytics/snapshot
//     Pure aggregation — a single read-mostly query per data family
//     over the local DB. Cheap; called on page load.
//
//   POST /v1/admin/analytics/ai-advise
//     Takes the snapshot, builds a comprehensive Chinese prompt, and
//     sends it through the caller's own AI config
//     (CallAIWithUserConfig, same path manual audit uses). Returns
//     the AI's markdown answer for the frontend to render.
//
// Admin-only, enforced at the route layer (AdminRequired in main.go).

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetAnalyticsSnapshot returns every aggregated metric the Global
// Analytics page renders. All queries run against the local DB so
// response time stays predictable even when the 4 platforms are down.
func GetAnalyticsSnapshot(c *gin.Context) {
	out := gin.H{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}

	// ── 1. 站点规模 ────────────────────────────────────────────────
	var totalSites, activeSites, auditedSites int
	db.DB.QueryRow(`SELECT COUNT(*) FROM sites`).Scan(&totalSites)
	db.DB.QueryRow(`SELECT COUNT(*) FROM sites WHERE status = 'active'`).Scan(&activeSites)
	db.DB.QueryRow(`
		SELECT COUNT(DISTINCT site_id) FROM audit_tasks WHERE status = 'done'
	`).Scan(&auditedSites)
	out["sites"] = gin.H{
		"total":   totalSites,
		"active":  activeSites,
		"audited": auditedSites,
	}

	// ── 2. 平台绑定 ────────────────────────────────────────────────
	platforms := gin.H{}
	for _, svc := range []string{"gsc", "ga", "yandex"} {
		var n int
		db.DB.QueryRow(
			`SELECT COUNT(*) FROM user_oauth_tokens WHERE service = $1`,
			svc,
		).Scan(&n)
		platforms[svc] = n
	}
	var bingN int
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM user_bing WHERE api_key IS NOT NULL AND api_key <> ''`,
	).Scan(&bingN)
	platforms["bing"] = bingN
	out["platforms"] = platforms

	// ── 3. 内容产出 ────────────────────────────────────────────────
	var articlesTotal, articlesDraft, articlesPublished, articles30d int
	db.DB.QueryRow(`SELECT COUNT(*) FROM geo_articles`).Scan(&articlesTotal)
	db.DB.QueryRow(`SELECT COUNT(*) FROM geo_articles WHERE status = 'draft'`).Scan(&articlesDraft)
	db.DB.QueryRow(`SELECT COUNT(*) FROM geo_articles WHERE status = 'published'`).Scan(&articlesPublished)
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM geo_articles WHERE created_at >= NOW() - INTERVAL '30 days'`,
	).Scan(&articles30d)
	out["content"] = gin.H{
		"total":         articlesTotal,
		"draft":         articlesDraft,
		"published":     articlesPublished,
		"last_30_days":  articles30d,
	}

	// ── 4. 审计健康度 ──────────────────────────────────────────────
	var auditTotal, auditDone, auditError, audit30d int
	db.DB.QueryRow(`SELECT COUNT(*) FROM audit_tasks`).Scan(&auditTotal)
	db.DB.QueryRow(`SELECT COUNT(*) FROM audit_tasks WHERE status = 'done'`).Scan(&auditDone)
	db.DB.QueryRow(`SELECT COUNT(*) FROM audit_tasks WHERE status = 'error'`).Scan(&auditError)
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM audit_tasks WHERE created_at >= NOW() - INTERVAL '30 days'`,
	).Scan(&audit30d)
	out["audits"] = gin.H{
		"total":         auditTotal,
		"done":          auditDone,
		"errored":       auditError,
		"last_30_days":  audit30d,
	}

	// ── 5. 协同任务 ────────────────────────────────────────────────
	coordStats := gin.H{}
	for _, st := range []string{"pending", "in_progress", "awaiting_answer", "under_review", "done"} {
		var n int
		db.DB.QueryRow(
			`SELECT COUNT(*) FROM coordination_tasks WHERE status = $1`, st,
		).Scan(&n)
		coordStats[st] = n
	}
	out["coord_tasks"] = coordStats

	// ── 6. 外链 ────────────────────────────────────────────────────
	var poolSize, siteExtlinks int
	db.DB.QueryRow(`SELECT COUNT(*) FROM extlink_pool`).Scan(&poolSize)
	db.DB.QueryRow(`SELECT COUNT(*) FROM external_links`).Scan(&siteExtlinks)
	out["extlinks"] = gin.H{
		"pool_size":     poolSize,
		"site_extlinks": siteExtlinks,
	}

	// ── 7. 死链 ────────────────────────────────────────────────────
	var deadLinks, deadLinksSites int
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM dead_link_results WHERE status_code >= 400 OR status_code = 0`,
	).Scan(&deadLinks)
	db.DB.QueryRow(
		`SELECT COUNT(DISTINCT site_id) FROM dead_link_results WHERE status_code >= 400 OR status_code = 0`,
	).Scan(&deadLinksSites)
	out["dead_links"] = gin.H{
		"total":          deadLinks,
		"affected_sites": deadLinksSites,
	}

	// ── 8. 蜘蛛活动 ────────────────────────────────────────────────
	var spider7d, spider30d int
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM spider_logs WHERE visited_at >= NOW() - INTERVAL '7 days'`,
	).Scan(&spider7d)
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM spider_logs WHERE visited_at >= NOW() - INTERVAL '30 days'`,
	).Scan(&spider30d)
	topSpiders := []gin.H{}
	rows, err := db.DB.Query(`
		SELECT spider, COUNT(*) AS hits FROM spider_logs
		WHERE visited_at >= NOW() - INTERVAL '30 days'
		GROUP BY spider ORDER BY hits DESC LIMIT 10`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var hits int
			if err := rows.Scan(&name, &hits); err == nil {
				topSpiders = append(topSpiders, gin.H{"spider": name, "hits": hits})
			}
		}
	}
	out["spider"] = gin.H{
		"hits_7d":      spider7d,
		"hits_30d":     spider30d,
		"top_spiders":  topSpiders,
	}

	// ── 9. GSC 汇总 ────────────────────────────────────────────────
	// Roll up across every site's latest gsc_data row. We don't try to
	// be time-window-accurate here — each row is the last sync, which
	// for most users is same-day. "Total across every site's latest
	// snapshot" is the useful admin-level number.
	var gscClicks, gscImpressions int
	var gscCTR, gscPosition float64
	db.DB.QueryRow(`
		WITH latest AS (
			SELECT DISTINCT ON (site_id) site_id, clicks, impressions, ctr, position
			FROM gsc_data ORDER BY site_id, date DESC
		)
		SELECT
			COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
			COALESCE(AVG(ctr),0),    COALESCE(AVG(position),0)
		FROM latest`).Scan(&gscClicks, &gscImpressions, &gscCTR, &gscPosition)
	// Top queries — we store keywords_json per latest-per-site row as
	// a JSON array of {query, clicks, impressions}. Stitch them all
	// together, bucket by query, pick top 15.
	topKeywords := gscTopKeywords(15)
	out["gsc"] = gin.H{
		"total_clicks":      gscClicks,
		"total_impressions": gscImpressions,
		"avg_ctr":           gscCTR,
		"avg_position":      gscPosition,
		"top_keywords":      topKeywords,
	}

	// ── 10. GA4 汇总 ───────────────────────────────────────────────
	var gaSessions, gaUsers, gaPageviews int
	db.DB.QueryRow(`
		WITH latest AS (
			SELECT DISTINCT ON (site_id) site_id, sessions, users, pageviews
			FROM ga_data ORDER BY site_id, date DESC
		)
		SELECT COALESCE(SUM(sessions),0), COALESCE(SUM(users),0), COALESCE(SUM(pageviews),0)
		FROM latest`).Scan(&gaSessions, &gaUsers, &gaPageviews)
	out["ga"] = gin.H{
		"total_sessions":  gaSessions,
		"total_users":     gaUsers,
		"total_pageviews": gaPageviews,
	}

	// ── 11. Bing / Yandex 索引 ─────────────────────────────────────
	var bingIdx, yandexIdx int
	db.DB.QueryRow(`
		SELECT COALESCE(SUM(indexed_pages),0) FROM (
			SELECT DISTINCT ON (site_id) site_id, COALESCE(indexed_pages,0) AS indexed_pages
			FROM bing_data ORDER BY site_id, date DESC
		) t`).Scan(&bingIdx)
	db.DB.QueryRow(`
		SELECT COALESCE(SUM(index_count),0) FROM (
			SELECT DISTINCT ON (site_id) site_id, COALESCE(index_count,0) AS index_count
			FROM yandex_data ORDER BY site_id, date DESC
		) t`).Scan(&yandexIdx)
	out["index"] = gin.H{
		"bing":   bingIdx,
		"yandex": yandexIdx,
	}

	// ── 12. 推送健康度 ─────────────────────────────────────────────
	var pushTotal, pushOK int
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM push_logs WHERE created_at >= NOW() - INTERVAL '30 days'`,
	).Scan(&pushTotal)
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM push_logs
		 WHERE created_at >= NOW() - INTERVAL '30 days' AND status IN ('success','ok')`,
	).Scan(&pushOK)
	successRate := 0.0
	if pushTotal > 0 {
		successRate = float64(pushOK) / float64(pushTotal) * 100.0
	}
	out["push"] = gin.H{
		"total_30d":      pushTotal,
		"success_30d":    pushOK,
		"success_rate":   successRate,
	}

	// ── 13. 用户活跃 ───────────────────────────────────────────────
	var usersTotal, usersActive7d int
	db.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE register_status = 'approved'`).Scan(&usersTotal)
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM users WHERE last_login >= NOW() - INTERVAL '7 days'`,
	).Scan(&usersActive7d)
	out["users"] = gin.H{
		"total":       usersTotal,
		"active_7d":   usersActive7d,
	}

	// ── 14. 预留：竞品抓取 / 完播率 等尚未接入的数据 ────────────────
	// Placeholders so the frontend can render the sections with
	// "数据对接中" badges rather than hard-code what's implemented.
	out["pending_sources"] = []string{
		"competitor_crawl",      // 同行站点深度抓取
		"dwell_time",             // 页面停留时长
		"completion_rate",        // 视频/长文完播率
		"serp_rank_tracking",     // 关键词排位跟踪
		"backlink_velocity",      // 外链增速
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// gscTopKeywords stitches every site's latest keywords_json together,
// buckets by query, and returns the top N by total clicks.
func gscTopKeywords(limit int) []gin.H {
	rows, err := db.DB.Query(`
		SELECT DISTINCT ON (site_id) keywords_json
		FROM gsc_data ORDER BY site_id, date DESC`)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	type row struct {
		Query       string  `json:"query"`
		Clicks      float64 `json:"clicks"`
		Impressions float64 `json:"impressions"`
	}
	agg := map[string]*row{}
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil || blob == "" {
			continue
		}
		var list []row
		if err := json.Unmarshal([]byte(blob), &list); err != nil {
			continue
		}
		for _, k := range list {
			if k.Query == "" {
				continue
			}
			r, ok := agg[k.Query]
			if !ok {
				r = &row{Query: k.Query}
				agg[k.Query] = r
			}
			r.Clicks += k.Clicks
			r.Impressions += k.Impressions
		}
	}
	// Sort desc by clicks then by impressions
	all := make([]*row, 0, len(agg))
	for _, v := range agg {
		all = append(all, v)
	}
	// O(n^2) selection sort is fine — we only care about top 15
	out := []gin.H{}
	for i := 0; i < limit && len(all) > 0; i++ {
		bestIdx := 0
		for j := 1; j < len(all); j++ {
			if all[j].Clicks > all[bestIdx].Clicks ||
				(all[j].Clicks == all[bestIdx].Clicks && all[j].Impressions > all[bestIdx].Impressions) {
				bestIdx = j
			}
		}
		b := all[bestIdx]
		out = append(out, gin.H{
			"query":       b.Query,
			"clicks":      int(b.Clicks),
			"impressions": int(b.Impressions),
		})
		all = append(all[:bestIdx], all[bestIdx+1:]...)
	}
	return out
}

// RunAnalyticsAdvise builds a structured prompt from the snapshot and
// calls the admin's own AI config. Returns the raw AI answer (markdown).
func RunAnalyticsAdvise(c *gin.Context) {
	adminID := c.GetInt64("user_id")
	if adminID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录"})
		return
	}

	// Take a fresh snapshot rather than accepting client-supplied data —
	// prevents a replayed old snapshot from tricking the AI into
	// analyzing last week's numbers.
	snapshot, err := collectSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "聚合数据失败: " + err.Error()})
		return
	}
	js, _ := json.MarshalIndent(snapshot, "", "  ")
	prompt := buildStrategyPrompt(string(js))

	// Use the admin's own AI slot — same path manual audit uses. This
	// makes sure a token-gated org pays against the right account.
	content, _, err := CallAIWithUserConfig(adminID, prompt, 8000)
	if err != nil {
		log.Printf("[analytics] AI call failed user=%d: %v", adminID, err)
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "AI 调用失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"advice":       content,
	}})
}

// collectSnapshot is GetAnalyticsSnapshot's logic without the HTTP
// write — reused by RunAnalyticsAdvise to feed the prompt.
func collectSnapshot() (map[string]interface{}, error) {
	// Cheap duplicate — the query set is small enough that running
	// again via the handler path (which would need us to intercept
	// gin.Context) isn't worth the abstraction cost. If perf becomes
	// an issue we can cache the last snapshot in-memory for 60s.
	fakeCtx := &gin.Context{}
	// Rather than reimplementing, just directly re-run the same
	// queries. Keep this in sync with GetAnalyticsSnapshot above.
	_ = fakeCtx
	snap := map[string]interface{}{}

	var n int
	db.DB.QueryRow(`SELECT COUNT(*) FROM sites`).Scan(&n)
	snap["total_sites"] = n
	db.DB.QueryRow(`SELECT COUNT(DISTINCT site_id) FROM audit_tasks WHERE status='done'`).Scan(&n)
	snap["audited_sites"] = n
	db.DB.QueryRow(`SELECT COUNT(*) FROM geo_articles`).Scan(&n)
	snap["total_articles"] = n
	db.DB.QueryRow(`SELECT COUNT(*) FROM geo_articles WHERE status='published'`).Scan(&n)
	snap["published_articles"] = n
	db.DB.QueryRow(`SELECT COUNT(*) FROM audit_tasks WHERE status='error'`).Scan(&n)
	snap["failed_audits"] = n
	db.DB.QueryRow(`SELECT COUNT(*) FROM coordination_tasks WHERE status='pending'`).Scan(&n)
	snap["coord_pending"] = n
	db.DB.QueryRow(`SELECT COUNT(*) FROM dead_link_results WHERE status_code >= 400 OR status_code = 0`).Scan(&n)
	snap["dead_links_total"] = n
	db.DB.QueryRow(`SELECT COUNT(*) FROM spider_logs WHERE visited_at >= NOW() - INTERVAL '30 days'`).Scan(&n)
	snap["spider_hits_30d"] = n

	// GSC aggregate
	var clicks, imps int
	var ctr, pos float64
	db.DB.QueryRow(`
		WITH latest AS (SELECT DISTINCT ON (site_id) site_id, clicks, impressions, ctr, position
		                FROM gsc_data ORDER BY site_id, date DESC)
		SELECT COALESCE(SUM(clicks),0), COALESCE(SUM(impressions),0),
		       COALESCE(AVG(ctr),0), COALESCE(AVG(position),0) FROM latest
	`).Scan(&clicks, &imps, &ctr, &pos)
	snap["gsc_total_clicks"] = clicks
	snap["gsc_total_impressions"] = imps
	snap["gsc_avg_ctr"] = ctr
	snap["gsc_avg_position"] = pos
	snap["gsc_top_keywords"] = gscTopKeywords(20)

	// GA
	var sessions, users int
	db.DB.QueryRow(`
		WITH latest AS (SELECT DISTINCT ON (site_id) site_id, sessions, users
		                FROM ga_data ORDER BY site_id, date DESC)
		SELECT COALESCE(SUM(sessions),0), COALESCE(SUM(users),0) FROM latest
	`).Scan(&sessions, &users)
	snap["ga_sessions"] = sessions
	snap["ga_users"] = users
	return snap, nil
}

// buildStrategyPrompt gives the AI a very explicit brief so the output
// is actionable rather than generic fluff. Structure asked for:
//   1. 数据健康诊断（指标异常 / 下滑趋势）
//   2. 关键词机会（热度、长尾、覆盖缺口）
//   3. 内容节奏建议（产出速度 / 审核积压 / 发布断层）
//   4. 竞品差异化切入点（基于当前数据分布的推测）
//   5. 技术健康（死链、推送失败、审计失败）
//   6. 3 个月战略路线图（优先级 + 负责人 + 预期指标）
func buildStrategyPrompt(snapshotJSON string) string {
	var b strings.Builder
	b.WriteString(`你是一位资深 SEO 战略顾问 + 数据分析师，同时精通内容运营、关键词策略、竞品分析和团队协作管理。下面给你的是整个 SEO 巡站系统的**全局数据快照**，覆盖几十个站点、4 个搜索平台（Google Search Console / Google Analytics 4 / Bing Webmaster / Yandex Webmaster）、内容生产、审计健康度、协同任务、外链池、死链、蜘蛛爬取频率等维度。

请基于这份数据，给我们一份**战略级的分析报告**（不是技术工单，是运营 + 战略方向）。请用 Markdown 输出，严格按以下 6 个章节：

## 1. 数据健康诊断
- 哪些指标在警戒线之外？（如审计失败率、死链占比、推送失败率、CTR 异常低、平均排名异常高）
- 有没有明显的数据缺口？（如某平台绑定率很低、内容产出已停滞）
- 每一条都附**具体数值**和**建议动作**

## 2. 关键词机会分析
- 基于 top_keywords 数据，哪些词已经有流量基础、值得继续深耕？
- 哪些词高曝光低点击（CTR < 2%）是 TDK 优化的机会？
- 哪些词高点击低位置（avg_position > 10）是靠内容深度可以抢排名的机会？
- 有没有看到可以延伸的长尾话题集群？各给 3-5 个具体词 + 建议动作

## 3. 内容生产节奏建议
- 已发布 / 草稿 / 审核中 的比例是否健康？
- 如果发布积压，根因是审核人力不够还是写作质量不行？
- 建议下一个月的产出节奏（每天/每周多少篇，哪类站点优先）
- 哪类内容应该 **加速产出**，哪类应该 **暂缓**？

## 4. 竞品差异化切入点
- 基于当前我们覆盖的关键词分布，**推测**竞品可能占据了哪些我们没覆盖的领域
- 建议 3-5 个**差异化切入的内容方向/关键词簇**（写明"为什么竞品弱 / 我们有什么优势"）
- 给出 1-2 个**可以立即启动的小型测试站点或专题页**的方向

## 5. 技术健康与协同效率
- 协同任务积压是什么原因？哪个环节最堵？
- 死链集中在哪些站点？有没有批量修复的机会？
- 蜘蛛爬取频率对比索引页面数，覆盖度如何？
- 给 SEO 运营/开发各 2-3 条具体行动项

## 6. 未来 3 个月战略路线图
- **第 1 个月**：优先级最高的 3 件事（每一条要可量化，比如"把 top 10 词的 CTR 从 X% 提到 Y%"）
- **第 2 个月**：在第 1 个月基础上的扩展
- **第 3 个月**：开始做长期差异化（新方向、新站点、新内容类型）
- 每一条都给**负责人类型**（运营 / 开发 / 内容编辑 / 外链专员）和**预期的数据指标变化**

要求：
- 不要说"如果数据更多会更好"这种废话，**只基于下面的快照**给结论
- 不要空谈理论，给**具体数字、具体词、具体站点方向**
- 避免罗列，多用**对比 + 判断 + 行动**的句式
- 输出纯 Markdown，不要 code block 包裹整个答复

以下是快照数据（JSON 格式）：

`)
	b.WriteString("```json\n")
	b.WriteString(snapshotJSON)
	b.WriteString("\n```\n")
	return b.String()
}
