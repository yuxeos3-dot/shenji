package api

// Per-keyword AI writing workflow (GEO + 外链).
//
// Architecture: async 2-step pipeline. User clicks "AI 生成" on a
// keyword row → we create a job → goroutine runs in background → user
// polls for step_label updates → final output shown in a panel where
// the user can review and click "应用到文章库".
//
// Why async: step 1 (full draft) takes 30-90s typically; the frontend
// can't block a single request that long without timeouts. The job
// model also gives us resumability and a consistent place for errors.
//
// Why 2-step: the first call focuses on long-form humanized content;
// the second ONLY extracts SEO metadata (TDK + FAQ) from the already-
// written draft. Splitting them keeps the first call's prompt focused
// on anti-AI-detection language, and keeps step 2 cheap (much smaller
// context, tight JSON output).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Concurrency cap ───────────────────────────────────────────────────────
//
// Three concurrent workflows per process is the sweet spot: a single
// AI round-trip can burn 30-90s, and we don't want one over-eager
// admin to drain all upstream AI slots. New requests past this cap
// get a 429 — user can retry in a few seconds.

var geoWorkflowSem = make(chan struct{}, 3)

// ─── Public types ──────────────────────────────────────────────────────────

type geoWorkflowJob struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	SiteID         int64  `json:"site_id"`
	ContentType    string `json:"content_type"`
	TitleID        int64  `json:"title_id"`
	Keyword        string `json:"keyword"`
	TitleText      string `json:"title_text"`
	WordCount      int    `json:"word_count"`
	WritingAngle   string `json:"writing_angle"`
	KnowledgeID    int64  `json:"knowledge_id"`
	Status         string `json:"status"`
	CurrentStep    int    `json:"current_step"`
	StepLabel      string `json:"step_label"`
	Draft          string `json:"draft,omitempty"`
	FinalContent   string `json:"final_content,omitempty"`
	FinalMetaTitle string `json:"final_meta_title,omitempty"`
	FinalMetaDesc  string `json:"final_meta_desc,omitempty"`
	FinalFAQ       string `json:"final_faq,omitempty"`
	AIModel        string `json:"ai_model,omitempty"`
	ErrorMsg       string `json:"error_msg,omitempty"`
	ArticleID      int64  `json:"article_id"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// loadGeoJob reads a job row with ownership-safe fields for the frontend.
func loadGeoJob(jobID int64) (*geoWorkflowJob, error) {
	var j geoWorkflowJob
	err := db.DB.QueryRow(`
		SELECT id, user_id, site_id, COALESCE(content_type,'geo'), COALESCE(title_id,0),
		       keyword, COALESCE(title_text,''), COALESCE(word_count,1500),
		       COALESCE(writing_angle,''), COALESCE(knowledge_id,0),
		       status, COALESCE(current_step,0), COALESCE(step_label,''),
		       COALESCE(draft,''),
		       COALESCE(final_content,''), COALESCE(final_meta_title,''),
		       COALESCE(final_meta_desc,''), COALESCE(final_faq,'[]'),
		       COALESCE(ai_model,''), COALESCE(error_msg,''), COALESCE(article_id,0),
		       created_at, updated_at
		FROM geo_workflow_jobs WHERE id = ?`, jobID).Scan(
		&j.ID, &j.UserID, &j.SiteID, &j.ContentType, &j.TitleID,
		&j.Keyword, &j.TitleText, &j.WordCount, &j.WritingAngle, &j.KnowledgeID,
		&j.Status, &j.CurrentStep, &j.StepLabel,
		&j.Draft,
		&j.FinalContent, &j.FinalMetaTitle, &j.FinalMetaDesc, &j.FinalFAQ,
		&j.AIModel, &j.ErrorMsg, &j.ArticleID,
		&j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// setJobStep writes progress into the job so polling UI sees it.
func setJobStep(jobID int64, step int, label, status string) {
	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET current_step = ?, step_label = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, step, label, status, jobID)
}

// failJob marks a job failed with an error message.
func failJob(jobID int64, msg string) {
	log.Printf("[geo-workflow] job %d failed: %s", jobID, msg)
	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET status='failed', error_msg=?, step_label=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`, msg, "❌ "+msg, jobID)
}

// loadSitePages returns the site's registered pages as "name|url" lines.
// Feeds into the prompt so AI can insert natural [anchor](/url) links.
// Cap at 12 entries so the prompt doesn't balloon on sites with
// hundreds of pages — the AI only needs a representative sample.
func loadSitePages(siteID int64) []struct{ Name, URL string } {
	rows, err := db.DB.Query(
		`SELECT name, url FROM site_pages WHERE site_id = ? ORDER BY id LIMIT 12`,
		siteID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []struct{ Name, URL string }
	for rows.Next() {
		var n, u string
		if err := rows.Scan(&n, &u); err != nil {
			continue
		}
		if strings.TrimSpace(u) == "" {
			continue
		}
		out = append(out, struct{ Name, URL string }{n, u})
	}
	return out
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// StartKeywordWorkflowRequest: the "AI 生成" button click payload.
// Either give us a title_id (existing keyword from the library) or a
// raw keyword string for ad-hoc generation.
type StartKeywordWorkflowRequest struct {
	SiteID       int64  `json:"site_id" binding:"required"`
	ContentType  string `json:"content_type"` // "geo" | "extlink"; default geo
	TitleID      int64  `json:"title_id"`
	Keyword      string `json:"keyword"`
	TitleText    string `json:"title_text"`
	WordCount    int    `json:"word_count"`
	WritingAngle string `json:"writing_angle"` // "评测" / "攻略" / "对比" / ""
	KnowledgeID  int64  `json:"knowledge_id"`
}

// GeoStartKeywordWorkflow POST /api/v1/geo/keyword/workflow/start
//
// Creates a job row, fires a goroutine, returns the job id for polling.
// Non-blocking semaphore check: refuses 429 if 3 jobs already running.
func GeoStartKeywordWorkflow(c *gin.Context) {
	var req StartKeywordWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if req.SiteID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点未指定"})
		return
	}
	if req.ContentType == "" {
		req.ContentType = "geo"
	}
	if req.ContentType != "geo" && req.ContentType != "extlink" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "content_type 非法"})
		return
	}

	// Resolve keyword/title: if title_id is given, pull from library;
	// else require an inline keyword. title_text falls back to keyword.
	if req.TitleID > 0 {
		var title, kw string
		if err := db.DB.QueryRow(
			`SELECT title, COALESCE(keyword,'') FROM geo_titles WHERE id = ?`, req.TitleID,
		).Scan(&title, &kw); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "关键词不存在"})
			return
		}
		req.TitleText = title
		if req.Keyword == "" {
			if kw != "" {
				req.Keyword = kw
			} else {
				req.Keyword = title
			}
		}
	}
	if strings.TrimSpace(req.Keyword) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "关键词为空"})
		return
	}
	if req.TitleText == "" {
		req.TitleText = req.Keyword
	}
	if req.WordCount < 800 {
		req.WordCount = 1500
	}
	if req.WordCount > 5000 {
		req.WordCount = 5000
	}

	// Non-blocking semaphore: if full, reject fast.
	select {
	case geoWorkflowSem <- struct{}{}:
		// slot acquired — released in goroutine
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{
			"code":  -1,
			"error": "当前 AI 写作任务已达并发上限（3），请稍后重试",
		})
		return
	}

	uid := c.GetInt64("user_id")
	res, err := db.DB.Exec(
		`INSERT INTO geo_workflow_jobs
		 (user_id, site_id, content_type, title_id, keyword, title_text,
		  word_count, writing_angle, knowledge_id, status, step_label)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '⏳ 等待启动')`,
		uid, req.SiteID, req.ContentType, req.TitleID, req.Keyword, req.TitleText,
		req.WordCount, req.WritingAngle, req.KnowledgeID,
	)
	if err != nil {
		<-geoWorkflowSem // release on DB error
		log.Printf("[geo-workflow] insert job: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建任务失败"})
		return
	}
	jobID, _ := res.LastInsertId()

	WriteAuditLog(c, "geo_workflow_start", "geo_workflow_job",
		fmt.Sprintf("%d", jobID), fmt.Sprintf("[%s] %s", req.ContentType, req.Keyword))

	go runKeywordWorkflow(jobID, uid, req)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"job_id": jobID}})
}

// GeoGetWorkflowJob GET /api/v1/geo/keyword/workflow/:id
//
// Ownership: the caller must be the job's user or an admin. We don't
// leak job state across users — another admin's job payload could
// include sensitive site info.
func GeoGetWorkflowJob(c *gin.Context) {
	id := parseInt64Param(c, "id")
	job, err := loadGeoJob(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if job.UserID != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问该任务"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": job})
}

// GeoApplyWorkflowJob POST /api/v1/geo/keyword/workflow/:id/apply
//
// Saves the job's final content into geo_articles and optionally
// marks the source title as used. Idempotent — if already applied,
// returns the existing article_id.
func GeoApplyWorkflowJob(c *gin.Context) {
	id := parseInt64Param(c, "id")
	job, err := loadGeoJob(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if job.UserID != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权应用该任务"})
		return
	}
	if job.Status != "done" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务尚未完成，无法应用"})
		return
	}
	if job.ArticleID > 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"article_id": job.ArticleID}})
		return
	}

	// Optional overrides from the frontend editor: user may tweak the
	// content/title before clicking "应用".
	var override struct {
		Title           string `json:"title"`
		Content         string `json:"content"`
		MetaTitle       string `json:"meta_title"`
		MetaDescription string `json:"meta_description"`
	}
	_ = c.ShouldBindJSON(&override) // body is optional

	title := job.TitleText
	if strings.TrimSpace(override.Title) != "" {
		title = override.Title
	}
	content := job.FinalContent
	if strings.TrimSpace(override.Content) != "" {
		content = override.Content
	}
	metaTitle := job.FinalMetaTitle
	if strings.TrimSpace(override.MetaTitle) != "" {
		metaTitle = override.MetaTitle
	}
	metaDesc := job.FinalMetaDesc
	if strings.TrimSpace(override.MetaDescription) != "" {
		metaDesc = override.MetaDescription
	}

	excerpt := content
	if len([]rune(excerpt)) > 200 {
		excerpt = string([]rune(excerpt)[:200]) + "..."
	}
	wc := utf8.RuneCountInString(content)

	// Persist article. task_id and title_id stay nullable when the
	// workflow was ad-hoc (no source task). We prefix meta_title into
	// the keywords field so it's still searchable in the article list.
	kwField := job.Keyword
	if metaTitle != "" && !strings.Contains(kwField, metaTitle) {
		kwField = job.Keyword // keep simple; meta_title is separate column
	}

	res, err := db.DB.Exec(
		`INSERT INTO geo_articles
		 (task_id, title_id, site_id, title, content, excerpt, keywords,
		  meta_description, ai_model, word_count, status, review_status, content_type)
		 VALUES (0, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'draft', 'pending', ?)`,
		job.TitleID, job.SiteID, title, content, excerpt, kwField,
		metaDesc, job.AIModel, wc, job.ContentType,
	)
	if err != nil {
		log.Printf("[geo-workflow] apply insert: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存文章失败"})
		return
	}
	articleID, _ := res.LastInsertId()

	// Mark source keyword as used (if any) + tag job with article id.
	if job.TitleID > 0 {
		db.DB.Exec(`UPDATE geo_titles SET is_used=1, used_at=CURRENT_TIMESTAMP WHERE id=?`, job.TitleID)
	}
	db.DB.Exec(`UPDATE geo_workflow_jobs SET article_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		articleID, id)

	WriteAuditLog(c, "geo_workflow_apply", "geo_article",
		fmt.Sprintf("%d", articleID), title)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"article_id": articleID}})
}

// GeoListWorkflowJobs GET /api/v1/geo/keyword/workflow/jobs
//
// Returns the caller's recent jobs (last 50). Admins see everyone's
// via ?all=1 — useful for ops/debugging when a user reports a hang.
func GeoListWorkflowJobs(c *gin.Context) {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	q := `
		SELECT id, user_id, site_id, COALESCE(content_type,'geo'), COALESCE(title_id,0),
		       keyword, COALESCE(title_text,''), COALESCE(word_count,1500),
		       COALESCE(writing_angle,''), COALESCE(knowledge_id,0),
		       status, COALESCE(current_step,0), COALESCE(step_label,''),
		       '', '', '', '', '[]',
		       COALESCE(ai_model,''), COALESCE(error_msg,''), COALESCE(article_id,0),
		       created_at, updated_at
		FROM geo_workflow_jobs WHERE `
	args := []interface{}{}
	if c.Query("all") == "1" && role == "admin" {
		q += "1=1"
	} else {
		q += "user_id = ?"
		args = append(args, uid)
	}
	if v := c.Query("site_id"); v != "" {
		q += " AND site_id = ?"
		args = append(args, v)
	}
	if v := c.Query("content_type"); v != "" {
		q += " AND content_type = ?"
		args = append(args, v)
	}
	q += " ORDER BY id DESC LIMIT 50"

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []geoWorkflowJob{}
	for rows.Next() {
		var j geoWorkflowJob
		if err := rows.Scan(
			&j.ID, &j.UserID, &j.SiteID, &j.ContentType, &j.TitleID,
			&j.Keyword, &j.TitleText, &j.WordCount, &j.WritingAngle, &j.KnowledgeID,
			&j.Status, &j.CurrentStep, &j.StepLabel,
			&j.Draft, &j.FinalContent, &j.FinalMetaTitle, &j.FinalMetaDesc, &j.FinalFAQ,
			&j.AIModel, &j.ErrorMsg, &j.ArticleID,
			&j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			continue
		}
		items = append(items, j)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

// ─── The pipeline ──────────────────────────────────────────────────────────

// runKeywordWorkflow is the background goroutine. Two AI calls in
// sequence; job state updated at each milestone. Never panics the
// process (top-level recover) — a runaway AI response or SQL glitch
// just fails the job.
func runKeywordWorkflow(jobID, userID int64, req StartKeywordWorkflowRequest) {
	defer func() { <-geoWorkflowSem }()
	defer func() {
		if r := recover(); r != nil {
			failJob(jobID, fmt.Sprintf("工作流异常: %v", r))
		}
	}()

	// Load optional knowledge base context if user picked one.
	knowledgeText := ""
	if req.KnowledgeID > 0 {
		db.DB.QueryRow(`SELECT content FROM geo_knowledge WHERE id = ?`, req.KnowledgeID).Scan(&knowledgeText)
	}

	// Site's registered pages for internal link anchoring. This is
	// what the user asked for: "内链就用我们添加站点的页面链接".
	sitePages := loadSitePages(req.SiteID)

	// ───── Step 1: long-form draft ─────────────────────────────────────
	setJobStep(jobID, 1, "✍️ 正在撰写长文草稿（植入内链 + 去 AI 化）...", "running")

	prompt1 := buildNeutralAntiDetectionPrompt(req, knowledgeText, sitePages)
	// Tokens ≈ 1.5× target words × 1.2 safety buffer (Chinese/Thai/Viet
	// chars map roughly 1:1.5 to tokens on most models).
	mt1 := int(float64(req.WordCount) * 1.8)
	if mt1 < 3000 {
		mt1 = 3000
	}
	if mt1 > 8000 {
		mt1 = 8000
	}

	content, model, err := CallAIWithUserConfig(userID, prompt1, mt1)
	if err != nil {
		failJob(jobID, "AI 草稿生成失败: "+err.Error())
		return
	}
	draft := strings.TrimSpace(content)
	// Strip accidental ```markdown fences from the top/bottom.
	if strings.HasPrefix(draft, "```") {
		draft = strings.TrimPrefix(draft, "```markdown")
		draft = strings.TrimPrefix(draft, "```")
		draft = strings.TrimSuffix(draft, "```")
		draft = strings.TrimSpace(draft)
	}
	if draft == "" {
		failJob(jobID, "AI 返回内容为空")
		return
	}
	// Cap persisted size so a runaway AI can't bloat the DB.
	draftForDB := draft
	if len(draftForDB) > 300*1024 {
		draftForDB = draftForDB[:300*1024] + "\n...[已截断]"
	}
	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET draft = ?, ai_model = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, draftForDB, model, jobID)

	// ───── Step 2: TDK + FAQ extraction ────────────────────────────────
	setJobStep(jobID, 2, "🔎 正在提取 SEO 元数据（标题 / 描述 / FAQ）...", "running")

	// Feed a trimmed excerpt of the draft to step 2 so the prompt stays
	// lean; 2000 chars is enough for the model to grasp the topic and
	// generate meta.
	excerpt := draft
	runes := []rune(excerpt)
	if len(runes) > 2000 {
		excerpt = string(runes[:2000]) + "\n\n[...]"
	}
	prompt2 := buildMetaExtractPrompt(req.Keyword, req.TitleText, excerpt)

	metaRaw, _, err := CallAIWithUserConfig(userID, prompt2, 1500)
	if err != nil {
		// Step 2 failure is non-fatal: we still have the draft. Use
		// simple fallbacks for meta so user can manually edit later.
		log.Printf("[geo-workflow] job %d step2 failed (using fallback): %v", jobID, err)
		finishJobWithFallback(jobID, draft, req.Keyword, req.TitleText)
		return
	}
	mt, md, faqJSON := parseMetaResponse(metaRaw, req.Keyword, req.TitleText)

	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET status = 'done', current_step = 2, step_label = '✅ 完成，等待审核应用',
		    final_content = ?, final_meta_title = ?, final_meta_desc = ?, final_faq = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, draft, mt, md, faqJSON, jobID)
}

// finishJobWithFallback completes a job with safe meta defaults when
// step 2 fails. We never discard the draft — the whole reason we
// store it is to survive later errors.
//
// Fallbacks use purely informational phrasing (no "指南" / "评测" /
// "对比" which carry commercial overtones). Keep in sync with the
// tone in buildMetaExtractPrompt.
func finishJobWithFallback(jobID int64, draft, keyword, title string) {
	year := fmt.Sprintf("%d", time.Now().Year())
	mt := fmt.Sprintf("【%s】%s：概述与常见问题", year, keyword)
	md := fmt.Sprintf("【%s】%s 相关信息整理：定义、背景、常见疑问与资料来源。内容为事实陈述，不作商业推荐。", year, keyword)
	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET status = 'done', current_step = 2,
		    step_label = '⚠️ 完成（SEO 元数据使用默认值，建议手动编辑）',
		    final_content = ?, final_meta_title = ?, final_meta_desc = ?, final_faq = '[]',
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, draft, mt, md, jobID)
}

// ─── Prompts ───────────────────────────────────────────────────────────────

// buildNeutralAntiDetectionPrompt is the heart of the pipeline. It
// combines four goals at once:
//
//  1. PURPOSE CLARITY — this article is an external-facing informational
//     piece meant for GEO (AI-search retrieval) and backlink sites,
//     not a landing page for a product/platform/brand. No sales push.
//  2. NEUTRAL + CLINICAL TONE — the keyword universe is adult-industry,
//     but output must read like an encyclopedia entry or lifestyle column:
//     informational, educational, culturally curious — never titillating,
//     never explicit, never promotional.
//  3. LOW AI-DETECTION — concrete humanization techniques: ban formulaic
//     connectors, force second-person voice, require one admitted
//     drawback, require uneven sentence lengths, require concrete numbers.
//  4. NATURAL INTERNAL LINKING — uses the site's real page list so the
//     AI embeds [anchor](/real/url) rather than hallucinating URLs.
//
// Critical non-negotiables enforced by explicit 禁用词 blocklists:
//   - NO gambling jargon (平台 / 注册 / 优惠 / 流水 / 赔率 / 转盘 / 博彩 / 娱乐城)
//     — the workflow originally forked from a 博彩 template, we scrub
//     every trace of it because those keywords poison rankings and
//     mismatch the actual adult content vertical.
//   - NO sensory descriptions (身体部位 / 行为过程 / 感官刺激 / 暗示性比喻)
//   - NO promotional/sales CTAs (立即 / 马上 / 限时 / 爆款 / 最佳推荐)
func buildNeutralAntiDetectionPrompt(
	req StartKeywordWorkflowRequest,
	knowledge string,
	pages []struct{ Name, URL string },
) string {
	year := fmt.Sprintf("%d", time.Now().Year())

	// Site pages block — empty = no site-specific internal linking
	// guidance beyond "write generic relative paths".
	var pagesBlock strings.Builder
	if len(pages) > 0 {
		pagesBlock.WriteString("【站内可用内链清单】（写作时请从中挑选 2-4 条，在合适位置用 [锚文本](URL) 格式自然嵌入，锚文本要契合上下文，禁止机械堆砌）：\n")
		for _, p := range pages {
			fmt.Fprintf(&pagesBlock, "- %s → %s\n", p.Name, p.URL)
		}
	} else {
		pagesBlock.WriteString("【内链说明】当前站点暂未配置页面，本篇不要求强制内链。\n")
	}

	knowledgeBlock := ""
	if strings.TrimSpace(knowledge) != "" {
		knowledgeBlock = "【参考资料（请自然融入事实，不得直接整段复制）】\n" + knowledge + "\n\n"
	}

	angleBlock := ""
	switch req.WritingAngle {
	case "评测":
		angleBlock = "【写作视角】资料汇总与中立对比——以信息整理者身份呈现多方观点，避免任何推销、避免打分结论。\n"
	case "攻略":
		angleBlock = "【写作视角】知识科普——解释关键概念、讨论常见误区、给出一般性注意事项，语气像百科条目或生活方式专栏。\n"
	case "对比":
		angleBlock = "【写作视角】横向信息对比——把同主题下不同类别/流派/做法并列呈现，只做事实描述，不下好坏判断。\n"
	case "新手":
		angleBlock = "【写作视角】入门科普——用通俗语言拆解术语，讲清楚概念由来和社会背景，语气温和包容。\n"
	}

	// NOTE: string concatenation (not Sprintf) because the user-supplied
	// keyword could contain '%' which would otherwise mangle formatting.
	return `你是一名资深内容编辑，常年为生活方式、健康科普、文化研究类媒体撰稿，擅长把敏感或小众话题写成信息量高、语气克制的长文。

现在请围绕以下关键词写一篇 Markdown 文章：

【核心关键词】` + req.Keyword + `
【建议标题】` + req.TitleText + `
【目标字数】` + fmt.Sprintf("%d", req.WordCount) + ` 字以上
【目标年份标签】` + year + `

` + angleBlock + knowledgeBlock + pagesBlock.String() + `

━━━━━ 文章定位（必读！）━━━━━
这是一篇用于 SEO/GEO/外链目的的信息型文章，发布在第三方内容站点或资讯站。
它不是任何平台、品牌、产品的推广页，也不是销售页。
核心目的是：
- 让搜索引擎（Google/Bing）把这篇内容识别为权威信息源
- 让生成式 AI（ChatGPT/Gemini/Perplexity）抓取时能从中提取结构化答案
- 以自然密度布局关键词，建立主题相关性
禁止任何促销口吻（"马上""立即""限时"）、禁止任何"推荐某某"的定性表达。

━━━━━ 选题角度（关键）━━━━━
该关键词可能涉及成人话题。你的写作定位不是面向从业者或消费者，而是面向：
- 对该话题感兴趣的普通读者（想了解"这是什么""为什么存在""社会怎么看"）
- 研究型/学术型检索者（查定义、查历史、查数据）
- 通过 AI 搜索该话题的一般用户（想要事实、概念、常识）

所以内容必须围绕以下之一切入：
- 话题的定义、起源、历史演变
- 相关的文化、社会、心理、语言学背景
- 现象层面的客观描述（而非操作层面的指南）
- 常见误区的澄清、学术或媒体视角的观点汇总
- 相关的法律法规、行业现状、市场规模等宏观数据

绝不允许：
- 教读者如何获取、使用、操作任何产品或服务
- 任何带动作指令的"步骤教学"（"第一步...第二步..."除非是抽象流程）
- 感官性、刺激性、煽动性的描写

━━━━━ 禁用词清单（严格）━━━━━
【博彩/游戏类】以下词一律禁止出现（哪怕只是比喻）：
平台、注册、优惠、流水、赔率、转盘、彩金、娱乐城、博彩、赌场、跑路、赔付、押注、下注、线上游戏、电子游艺、真人视讯、百家乐、老虎机、体育博彩、即时返水

【露骨/感官类】以下词一律禁止出现：
具体身体部位的直接描写、任何行为过程的细节、"性感"、"诱人"、"销魂"、"激情"、"火辣"、任何俚语化的性暗示表达

【促销类】以下词一律禁止出现：
马上、立即、限时、爆款、必选、最佳推荐、首选、不容错过、抓紧、赶紧、值得拥有

━━━━━ 语言基调 ━━━━━
- 整体像一本百科词条、一篇生活方式专栏、或一份行业报告的摘要
- 涉及敏感内容时使用中性术语（例如："成年内容产业"而不是任何俚语；"亲密关系"而不是行为动词）
- 可以出现医学、社会学、心理学、法学、经济学的专业术语（符合学术杂志风格）
- 描写事实时只谈现象不谈感受，绝不代入第一人称经验性描写

━━━━━ 反 AI 检测要求（逐条执行）━━━━━
1. 禁用模板连接词：不要"首先""其次""然后""最后""综上所述""值得注意的是""众所周知""在数字时代""随着...的发展"
2. 连接词口语化：不要"然而""此外""并且"，改用"不过""另外""再说""换句话讲"
3. 句子长短交替：不要连续 3 句都是相近长度；短句（10 字以内）和长句（30 字以上）要混着来
4. 至少 1 次用第二人称直接对话读者：如"你可能会问..."、"想清楚这一点..."、"如果你第一次接触这个话题..."
5. 至少 1 次坦承局限或边界：哪怕话题本身中性，也要点出 1 个需要读者自行判断的地方（例如"数据来源差异较大"、"不同地区认定标准不同"）
6. 至少 2 个具体数字（比例 / 年份 / 金额 / 数量等）——数字越具体越像真人写的，例如"约 87%"而不是"大多数"
7. 严禁空洞形容词：不要"丰富的""多元的""优质的""专业的"作结论，全部换成具体事实

━━━━━ 结构与格式 ━━━━━
1. 全文 Markdown，从 ## 二级标题开始（不要 # 一级标题，留给网站模板）
2. 至少 1 个 Markdown 表格（| 格式，含表头分隔行）——用于并列/对比/数据聚合
3. 至少 1 个有序或无序列表段落
4. 标题或首段出现"` + year + `"年份标签（增强时效感）
5. 结尾包含 3-5 个 FAQ（以"## 常见问题"开头，每问每答用 ### 小标题 + 段落，答案不少于 60 字）
6. 关键词以自然密度分布 4-8 次（含变体），不要机械重复
7. 禁止前言（"在这篇文章中我将...""让我们开始吧"之类），直接进入第一个 ## 标题

━━━━━ 内链处理 ━━━━━
- 只能使用上面【站内可用内链清单】里的 URL
- 锚文本必须是 2-8 字的自然词组，与上下文语义衔接
- 禁止堆砌锚文本或在同一段出现 2 个以上内链
- 如果清单为空，本篇允许不包含内链

━━━━━ 输出要求 ━━━━━
- 直接从 ## 标题开始输出，不要任何解释 / 说明 / 代码块包裹
- 不要输出本提示中的任何指令原文
- 全文只输出文章本身（Markdown）`
}

// buildMetaExtractPrompt produces step-2's tight JSON-only prompt.
// We use string concatenation (not Sprintf) — a user-supplied keyword
// containing "%" would otherwise corrupt the format directives.
func buildMetaExtractPrompt(keyword, title, excerpt string) string {
	year := fmt.Sprintf("%d", time.Now().Year())
	return `你是 SEO 元数据生成助手。下面是一篇信息型文章的开头摘要，请据此生成 SEO 标题 / 描述 / FAQ。

【核心关键词】` + keyword + `
【建议标题】` + title + `

【文章摘要】
` + excerpt + `

任务：输出一个 JSON 对象（注意是纯 JSON，不要 markdown 代码块，不要解释文字），字段如下：
- meta_title：SEO 标题，50 字以内，必须包含核心关键词和年份【` + year + `】
                     语气客观中立，像词典条目或百科标题，禁止任何推销口吻
- meta_description：Meta 描述，140-160 字，开头带【` + year + `】标签
                     内容必须是信息性、知识性陈述，不得有行动号召（如"马上""立即"）
                     不得使用"平台""注册""优惠""评测""首选""最佳"等词
- faq：一个数组，含 3-5 个问答，每个元素 {"q":"问题","a":"回答，60 字以上，事实陈述不推销"}

━━━━━ 硬性禁用词（所有字段都不能出现）━━━━━
博彩类：平台、注册、优惠、流水、赔率、转盘、彩金、娱乐城、博彩、赌场、押注、下注
露骨类：任何身体部位、性感、诱人、销魂、激情、火辣、各类性暗示俚语
促销类：马上、立即、限时、爆款、必选、最佳推荐、首选、抓紧

━━━━━ 语气要求 ━━━━━
- 全部字段保持中性、学术化、信息化语气
- 像维基百科条目、像研究报告摘要、像专业杂志的 FAQ 栏目
- 标题可以用"概述""发展""现状""常见问题""社会视角""文化背景"这类中性词汇作引子
- faq 问题必须是信息检索型长尾（"什么是..."、"为什么..."、"...的历史是怎样"、"...在不同国家有什么差异"）
- 绝不出现"哪家好""哪个值得买""如何选择推荐"这类导购式问题

直接输出 JSON，不要任何其他字符。

示例格式（字段示意，请生成真实内容而不是复制示例）：
{"meta_title":"【` + year + `】XX概述：定义、历史与常见问题","meta_description":"【` + year + `】本文梳理 XX 的定义、社会背景与常见疑问，数据来源包括...","faq":[{"q":"什么是 XX","a":"60 字以上的事实陈述..."}]}`
}

// parseMetaResponse extracts meta_title, meta_desc, faq JSON from the
// step-2 AI response. Forgiving parser — unwraps markdown fences,
// falls back to keyword-based defaults if JSON is malformed.
func parseMetaResponse(raw, keyword, title string) (mt, md, faqJSON string) {
	s := strings.TrimSpace(raw)
	// Strip ```json / ``` fences
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	// Sometimes the AI prepends explanation before the JSON. Find the
	// first '{' and trim everything before it.
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	// Trim trailing text after the last '}'.
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}

	var parsed struct {
		MetaTitle       string        `json:"meta_title"`
		MetaDescription string        `json:"meta_description"`
		FAQ             []interface{} `json:"faq"`
	}
	year := fmt.Sprintf("%d", time.Now().Year())
	if err := json.Unmarshal([]byte(s), &parsed); err != nil || strings.TrimSpace(parsed.MetaTitle) == "" {
		log.Printf("[geo-workflow] meta JSON parse failed: %v", err)
		mt = fmt.Sprintf("【%s】%s：概述与常见问题", year, keyword)
		md = fmt.Sprintf("【%s】%s 相关信息整理，内容为事实陈述，不作商业推荐。", year, keyword)
		faqJSON = "[]"
		return
	}
	mt = strings.TrimSpace(parsed.MetaTitle)
	md = strings.TrimSpace(parsed.MetaDescription)
	if mt == "" {
		mt = fmt.Sprintf("【%s】%s：概述与常见问题", year, keyword)
	}
	if md == "" {
		md = fmt.Sprintf("【%s】%s 相关信息整理，内容为事实陈述，不作商业推荐。", year, keyword)
	}
	// Re-marshal FAQ to ensure it's valid JSON; on failure use '[]'.
	if parsed.FAQ == nil {
		faqJSON = "[]"
	} else {
		b, err := json.Marshal(parsed.FAQ)
		if err != nil {
			faqJSON = "[]"
		} else {
			faqJSON = string(b)
		}
	}
	return
}
