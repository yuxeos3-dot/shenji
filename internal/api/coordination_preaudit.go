package api

// Coord pre-audit: test-server pages + AI review + reject flow.
//
// Purpose: close the verification loop. When a developer marks a
// fix as 待验收 (under_review), the admin attaches one or more
// test-server URLs that were modified, we crawl them, then ask AI
// to compare the captured HTML against the original issue_snapshot.
// AI returns a verdict (fixed / not_fixed / unclear) + reasoning.
// The admin still has final say — they can click 审核通过 (→ done)
// or 打回 (→ in_progress, required reject note goes back to dev).
//
// Design choices:
//   - Pages live in a separate table coord_test_pages, keyed by
//     task_id. Same shape as site_pages so the frontend pattern
//     is familiar to the reviewer.
//   - AI verdict persists on the task row (pre_audit_verdict,
//     pre_audit_result). One task has at most one "latest" verdict;
//     re-running pre-audit overwrites it. Old verdicts are not kept
//     because they'd confuse the review panel — only the most
//     recent snapshot matters.
//   - Reject moves under_review → in_progress and appends the note
//     as a coordination_events row (type 'reject'), visible in the
//     timeline on the detail page.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

// loadCoordTaskForReview fetches the task for any endpoint that only
// works while under_review. Returns the task or nil; on nil the
// response has already been written.
func loadCoordTaskForReview(c *gin.Context, id int64) *coordTask {
	t, err := loadCoordTask(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return nil
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return nil
	}
	// Access gate: creator, assignee, or admin.
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if t.CreatedBy != uid && t.AssigneeID != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问该任务"})
		return nil
	}
	return t
}

// ─── Pages CRUD ─────────────────────────────────────────────────────────────

// ListCoordTestPages returns all test-server pages for a task.
// Accessible to creator/assignee/admin. Read-only — the HTML might
// be large so we return it only when the frontend explicitly asks
// for source via GetCoordTestPageSource.
func ListCoordTestPages(c *gin.Context) {
	taskID := parseInt64Param(c, "id")
	if t := loadCoordTaskForReview(c, taskID); t == nil {
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, task_id, name, url, last_fetched_at, status_code, ttfb_ms, created_at
		FROM coord_test_pages WHERE task_id = $1 ORDER BY created_at ASC`, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	pages := []gin.H{}
	for rows.Next() {
		var id, tid, statusCode, ttfb int64
		var name, u, createdAt string
		var lastFetchedAt *string
		if err := rows.Scan(&id, &tid, &name, &u, &lastFetchedAt, &statusCode, &ttfb, &createdAt); err != nil {
			continue
		}
		pages = append(pages, gin.H{
			"id": id, "task_id": tid, "name": name, "url": u,
			"last_fetched_at": lastFetchedAt, "status_code": statusCode,
			"ttfb_ms": ttfb, "created_at": createdAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": pages})
}

// CreateCoordTestPage adds a test-server URL to a task. Only makes
// sense while the task is under_review — adding a test page to a
// done task would be misleading (the record is frozen).
func CreateCoordTestPage(c *gin.Context) {
	taskID := parseInt64Param(c, "id")
	t := loadCoordTaskForReview(c, taskID)
	if t == nil {
		return
	}
	if t.Status == coordStatusDone {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "已完成的任务不可再添加测试页"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误：name 和 url 必填"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if err := crawler.ValidateURL(req.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "URL 无效: " + err.Error()})
		return
	}

	uid := c.GetInt64("user_id")
	var id int64
	err := db.DB.QueryRow(
		`INSERT INTO coord_test_pages (task_id, name, url, created_by) VALUES ($1, $2, $3, $4) RETURNING id`,
		taskID, req.Name, req.URL, uid).Scan(&id)
	if err != nil {
		log.Printf("[coord-preaudit] create page: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	WriteAuditLog(c, "coord_test_page_add", "coord_test_page",
		fmt.Sprintf("%d", id), req.URL)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// DeleteCoordTestPage removes a test page. Same access gate as list.
func DeleteCoordTestPage(c *gin.Context) {
	taskID := parseInt64Param(c, "id")
	pageID := parseInt64Param(c, "page_id")
	if t := loadCoordTaskForReview(c, taskID); t == nil {
		return
	}
	res, err := db.DB.Exec(
		`DELETE FROM coord_test_pages WHERE id = $1 AND task_id = $2`, pageID, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "页面不存在"})
		return
	}
	WriteAuditLog(c, "coord_test_page_delete", "coord_test_page",
		fmt.Sprintf("%d", pageID), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}

// FetchCoordTestPage crawls a test page and stores its HTML. The same
// crawler the site_pages flow uses, so we get identical behavior
// (content-type guards, TTFB measurement, size limits).
func FetchCoordTestPage(c *gin.Context) {
	taskID := parseInt64Param(c, "id")
	pageID := parseInt64Param(c, "page_id")
	if t := loadCoordTaskForReview(c, taskID); t == nil {
		return
	}
	var name, u string
	err := db.DB.QueryRow(
		`SELECT name, url FROM coord_test_pages WHERE id = $1 AND task_id = $2`,
		pageID, taskID).Scan(&name, &u)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "页面不存在"})
		return
	}

	result := crawler.FetchPage(crawler.URLSpec{PageType: name, URL: u}, 30*time.Second)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.DB.Exec(
		`UPDATE coord_test_pages
		 SET last_html = $1, last_fetched_at = $2, status_code = $3, ttfb_ms = $4
		 WHERE id = $5`,
		result.HTML, now, result.StatusCode, result.TTFB, pageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存抓取结果失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"status_code":     result.StatusCode,
			"ttfb_ms":         result.TTFB,
			"html_size":       result.HTMLSize,
			"last_fetched_at": now,
			"error":           result.Error,
		},
	})
}

// GetCoordTestPageSource returns the stored HTML snippet. The admin
// review UI uses this to display the captured source inside a
// collapsible panel without re-crawling.
func GetCoordTestPageSource(c *gin.Context) {
	taskID := parseInt64Param(c, "id")
	pageID := parseInt64Param(c, "page_id")
	if t := loadCoordTaskForReview(c, taskID); t == nil {
		return
	}
	var name, u, html string
	var lastFetchedAt sql.NullString
	err := db.DB.QueryRow(
		`SELECT name, url, COALESCE(last_html, ''), last_fetched_at
		 FROM coord_test_pages WHERE id = $1 AND task_id = $2`,
		pageID, taskID).Scan(&name, &u, &html, &lastFetchedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "页面不存在"})
		return
	}
	var fetched string
	if lastFetchedAt.Valid {
		fetched = lastFetchedAt.String
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"name": name, "url": u, "html": html, "last_fetched_at": fetched,
	}})
}

// ─── Pre-audit (AI) ────────────────────────────────────────────────────────

// PreAuditCoordTask runs AI over the test pages' HTML + the original
// issue snapshot and stores a verdict on the task row. Only works
// while under_review (running it in other states is meaningless).
//
// Cost-wise: we cap each page's HTML fed to AI at 15 KB and the total
// at 60 KB. For larger pages the admin probably needs to look
// manually — AI snippets past 60 KB start drifting.
func PreAuditCoordTask(c *gin.Context) {
	taskID := parseInt64Param(c, "id")
	t := loadCoordTaskForReview(c, taskID)
	if t == nil {
		return
	}
	if t.Status != coordStatusUnderReview {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "仅「待验收」状态可以运行 AI 预审核"})
		return
	}
	// Pre-audit is a review aid — the developer running it on their own
	// submission would bias the verdict. Restrict to creator/admin, same
	// as the final verify / reject actions.
	if !requireCreatorOrAdmin(c, t, "运行 AI 预审核") {
		return
	}

	// Pull all attached test pages that have been fetched at least
	// once. Skip unfetched pages with a warning rather than hard-fail
	// — admin can fetch them and rerun.
	rows, err := db.DB.Query(
		`SELECT id, name, url, COALESCE(last_html,''), COALESCE(last_fetched_at,''),
		        COALESCE(status_code,0)
		 FROM coord_test_pages WHERE task_id = $1 ORDER BY created_at ASC`, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询测试页失败"})
		return
	}
	defer rows.Close()

	type pg struct {
		ID         int64
		Name, URL  string
		HTML       string
		Fetched    string
		StatusCode int64
	}
	var pages []pg
	var missingFetch []string
	for rows.Next() {
		var p pg
		if err := rows.Scan(&p.ID, &p.Name, &p.URL, &p.HTML, &p.Fetched, &p.StatusCode); err != nil {
			continue
		}
		if p.HTML == "" || p.Fetched == "" {
			missingFetch = append(missingFetch, p.Name)
			continue
		}
		pages = append(pages, p)
	}
	if len(pages) == 0 {
		msg := "请先添加测试页并抓取 HTML 后再运行预审核"
		if len(missingFetch) > 0 {
			msg = fmt.Sprintf("以下测试页尚未抓取 HTML: %s", strings.Join(missingFetch, "、"))
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": msg})
		return
	}

	// Build the prompt: feed the original issue snapshot + each test
	// page's trimmed HTML. Strip scripts/styles to focus the model on
	// actual DOM content.
	const perPageCap = 15 * 1024
	const totalCap = 60 * 1024
	var htmlBlock strings.Builder
	usedBytes := 0
	for _, p := range pages {
		snippet := stripHTMLNoise(p.HTML)
		if len(snippet) > perPageCap {
			snippet = snippet[:perPageCap] + "\n...[已截断]"
		}
		if usedBytes+len(snippet) > totalCap {
			break
		}
		fmt.Fprintf(&htmlBlock,
			"\n========== 测试页：%s ==========\nURL: %s\n抓取时间: %s\nHTTP 状态: %d\n\n%s\n\n",
			p.Name, p.URL, p.Fetched, p.StatusCode, snippet)
		usedBytes += len(snippet)
	}

	prompt := buildPreAuditPrompt(t.IssueTitle, t.IssueSnapshot, t.Note, htmlBlock.String())

	uid, uname := currentUser(c)
	raw, _, err := CallAIWithUserConfig(uid, prompt, 1500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "AI 调用失败: " + err.Error()})
		return
	}

	verdict, reasoning := parsePreAuditVerdict(raw)

	now := time.Now().UTC()
	_, err = db.DB.Exec(
		`UPDATE coordination_tasks
		 SET pre_audit_verdict = $1, pre_audit_result = $2, pre_audit_at = $3
		 WHERE id = $4`,
		verdict, reasoning, now, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存预审核结果失败"})
		return
	}
	recordEvent(taskID, "pre_audit", t.Status, t.Status, uid, uname,
		fmt.Sprintf("AI 预审核结果：%s", verdict))
	WriteAuditLog(c, "coord_pre_audit", "coordination_task",
		fmt.Sprintf("%d", taskID), verdict)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"verdict":    verdict,
		"reasoning":  reasoning,
		"page_count": len(pages),
		"skipped":    missingFetch,
		"audited_at": now.Format(time.RFC3339),
	}})
}

// stripHTMLNoise yanks <script> / <style> / <noscript> blocks so the
// AI reads only the visible content. Single-pass approach: lowercase
// once, find all removable regions, then build the result in one
// allocation instead of the previous O(n^2) repeated string splicing.
func stripHTMLNoise(html string) string {
	lowered := strings.ToLower(html)
	tags := []string{"script", "style", "noscript"}

	// Collect all [start, end) byte ranges to remove.
	type span struct{ start, end int }
	var removes []span

	for _, tag := range tags {
		openT := "<" + tag
		closeT := "</" + tag + ">"
		searchFrom := 0
		for searchFrom < len(lowered) {
			i := strings.Index(lowered[searchFrom:], openT)
			if i < 0 {
				break
			}
			i += searchFrom
			// Find the closing '>' of the open tag (accounts for attributes).
			gt := strings.Index(lowered[i:], ">")
			if gt < 0 {
				// Unclosed tag — remove from opener to EOF.
				removes = append(removes, span{i, len(html)})
				break
			}
			openEnd := i + gt + 1
			j := strings.Index(lowered[openEnd:], closeT)
			if j < 0 {
				// No closing tag — remove from opener to EOF.
				removes = append(removes, span{i, len(html)})
				break
			}
			end := openEnd + j + len(closeT)
			removes = append(removes, span{i, end})
			searchFrom = end
		}
	}

	if len(removes) == 0 {
		return strings.TrimSpace(html)
	}

	// Sort spans by start position so we can merge overlaps and build
	// the output in a single forward pass.
	for i := 1; i < len(removes); i++ {
		for j := i; j > 0 && removes[j].start < removes[j-1].start; j-- {
			removes[j], removes[j-1] = removes[j-1], removes[j]
		}
	}

	// Merge overlapping spans.
	merged := removes[:1]
	for _, s := range removes[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end {
			if s.end > last.end {
				last.end = s.end
			}
		} else {
			merged = append(merged, s)
		}
	}

	// Build result keeping only the non-removed regions.
	var buf strings.Builder
	buf.Grow(len(html))
	pos := 0
	for _, s := range merged {
		if s.start > pos {
			buf.WriteString(html[pos:s.start])
		}
		pos = s.end
	}
	if pos < len(html) {
		buf.WriteString(html[pos:])
	}
	return strings.TrimSpace(buf.String())
}

// buildPreAuditPrompt constructs the verdict-oriented AI prompt.
// We ask for strict JSON output so parsing is trivial.
func buildPreAuditPrompt(issueTitle, issueSnapshot, taskNote, htmlBlock string) string {
	return `你是一名 SEO 质量审核员。你的任务是对比"原始问题"与"开发修复后的测试服页面 HTML"，判断问题是否已修复。

━━━━━ 原始问题 ━━━━━
标题：` + issueTitle + `

问题详情（JSON）：
` + issueSnapshot + `

任务备注：
` + taskNote + `

━━━━━ 测试服页面 HTML ━━━━━
` + htmlBlock + `

━━━━━ 审核规则 ━━━━━
1. 只关心"原始问题"里写明的那一点是否修复；不要评估页面的其他问题
2. 如果 HTML 里已不再出现问题描述的错误特征 → verdict = "fixed"
3. 如果 HTML 里明显还有问题描述的错误特征 → verdict = "not_fixed"
4. 如果 HTML 里看不到相关元素（可能抓取不完整、或需要登录才能看到）→ verdict = "unclear"
5. 不要被细枝末节干扰：开发者只需要修好"原始问题"指出的具体缺陷

━━━━━ 输出格式 ━━━━━
严格输出一个 JSON 对象，不要任何其他文字，不要 markdown 代码块。字段：
{
  "verdict": "fixed" | "not_fixed" | "unclear",
  "reasoning": "200 字内中文说明，描述你在测试页 HTML 里看到了什么、是否符合修复预期、具体依据"
}`
}

// parsePreAuditVerdict extracts verdict + reasoning from AI response.
// Forgiving — strips code fences, finds JSON by braces, falls back to
// 'unclear' on parse failure so the admin still sees the raw response.
func parsePreAuditVerdict(raw string) (verdict, reasoning string) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	// Small inline struct to avoid import of encoding/json at top;
	// the module already imports it, so this is fine.
	var p struct {
		Verdict   string `json:"verdict"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return "unclear", "⚠️ AI 响应未能解析为 JSON。原始输出：\n" + raw
	}
	v := strings.ToLower(strings.TrimSpace(p.Verdict))
	switch v {
	case "fixed", "not_fixed", "unclear":
		verdict = v
	default:
		verdict = "unclear"
	}
	reasoning = strings.TrimSpace(p.Reasoning)
	if reasoning == "" {
		reasoning = "AI 未返回理由说明"
	}
	return
}

// ─── Reject (打回开发) ─────────────────────────────────────────────────────

// RejectCoordTask is the "审核未通过、打回" button. Moves under_review
// → in_progress and forces a required note that's appended to the
// timeline so the developer sees exactly what still needs fixing.
//
// Why in_progress not awaiting_answer: the dev isn't waiting for a
// question — they're continuing work. The state name in_progress
// matches that intent correctly.
func RejectCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body struct {
		Note string `json:"note" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写打回原因（必填）"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if t.Status != coordStatusUnderReview {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「待验收」的任务才能打回"})
		return
	}
	// Only admin or the original creator can reject — the assignee
	// rejecting their own submission would be confusing.
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if t.CreatedBy != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅创建人或管理员可打回"})
		return
	}

	res, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET status = $1, submitted_at = NULL, pre_audit_verdict = '', pre_audit_result = ''
		 WHERE id = $2 AND status = $3`,
		coordStatusInProgress, id, coordStatusUnderReview)
	if err != nil {
		log.Printf("[coord] RejectCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "打回失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
		return
	}
	_, uname := currentUser(c)
	recordEvent(id, "reject", coordStatusUnderReview, coordStatusInProgress,
		uid, uname, body.Note)
	WriteAuditLog(c, "coord_reject", "coordination_task",
		fmt.Sprintf("%d", id), body.Note)
	// Ping the developer with the rejection reason so they know what
	// still needs fixing — status flips back to in_progress on their side.
	t.Status = coordStatusInProgress
	notifyCoord(t.AssigneeID, "🔁 任务被打回", t, fmt.Sprintf("%s 打回原因：%s", uname, body.Note))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"status": coordStatusInProgress,
	}})
}
