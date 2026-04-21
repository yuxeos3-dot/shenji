package api

// Weekly reports (周汇报). A user picks a week range, we pull every
// daily_reports row they filed in that window, concatenate into a
// prompt, and have the AI produce a tidy weekly summary. The draft
// lands in the editor for human polish; 'submit' locks the row.
//
// Permission: members see/manage their own; admins can query anyone
// via ?user_id=N on list, and can read any row on detail. Writes are
// always limited to the owner (admins don't ghost-write subordinates'
// weekly reports — they're accountable for their own content).

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

	"seo-system/internal/ai"
	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

// ─── Types ──────────────────────────────────────────────────────────────────

type weeklyReportRow struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"user_id"`
	UserName       string  `json:"user_name,omitempty"`
	DepartmentName string  `json:"department_name,omitempty"`
	GroupName      string  `json:"group_name,omitempty"`
	WeekStart      string  `json:"week_start"`
	WeekEnd        string  `json:"week_end"`
	Content        string  `json:"content"`
	SourceDailyIDs string  `json:"source_daily_ids"`
	Status         string  `json:"status"`
	SubmittedAt    *string `json:"submitted_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// validDateYMD checks a YYYY-MM-DD string by trying to parse it. We
// accept only this strict format to avoid timezone ambiguity — the
// frontend's el-date-picker emits exactly this.
func validDateYMD(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// dateDiffDays returns end - start in days, inclusive. Negative if
// start > end.
func dateDiffDays(start, end string) int {
	s, err1 := time.Parse("2006-01-02", start)
	e, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int(e.Sub(s).Hours()/24) + 1
}

// loadWeeklyReport reads a single row. Returns sql.ErrNoRows if missing.
// Used by every handler that needs to gate access on ownership + status.
func loadWeeklyReport(id int64) (*weeklyReportRow, error) {
	var r weeklyReportRow
	var submittedAt sql.NullString
	err := db.DB.QueryRow(`
		SELECT w.id, w.user_id, COALESCE(u.name, ''),
		       COALESCE(d.name, ''), COALESCE(g.name, ''),
		       w.week_start, w.week_end, w.content, w.source_daily_ids,
		       w.status, w.submitted_at, w.created_at, w.updated_at
		FROM weekly_reports w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE w.id = $1`, id,
	).Scan(&r.ID, &r.UserID, &r.UserName, &r.DepartmentName, &r.GroupName,
		&r.WeekStart, &r.WeekEnd, &r.Content, &r.SourceDailyIDs,
		&r.Status, &submittedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if submittedAt.Valid {
		r.SubmittedAt = &submittedAt.String
	}
	return &r, nil
}

// canAccessWeekly: admin always, owner always, everyone else denied.
// Modifies the response if access is denied; caller just returns.
func canAccessWeekly(c *gin.Context, r *weeklyReportRow) bool {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if role == "admin" || r.UserID == uid {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问该周汇报"})
	return false
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// ListWeeklyReports returns the caller's own weekly reports, or for
// admins, optionally anyone's via ?user_id=N. Scope is clear-cut so
// the frontend doesn't need to guess which set it's looking at.
func ListWeeklyReports(c *gin.Context) {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")

	target := uid
	if qUID := c.Query("user_id"); qUID != "" {
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员可查看他人汇报"})
			return
		}
		// Just parse — backend validates the id exists implicitly via JOIN.
		fmt.Sscanf(qUID, "%d", &target)
	}

	rows, err := db.DB.Query(`
		SELECT w.id, w.user_id, COALESCE(u.name, ''),
		       COALESCE(d.name, ''), COALESCE(g.name, ''),
		       w.week_start, w.week_end, w.content, w.source_daily_ids,
		       w.status, w.submitted_at, w.created_at, w.updated_at
		FROM weekly_reports w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE w.user_id = $1
		ORDER BY w.week_start DESC, w.id DESC`, target)
	if err != nil {
		log.Printf("[weekly] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []weeklyReportRow{}
	for rows.Next() {
		var r weeklyReportRow
		var submittedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.UserID, &r.UserName, &r.DepartmentName, &r.GroupName,
			&r.WeekStart, &r.WeekEnd, &r.Content, &r.SourceDailyIDs,
			&r.Status, &submittedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		if submittedAt.Valid {
			r.SubmittedAt = &submittedAt.String
		}
		items = append(items, r)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

// AdminListAllWeeklyReports lets an admin browse every user's reports
// across the organization. Groups by department and group. Useful for
// the 「汇报管理」 page when an admin wants to see who filed what this
// week. Supports filters: ?department_id=, ?group_id=, ?week_start=
// (exact match).
func AdminListAllWeeklyReports(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员"})
		return
	}

	where := []string{"1=1"}
	args := []interface{}{}
	if v := c.Query("department_id"); v != "" {
		where = append(where, fmt.Sprintf("u.department_id = $%d", len(args)+1))
		args = append(args, v)
	}
	if v := c.Query("group_id"); v != "" {
		where = append(where, fmt.Sprintf("u.group_id = $%d", len(args)+1))
		args = append(args, v)
	}
	if v := c.Query("week_start"); v != "" {
		where = append(where, fmt.Sprintf("w.week_start = $%d", len(args)+1))
		args = append(args, v)
	}
	if q := strings.TrimSpace(c.Query("user_search")); q != "" {
		where = append(where, fmt.Sprintf("LOWER(COALESCE(u.name,'')) LIKE $%d", len(args)+1))
		args = append(args, "%"+strings.ToLower(q)+"%")
	}

	q := `
		SELECT w.id, w.user_id, COALESCE(u.name, ''),
		       COALESCE(d.name, ''), COALESCE(g.name, ''),
		       w.week_start, w.week_end, w.content, w.source_daily_ids,
		       w.status, w.submitted_at, w.created_at, w.updated_at
		FROM weekly_reports w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY w.week_start DESC, u.department_id, u.group_id, w.user_id
		LIMIT 500`

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		log.Printf("[weekly] admin list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []weeklyReportRow{}
	for rows.Next() {
		var r weeklyReportRow
		var submittedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.UserID, &r.UserName, &r.DepartmentName, &r.GroupName,
			&r.WeekStart, &r.WeekEnd, &r.Content, &r.SourceDailyIDs,
			&r.Status, &submittedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		if submittedAt.Valid {
			r.SubmittedAt = &submittedAt.String
		}
		items = append(items, r)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

// GenerateWeeklyRequest is the payload for creating a new weekly draft
// from a date range. Both dates are inclusive.
type GenerateWeeklyRequest struct {
	WeekStart string `json:"week_start" binding:"required"`
	WeekEnd   string `json:"week_end" binding:"required"`
}

// GenerateWeeklyReport creates a draft by pulling all daily reports in
// the given range, concatenating, asking the AI to summarize, and
// storing the result. If a weekly report for the same (user, range)
// already exists, we 409 with the existing id so the frontend can
// redirect instead of silently overwriting.
func GenerateWeeklyReport(c *gin.Context) {
	var req GenerateWeeklyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if !validDateYMD(req.WeekStart) || !validDateYMD(req.WeekEnd) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "日期格式应为 YYYY-MM-DD"})
		return
	}
	days := dateDiffDays(req.WeekStart, req.WeekEnd)
	if days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "起止日期不合法（end 必须 ≥ start）"})
		return
	}
	// Cap at 14 days so someone can't submit a six-month window and
	// tank the AI context. Typical use is 7 days; 14 covers edge cases
	// like taking a week off and combining two weeks.
	if days > 14 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "周汇报范围不可超过 14 天"})
		return
	}

	uid := c.GetInt64("user_id")

	// Dedup by (user, week_start, week_end). If a draft already exists,
	// return its id and let the frontend jump to the editor rather than
	// overwriting what the user polished before.
	var existingID int64
	db.DB.QueryRow(
		`SELECT id FROM weekly_reports WHERE user_id = $1 AND week_start = $2 AND week_end = $3`,
		uid, req.WeekStart, req.WeekEnd,
	).Scan(&existingID)
	if existingID > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code":  -1,
			"error": "该时间段已有周汇报（可直接进入编辑）",
			"data":  gin.H{"id": existingID},
		})
		return
	}

	// Pull the caller's dailies in range.
	rows, err := db.DB.Query(
		`SELECT id, report_date, content FROM daily_reports
		 WHERE user_id = $1 AND report_date BETWEEN $2 AND $3
		 ORDER BY report_date ASC`,
		uid, req.WeekStart, req.WeekEnd,
	)
	if err != nil {
		log.Printf("[weekly] daily fetch: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询日汇报失败"})
		return
	}
	defer rows.Close()

	type dailyPiece struct {
		ID      int64
		Date    string
		Content string
	}
	var pieces []dailyPiece
	for rows.Next() {
		var p dailyPiece
		if err := rows.Scan(&p.ID, &p.Date, &p.Content); err == nil {
			pieces = append(pieces, p)
		}
	}
	if len(pieces) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1, "error": "该时间范围内没有日汇报，无法生成周汇报",
		})
		return
	}

	// Build a JSON array of source daily ids for attribution + audit.
	srcIDs := make([]int64, 0, len(pieces))
	for _, p := range pieces {
		srcIDs = append(srcIDs, p.ID)
	}
	srcJSON, _ := json.Marshal(srcIDs)

	// Concatenate into a big prompt. We intentionally pass each
	// daily's raw content verbatim so the AI can deduplicate recurring
	// tasks / issues / agent applications across the week.
	var concat strings.Builder
	for _, p := range pieces {
		fmt.Fprintf(&concat, "## %s\n%s\n\n---\n\n", p.Date, p.Content)
	}

	summary, err := summarizeWeekWithAI(uid, req.WeekStart, req.WeekEnd, concat.String())
	if err != nil || strings.TrimSpace(summary) == "" {
		// Fallback: keep the raw concatenation so the user at least has
		// something to edit. The UI shows a banner that AI polishing
		// didn't run, so they can retry via the "AI 润色" button.
		log.Printf("[weekly] AI summary failed (falling back to concat): %v", err)
		summary = "> ⚠️ AI 润色未能运行，以下是本周日汇报的原始拼接，请手动整理后提交。\n\n" + concat.String()
	}

	var id int64
	err = db.DB.QueryRow(
		`INSERT INTO weekly_reports
		 (user_id, week_start, week_end, content, source_daily_ids, status)
		 VALUES ($1, $2, $3, $4, $5, 'draft') RETURNING id`,
		uid, req.WeekStart, req.WeekEnd, summary, string(srcJSON),
	).Scan(&id)
	if err != nil {
		log.Printf("[weekly] insert: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	WriteAuditLog(c, "weekly_generate", "weekly_report", fmt.Sprintf("%d", id),
		fmt.Sprintf("%s ~ %s (%d 篇日汇报)", req.WeekStart, req.WeekEnd, len(pieces)))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// summarizeWeekWithAI calls the configured AI endpoint with a prompt
// that asks for a structured weekly summary. Mirrors polishWithAI's
// HTTP plumbing but uses a different instruction.
func summarizeWeekWithAI(userID int64, start, end, concatenated string) (string, error) {
	apiURL, apiKey, model := ResolveAI(userID)
	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if apiKey == "" {
		return "", fmt.Errorf("AI 未配置：请先在「个人 AI ���置」中配置 API Key")
	}

	prompt := fmt.Sprintf(`你是一名职场周报总结专家。下面是一名员工本周（%s ~ %s）每天写的日汇报原文，按日期拼接而成。

你的任务：
1. 梳理并去重本周所有实质性工作，合并相近事项（同一件事不同日期推进的，合并成一条并注明起止）
2. 抽出核心数据、关键交付、重大进展
3. 总结本周遇到的问题 + 解决情况（未解决的明确列出）
4. 汇总本周 Agent 自动化应用（如果日汇报里有写），统计实际节省工时总和
5. 给出下周工作重点（基于本周未完成事项 + 日汇报里的明日计划整合）

## 本周日汇报原文
%s

## 输出格式（严格 Markdown，五级结构）

### 一、本周工作总结
（按业务模块分类列出本周完成的工作，每条一行，简洁克制）

### 二、核心数据 / 关键交付
（用 - 列表，带具体数字；没有则写"本周无量化交付"）

### 三、遇到的问题及解决
- 已解决：...
- 未解决：...（如果没有未解决的写"无"）

### 四、Agent 自动化应用汇总
（合并本周所有 Agent 应用条目，去重后列表展示；最底部加一行：本周累计节省工时：XX 小时）
（如日汇报里完全没有 Agent 条目，写"本周无新增自动化应用"）

### 五、下周工作重点
（3-5 条，按优先级排列）

## 输出规则
- 语气专业克制，不要使用感叹号
- 不要加「本人」「我今天」「这周我」等口语化字眼
- 每个一级标题后空一行再写内容
- 不要输出任何超出上述 5 个章节的多余内容
- 不要包裹在 markdown 代码块里`,
		start, end, concatenated,
	)

	body := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		ai.MaxTokensKey(model):  4000,
		"temperature": 0.3,
	}
	bj, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(bj))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(120 * time.Second)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
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
	// Strip any accidental ```markdown wrapping.
	if strings.HasPrefix(out, "```") {
		out = strings.TrimPrefix(out, "```markdown")
		out = strings.TrimPrefix(out, "```")
		out = strings.TrimSuffix(out, "```")
		out = strings.TrimSpace(out)
	}
	return out, nil
}

// GetWeeklyReport returns a single row. Readable by owner + any admin.
func GetWeeklyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadWeeklyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "周汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	if !canAccessWeekly(c, r) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": r})
}

// UpdateWeeklyReportRequest is partial — only content is editable.
// Date range is fixed at creation time to avoid weird edge cases
// (what about the dailies that no longer fall in the new range$1).
type UpdateWeeklyReportRequest struct {
	Content string `json:"content"`
}

func UpdateWeeklyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var req UpdateWeeklyReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	r, err := loadWeeklyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "周汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	// Only the owner can edit — admins don't rewrite subordinates' reports.
	uid := c.GetInt64("user_id")
	if r.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能编辑自己的周汇报"})
		return
	}
	if r.Status != "draft" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "周汇报已提交，不可再编辑"})
		return
	}

	_, err = db.DB.Exec(
		`UPDATE weekly_reports SET content = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
		req.Content, id,
	)
	if err != nil {
		log.Printf("[weekly] update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存"})
}

// SubmitWeeklyReport locks the row. After this, content is read-only
// (but admins can still view via GET). Same flow as daily reports —
// the submission IS the formal handoff to the manager.
func SubmitWeeklyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadWeeklyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "周汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	if r.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能提交自己的周汇报"})
		return
	}
	if r.Status == "submitted" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已是提交状态"})
		return
	}
	if strings.TrimSpace(r.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "内容为空，不能提交"})
		return
	}

	_, err = db.DB.Exec(
		`UPDATE weekly_reports SET status = 'submitted', submitted_at = CURRENT_TIMESTAMP,
		 updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id,
	)
	if err != nil {
		log.Printf("[weekly] submit: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}
	WriteAuditLog(c, "weekly_submit", "weekly_report", fmt.Sprintf("%d", id),
		fmt.Sprintf("%s ~ %s", r.WeekStart, r.WeekEnd))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已提交"})
}

// PolishWeeklyReport re-runs the AI summary on the current draft. Useful
// when the user tweaked something and wants AI to clean it up again.
// Requires status='draft' and a non-empty body.
func PolishWeeklyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadWeeklyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "周汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	if r.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能润色自己的周汇报"})
		return
	}
	if r.Status != "draft" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "周汇报已提交，不可再润色"})
		return
	}
	if strings.TrimSpace(r.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "内容为空，无需润色"})
		return
	}

	// Use the current content as the "concatenated input" — re-polishing
	// a polished draft is fine, the AI prompt tolerates either.
	polished, err := summarizeWeekWithAI(uid, r.WeekStart, r.WeekEnd, r.Content)
	if err != nil || strings.TrimSpace(polished) == "" {
		log.Printf("[weekly] polish failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "AI 润色失败：" + fmt.Sprint(err)})
		return
	}
	// Don't overwrite yet — return the candidate so the user can decide.
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"content": polished}})
}

// DeleteWeeklyReport removes a draft. Submitted reports cannot be deleted
// (留痕 principle — once handed up the chain, we keep it).
func DeleteWeeklyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadWeeklyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "周汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if r.UserID != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅作者或管理员可删除"})
		return
	}
	if r.Status == "submitted" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "已提交的周汇报不可删除"})
		return
	}
	if _, err := db.DB.Exec(`DELETE FROM weekly_reports WHERE id = $1`, id); err != nil {
		log.Printf("[weekly] delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	WriteAuditLog(c, "weekly_delete", "weekly_report", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}
