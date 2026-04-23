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
	"regexp"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"seo-system/internal/db"
	"seo-system/internal/frw"

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
	// Wizard-supplied fields (nullable-with-default columns added in
	// the geo_workflow_jobs migration). All strings so the frontend
	// can treat them uniformly.
	DetailType        string `json:"detail_type,omitempty"`
	Language          string `json:"language,omitempty"`
	Style             string `json:"style,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	ParagraphStruct   string `json:"paragraph_struct,omitempty"`
	ExtraRequirements string `json:"extra_requirements,omitempty"`
	ReferenceURL      string `json:"reference_url,omitempty"`
	CustomPrompt      string `json:"custom_prompt,omitempty"`
	Status            string `json:"status"`
	CurrentStep       int    `json:"current_step"`
	StepLabel         string `json:"step_label"`
	Draft             string `json:"draft,omitempty"`
	FinalContent      string `json:"final_content,omitempty"`
	FinalMetaTitle    string `json:"final_meta_title,omitempty"`
	FinalMetaDesc     string `json:"final_meta_desc,omitempty"`
	FinalFAQ          string `json:"final_faq,omitempty"`
	AIModel           string `json:"ai_model,omitempty"`
	ErrorMsg          string `json:"error_msg,omitempty"`
	ArticleID         int64  `json:"article_id"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// loadGeoJob reads a job row with ownership-safe fields for the frontend.
func loadGeoJob(jobID int64) (*geoWorkflowJob, error) {
	var j geoWorkflowJob
	err := db.DB.QueryRow(`
		SELECT id, user_id, site_id, COALESCE(content_type,'geo'), COALESCE(title_id,0),
		       keyword, COALESCE(title_text,''), COALESCE(word_count,1500),
		       COALESCE(writing_angle,''), COALESCE(knowledge_id,0),
		       COALESCE(detail_type,'article'), COALESCE(language,'zh'),
		       COALESCE(style,'professional'), COALESCE(output_format,'markdown'),
		       COALESCE(paragraph_struct,''), COALESCE(extra_requirements,''),
		       COALESCE(reference_url,''), COALESCE(custom_prompt,''),
		       status, COALESCE(current_step,0), COALESCE(step_label,''),
		       COALESCE(draft,''),
		       COALESCE(final_content,''), COALESCE(final_meta_title,''),
		       COALESCE(final_meta_desc,''), COALESCE(final_faq,'[]'),
		       COALESCE(ai_model,''), COALESCE(error_msg,''), COALESCE(article_id,0),
		       created_at, updated_at
		FROM geo_workflow_jobs WHERE id = $1`, jobID).Scan(
		&j.ID, &j.UserID, &j.SiteID, &j.ContentType, &j.TitleID,
		&j.Keyword, &j.TitleText, &j.WordCount, &j.WritingAngle, &j.KnowledgeID,
		&j.DetailType, &j.Language, &j.Style, &j.OutputFormat,
		&j.ParagraphStruct, &j.ExtraRequirements, &j.ReferenceURL, &j.CustomPrompt,
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
		SET current_step = $1, step_label = $2, status = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $4`, step, label, status, jobID)
}

// failJob marks a job failed with an error message.
func failJob(jobID int64, msg string) {
	log.Printf("[geo-workflow] job %d failed: %s", jobID, msg)
	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET status='failed', error_msg=$1, step_label=$2, updated_at=CURRENT_TIMESTAMP
		WHERE id=$3`, msg, "❌ "+msg, jobID)
}

// loadSitePages returns the site's registered pages as "name|url" lines.
// Feeds into the prompt so AI can insert natural [anchor](/url) links.
// Cap at 12 entries so the prompt doesn't balloon on sites with
// hundreds of pages — the AI only needs a representative sample.
func loadSitePages(siteID int64) []struct{ Name, URL string } {
	rows, err := db.DB.Query(
		`SELECT name, url FROM site_pages WHERE site_id = $1 ORDER BY id LIMIT 12`,
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
	// ─── Wizard fields (step 1 基础信息 / step 2 生成要求 / step 3 Prompt) ───
	DetailType        string `json:"detail_type"`        // article | qa | review | tutorial
	Language          string `json:"language"`           // zh | en | ja | ko | es | other
	Style             string `json:"style"`              // professional | casual | marketing | technical
	OutputFormat      string `json:"output_format"`      // markdown | html | plain
	ParagraphStruct   string `json:"paragraph_struct"`   // 自由文本：段落结构说明
	ExtraRequirements string `json:"extra_requirements"` // 自由文本：其他要求
	ReferenceURL      string `json:"reference_url"`      // 可选参考链接
	CustomPrompt      string `json:"custom_prompt"`      // 用户整段覆盖的 prompt（为空则使用内置默认）
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
	if !canAccessSite(c, fmt.Sprintf("%d", req.SiteID)) {
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
			`SELECT title, COALESCE(keyword,'') FROM geo_titles WHERE id = $1`, req.TitleID,
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

	// Bound free-form wizard inputs. These get spliced into the AI prompt,
	// so a runaway 1 MB custom_prompt would blow past the model's context
	// window and waste an API call. Caps are generous (enough for a full
	// prompt rewrite) but not unbounded. Keyword itself is capped shorter —
	// it's a search term, not an essay.
	const (
		maxCustomPrompt = 32 * 1024
		maxFreeText     = 4 * 1024
		maxKeyword      = 200
	)
	if len(req.CustomPrompt) > maxCustomPrompt {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "自定义 Prompt 超长（上限 32KB）"})
		return
	}
	if len(req.ExtraRequirements) > maxFreeText {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "附加要求超长（上限 4KB）"})
		return
	}
	if len(req.ParagraphStruct) > maxFreeText {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "段落结构描述超长（上限 4KB）"})
		return
	}
	if len(req.ReferenceURL) > 2048 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参考链接超长"})
		return
	}
	if len(req.Keyword) > maxKeyword {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "关键词过长"})
		return
	}
	if len(req.TitleText) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "标题过长"})
		return
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

	// Wizard defaults — frontend may omit these on ad-hoc calls.
	if req.DetailType == "" {
		req.DetailType = "article"
	}
	if req.Language == "" {
		req.Language = "zh"
	}
	if req.Style == "" {
		req.Style = "professional"
	}
	if req.OutputFormat == "" {
		req.OutputFormat = "markdown"
	}

	uid := c.GetInt64("user_id")
	var jobID int64
	err := db.DB.QueryRow(
		`INSERT INTO geo_workflow_jobs
		 (user_id, site_id, content_type, title_id, keyword, title_text,
		  word_count, writing_angle, knowledge_id,
		  detail_type, language, style, output_format,
		  paragraph_struct, extra_requirements, reference_url, custom_prompt,
		  status, step_label)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, 'pending', '⏳ 等待启动') RETURNING id`,
		uid, req.SiteID, req.ContentType, req.TitleID, req.Keyword, req.TitleText,
		req.WordCount, req.WritingAngle, req.KnowledgeID,
		req.DetailType, req.Language, req.Style, req.OutputFormat,
		req.ParagraphStruct, req.ExtraRequirements, req.ReferenceURL, req.CustomPrompt,
	).Scan(&jobID)
	if err != nil {
		<-geoWorkflowSem // release on DB error
		log.Printf("[geo-workflow] insert job: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建任务失败"})
		return
	}

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

	// Wrap article INSERT + title/job UPDATEs in a single transaction.
	tx, txErr := db.DB.Begin()
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存文章失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	var articleID int64
	err = tx.QueryRow(
		`INSERT INTO geo_articles
		 (title_id, site_id, title, content, excerpt, keywords,
		  meta_title, meta_description, ai_model, word_count, status, review_status, content_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', 'pending', $11) RETURNING id`,
		job.TitleID, job.SiteID, title, content, excerpt, kwField,
		metaTitle, metaDesc, job.AIModel, wc, job.ContentType,
	).Scan(&articleID)
	if err != nil {
		log.Printf("[geo-workflow] apply insert: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存文章失败"})
		return
	}

	// Mark source keyword as used (if any) + tag job with article id.
	if job.TitleID > 0 {
		tx.Exec(`UPDATE geo_titles SET is_used=1, used_at=CURRENT_TIMESTAMP WHERE id=$1`, job.TitleID)
	}
	tx.Exec(`UPDATE geo_workflow_jobs SET article_id=$1, updated_at=CURRENT_TIMESTAMP WHERE id=$2`,
		articleID, id)

	if err := tx.Commit(); err != nil {
		log.Printf("[geo-workflow] apply commit: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存文章失败"})
		return
	}

	WriteAuditLog(c, "geo_workflow_apply", "geo_article",
		fmt.Sprintf("%d", articleID), title)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"article_id": articleID}})
}

// GeoDiscardWorkflowJob DELETE /api/v1/geo/keyword/workflow/:id
//
// Permanently deletes a workflow job. Used by the wizard step 4
// "放弃" button when the user decides a generation isn't worth
// keeping. Only the owner or an admin can discard.
func GeoDiscardWorkflowJob(c *gin.Context) {
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
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权删除该任务"})
		return
	}
	// If the job has already been applied to an article, the article
	// stays — user has to delete that separately. Just drop the job.
	if _, err := db.DB.Exec(`DELETE FROM geo_workflow_jobs WHERE id = $1`, id); err != nil {
		log.Printf("[geo-workflow] discard %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	WriteAuditLog(c, "geo_workflow_discard", "geo_workflow_job",
		fmt.Sprintf("%d", id), job.Keyword)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已放弃"})
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
		q += fmt.Sprintf("user_id = $%d", len(args)+1)
		args = append(args, uid)
	}
	if v := c.Query("site_id"); v != "" {
		q += fmt.Sprintf(" AND site_id = $%d", len(args)+1)
		args = append(args, v)
	}
	if v := c.Query("content_type"); v != "" {
		q += fmt.Sprintf(" AND content_type = $%d", len(args)+1)
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

	// Knowledge base tables were dropped as part of the wizard refactor;
	// any domain context now lives in the wizard's "extra_requirements"
	// field or inline in the user's custom prompt. Kept as an empty
	// string so downstream prompt templates that still reference
	// {{knowledge}} don't blow up.
	knowledgeText := ""

	// Site's registered pages for internal link anchoring. This is
	// what the user asked for: "内链就用我们添加站点的页面链接".
	sitePages := loadSitePages(req.SiteID)

	// ───── Step 1: long-form draft ─────────────────────────────────────
	setJobStep(jobID, 1, "✍️ 正在撰写长文草稿（植入内链 + 去 AI 化）...", "running")

	var prompt1 string
	if strings.TrimSpace(req.CustomPrompt) != "" {
		// User provided a fully custom prompt via the wizard step 3 —
		// run it verbatim with variable substitution.
		prompt1 = applyPromptVariables(req.CustomPrompt, req, knowledgeText, sitePages)
	} else {
		prompt1 = buildWizardPrompt(req, knowledgeText, sitePages)
	}
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
		SET draft = $1, ai_model = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`, draftForDB, model, jobID)

	// ───── Step 2: TDK + FAQ extraction ────────────────────────────────
	setJobStep(jobID, 2, "🔎 正在提取 SEO 元数据（标题 / 描述 / FAQ）...", "running")

	// Feed a larger slice of the draft to step 2 than before. Step 2
	// now also extracts FAQs — which live at the *end* of the article —
	// so the old 2000-char head-only excerpt would miss them entirely.
	// 6000 chars comfortably covers the structure on a 2000-word piece.
	excerpt := draft
	runes := []rune(excerpt)
	if len(runes) > 6000 {
		// Keep the opening 1500 chars (for topic grounding) and the
		// final 4500 chars (for the FAQ block and final sections).
		head := string(runes[:1500])
		tail := string(runes[len(runes)-4500:])
		excerpt = head + "\n\n[...中间内容省略...]\n\n" + tail
	}
	prompt2 := buildMetaExtractPrompt(req.Keyword, req.TitleText, excerpt)

	metaRaw, _, err := CallAIWithUserConfig(userID, prompt2, 2500)
	if err != nil {
		// Step 2 failure is non-fatal: we still have the draft. Use
		// simple fallbacks for meta so user can manually edit later.
		log.Printf("[geo-workflow] job %d step2 failed (using fallback): %v", jobID, err)
		finishJobWithFallback(jobID, draft, req.Keyword, req.TitleText)
		return
	}
	mt, md, faqJSON := parseMetaResponse(metaRaw, req.Keyword, req.TitleText)

	// Step 3 (news only): generate images and inline them
	finalContent := draft
	if req.DetailType == "news" {
		imgPrompts := extractImagePrompts(draft)
		if len(imgPrompts) > 0 {
			setJobStep(jobID, 3, fmt.Sprintf("🎨 正在生成配图 (%d 张, 需1-3分钟)...", len(imgPrompts)), "running")
			urls := generateArticleImages(userID, imgPrompts)
			finalContent = replaceImagePlaceholders(draft, imgPrompts, urls)
			urlsJSON, _ := json.Marshal(urls)
			db.DB.Exec(`UPDATE geo_workflow_jobs SET image_urls = $1 WHERE id = $2`, string(urlsJSON), jobID)
		}
	}

	db.DB.Exec(`UPDATE geo_workflow_jobs
		SET status = 'done', current_step = 2, step_label = '✅ 完成，等待审核应用',
		    final_content = $1, final_meta_title = $2, final_meta_desc = $3, final_faq = $4,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $5`, finalContent, mt, md, faqJSON, jobID)
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
		    final_content = $1, final_meta_title = $2, final_meta_desc = $3, final_faq = '[]',
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $4`, draft, mt, md, jobID)
}

// ─── Wizard defaults endpoint ──────────────────────────────────────────────

// GeoGetDefaults GET /api/v1/geo/defaults$1content_type=geo|extlink
//
// Returns the built-in default prompt template + recommended params
// so the wizard can pre-fill step 2/3. The frontend can render this
// as editable text and the user can override it on each generation.
func GeoGetDefaults(c *gin.Context) {
	contentType := c.DefaultQuery("content_type", "geo")
	detailType := c.DefaultQuery("detail_type", "")
	if contentType != "geo" && contentType != "extlink" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "content_type 非法"})
		return
	}

	// Default prompt template — documented variables the UI should
	// surface in the step 3 editor. Users can delete them or add extras.
	tmpl := `你是一名资深内容编辑。请根据以下信息撰写内容：

【核心关键词】{{keyword}}
【建议标题】{{title}}
【写作语言】{{lang}}
【行文风格】{{style}}
【输出格式】{{format}}
【目标字数】{{wordcount}} 字以上
【目标年份】{{year}}

【附加要求】
{{extra}}

━━━━━ 写作原则 ━━━━━
1. H2 / H3 标题都是"能被独立回答"的问题
2. 每段首句给结论，后文展开细节
3. 大量具体数字（比例 / 年份 / 金额）
4. 至少 1 个表格 + 1 处引用块
5. 结尾"常见问题"章节，6-8 个 Q&A，每个 ≥ 80 字
6. 关键词自然分布 4-8 次
7. 禁止模板连接词（首先 / 其次 / 综上所述）
8. 至少 1 处第二人称 + 1 处坦承数据局限

━━━━━ 禁用词 ━━━━━
博彩类：注册、优惠、流水、赔率、转盘、彩金、娱乐城、博彩、赌场
诱导类：性感、诱人、销魂、激情、火辣、撩人、深夜福利、老司机
促销类：马上、立即、限时、爆款、必选、最佳、不容错过

直接从标题 / 首段开始输出，不要代码块包裹。`

	// For news detail_type, return a preview of the news prompt skeleton.
	// The actual prompt is still built by buildNewsPrompt at generation time —
	// this preview shows the user WHAT structure will be generated so they
	// don't expect editing this text to change the news-article output.
	// (News detail_type ignores custom_prompt; structure is fixed.)
	if detailType == "news" {
		tmpl = buildNewsPromptPreview()
	}
	defaults := gin.H{
		"prompt":              tmpl,
		"prompt_readonly":     detailType == "news",
		"prompt_notice":       newsPromptNotice(detailType),
		"default_word_count":  1500,
		"default_language":    "zh",
		"default_detail_type": "article",
		"default_style":       "professional",
		"default_format":      "markdown",
		"languages": []gin.H{
			{"value": "zh", "label": "中文（简体）"},
			{"value": "en", "label": "English"},
			{"value": "ja", "label": "日本語"},
			{"value": "ko", "label": "한국어"},
			{"value": "es", "label": "Español"},
			{"value": "other", "label": "其他"},
		},
		"detail_types": []gin.H{
			{"value": "article", "label": "资讯文章"},
			{"value": "news", "label": "📰 新闻资讯（时间线+FAQ+配图）"},
			{"value": "qa", "label": "问答合集"},
			{"value": "review", "label": "横向对照"},
			{"value": "tutorial", "label": "入门科普"},
		},
		"styles": []gin.H{
			{"value": "professional", "label": "专业权威"},
			{"value": "casual", "label": "口语轻松"},
			{"value": "marketing", "label": "品牌营销"},
			{"value": "technical", "label": "技术深度"},
		},
		"formats": []gin.H{
			{"value": "markdown", "label": "Markdown"},
			{"value": "html", "label": "HTML 片段"},
			{"value": "plain", "label": "纯文本"},
		},
		"variables": []gin.H{
			{"name": "{{keyword}}", "desc": "关键词"},
			{"name": "{{title}}", "desc": "建议标题"},
			{"name": "{{wordcount}}", "desc": "目标字数"},
			{"name": "{{style}}", "desc": "风格"},
			{"name": "{{format}}", "desc": "格式"},
			{"name": "{{lang}}", "desc": "语言"},
			{"name": "{{type}}", "desc": "内容类型"},
			{"name": "{{extra}}", "desc": "附加要求"},
			{"name": "{{paragraph}}", "desc": "段落结构"},
			{"name": "{{reference}}", "desc": "参考链接"},
			{"name": "{{knowledge}}", "desc": "参考资料"},
			{"name": "{{pages}}", "desc": "站内页面列表"},
			{"name": "{{year}}", "desc": "当前年份"},
		},
		"content_type": contentType,
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": defaults})
}

// ─── Wizard prompt assembly ────────────────────────────────────────────────

// applyPromptVariables replaces {{keyword}} / {{title}} / {{wordcount}} /
// {{style}} / {{format}} / {{lang}} / {{type}} / {{extra}} / {{knowledge}}
// in a user-supplied custom prompt. Keeps the substitution explicit so a
// prompt writer can opt into any subset and unknown variables stay visible
// rather than silently dropped.
func applyPromptVariables(
	prompt string,
	req StartKeywordWorkflowRequest,
	knowledge string,
	pages []struct{ Name, URL string },
) string {
	repl := map[string]string{
		"{{keyword}}":      req.Keyword,
		"{{title}}":        req.TitleText,
		"{{wordcount}}":    fmt.Sprintf("%d", req.WordCount),
		"{{style}}":        req.Style,
		"{{format}}":       req.OutputFormat,
		"{{lang}}":         req.Language,
		"{{type}}":         req.DetailType,
		"{{extra}}":        req.ExtraRequirements,
		"{{paragraph}}":    req.ParagraphStruct,
		"{{reference}}":    req.ReferenceURL,
		"{{knowledge}}":    knowledge,
		"{{year}}":         fmt.Sprintf("%d", time.Now().Year()),
		"{{writing_angle}}": req.WritingAngle,
	}
	out := prompt
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	// Pages list gets rendered only if {{pages}} appears.
	if strings.Contains(out, "{{pages}}") {
		var b strings.Builder
		for _, p := range pages {
			fmt.Fprintf(&b, "- %s → %s\n", p.Name, p.URL)
		}
		out = strings.ReplaceAll(out, "{{pages}}", b.String())
	}
	return out
}

// buildWizardPrompt is the wizard-driven default prompt used when the user
// does not supply a custom prompt. It's a thinner, more flexible sibling
// to buildNeutralAntiDetectionPrompt — it respects the 4-step wizard
// fields (style / format / language / detail type / extra requirements)
// while keeping the anti-detection and GEO-optimized structure.
func buildWizardPrompt(
	req StartKeywordWorkflowRequest,
	knowledge string,
	pages []struct{ Name, URL string },
) string {
	// News detail type: dedicated structured-news prompt with timeline +
	// parties' responses + FAQ + image placeholders for photo gen.
	if req.DetailType == "news" {
		return buildNewsPrompt(req, knowledge, pages)
	}
	// When the caller uses the GEO content_type and didn't supply wizard
	// fields (legacy callers), fall back to the original hard-coded
	// prompt so we don't regress existing behavior.
	if req.DetailType == "" && req.Style == "" && req.OutputFormat == "" {
		return buildNeutralAntiDetectionPrompt(req, knowledge, pages)
	}

	year := fmt.Sprintf("%d", time.Now().Year())

	langLabel := map[string]string{
		"zh": "中文（简体）", "en": "English", "ja": "日本語",
		"ko": "한국어", "es": "Español", "other": "其他（遵循建议标题的语言）",
	}[req.Language]
	if langLabel == "" {
		langLabel = "中文（简体）"
	}

	detailLabel := map[string]string{
		"article":  "长文资讯 / 主题解析",
		"qa":       "问答合集（每个子标题是一个真实问题）",
		"review":   "横向资料对照（事实陈述，不做价值判断）",
		"tutorial": "入门科普（定义→背景→常见问题）",
	}[req.DetailType]
	if detailLabel == "" {
		detailLabel = "长文资讯"
	}

	styleLabel := map[string]string{
		"professional": "产业观察 / 行业分析，客观克制",
		"casual":       "口语化对话风，适合社交媒体分发",
		"marketing":    "品牌故事风，强调信息价值但避免硬广",
		"technical":    "技术深度，用具体数据和参数支撑",
	}[req.Style]
	if styleLabel == "" {
		styleLabel = "产业观察"
	}

	formatLabel := map[string]string{
		"markdown": "Markdown（## / ### 标题 / 表格 / 引用块）",
		"html":     "HTML 片段（允许 <h2>/<h3>/<p>/<ul>/<table>/<blockquote>），不含 <html>/<body>",
		"plain":    "纯文本（无格式标记，段落之间空一行）",
	}[req.OutputFormat]
	if formatLabel == "" {
		formatLabel = "Markdown"
	}

	var pagesBlock strings.Builder
	if len(pages) > 0 && req.ContentType == "geo" {
		pagesBlock.WriteString("【站内可用内链清单】（从中挑选 2-4 条，以自然锚文本嵌入）：\n")
		for _, p := range pages {
			fmt.Fprintf(&pagesBlock, "- %s → %s\n", p.Name, p.URL)
		}
	}

	extraBlock := ""
	if strings.TrimSpace(req.ExtraRequirements) != "" {
		extraBlock = "\n【附加要求】\n" + strings.TrimSpace(req.ExtraRequirements) + "\n"
	}
	paragraphBlock := ""
	if strings.TrimSpace(req.ParagraphStruct) != "" {
		paragraphBlock = "\n【段落结构建议】\n" + strings.TrimSpace(req.ParagraphStruct) + "\n"
	}
	referenceBlock := ""
	if strings.TrimSpace(req.ReferenceURL) != "" {
		referenceBlock = "\n【参考链接】" + strings.TrimSpace(req.ReferenceURL) + "（可用于启发思路，不得直接复制原文）\n"
	}
	knowledgeBlock := ""
	if strings.TrimSpace(knowledge) != "" {
		knowledgeBlock = "\n【参考资料（请自然融入事实，不得整段照抄）】\n" + strings.TrimSpace(knowledge) + "\n"
	}

	return `你是一名资深内容编辑。请根据以下信息撰写内容：

【核心关键词】` + req.Keyword + `
【建议标题】` + req.TitleText + `
【内容类型】` + detailLabel + `
【写作语言】` + langLabel + `
【行文风格】` + styleLabel + `
【输出格式】` + formatLabel + `
【目标字数】` + fmt.Sprintf("%d", req.WordCount) + ` 字以上
【目标年份】` + year + `
` + paragraphBlock + extraBlock + referenceBlock + knowledgeBlock + pagesBlock.String() + `

━━━━━ 写作原则 ━━━━━
1. 所有 H2 / H3 标题都是"能被独立回答"的问题或明确陈述，不使用模糊分类词
2. 每段首句直接给结论，后文再展开细节，利于 AI 搜索引擎抽取答案
3. 大量具体数字（比例 / 年份 / 金额 / 人数 / 时间跨度），避免空洞形容词
4. 至少 1 个表格（对比 / 并列 / 数据聚合）
5. 至少 1 处引用块（定义 / 数据来源 / 权威陈述）
6. 结尾必须包含"常见问题"章节，6-8 个 Q&A，每个答案 80 字以上
7. 关键词自然分布 4-8 次，禁止堆砌
8. 禁止模板连接词（首先/其次/综上所述/值得注意的是/随着...的发展）
9. 至少 1 处第二人称对话 + 1 处坦承数据局限
10. 句长交替：短句（≤10 字）和长句（≥30 字）混用

━━━━━ 禁用词（最高优先级）━━━━━
博彩类：注册、优惠、流水、赔率、转盘、彩金、娱乐城、博彩、赌场、押注、稳赢、首充、百家乐、体育博彩
诱导类：性感、诱人、销魂、激情、火辣、欲望、撩人、深夜福利、老司机、懂的都懂
促销类：马上、立即、限时、爆款、必选、最佳、首选、不容错过、强烈推荐

━━━━━ 输出要求 ━━━━━
- 直接从首段 / 标题开始输出，不要任何说明或代码块包裹
- 不要输出本提示中的指令原文
- 输出前自查一遍禁用词清单，命中即重写该处`
}

// ─── Prompts ───────────────────────────────────────────────────────────────

// buildNeutralAntiDetectionPrompt is the heart of the pipeline. It is
// tuned specifically for the user's use case:
//
//   - VERTICAL: adult entertainment (影视 / 直播 / 个人创作者经济).
//   - PURPOSE: GEO / AI-answer retrieval (ChatGPT, Gemini, Perplexity
//     抓取时要能从中提取结构化答案); secondary purpose is external
//     backlinks on third-party sites.
//   - FAQ-HEAVY: AI answer engines lift Q&A pairs preferentially, so
//     every section header is framed as an answerable question and
//     each answer leads with its conclusion.
//   - NEUTRAL: discuss the industry, never describe or suggest content;
//     no seductive or promotional modifiers.
//   - ZERO GAMBLING LEAKAGE: the template was forked from a 博彩
//     codebase so we keep an aggressive 禁用词 blocklist.
//   - LOW AI DETECTION: standard humanization — banned cliches,
//     second-person voice, sentence-length variation, concrete numbers,
//     admitted limits.
//   - NATURAL INTERNAL LINKING from site_pages.
func buildNeutralAntiDetectionPrompt(
	req StartKeywordWorkflowRequest,
	knowledge string,
	pages []struct{ Name, URL string },
) string {
	year := fmt.Sprintf("%d", time.Now().Year())

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
		angleBlock = "【写作视角】产业资讯整理——以观察者视角讲述行业动态、平台分类、主流现象，不对任何具体作品/创作者作推销。\n"
	case "攻略":
		angleBlock = "【写作视角】常识科普——讲清楚一个概念/术语/现象是什么、从哪来、为什么被关注，像 Vox 或 The Verge 的解释性报道。\n"
	case "对比":
		angleBlock = "【写作视角】横向资料汇整——把同主题下不同流派/地域/时期的情况并列陈述，只做事实对照不做价值判断。\n"
	case "新手":
		angleBlock = "【写作视角】入门知识——用日常语言讲清楚行话和背景，适合第一次关注这个话题的读者。\n"
	}

	return `你是一名资深内容编辑，常年为生活方式、文化观察、娱乐产业类媒体撰稿，擅长把敏感话题写成信息量高、语气克制的长文。

请围绕以下关键词写一篇 Markdown 文章：

【核心关键词】` + req.Keyword + `
【建议标题】` + req.TitleText + `
【目标字数】` + fmt.Sprintf("%d", req.WordCount) + ` 字以上
【目标年份标签】` + year + `

` + angleBlock + knowledgeBlock + pagesBlock.String() + `

━━━━━ 文章定位（必读！）━━━━━
这是一篇面向 GEO/AI 抓取 和 外链发布 的信息型文章。
核心目标是：让 ChatGPT / Gemini / Perplexity / Google AI Overviews 在回答相关问题时，能从本文中直接抽取结构化答案。
因此本文的最高优先级是：
  ① 每个段落能独立回答一个问题（句首先给结论，再展开细节）
  ② 大量真实的长尾 FAQ（AI 引擎特别偏爱 Q&A 配对）
  ③ 事实密度高、数据具体、来源可追溯（年份 / 比例 / 机构名）
  ④ 自然关键词密度，不做关键词堆砌

这不是品牌推广页、不是平台宣传页、不是导购评测。
全文不得出现任何"推荐""首选""必看""不容错过""最佳"这类导购用语。

━━━━━ 选题角度（关键）━━━━━
该关键词属于成人娱乐领域（可能涉及影视、直播、创作者经济、订阅制产品等产业现象）。
你的写作视角是：**产业观察者 / 文化记者 / 行业分析师**。

允许并鼓励谈这些：
- 行业历史、商业模式演变、市场规模（用具体数字）
- 订阅制 / 付费墙 / 创作者分成 等模式分类与差异
- 不同地区的合规差异、法律法规、内容分级制度
- 社会现象、文化解读、媒体研究视角、学术讨论
- 用户群体画像（年龄段、地域分布、消费习惯的宏观统计）
- 创作者经济的经济学分析（定价、留存、收入分布的公开数据）
- 技术演变（AI 生成内容对行业冲击、DRM、支付通道、年龄验证）

严禁出现这些：
- 任何具体作品、演员、创作者姓名的推荐或排名
- 任何带动作的指引（"如何观看""在哪里下载""怎样订阅"）
- 任何感官、情感、诱惑性的修饰词
- 任何暗示"读完就能看到 / 买到 / 获取"的钩子

━━━━━ 禁用词清单（最高优先级硬约束）━━━━━
【博彩 / 游戏类】出现其中任何一个词 = 整篇作废。输出前必须自查：
注册、优惠、流水、赔率、转盘、彩金、娱乐城、博彩、赌场、跑路、赔付、押注、下注
返水、返利、首充、次充、稳赢、稳赚、百家乐、老虎机、体育博彩、真人视讯、电子游艺、即时返水
"存款 / 提款"（当作博彩语境使用时）
"平台"如果与"注册 / 优惠 / 存款"任一词共同出现也禁止（单独作为中性词用于"视频平台""订阅平台"时例外）

【诱导 / 暗示类】一律禁止：
诱人、销魂、激情、火辣、狂野、欲望、渴望、性感、撩人、勾引、挑逗、刺激
"让你欲罢不能""满足你所有幻想""深夜福利""老司机""懂的都懂"
任何身体部位的直接描写或俚语化替代词

【促销 / 导购类】一律禁止：
马上、立即、限时、爆款、必选、首选、最佳、不容错过、赶紧、抓紧、值得拥有、强烈推荐
"推荐给你""你会爱上""超值""独家""免费获取"

━━━━━ GEO 优化：AI 抓取友好型结构（最高优先级！）━━━━━
这篇文章的最终读者是 AI 搜索引擎，所以结构必须对 AI 抽取友好：

1. **所有 H2 / H3 标题本身是问题或明确陈述，不是模糊类别**
   不要："行业现状"（太笼统）
   要："` + year + ` 年成人娱乐产业的市场规模有多大？"
   或："订阅制和按次付费模式有什么区别"

2. **每个段落的第一句直接给结论，AI 抓取时可以直接用作答案**
   不要："近年来，随着互联网技术的发展，订阅制在..."
   要："订阅制是目前成人娱乐领域最主流的商业模式。根据 2024 年..."

3. **关键数据独立成短句，便于 AI 摘引**
   不要把数字埋在复杂从句里
   好例子："2023 年全球订阅制成人娱乐市场规模约 150 亿美元。"

4. **使用结构化标签强化事实密度**
   - 用 > 引用块标注定义、数据来源、权威引述
   - 用 **粗体** 标注核心数字和专有名词
   - 用表格对比不同维度（年份 / 地区 / 类型 / 占比）

5. **长尾 FAQ 是本文最重要的部分**（结尾至少 6-8 个 Q&A）
   每个 Q 必须是真实用户会在 AI 里问的问法：
   "什么是..."、"为什么..."、"...有多少"、"...是怎么运作的"、"...和 XX 有什么区别"、"...合法吗"、"...市场规模"、"...怎么发展起来的"
   每个 A 必须 80 字以上，首句直接给答案，后续补充背景

━━━━━ 反 AI 检测要求（逐条执行）━━━━━
1. 禁用模板连接词：不要"首先""其次""然后""最后""综上所述""值得注意的是""众所周知""在数字时代""随着...的发展"
2. 连接词口语化：不要"然而""此外""并且"，改用"不过""另外""再说""换句话讲""话说回来"
3. 句长交替：不要连续 3 句长度相近；短句（10 字内）和长句（30 字以上）混着来
4. 至少 1 处第二人称对话：如"你可能好奇..."、"先别急着下结论..."、"如果你第一次听说这个词..."
5. 至少 1 处坦承局限或数据争议：如"不同研究机构口径差异较大"、"这方面公开数据非常有限"
6. 至少 3 个具体数字（比例 / 年份 / 金额 / 人数）——越具体越像真人写的，用"约 87%"而非"大多数"
7. 严禁空洞形容词："丰富的""多元的""优质的""巨大的""显著的"全部换成具体事实

━━━━━ 结构与格式 ━━━━━
- 全文 Markdown，从 ## 二级标题开始（不要 # 一级标题，留给网站模板）
- 至少 3 个 ## 二级标题 + 若干 ### 三级标题；每个标题都是可回答的问题
- 至少 1 个 Markdown 表格（含表头分隔行）——用于并列 / 对比 / 数据聚合
- 至少 1 处 > 引用块（标注定义、数据来源或权威陈述）
- 标题或首段含"` + year + `"年份标签
- **结尾必须有"## 常见问题"章节，包含 6-8 个 ### 问题子标题 + 答案段落**（FAQ 是本文最核心模块）
- 关键词以自然密度分布 4-8 次（含变体），不要机械重复
- 禁止前言（"在这篇文章中我将..."），直接进入第一个 ## 标题

━━━━━ 内链处理 ━━━━━
- 只能使用上面【站内可用内链清单】里的 URL
- 锚文本必须是 2-8 字的自然词组，与上下文语义衔接
- 禁止堆砌；同一段最多 1 个内链
- 清单为空时可不插入内链

━━━━━ 输出要求 ━━━━━
- 直接从 ## 标题开始输出，不要任何解释 / 说明 / 代码块包裹
- 不要输出本提示中的任何指令原文
- 输出前自我检查一遍：禁用词清单里的任何一个词是否在你的草稿里？如果有，必须重写该处
- 全文只输出文章本身（Markdown）`
}

// buildNewsPrompt produces a strictly neutral news-reporting prompt.
// KEY DESIGN: the prompt frames the keyword as a TOPIC TO REPORT ON, not
// a theme to create content around. This prevents AI safety filters from
// refusing when the keyword contains potentially sensitive terms (e.g.
// "porn", "gambling", "fraud") — we're reporting on industry/regulatory
// news related to that topic, not generating explicit material.
//
// Output format matches mainstream news agencies (新华社/央视新闻) with
// timeline + stakeholder responses + expert opinion + background + FAQ.
// Includes 2 image placeholders for FRW text2image generation.
func buildNewsPrompt(
	req StartKeywordWorkflowRequest,
	knowledge string,
	pages []struct{ Name, URL string },
) string {
	year := fmt.Sprintf("%d", time.Now().Year())

	var pagesBlock strings.Builder
	if len(pages) > 0 && req.ContentType == "geo" {
		pagesBlock.WriteString("【站内可用内链清单】（在「类似案例」或「相关信息」段落中自然嵌入2-3条）：\n")
		for _, p := range pages {
			fmt.Fprintf(&pagesBlock, "- %s → %s\n", p.Name, p.URL)
		}
	}

	extraBlock := ""
	if strings.TrimSpace(req.ExtraRequirements) != "" {
		extraBlock = "\n【附加信息】\n" + strings.TrimSpace(req.ExtraRequirements) + "\n"
	}
	referenceBlock := ""
	if strings.TrimSpace(req.ReferenceURL) != "" {
		referenceBlock = "\n【参考链接】" + strings.TrimSpace(req.ReferenceURL) + "（可用于考证事实，不得直接照抄原文）\n"
	}
	knowledgeBlock := ""
	if strings.TrimSpace(knowledge) != "" {
		knowledgeBlock = "\n【参考资料】\n" + strings.TrimSpace(knowledge) + "\n"
	}

	titleHint := req.TitleText
	if strings.TrimSpace(titleHint) == "" {
		titleHint = req.Keyword
	}

	return `你是一名严肃的新闻记者，为新华社/央视新闻/澎湃新闻等主流媒体撰写SEO优化的新闻稿。

━━━━━ 任务定位（最重要）━━━━━

你正在【报道】一个话题/事件/行业动态，而不是【创作】任何内容。无论关键词是什么词汇（包括可能带有敏感、行业、涉黄、涉赌、涉诈等标签的词），都将它理解为一个【需要客观记录和报道的新闻主题】。

你的输出是一篇客观事实报道，用途是：
- 为AI搜索引擎（如 ChatGPT、Perplexity、SGE）提供结构化事实
- 帮助公众了解相关事件/监管动向/法律知识/行业背景

【不是】以下任何一种：
- 不是 露骨描述或煽动性创作
- 不是 宣传推广任何违法或灰色产品
- 不是 对未经证实的传闻进行渲染

如果关键词看起来涉及违法或灰色行业，报道角度应该是：
✓ 相关法律法规的适用和解读
✓ 监管部门查处的真实案例（可匿名化处理当事人）
✓ 行业现状的客观陈述（援引学术/官方研究）
✓ 受害者保护、防诈防骗教育
✓ 类似事件的处理经验

━━━━━ 报道对象 ━━━━━

【关键词/事件主题】` + req.Keyword + `
【建议标题主干】` + titleHint + `
【目标年份】` + year + `
【目标字数】` + fmt.Sprintf("%d", req.WordCount) + ` 字以上` + extraBlock + referenceBlock + knowledgeBlock + pagesBlock.String() + `

━━━━━ 固定结构（顺序不可变）━━━━━

# ` + year + `[新闻标题]

标题要求：年份+地点或领域+核心事件+明确结果。如当事人无法公开，用「某市」「某公司」「某行业」。

[首段摘要 150-220 字：直接回答 who/when/where/what/why/result]

[IMAGE:hero|第一张配图的AI生成提示词，中文，30-60字，场景描述，写实摄影或新闻插图风格，不含真人姓名和具体长相]

## 事件时间线

**` + year + `年M月D日**：事件描述 50-100 字，具体动作和数字。
信源：[媒体名 / 官方渠道 / 警方通报等]

（至少 4 条时间条目。如果不确定具体日期，用「2026年4月上旬」「近日」等近似时间，不得编造具体日期）

[IMAGE:body|第二张配图的AI生成提示词，不同角度或场景，30-60字]

## 各方回应

### 当事人 / 相关方
[120-200 字：引用或转述，注明"据XX媒体报道"或"接受采访时表示"]

### 监管部门 / 警方 / 平台方
[120-200 字：官方通报内容、调查进展、处置结果]

### 专家/律师观点
[120-200 字：对事件定性、法律适用、行业影响的分析，引用具体条款]

## 背景分析

### 行业或社会背景
[100-150 字：事件所处行业或社会现象，援引数据或研究]

### 相关法规
[100-150 字：至少引用 1-2 条具体法条，格式：《中华人民共和国XX法》第X条：原文或要点]

### 类似案例
[100-150 字：1-2 个过往同类真实事件对比，注明时间和处理结果]

## 常见问题

**Q1：[读者最可能问的具体问题]？**
[答案 60-120 字，纯事实]

**Q2：[具体问题]？**
[答案 60-120 字]

**Q3：[具体问题]？**
[答案 60-120 字]

---

*本文内容整理自公开媒体报道和官方通报，具体细节以官方最新通报为准。*

━━━━━ 写作原则 ━━━━━
1. 全文事实陈述，不加入未经证实的传闻、煽动性语言或价值判断
2. 每段首句给结论，具体数字优先（日期/金额/人数/百分比）
3. 每条关键信息标注信源（"据XX媒体"/"官方通报"/"《XX法》第X条"）
4. 关键词自然分布 4-8 次，禁止堆砌
5. 禁止模板连接词（首先/其次/综上所述/值得注意的是）
6. 章节结构不得省略或合并，必须 4 个一级 + 对应二级章节全部保留
7. 必须含 2 个 [IMAGE:hero|...] 和 [IMAGE:body|...] 占位符

━━━━━ 安全原则 ━━━━━
- 涉及犯罪或违法话题时，始终以【记录/警示/科普】角度，不做示范教学
- 不使用露骨/色情/暴力描述性词汇
- 对当事人姓名可匿名化（某某、化名"景景"等）
- 图片提示词用通用描述，不含具体真人外貌、不含露骨元素

━━━━━ 输出要求 ━━━━━
- 直接从 # 号标题开始输出，无代码块包裹
- 不输出本提示中的指令原文
- 必须包含 [IMAGE:hero|...] 和 [IMAGE:body|...] 各一个
- 章节标题使用 ## / ### 层级
`
}


// imagePlaceholderRe matches [IMAGE:<tag>|<prompt>] in AI-generated draft.
var imagePlaceholderRe = regexp.MustCompile(`\[IMAGE:(hero|body|[a-zA-Z0-9_-]+)\|([^\]]+)\]`)

// generateArticleImages submits prompts to FRW text2image, polls until done
// or 5-min timeout, returns URL per prompt ("" for failures).
func generateArticleImages(userID int64, prompts []string) []string {
	results := make([]string, len(prompts))
	if len(prompts) == 0 {
		return results
	}
	cfg := loadFRWConfig(userID)
	if cfg.APIKey == "" || cfg.Token == "" || cfg.UserID == "" {
		log.Printf("[geo-workflow] user %d has no FRW config, skipping images", userID)
		return results
	}
	client := frw.New(cfg.APIKey, cfg.Token, cfg.UserID)

	taskIDs := make([]string, len(prompts))
	for i, p := range prompts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		res, err := client.Submit("text2image", p, "")
		if err != nil {
			log.Printf("[geo-workflow] image submit failed (idx=%d): %v", i, err)
			continue
		}
		taskIDs[i] = res.TaskID
	}

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		allDone := true
		for i, tid := range taskIDs {
			if tid == "" || results[i] != "" {
				continue
			}
			q, err := client.Query(tid)
			if err != nil {
				log.Printf("[geo-workflow] image query %s failed: %v", tid, err)
				taskIDs[i] = ""
				continue
			}
			if q.Status == 2 && q.OutData != "" {
				results[i] = q.OutData
				log.Printf("[geo-workflow] image %d done: %s", i, q.OutData)
			} else if q.Status == -1 || q.Status == 4 {
				log.Printf("[geo-workflow] image %d failed (status=%d): %s", i, q.Status, q.Message)
				taskIDs[i] = ""
			} else {
				allDone = false
			}
		}
		if allDone {
			return results
		}
		time.Sleep(20 * time.Second)
	}
	log.Printf("[geo-workflow] image gen timed out; partial results returned")
	return results
}

func replaceImagePlaceholders(draft string, prompts []string, urls []string) string {
	idx := 0
	return imagePlaceholderRe.ReplaceAllStringFunc(draft, func(match string) string {
		cur := idx
		idx++
		if cur >= len(urls) {
			return ""
		}
		url := urls[cur]
		if url == "" {
			return ""
		}
		alt := ""
		if cur < len(prompts) {
			alt = prompts[cur]
			if len([]rune(alt)) > 40 {
				alt = string([]rune(alt)[:40]) + "..."
			}
		}
		return fmt.Sprintf("![%s](%s)", alt, url)
	})
}

func extractImagePrompts(draft string) []string {
	matches := imagePlaceholderRe.FindAllStringSubmatch(draft, -1)
	out := make([]string, len(matches))
	for i, m := range matches {
		if len(m) >= 3 {
			out[i] = strings.TrimSpace(m[2])
		}
	}
	return out
}

// buildMetaExtractPrompt produces step-2's tight JSON-only prompt.
// Step 1 already generates 6-8 FAQs inside the article body, so
// Step 2's job is narrower: pick a solid meta_title + meta_desc
// and surface the FAQs that are already in the draft as a
// structured array (for schema.org FAQ injection on the site).
// We use string concatenation (not Sprintf) — a user-supplied keyword
// containing "%" would otherwise corrupt the format directives.
func buildMetaExtractPrompt(keyword, title, excerpt string) string {
	year := fmt.Sprintf("%d", time.Now().Year())
	return `你是 SEO / GEO 元数据提取助手。下面是一篇已写好的信息型文章（主题偏成人娱乐产业分析，目标是被 AI 搜索引擎引用）。

请从中提取 SEO 元数据 + 已存在的 FAQ 列表。

【核心关键词】` + keyword + `
【建议标题】` + title + `

【文章内容】
` + excerpt + `

任务：输出纯 JSON（不要 markdown 代码块，不要解释文字），字段：
- meta_title：SEO 标题，50 字内
    必须含核心关键词 + 年份【` + year + `】
    语气像维基百科条目或产业报告摘要，禁止推销
- meta_description：Meta 描述，140-160 字
    【` + year + `】开头，全文必须是事实陈述
    禁止行动号召（"马上""立即""推荐"）
- faq：从正文"## 常见问题"章节里提取出来的 Q&A 数组
    每个元素 {"q":"问题","a":"答案，至少 60 字"}
    按正文里的顺序保留，不要改写问题，只做必要的答案精简
    正文如果有 6 个 FAQ 就返回 6 个，有 8 个就返回 8 个
    正文没有 FAQ 章节时返回空数组

━━━━━ 硬性禁用词（任何字段都不能出现）━━━━━
博彩类：注册、优惠、流水、赔率、转盘、彩金、娱乐城、博彩、赌场、跑路、押注、下注、稳赢、首充、百家乐、老虎机、体育博彩、真人视讯
诱导类：性感、诱人、销魂、激情、火辣、欲望、撩人、勾引、深夜福利、老司机、懂的都懂
促销类：马上、立即、限时、爆款、必选、最佳推荐、首选、抓紧、不容错过

━━━━━ 语气要求 ━━━━━
- 全部字段保持 "产业观察 / 文化记者 / 行业分析" 的客观语气
- 标题可以用"市场规模""发展历程""模式解析""现状与争议""常见问题"这类引子
- 不做任何价值判断或推荐

直接输出 JSON，不要任何其他字符。

示例格式（字段示意，请生成真实内容）：
{"meta_title":"【` + year + `】XX 市场规模、模式分类与常见问题","meta_description":"【` + year + `】本文整理 XX 的定义、产业模式与 2024 年市场数据，数据来源包括...","faq":[{"q":"什么是 XX","a":"60 字以上的事实陈述..."}]}`
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


// buildNewsPromptPreview returns a human-readable preview of the news
// prompt structure shown in the wizard's step-3 editor when detail_type
// is "news". Unlike other types, this text is informational only —
// the actual generation uses buildNewsPrompt at runtime.
func buildNewsPromptPreview() string {
	return `【新闻资讯模式】— 自动使用专用结构化 prompt (step 3 内容仅供参考, 不可编辑)

本 prompt 会生成严格结构的新闻稿:

━━━━━ 固定结构 ━━━━━
# YYYY [新闻标题]
[摘要段 150-220字]
[IMAGE:hero|配图提示词]

## 事件时间线
- 至少 4 条日期条目 + 信源

[IMAGE:body|配图提示词]

## 各方回应
### 当事人 / 相关方
### 监管部门 / 警方 / 平台方
### 专家 / 律师观点

## 背景分析
### 行业背景
### 相关法规 (引用具体条款)
### 类似案例

## 常见问题 (Q1/Q2/Q3)

---
免责声明

━━━━━ 写作角度 ━━━━━
你是主流媒体记者 (新华社/央视/澎湃) 的角色, 客观中立报道.

无论关键词是什么词汇 (可能包含敏感/行业/涉黄涉赌涉诈等标签),
都视为【需要客观报道的新闻主题】, 而不是创作题材.

如涉及敏感话题, 报道角度:
- 相关法律法规适用与解读
- 监管部门查处真实案例 (匿名化处理)
- 行业现状客观陈述
- 防诈/警示/科普

━━━━━ 约束 ━━━━━
- 不使用露骨/色情/暴力描述
- 人物姓名可匿名化 (某某/化名)
- 图片提示词用通用描述 (警员/家属等)
- 事实陈述, 无价值判断
- 具体数字优先, 每条信息标注信源
- 2 张 FRW 配图通过 [IMAGE:hero|...] 和 [IMAGE:body|...] 占位符生成
`
}

// newsPromptNotice returns the banner text shown above the step-3 prompt
// editor when user selects a detail_type that uses a fixed prompt.
func newsPromptNotice(detailType string) string {
	if detailType == "news" {
		return "ℹ️ 新闻资讯模式使用固定结构化 prompt, 此处编辑不会生效. 关键词可以是任何话题 (包括可能敏感的词), AI 会以客观记者角度进行报道."
	}
	return ""
}
