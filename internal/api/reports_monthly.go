package api

// Monthly reports (月汇报). One per (user, year, month). Generation
// pulls every submitted weekly report for the user whose week_start
// falls in that month and asks AI to produce a structured monthly
// summary. Draft → submitted state machine, same as weekly.
//
// Why "submitted" weeklies only: a draft weekly is still being edited,
// pulling it into the month summary would capture half-baked content
// and put the manager's trust at risk. The user controls the rollup by
// deciding when each weekly is final.

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

// ─── Types ──────────────────────────────────────────────────────────────────

type monthlyReportRow struct {
	ID              int64   `json:"id"`
	UserID          int64   `json:"user_id"`
	UserName        string  `json:"user_name,omitempty"`
	DepartmentName  string  `json:"department_name,omitempty"`
	GroupName       string  `json:"group_name,omitempty"`
	Year            int     `json:"year"`
	Month           int     `json:"month"`
	Content         string  `json:"content"`
	SourceWeeklyIDs string  `json:"source_weekly_ids"`
	Status          string  `json:"status"`
	SubmittedAt     *string `json:"submitted_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// loadMonthlyReport reads a single row with user + org metadata joined.
func loadMonthlyReport(id int64) (*monthlyReportRow, error) {
	var r monthlyReportRow
	var submittedAt sql.NullString
	err := db.DB.QueryRow(`
		SELECT m.id, m.user_id, COALESCE(u.name, ''),
		       COALESCE(d.name, ''), COALESCE(g.name, ''),
		       m.year, m.month, m.content, m.source_weekly_ids,
		       m.status, m.submitted_at, m.created_at, m.updated_at
		FROM monthly_reports m
		LEFT JOIN users u ON u.id = m.user_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE m.id = ?`, id,
	).Scan(&r.ID, &r.UserID, &r.UserName, &r.DepartmentName, &r.GroupName,
		&r.Year, &r.Month, &r.Content, &r.SourceWeeklyIDs,
		&r.Status, &submittedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if submittedAt.Valid {
		r.SubmittedAt = &submittedAt.String
	}
	return &r, nil
}

// canAccessMonthly: admin or owner.
func canAccessMonthly(c *gin.Context, r *monthlyReportRow) bool {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if role == "admin" || r.UserID == uid {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问该月汇报"})
	return false
}

// monthRange returns the inclusive date bounds (YYYY-MM-DD) for a
// (year, month) tuple. Used to query weekly_reports by week_start.
// We use week_start rather than week_end as the "month anchor" so a
// week that straddles months counts toward its starting month —
// consistent with how people talk about their week ("last week of May").
func monthRange(year, month int) (start, end string) {
	if month < 1 || month > 12 {
		return "", ""
	}
	s := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	e := s.AddDate(0, 1, -1)
	return s.Format("2006-01-02"), e.Format("2006-01-02")
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// ListMonthlyReports: caller's own, or admin with ?user_id=N.
func ListMonthlyReports(c *gin.Context) {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	target := uid
	if qUID := c.Query("user_id"); qUID != "" {
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员可查看他人汇报"})
			return
		}
		fmt.Sscanf(qUID, "%d", &target)
	}

	rows, err := db.DB.Query(`
		SELECT m.id, m.user_id, COALESCE(u.name, ''),
		       COALESCE(d.name, ''), COALESCE(g.name, ''),
		       m.year, m.month, m.content, m.source_weekly_ids,
		       m.status, m.submitted_at, m.created_at, m.updated_at
		FROM monthly_reports m
		LEFT JOIN users u ON u.id = m.user_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE m.user_id = ?
		ORDER BY m.year DESC, m.month DESC`, target)
	if err != nil {
		log.Printf("[monthly] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []monthlyReportRow{}
	for rows.Next() {
		var r monthlyReportRow
		var submittedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.UserID, &r.UserName, &r.DepartmentName, &r.GroupName,
			&r.Year, &r.Month, &r.Content, &r.SourceWeeklyIDs,
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

// AdminListAllMonthlyReports: cross-user browse for admins.
func AdminListAllMonthlyReports(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员"})
		return
	}

	where := []string{"1=1"}
	args := []interface{}{}
	if v := c.Query("department_id"); v != "" {
		where = append(where, "u.department_id = ?")
		args = append(args, v)
	}
	if v := c.Query("group_id"); v != "" {
		where = append(where, "u.group_id = ?")
		args = append(args, v)
	}
	if v := c.Query("year"); v != "" {
		where = append(where, "m.year = ?")
		args = append(args, v)
	}
	if v := c.Query("month"); v != "" {
		where = append(where, "m.month = ?")
		args = append(args, v)
	}

	q := `
		SELECT m.id, m.user_id, COALESCE(u.name, ''),
		       COALESCE(d.name, ''), COALESCE(g.name, ''),
		       m.year, m.month, m.content, m.source_weekly_ids,
		       m.status, m.submitted_at, m.created_at, m.updated_at
		FROM monthly_reports m
		LEFT JOIN users u ON u.id = m.user_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY m.year DESC, m.month DESC, u.department_id, u.group_id, m.user_id
		LIMIT 500`

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		log.Printf("[monthly] admin list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []monthlyReportRow{}
	for rows.Next() {
		var r monthlyReportRow
		var submittedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.UserID, &r.UserName, &r.DepartmentName, &r.GroupName,
			&r.Year, &r.Month, &r.Content, &r.SourceWeeklyIDs,
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

// GenerateMonthlyRequest is the payload for creating a new monthly
// draft from a (year, month) tuple.
type GenerateMonthlyRequest struct {
	Year  int `json:"year" binding:"required"`
	Month int `json:"month" binding:"required"`
}

// GenerateMonthlyReport:
//  1. Validate (year, month)
//  2. 409 if already exists
//  3. Pull user's submitted weeklies whose week_start falls in month
//  4. Concatenate + AI summarize
//  5. Insert draft row
func GenerateMonthlyReport(c *gin.Context) {
	var req GenerateMonthlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if req.Year < 2020 || req.Year > 2100 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "年份不合法"})
		return
	}
	if req.Month < 1 || req.Month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "月份应为 1-12"})
		return
	}

	uid := c.GetInt64("user_id")

	// Dedup: one monthly report per (user, year, month).
	var existingID int64
	db.DB.QueryRow(
		`SELECT id FROM monthly_reports WHERE user_id = ? AND year = ? AND month = ?`,
		uid, req.Year, req.Month,
	).Scan(&existingID)
	if existingID > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code":  -1,
			"error": "该月份已有月汇报（可直接进入编辑）",
			"data":  gin.H{"id": existingID},
		})
		return
	}

	start, end := monthRange(req.Year, req.Month)

	// Pull submitted weeklies whose week_start falls in this month. We
	// filter on week_start rather than a BETWEEN on both bounds to
	// avoid double-counting a week that spans into an adjacent month.
	rows, err := db.DB.Query(
		`SELECT id, week_start, week_end, content FROM weekly_reports
		 WHERE user_id = ? AND status = 'submitted'
		   AND week_start BETWEEN ? AND ?
		 ORDER BY week_start ASC`,
		uid, start, end,
	)
	if err != nil {
		log.Printf("[monthly] weekly fetch: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询周汇报失败"})
		return
	}
	defer rows.Close()

	type weeklyPiece struct {
		ID        int64
		WeekStart string
		WeekEnd   string
		Content   string
	}
	var pieces []weeklyPiece
	for rows.Next() {
		var p weeklyPiece
		if err := rows.Scan(&p.ID, &p.WeekStart, &p.WeekEnd, &p.Content); err == nil {
			pieces = append(pieces, p)
		}
	}
	if len(pieces) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":  -1,
			"error": "该月份下没有已提交的周汇报，无法生成月汇报（草稿周汇报不会计入）",
		})
		return
	}

	srcIDs := make([]int64, 0, len(pieces))
	for _, p := range pieces {
		srcIDs = append(srcIDs, p.ID)
	}
	srcJSON, _ := json.Marshal(srcIDs)

	// Concat the weekly summaries into one prompt. Each week gets a
	// header so the AI can see the chronology and naturally group by
	// weeks when synthesizing the month.
	var concat strings.Builder
	for _, p := range pieces {
		fmt.Fprintf(&concat, "## 第 %s ~ %s 周\n%s\n\n---\n\n", p.WeekStart, p.WeekEnd, p.Content)
	}

	summary, err := summarizeMonthWithAI(req.Year, req.Month, concat.String())
	if err != nil || strings.TrimSpace(summary) == "" {
		log.Printf("[monthly] AI summary failed (falling back): %v", err)
		summary = "> ⚠️ AI 润色未能运行，以下是本月周汇报的原始拼接，请手动整理后提交。\n\n" + concat.String()
	}

	res, err := db.DB.Exec(
		`INSERT INTO monthly_reports
		 (user_id, year, month, content, source_weekly_ids, status)
		 VALUES (?, ?, ?, ?, ?, 'draft')`,
		uid, req.Year, req.Month, summary, string(srcJSON),
	)
	if err != nil {
		log.Printf("[monthly] insert: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	id, _ := res.LastInsertId()
	WriteAuditLog(c, "monthly_generate", "monthly_report", fmt.Sprintf("%d", id),
		fmt.Sprintf("%d-%02d (%d 篇周汇报)", req.Year, req.Month, len(pieces)))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// summarizeMonthWithAI calls the configured AI endpoint with a
// month-oriented prompt. Same plumbing as weekly; different instruction.
func summarizeMonthWithAI(year, month int, concatenated string) (string, error) {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")
	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if apiKey == "" {
		return "", fmt.Errorf("AI API key 未配置")
	}

	prompt := fmt.Sprintf(`你是一名职场月报总结专家。下面是一名员工 %d 年 %d 月本月已提交的全部周汇报，按周拼接而成。

你的任务：
1. 提炼本月核心工作主线（按业务模块聚合，不按周列）
2. 汇总本月关键交付物、里程碑、数据结果
3. 汇总本月遇到的重大问题及解决情况
4. 汇总本月 Agent 自动化应用累计产出（节省工时、沉淀工作流）
5. 基于本月情况给出下月规划（3-5 条）

## 本月周汇报原文
%s

## 输出格式（严格 Markdown）

### 一、本月工作主线
（按业务模块 3-6 条，每条 1-3 行说明阶段性成果，不按周罗列）

### 二、关键交付与数据
（列表形式，量化数字 + 对应成果；本月无量化交付则写"本月无量化交付"）

### 三、问题与解决
- 已解决：...
- 未解决（需重点关注）：...（没有写"无"）

### 四、自动化应用累计
（合并本月所有 Agent 应用条目，去重汇总；最底部一行：本月累计节省工时：XX 小时 / 新增沉淀工作流：X 条）

### 五、下月规划
（3-5 条，按优先级排列，基于本月未完成事项 + 周汇报里的后续计划整合）

## 输出规则
- 语气专业克制，不要使用感叹号
- 不要加「本人」「我本月」「这个月我」等口语化字眼
- 每个一级标题后空一行再写内容
- 不要输出任何超出上述 5 个章节的多余内容
- 不要包裹在 markdown 代码块里`,
		year, month, concatenated,
	)

	body := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  5000,
		"temperature": 0.3,
	}
	bj, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(bj))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(180 * time.Second) // months need more time
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
	if strings.HasPrefix(out, "```") {
		out = strings.TrimPrefix(out, "```markdown")
		out = strings.TrimPrefix(out, "```")
		out = strings.TrimSuffix(out, "```")
		out = strings.TrimSpace(out)
	}
	return out, nil
}

func GetMonthlyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadMonthlyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "月汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	if !canAccessMonthly(c, r) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": r})
}

type UpdateMonthlyReportRequest struct {
	Content string `json:"content"`
}

func UpdateMonthlyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var req UpdateMonthlyReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	r, err := loadMonthlyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "月汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	if r.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能编辑自己的月汇报"})
		return
	}
	if r.Status != "draft" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "月汇报已提交，不可再编辑"})
		return
	}

	_, err = db.DB.Exec(
		`UPDATE monthly_reports SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		req.Content, id,
	)
	if err != nil {
		log.Printf("[monthly] update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存"})
}

func SubmitMonthlyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadMonthlyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "月汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	if r.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能提交自己的月汇报"})
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
		`UPDATE monthly_reports SET status = 'submitted', submitted_at = CURRENT_TIMESTAMP,
		 updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	if err != nil {
		log.Printf("[monthly] submit: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}
	WriteAuditLog(c, "monthly_submit", "monthly_report", fmt.Sprintf("%d", id),
		fmt.Sprintf("%d-%02d", r.Year, r.Month))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已提交"})
}

func PolishMonthlyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadMonthlyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "月汇报不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	uid := c.GetInt64("user_id")
	if r.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能润色自己的月汇报"})
		return
	}
	if r.Status != "draft" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "月汇报已提交，不可再润色"})
		return
	}
	if strings.TrimSpace(r.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "内容为空，无需润色"})
		return
	}

	polished, err := summarizeMonthWithAI(r.Year, r.Month, r.Content)
	if err != nil || strings.TrimSpace(polished) == "" {
		log.Printf("[monthly] polish failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "AI 润色失败：" + fmt.Sprint(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"content": polished}})
}

func DeleteMonthlyReport(c *gin.Context) {
	id := parseInt64Param(c, "id")
	r, err := loadMonthlyReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "月汇报不存在"})
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
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "已提交的月汇报不可删除"})
		return
	}
	if _, err := db.DB.Exec(`DELETE FROM monthly_reports WHERE id = ?`, id); err != nil {
		log.Printf("[monthly] delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	WriteAuditLog(c, "monthly_delete", "monthly_report", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}
