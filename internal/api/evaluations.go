package api

// Report evaluations (汇报评价).
//
// Polymorphic: one evaluations table serves daily/weekly/monthly
// reports via (report_type, report_id). Admins write; report owners
// + any admin can read.
//
// We keep it a single small module because the three report variants
// all share the same evaluation shape — pulling them apart into three
// tables would triple the code without adding clarity.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

// validEvalReportType ensures the caller isn't passing arbitrary
// strings into SQL. Keep in sync with the frontend's
// report_type union and with every place we insert a polymorphic
// row.
func validEvalReportType(s string) bool {
	switch s {
	case "daily", "weekly", "monthly":
		return true
	}
	return false
}

// evaluationRow is the API shape; matches frontend expectations.
type evaluationRow struct {
	ID            int64  `json:"id"`
	ReportType    string `json:"report_type"`
	ReportID      int64  `json:"report_id"`
	EvaluatorID   int64  `json:"evaluator_id"`
	EvaluatorName string `json:"evaluator_name"`
	Content       string `json:"content"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// lookupReportOwner returns the user_id that owns a given report.
// Used to gate read access so non-admin, non-owner callers can't
// poke at other people's evaluations. Returns 0 if the report
// doesn't exist (handler treats that as 404).
func lookupReportOwner(reportType string, reportID int64) int64 {
	var table string
	switch reportType {
	case "daily":
		table = "daily_reports"
	case "weekly":
		table = "weekly_reports"
	case "monthly":
		table = "monthly_reports"
	default:
		return 0
	}
	var uid int64
	// Safe: table is from a whitelist above, not user input.
	q := "SELECT user_id FROM " + table + " WHERE id = ?"
	if err := db.DB.QueryRow(q, reportID).Scan(&uid); err == sql.ErrNoRows {
		return 0
	} else if err != nil {
		log.Printf("[eval] lookupReportOwner(%s,%d): %v", reportType, reportID, err)
		return 0
	}
	return uid
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// ListEvaluations returns every evaluation attached to a given
// (report_type, report_id). Readable by the report's owner and any
// admin — the detail page shows evaluations inline.
func ListEvaluations(c *gin.Context) {
	reportType := c.Query("report_type")
	reportIDStr := c.Query("report_id")
	if !validEvalReportType(reportType) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "report_type 非法"})
		return
	}
	var reportID int64
	fmt.Sscanf(reportIDStr, "%d", &reportID)
	if reportID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "report_id 必填"})
		return
	}

	// Access check: owner or admin only.
	ownerID := lookupReportOwner(reportType, reportID)
	if ownerID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "汇报不存在"})
		return
	}
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if role != "admin" && ownerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权查看该汇报的评价"})
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, report_type, report_id, evaluator_id,
		       COALESCE(evaluator_name, ''), content,
		       created_at, updated_at
		FROM report_evaluations
		WHERE report_type = ? AND report_id = ?
		ORDER BY created_at ASC`,
		reportType, reportID,
	)
	if err != nil {
		log.Printf("[eval] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []evaluationRow{}
	for rows.Next() {
		var r evaluationRow
		if err := rows.Scan(&r.ID, &r.ReportType, &r.ReportID, &r.EvaluatorID,
			&r.EvaluatorName, &r.Content, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		items = append(items, r)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

// CreateEvaluationRequest covers the upsert POST body.
type CreateEvaluationRequest struct {
	ReportType string `json:"report_type" binding:"required"`
	ReportID   int64  `json:"report_id" binding:"required"`
	Content    string `json:"content" binding:"required"`
}

// CreateEvaluation upserts the evaluation for (report, evaluator).
// SQLite handles the "UPDATE on conflict" via the unique index. We
// explicitly update updated_at + content on conflict so an admin who
// edits their evaluation bumps the timestamp for audit.
//
// Admin-only.
func CreateEvaluation(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员可评价"})
		return
	}

	var req CreateEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if !validEvalReportType(req.ReportType) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "report_type 非法"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "评价内容不能为空"})
		return
	}
	if ownerID := lookupReportOwner(req.ReportType, req.ReportID); ownerID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "汇报不存在"})
		return
	}

	uid := c.GetInt64("user_id")
	if uid <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "未登录或登录已失效"})
		return
	}
	// Look up evaluator display name for the stored row — we denormalise
	// so the list endpoint doesn't need to join users (which would
	// confuse the later "admin renamed" case, where we want the
	// evaluation to show the name that was in effect at write time).
	var name string
	if err := db.DB.QueryRow(`SELECT name FROM users WHERE id = ?`, uid).Scan(&name); err != nil && err != sql.ErrNoRows {
		log.Printf("[eval] evaluator name lookup failed uid=%d: %v", uid, err)
	}
	if name == "" {
		name = fmt.Sprintf("用户#%d", uid)
	}

	_, err := db.DB.Exec(`
		INSERT INTO report_evaluations
		  (report_type, report_id, evaluator_id, evaluator_name, content)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(report_type, report_id, evaluator_id) DO UPDATE SET
		  content = excluded.content,
		  evaluator_name = excluded.evaluator_name,
		  updated_at = CURRENT_TIMESTAMP
	`, req.ReportType, req.ReportID, uid, name, req.Content)
	if err != nil {
		log.Printf("[eval] upsert: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	WriteAuditLog(c, "eval_upsert", "report_evaluation",
		fmt.Sprintf("%s:%d", req.ReportType, req.ReportID), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存评价"})
}

// DeleteEvaluation removes the caller's own evaluation. Admin can only
// delete evaluations they wrote themselves — we never let one admin
// nuke another admin's comment silently.
func DeleteEvaluation(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员"})
		return
	}
	id := parseInt64Param(c, "id")
	uid := c.GetInt64("user_id")

	// Confirm ownership before delete. Doing this in a single exec
	// with a WHERE id=? AND evaluator_id=? is also safe, but the
	// explicit lookup gives a clearer error message when the id is
	// valid-but-not-yours.
	var evalUID int64
	err := db.DB.QueryRow(
		`SELECT evaluator_id FROM report_evaluations WHERE id = ?`, id,
	).Scan(&evalUID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "评价不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	if evalUID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能删除自己发表的评价"})
		return
	}

	if _, err := db.DB.Exec(`DELETE FROM report_evaluations WHERE id = ?`, id); err != nil {
		log.Printf("[eval] delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	WriteAuditLog(c, "eval_delete", "report_evaluation", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}

// PolishEvaluationRequest: the current draft + its target so we can
// optionally pass context ("this is a weekly report evaluation...")
// to the AI. Target is optional — if not set, we still polish but
// without report-type context.
type PolishEvaluationRequest struct {
	Content    string `json:"content" binding:"required"`
	ReportType string `json:"report_type"`
	ReportID   int64  `json:"report_id"`
}

// PolishEvaluation runs the draft through AI and returns a candidate
// rewrite. Doesn't touch the DB — the client decides whether to keep
// the AI version via the compare dialog, then saves via the existing
// CreateEvaluation upsert.
//
// Admin only (only admins write evaluations).
func PolishEvaluation(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员"})
		return
	}
	var req PolishEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "内容为空，无需润色"})
		return
	}

	uid := c.GetInt64("user_id")
	polished, err := polishEvaluationWithAI(uid, req.ReportType, req.Content)
	if err != nil || strings.TrimSpace(polished) == "" {
		log.Printf("[eval] polish failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":  -1,
			"error": "AI 润色失败：" + fmt.Sprint(err),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"content": polished}})
}

// polishEvaluationWithAI mirrors the weekly/monthly summarizer plumbing
// but uses an evaluation-specific prompt: short, professional, objective.
// We specifically tell the model NOT to change the evaluator's stance
// or inject new opinions — that would undermine the admin's accountability
// for their own feedback.
func polishEvaluationWithAI(userID int64, reportType, draft string) (string, error) {
	apiURL, apiKey, model := ResolveAI(userID)
	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if apiKey == "" {
		return "", fmt.Errorf("AI 未配置：请先在「个人 AI 设置」中配置 API Key")
	}

	typeLabel := "汇报"
	switch reportType {
	case "daily":
		typeLabel = "日汇报"
	case "weekly":
		typeLabel = "周汇报"
	case "monthly":
		typeLabel = "月汇报"
	}

	prompt := fmt.Sprintf(`你是一名职场评语润色助手。下面是一名管理者对员工 %s 写的评价草稿，请帮 ta 润色成更简洁专业的表达。

## 原始草稿
%s

## 润色规则（严格遵守）
1. 保持原意、立场、语气倾向 100%% 不变 —— 不得强化、弱化、反转评价倾向
2. 不得新增原文没有的事实、数据、建议
3. 不得删除原文提到的具体事项（除非是纯口语化的填充词）
4. 不得把原文的批评软化成表扬，也不得反过来
5. 允许改善的范围: 
   - 去除口语/重复/冗余
   - 改善措辞的专业度
   - 修正错别字、标点、语法
   - 合并相邻的相似观点
6. 不要使用感叹号
7. 不要加"本人""我觉得""个人认为"等第一人称口水词
8. 不要加签名、抬头、结束语
9. 直接输出润色后的评价文本，不要包裹在 markdown 代码块里，不要加任何解释`,
		typeLabel, draft)

	body := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  1200,
		"temperature": 0.25,
	}
	bj, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(bj))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(60 * time.Second)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("AI HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("AI 无返回")
	}
	out := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if strings.HasPrefix(out, "```") {
		out = strings.TrimPrefix(out, "```markdown")
		out = strings.TrimPrefix(out, "```")
		out = strings.TrimSuffix(out, "```")
		out = strings.TrimSpace(out)
	}
	return out, nil
}
