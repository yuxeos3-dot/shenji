package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

type ReportRequest struct {
	Content   string   `json:"content" binding:"required"`
	TasksDone []string `json:"tasks_done"`
	TasksTodo []string `json:"tasks_todo"`
	Issues    string   `json:"issues"`
}

func ListReports(c *gin.Context) {
	currentUserID := c.GetInt64("user_id")
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	date := c.Query("date")
	userIDFilter := c.Query("user_id") // admin can filter by specific user

	query := `SELECT r.id, r.user_id, u.name, r.report_date, r.content, r.tasks_done, r.tasks_todo, r.issues, r.created_at
		FROM daily_reports r JOIN users u ON r.user_id = u.id WHERE 1=1`
	args := []interface{}{}

	if !isAdmin {
		// Non-admin: force filter to own reports only
		query += fmt.Sprintf(" AND r.user_id = $%d", len(args)+1)
		args = append(args, currentUserID)
	} else if userIDFilter != "" {
		// Admin filtered by specific user
		query += fmt.Sprintf(" AND r.user_id = $%d", len(args)+1)
		args = append(args, userIDFilter)
	}

	if date != "" {
		normalized, err := parseISODate(date)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "日期格式需为 YYYY-MM-DD"})
			return
		}
		query += fmt.Sprintf(" AND r.report_date = $%d", len(args)+1)
		args = append(args, normalized)
	}

	// Optional date range filter — used by the weekly report preview
	// (NewWeeklyReport.vue) to show which dailies would be pulled in.
	// Accepts either bound independently; both together = closed range.
	if start := c.Query("start"); start != "" {
		if _, err := parseISODate(start); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "start 日期格式需为 YYYY-MM-DD"})
			return
		}
		query += fmt.Sprintf(" AND r.report_date >= $%d", len(args)+1)
		args = append(args, start)
	}
	if end := c.Query("end"); end != "" {
		if _, err := parseISODate(end); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "end 日期格式需为 YYYY-MM-DD"})
			return
		}
		query += fmt.Sprintf(" AND r.report_date <= $%d", len(args)+1)
		args = append(args, end)
	}

	query += " ORDER BY r.report_date DESC, r.user_id LIMIT 200"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var reports []gin.H
	for rows.Next() {
		var id, userID int64
		var userName, reportDate, content, tasksDone, tasksTodo, issues, createdAt string
		if err := rows.Scan(&id, &userID, &userName, &reportDate, &content, &tasksDone, &tasksTodo, &issues, &createdAt); err != nil {
			continue
		}
		reports = append(reports, gin.H{
			"id": id, "user_id": userID, "user_name": userName,
			"report_date": reportDate, "content": content,
			"tasks_done": json.RawMessage(tasksDone), "tasks_todo": json.RawMessage(tasksTodo),
			"issues": issues, "created_at": createdAt,
		})
	}
	if reports == nil {
		reports = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": reports})
}

func SubmitReport(c *gin.Context) {
	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	userID := c.GetInt64("user_id")
	reportDate := time.Now().Format("2006-01-02")

	tasksDoneJSON, _ := json.Marshal(req.TasksDone)
	tasksTodoJSON, _ := json.Marshal(req.TasksTodo)

	_, err := db.DB.Exec(
		`INSERT INTO daily_reports (user_id, report_date, content, tasks_done, tasks_todo, issues, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id, report_date) DO UPDATE SET
		 content=excluded.content, tasks_done=excluded.tasks_done, tasks_todo=excluded.tasks_todo,
		 issues=excluded.issues, updated_at=CURRENT_TIMESTAMP`,
		userID, reportDate, req.Content, string(tasksDoneJSON), string(tasksTodoJSON), req.Issues,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}

	// Per-user model: send the confirmation to the report writer's own
	// Telegram bot. (Previously this fired a system-wide notification
	// off the shared bot, which leaked every user's activity to whoever
	// happened to own the global bot.)
	var userName string
	db.DB.QueryRow("SELECT name FROM users WHERE id = $1", userID).Scan(&userName)
	NotifyUser(userID, "每日汇报已提交", userName+" 提交了 "+reportDate+" 的工作汇报")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "汇报提交成功"})
}

func GetReportByDate(c *gin.Context) {
	rawDate := c.Param("date")
	date, err := parseISODate(rawDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "日期格式需为 YYYY-MM-DD"})
		return
	}
	userID := c.GetInt64("user_id")

	var id int64
	var content, tasksDone, tasksTodo, issues, createdAt string
	err = db.DB.QueryRow(
		"SELECT id, content, tasks_done, tasks_todo, issues, created_at FROM daily_reports WHERE user_id = $1 AND report_date = $2",
		userID, date,
	).Scan(&id, &content, &tasksDone, &tasksTodo, &issues, &createdAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "汇报不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"id": id, "content": content,
		"tasks_done": json.RawMessage(tasksDone), "tasks_todo": json.RawMessage(tasksTodo),
		"issues": issues, "created_at": createdAt,
	}})
}

// ─── AI 日报润色 ──────────────────────────────────────────────────────────────

type PolishRequest struct {
	DoneRaw      string `json:"done_raw"`     // 今天做的事情，流水账
	CoreData     string `json:"core_data"`    // 核心数据（GSC / GA / 流量等）
	TomorrowRaw  string `json:"tomorrow_raw"` // 明天的计划
	Issues       string `json:"issues"`       // 遇到的问题
	AgentEntries []struct {
		Scenario   string `json:"scenario"`    // 跑通场景
		Flow       string `json:"flow"`        // 自动化流程
		Output     string `json:"output"`      // 自动化产出
		HoursSaved string `json:"hours_saved"` // 节省工时
		SkillName  string `json:"skill_name"`  // 封装 Skill / 工作流名称
		InLibrary  string `json:"in_library"`  // 是否入库：是 / 否
	} `json:"agent_entries"`
}

// 被禁止出现在 Agent 自动化应用里的词（违规直接拒绝）
var agentBannedWords = []string{
	"学习 AI", "学习AI", "学习人工智能",
	"调研工具", "工具调研",
	"研究自动化", "研究 AI",
	"部署环境", "环境部署", "环境搭建",
}

// PolishReport 调用 AI 把流水账润色成规范格式，并做 Agent 自动化应用合规校验。
func PolishReport(c *gin.Context) {
	var req PolishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	// ── 1. 硬校验 Agent 自动化应用 ────────────────────────────────────────
	if len(req.AgentEntries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1,
			"error": "Agent 自动化应用为必填项，至少需要 1 条实际跑通的场景"})
		return
	}
	for i, e := range req.AgentEntries {
		if strings.TrimSpace(e.Scenario) == "" || strings.TrimSpace(e.Flow) == "" ||
			strings.TrimSpace(e.Output) == "" || strings.TrimSpace(e.HoursSaved) == "" ||
			strings.TrimSpace(e.SkillName) == "" || strings.TrimSpace(e.InLibrary) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1,
				"error": fmt.Sprintf("Agent 自动化应用 #%d 存在空字段，所有 6 项必须填写", i+1)})
			return
		}
		blob := e.Scenario + " " + e.Flow + " " + e.Output + " " + e.SkillName
		for _, bad := range agentBannedWords {
			if strings.Contains(blob, bad) {
				c.JSON(http.StatusBadRequest, gin.H{"code": -1,
					"error": fmt.Sprintf("Agent 自动化应用 #%d 出现禁止内容「%s」。本项只接受实际业务场景 / 自动化产出 / 人效提升，日报已退回重写", i+1, bad)})
				return
			}
		}
		if e.InLibrary != "是" && e.InLibrary != "否" {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1,
				"error": fmt.Sprintf("Agent 自动化应用 #%d 的「是否入库」必须填「是」或「否」", i+1)})
			return
		}
	}

	// ── 2. 调用 AI 润色（允许失败兜底：AI 失败则用结构化模板拼装） ────────────
	userID := c.GetInt64("user_id")
	polished, err := polishWithAI(userID, req)
	if err != nil || strings.TrimSpace(polished) == "" {
		polished = assembleFallback(req)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"content":     polished,
		"format_used": "v1",
	}})
}

func polishWithAI(userID int64, req PolishRequest) (string, error) {
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

	agentsText := ""
	for i, e := range req.AgentEntries {
		agentsText += fmt.Sprintf(`
自动化应用 %d
- 跑通场景：%s
- 自动化流程：%s
- 自动化产出：%s
- 节省工时：%s
- 封装 Skill / 工作流名称：%s
- 是否入库（集团工作流库）：%s
`, i+1, e.Scenario, e.Flow, e.Output, e.HoursSaved, e.SkillName, e.InLibrary)
	}

	prompt := fmt.Sprintf(`你是一名职场日报润色专家。用户提供流水账，你整理成简洁、专业、可提交给 leader 的日报。

## 用户输入

【今日做的事情（流水账）】
%s

【核心数据】
%s

【明日计划】
%s

【遇到的问题】
%s

【Agent 自动化应用（用户已按规范填写，你只需要保持原格式、适度润色表述，不得增删条目、不得改变结构化字段的含义）】
%s

## 输出要求

严格按以下格式输出 Markdown，5 个一级标题必须齐全且顺序不变。不要加任何解释、不要加代码块包裹，直接输出日报全文：

今日工作完成：
（把用户流水账整理成 3-8 条项目符号，每条 ≤ 30 字，按重要性排序；口径要具体、可量化；避免空话）

核心数据：
（把核心数据整理成清单，每项给出数字和同比/环比变化；若用户未提供则写「本日无关键数据更新」）

明日工作规划：
（3-6 条项目符号，每条要有明确动作和交付物）

遇到问题：
（1-4 条；如果没有问题则写「无」）

Agent自动化应用（必须有）：
（保持用户输入的结构化字段不变，仅对措辞做轻度润色；不得合并条目、不得删减字段）

输出规则：
- 语气专业克制，不要使用感叹号
- 不要加「本人」「我今天」等口语化字眼
- 每个一级标题后空一行再写内容
- 不要输出任何超出上述 5 个章节的多余内容`,
		strings.TrimSpace(req.DoneRaw),
		strings.TrimSpace(req.CoreData),
		strings.TrimSpace(req.TomorrowRaw),
		strings.TrimSpace(req.Issues),
		agentsText,
	)

	body := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  4000,
		"temperature": 0.3,
	}
	bj, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(bj))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(90 * time.Second)
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
	// 剥 markdown 代码块（如果 AI 误包）
	if strings.HasPrefix(out, "```") {
		out = strings.TrimPrefix(out, "```markdown")
		out = strings.TrimPrefix(out, "```")
		out = strings.TrimSuffix(out, "```")
		out = strings.TrimSpace(out)
	}
	return out, nil
}

// assembleFallback 在 AI 失败时直接按模板拼装，保证不会因为 AI 挂掉就无法提交
func assembleFallback(req PolishRequest) string {
	var b strings.Builder
	b.WriteString("今日工作完成：\n")
	b.WriteString(strings.TrimSpace(req.DoneRaw))
	b.WriteString("\n\n核心数据：\n")
	if s := strings.TrimSpace(req.CoreData); s != "" {
		b.WriteString(s)
	} else {
		b.WriteString("本日无关键数据更新")
	}
	b.WriteString("\n\n明日工作规划：\n")
	b.WriteString(strings.TrimSpace(req.TomorrowRaw))
	b.WriteString("\n\n遇到问题：\n")
	if s := strings.TrimSpace(req.Issues); s != "" {
		b.WriteString(s)
	} else {
		b.WriteString("无")
	}
	b.WriteString("\n\nAgent自动化应用（必须有）：\n")
	for i, e := range req.AgentEntries {
		fmt.Fprintf(&b, "\n自动化应用 %d\n- 跑通场景：%s\n- 自动化流程：%s\n- 自动化产出：%s\n- 节省工时：%s\n- 封装 Skill / 工作流名称：%s\n- 是否入库（集团工作流库）：%s\n",
			i+1, e.Scenario, e.Flow, e.Output, e.HoursSaved, e.SkillName, e.InLibrary)
	}
	return b.String()
}

func ExportReports(c *gin.Context) {
	startDate := c.Query("start")
	endDate := c.Query("end")

	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请提供start和end日期参数"})
		return
	}

	rows, err := db.DB.Query(
		`SELECT r.report_date, u.name, r.content, r.tasks_done, r.tasks_todo, r.issues
		 FROM daily_reports r JOIN users u ON r.user_id = u.id
		 WHERE r.report_date BETWEEN $1 AND $2 ORDER BY r.report_date, u.name`,
		startDate, endDate,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var data []gin.H
	for rows.Next() {
		var date, name, content, tasksDone, tasksTodo, issues string
		if err := rows.Scan(&date, &name, &content, &tasksDone, &tasksTodo, &issues); err != nil {
			continue
		}
		data = append(data, gin.H{
			"date": date, "name": name, "content": content,
			"tasks_done": json.RawMessage(tasksDone), "tasks_todo": json.RawMessage(tasksTodo),
			"issues": issues,
		})
	}
	if data == nil {
		data = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
