package api

// Coordination (协同管理) turns an individual audit issue into a tracked
// work item that flows through a fixed state machine:
//
//   pending       ← task created from an audit issue
//   in_progress   ← assigned to / picked up by a developer
//   under_review  ← developer marks the fix complete
//   done          ← admin verifies the fix after the next audit
//
// The state machine is strictly forward. Once a task hits `done`, every
// handler in this file refuses to mutate it — no edits, no deletes, no
// new events. That "工作留痕" requirement is enforced at the write layer
// (see `assertEditable`).
//
// The history of state transitions + comments is stored in a sibling
// table `coordination_events`, so the detail page can render a full
// timeline with actor + timestamp for every action.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Domain types ───────────────────────────────────────────────────────────

const (
	coordStatusPending        = "pending"
	coordStatusInProgress     = "in_progress"
	coordStatusAwaitingAnswer = "awaiting_answer" // developer raised a question, waiting on creator
	coordStatusUnderReview    = "under_review"
	coordStatusDone           = "done"
)

// validCoordStatus reports whether s is one of the legal states.
func validCoordStatus(s string) bool {
	switch s {
	case coordStatusPending, coordStatusInProgress, coordStatusAwaitingAnswer,
		coordStatusUnderReview, coordStatusDone:
		return true
	}
	return false
}

type coordTask struct {
	ID              int64   `json:"id"`
	SiteID          int64   `json:"site_id"`
	SiteName        string  `json:"site_name,omitempty"`
	SiteDomain      string  `json:"site_domain,omitempty"`
	AuditTaskID     int64   `json:"audit_task_id"`
	IssueID         string  `json:"issue_id"`
	IssueTitle      string  `json:"issue_title"`
	IssueSeverity   string  `json:"issue_severity"`
	IssueModule     string  `json:"issue_module"`
	IssueChapter    string  `json:"issue_chapter"`
	IssueSnapshot   string  `json:"issue_snapshot"`
	Note            string  `json:"note"`
	Status          string  `json:"status"`
	CreatedBy       int64   `json:"created_by"`
	CreatorName     string  `json:"creator_name,omitempty"`
	AssigneeID      int64   `json:"assignee_id"`
	AssigneeName    string  `json:"assignee_name,omitempty"`
	CreatedAt       string  `json:"created_at"`
	StartedAt       *string `json:"started_at"`
	SubmittedAt     *string `json:"submitted_at"`
	VerifiedAt      *string `json:"verified_at"`
	PreAuditVerdict string  `json:"pre_audit_verdict,omitempty"`
	PreAuditResult  string  `json:"pre_audit_result,omitempty"`
	PreAuditAt      *string `json:"pre_audit_at,omitempty"`
}

type coordEvent struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	EventType  string `json:"event_type"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	ActorID    int64  `json:"actor_id"`
	ActorName  string `json:"actor_name"`
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// assertEditable aborts the request with 409 Conflict when the task is
// already in the `done` terminal state. This is the single choke point
// that enforces "流程走完了不可以编辑而且不可以删除".
func assertEditable(c *gin.Context, taskID int64) (*coordTask, bool) {
	t, err := loadCoordTask(taskID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return nil, false
	}
	if err != nil {
		log.Printf("[coord] loadCoordTask(%d) failed: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "加载任务失败"})
		return nil, false
	}
	// Enforce site-level access so users can't mutate coordination tasks
	// belonging to sites they don't own (IDOR).
	if !canAccessSite(c, fmt.Sprintf("%d", t.SiteID)) {
		return nil, false
	}
	if t.Status == coordStatusDone {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务已完成，不可修改"})
		return nil, false
	}
	return t, true
}

// loadCoordTask reads a single task by id and joins the site + users for
// display. Returns sql.ErrNoRows if not found (kept separate from other
// errors so callers can special-case 404 vs 500).
func loadCoordTask(id int64) (*coordTask, error) {
	const q = `
		SELECT t.id, t.site_id, COALESCE(s.name,''), COALESCE(s.domain,''),
		       t.audit_task_id, t.issue_id, t.issue_title, t.issue_severity,
		       t.issue_module, t.issue_chapter, t.issue_snapshot, t.note,
		       t.status, t.created_by, COALESCE(u1.name,''),
		       t.assignee_id, COALESCE(u2.name,''),
		       t.created_at, t.started_at, t.submitted_at, t.verified_at,
		       COALESCE(t.pre_audit_verdict,''), COALESCE(t.pre_audit_result,''),
		       t.pre_audit_at
		FROM coordination_tasks t
		LEFT JOIN sites s ON s.id = t.site_id
		LEFT JOIN users u1 ON u1.id = t.created_by
		LEFT JOIN users u2 ON u2.id = t.assignee_id
		WHERE t.id = ?`
	var t coordTask
	var started, submitted, verified, preAuditAt sql.NullString
	err := db.DB.QueryRow(q, id).Scan(
		&t.ID, &t.SiteID, &t.SiteName, &t.SiteDomain,
		&t.AuditTaskID, &t.IssueID, &t.IssueTitle, &t.IssueSeverity,
		&t.IssueModule, &t.IssueChapter, &t.IssueSnapshot, &t.Note,
		&t.Status, &t.CreatedBy, &t.CreatorName,
		&t.AssigneeID, &t.AssigneeName,
		&t.CreatedAt, &started, &submitted, &verified,
		&t.PreAuditVerdict, &t.PreAuditResult, &preAuditAt,
	)
	if err != nil {
		return nil, err
	}
	if started.Valid {
		t.StartedAt = &started.String
	}
	if submitted.Valid {
		t.SubmittedAt = &submitted.String
	}
	if verified.Valid {
		t.VerifiedAt = &verified.String
	}
	if preAuditAt.Valid {
		t.PreAuditAt = &preAuditAt.String
	}
	return &t, nil
}

// recordEvent inserts a row into the timeline. Never fails the outer
// transaction — loses the event but keeps the state transition if the
// insert somehow errors (shouldn't happen, but event history is
// advisory, while the task state is authoritative).
func recordEvent(taskID int64, eventType, fromStatus, toStatus string, actorID int64, actorName, note string) {
	_, err := db.DB.Exec(
		`INSERT INTO coordination_events
		 (task_id, event_type, from_status, to_status, actor_id, actor_name, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, eventType, fromStatus, toStatus, actorID, actorName, note,
	)
	if err != nil {
		log.Printf("[coord] recordEvent(task=%d,%s) failed: %v", taskID, eventType, err)
	}
}

// currentUser returns (id, name) for the caller. `user_name` is NOT in
// JWT claims, so we fall back to a users table lookup. Caches nothing —
// this runs once per request and the users table stays hot.
func currentUser(c *gin.Context) (int64, string) {
	uid := c.GetInt64("user_id")
	if uid <= 0 {
		return 0, ""
	}
	var name string
	if err := db.DB.QueryRow("SELECT name FROM users WHERE id = ?", uid).Scan(&name); err != nil && err != sql.ErrNoRows {
		log.Printf("[coord] currentUser(%d) lookup failed: %v", uid, err)
	}
	return uid, name
}

// parseInt64Param extracts a positive int64 route parameter, returning
// 0 for malformed input so handlers can early-return with a 400.
func parseInt64Param(c *gin.Context, name string) int64 {
	var v int64
	fmt.Sscanf(c.Param(name), "%d", &v)
	if v < 0 {
		return 0
	}
	return v
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// CreateCoordTaskRequest is accepted from the audit detail page's "加入协同"
// button. The frontend snapshots the issue's display fields so the
// coordination task remains readable even after the source audit is
// re-run (issue ids can repeat across audits).
type CreateCoordTaskRequest struct {
	SiteID        int64  `json:"site_id" binding:"required"`
	AuditTaskID   int64  `json:"audit_task_id" binding:"required"`
	IssueID       string `json:"issue_id"`
	IssueTitle    string `json:"issue_title" binding:"required"`
	IssueSeverity string `json:"issue_severity"`
	IssueModule   string `json:"issue_module"`
	IssueChapter  string `json:"issue_chapter"`
	IssueSnapshot string `json:"issue_snapshot"` // raw JSON of the issue card
	Note          string `json:"note"`
	AssigneeID    int64  `json:"assignee_id"` // optional, can be assigned later
}

// CreateCoordTask adds an issue to the coordination queue. Duplicates
// are explicitly allowed — the user chose this workflow, and the audit
// snapshot captures the state at creation time so each task can be
// tracked independently.
func CreateCoordTask(c *gin.Context) {
	var req CreateCoordTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	// Validate the snapshot is either empty or valid JSON — we store it
	// verbatim and the frontend re-parses on display, so broken JSON
	// here would break the task detail page forever.
	if req.IssueSnapshot != "" {
		if !json.Valid([]byte(req.IssueSnapshot)) {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "issue_snapshot 不是合法 JSON"})
			return
		}
	}

	uid, uname := currentUser(c)
	res, err := db.DB.Exec(
		`INSERT INTO coordination_tasks
		 (site_id, audit_task_id, issue_id, issue_title, issue_severity,
		  issue_module, issue_chapter, issue_snapshot, note, status,
		  created_by, assignee_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		req.SiteID, req.AuditTaskID, req.IssueID, req.IssueTitle, req.IssueSeverity,
		req.IssueModule, req.IssueChapter, req.IssueSnapshot, req.Note,
		uid, req.AssigneeID,
	)
	if err != nil {
		log.Printf("[coord] CreateCoordTask insert failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	taskID, _ := res.LastInsertId()

	// Timeline: task was created (+ optionally assigned in the same call).
	recordEvent(taskID, "create", "", coordStatusPending, uid, uname, req.Note)
	if req.AssigneeID > 0 {
		var asignName string
		db.DB.QueryRow("SELECT name FROM users WHERE id = ?", req.AssigneeID).Scan(&asignName)
		recordEvent(taskID, "assign", coordStatusPending, coordStatusPending, uid, uname,
			fmt.Sprintf("分配给 %s", asignName))
	}

	WriteAuditLog(c, "coord_create", "coordination_task", fmt.Sprintf("%d", taskID), req.IssueTitle)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": taskID}})
}

// ListCoordTasks returns tasks with optional filters by site_id, status,
// and assignee_id. Paginated because a long-running system can accumulate
// hundreds of tasks.
func ListCoordTasks(c *gin.Context) {
	page, perPage := parsePageQuery(c, 50, 200)
	offset := (page - 1) * perPage

	where := []string{"1=1"}
	args := []interface{}{}
	if sid := c.Query("site_id"); sid != "" {
		where = append(where, "t.site_id = ?")
		args = append(args, sid)
	}
	if s := c.Query("status"); s != "" {
		if !validCoordStatus(s) {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "status 取值非法"})
			return
		}
		where = append(where, "t.status = ?")
		args = append(args, s)
	}
	if a := c.Query("assignee_id"); a != "" {
		where = append(where, "t.assignee_id = ?")
		args = append(args, a)
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM coordination_tasks t WHERE "+strings.Join(where, " AND "), args...).Scan(&total)

	q := `
		SELECT t.id, t.site_id, COALESCE(s.name,''), COALESCE(s.domain,''),
		       t.audit_task_id, t.issue_id, t.issue_title, t.issue_severity,
		       t.issue_module, t.issue_chapter, t.issue_snapshot, t.note,
		       t.status, t.created_by, COALESCE(u1.name,''),
		       t.assignee_id, COALESCE(u2.name,''),
		       t.created_at, t.started_at, t.submitted_at, t.verified_at,
		       COALESCE(t.pre_audit_verdict,'')
		FROM coordination_tasks t
		LEFT JOIN sites s ON s.id = t.site_id
		LEFT JOIN users u1 ON u1.id = t.created_by
		LEFT JOIN users u2 ON u2.id = t.assignee_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY
			CASE t.status
				WHEN 'awaiting_answer' THEN 0
				WHEN 'under_review'    THEN 1
				WHEN 'in_progress'     THEN 2
				WHEN 'pending'         THEN 3
				WHEN 'done'            THEN 4
				ELSE 5
			END,
			t.created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := db.DB.Query(q, append(args, perPage, offset)...)
	if err != nil {
		log.Printf("[coord] ListCoordTasks query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []coordTask{}
	for rows.Next() {
		var t coordTask
		var started, submitted, verified sql.NullString
		if err := rows.Scan(
			&t.ID, &t.SiteID, &t.SiteName, &t.SiteDomain,
			&t.AuditTaskID, &t.IssueID, &t.IssueTitle, &t.IssueSeverity,
			&t.IssueModule, &t.IssueChapter, &t.IssueSnapshot, &t.Note,
			&t.Status, &t.CreatedBy, &t.CreatorName,
			&t.AssigneeID, &t.AssigneeName,
			&t.CreatedAt, &started, &submitted, &verified,
			&t.PreAuditVerdict,
		); err != nil {
			continue
		}
		if started.Valid {
			t.StartedAt = &started.String
		}
		if submitted.Valid {
			t.SubmittedAt = &submitted.String
		}
		if verified.Valid {
			t.VerifiedAt = &verified.String
		}
		items = append(items, t)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"items": items, "total": total, "page": page, "per_page": perPage},
	})
}

// GetCoordTask returns a single task + its full event timeline +
// attachment metadata. Used by the detail page.
func GetCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "id 无效"})
		return
	}

	t, err := loadCoordTask(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if err != nil {
		log.Printf("[coord] GetCoordTask load failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}

	// Timeline
	events := []coordEvent{}
	rows, err := db.DB.Query(
		`SELECT id, task_id, event_type, from_status, to_status,
		        actor_id, actor_name, note, created_at
		 FROM coordination_events
		 WHERE task_id = ?
		 ORDER BY created_at ASC, id ASC`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var e coordEvent
			if err := rows.Scan(&e.ID, &e.TaskID, &e.EventType, &e.FromStatus, &e.ToStatus,
				&e.ActorID, &e.ActorName, &e.Note, &e.CreatedAt); err == nil {
				events = append(events, e)
			}
		}
	}

	// Attachments (metadata only — file contents are served separately)
	type attach struct {
		ID         int64  `json:"id"`
		EventID    int64  `json:"event_id"`
		Filename   string `json:"filename"`
		Size       int64  `json:"size"`
		Mime       string `json:"mime"`
		UploadedBy int64  `json:"uploaded_by"`
		UploadedAt string `json:"uploaded_at"`
	}
	attachments := []attach{}
	arows, err := db.DB.Query(
		`SELECT id, event_id, filename, size, mime, uploaded_by, uploaded_at
		 FROM coordination_attachments
		 WHERE task_id = ? ORDER BY uploaded_at ASC`, id)
	if err == nil {
		defer arows.Close()
		for arows.Next() {
			var a attach
			if err := arows.Scan(&a.ID, &a.EventID, &a.Filename, &a.Size, &a.Mime,
				&a.UploadedBy, &a.UploadedAt); err == nil {
				attachments = append(attachments, a)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"task":        t,
			"events":      events,
			"attachments": attachments,
		},
	})
}

// ─── State transitions ──────────────────────────────────────────────────────
//
// Each transition is a separate handler so the API surface matches the
// UI buttons 1:1. This is easier to reason about than a single generic
// "update status" endpoint where the client decides the target.

type transitionBody struct {
	Note       string `json:"note"`
	AssigneeID int64  `json:"assignee_id"` // only used by /assign
}

// AssignCoordTask moves a `pending` task to `in_progress` and records who
// the developer is. If the task was already in_progress we still allow
// reassigning (without a state change) — projects get juggled.
func AssignCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	_ = c.ShouldBindJSON(&body)
	if body.AssigneeID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "assignee_id 必填"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}

	// Look up the name for the timeline
	var assignName string
	if err := db.DB.QueryRow("SELECT name FROM users WHERE id = ?", body.AssigneeID).Scan(&assignName); err != nil && err != sql.ErrNoRows {
		log.Printf("[coord] AssignCoordTask(%d) assignee lookup failed: %v", id, err)
	}
	if assignName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "指定的开发账号不存在"})
		return
	}

	uid, uname := currentUser(c)
	now := time.Now().UTC()

	// First-time assignment moves from pending → in_progress and stamps
	// started_at. A subsequent reassignment stays in_progress.
	var newStatus string
	if t.Status == coordStatusPending {
		newStatus = coordStatusInProgress
		_, err := db.DB.Exec(
			`UPDATE coordination_tasks
			 SET assignee_id = ?, status = ?, started_at = ?
			 WHERE id = ? AND status = ?`,
			body.AssigneeID, newStatus, now, id, t.Status)
		if err != nil {
			log.Printf("[coord] AssignCoordTask(%d) update failed: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "分配失败"})
			return
		}
	} else {
		newStatus = t.Status
		_, err := db.DB.Exec(
			`UPDATE coordination_tasks SET assignee_id = ? WHERE id = ?`,
			body.AssigneeID, id)
		if err != nil {
			log.Printf("[coord] AssignCoordTask(%d) reassign failed: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "分配失败"})
			return
		}
	}

	note := fmt.Sprintf("分配给 %s", assignName)
	if body.Note != "" {
		note += " · " + body.Note
	}
	recordEvent(id, "assign", t.Status, newStatus, uid, uname, note)
	WriteAuditLog(c, "coord_assign", "coordination_task", fmt.Sprintf("%d", id), assignName)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": newStatus}})
}

// SubmitCoordTask is the "修改完毕" button — developer marks the fix as
// ready for re-audit. Accepts transitions from in_progress OR
// awaiting_answer (dev might resolve the question themselves after
// thinking, and then finish).
func SubmitCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	_ = c.ShouldBindJSON(&body)

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if t.Status != coordStatusInProgress && t.Status != coordStatusAwaitingAnswer {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「开发中」或「待回复」的任务才能标记为修改完毕"})
		return
	}

	uid, uname := currentUser(c)
	now := time.Now().UTC()
	_, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET status = ?, submitted_at = ?
		 WHERE id = ? AND (status = ? OR status = ?)`,
		coordStatusUnderReview, now, id, coordStatusInProgress, coordStatusAwaitingAnswer)
	if err != nil {
		log.Printf("[coord] SubmitCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}
	recordEvent(id, "submit", t.Status, coordStatusUnderReview, uid, uname, body.Note)
	WriteAuditLog(c, "coord_submit", "coordination_task", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusUnderReview}})
}

// VerifyCoordTask is the "审计完成" button — admin confirms the fix after
// running a new audit. Moves under_review → done. Terminal.
func VerifyCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	_ = c.ShouldBindJSON(&body)

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if t.Status != coordStatusUnderReview {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「待验收」的任务才能标记为审计完成"})
		return
	}

	uid, uname := currentUser(c)
	now := time.Now().UTC()
	_, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET status = ?, verified_at = ?
		 WHERE id = ? AND status = ?`,
		coordStatusDone, now, id, coordStatusUnderReview)
	if err != nil {
		log.Printf("[coord] VerifyCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "验收失败"})
		return
	}
	recordEvent(id, "verify", coordStatusUnderReview, coordStatusDone, uid, uname, body.Note)
	WriteAuditLog(c, "coord_verify", "coordination_task", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusDone}})
}

// AskCoordTask is the developer's "我有个问题" button — pauses their work
// and flips the ball back to the task creator for clarification.
//
//	in_progress → awaiting_answer
//
// The question text is required (otherwise there's nothing for the
// creator to respond to).
func AskCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写你要问的问题"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if t.Status != coordStatusInProgress {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「开发中」的任务才能发起提问"})
		return
	}

	uid, uname := currentUser(c)
	_, err := db.DB.Exec(
		`UPDATE coordination_tasks SET status = ?
		 WHERE id = ? AND status = ?`,
		coordStatusAwaitingAnswer, id, coordStatusInProgress)
	if err != nil {
		log.Printf("[coord] AskCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}
	recordEvent(id, "ask", coordStatusInProgress, coordStatusAwaitingAnswer, uid, uname, body.Note)
	WriteAuditLog(c, "coord_ask", "coordination_task", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusAwaitingAnswer}})
}

// AnswerCoordTask is the creator's reply — flips the task back to the
// developer so they can continue.
//
//	awaiting_answer → in_progress
//
// Answer text is required; the timeline will pair it with the preceding
// ask event for context.
func AnswerCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写回复内容"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if t.Status != coordStatusAwaitingAnswer {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「待回复」的任务才能回复"})
		return
	}

	uid, uname := currentUser(c)
	_, err := db.DB.Exec(
		`UPDATE coordination_tasks SET status = ?
		 WHERE id = ? AND status = ?`,
		coordStatusInProgress, id, coordStatusAwaitingAnswer)
	if err != nil {
		log.Printf("[coord] AnswerCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "回复失败"})
		return
	}
	recordEvent(id, "answer", coordStatusAwaitingAnswer, coordStatusInProgress, uid, uname, body.Note)
	WriteAuditLog(c, "coord_answer", "coordination_task", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusInProgress}})
}

// AddCoordComment appends a free-form note to the timeline without
// changing state. Disabled once the task is done.
func AddCoordComment(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "备注内容必填"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	uid, uname := currentUser(c)
	recordEvent(id, "comment", t.Status, t.Status, uid, uname, body.Note)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已添加备注"})
}

// UpdateCoordTaskRequest covers editing an in-flight task before it
// gets pushed to dev. Besides the title and note we also let the creator
// rewrite the "original issue snapshot" — sometimes the AI-generated
// wording is too verbose or the creator wants to add context the
// developer cares about. This is a direct overwrite (no backup kept).
type UpdateCoordTaskRequest struct {
	Note          string `json:"note"`
	IssueTitle    string `json:"issue_title"`
	IssueSnapshot string `json:"issue_snapshot"` // full JSON; when empty, kept as-is
}

func UpdateCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var req UpdateCoordTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	// Per spec: issue title / note / snapshot can only be polished by
	// the creator BEFORE it's handed to a developer. Once started, the
	// issue is the contract — changing it mid-stream would confuse the
	// developer. Attachments and comments remain allowed in later states.
	if t.Status != coordStatusPending {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务已进入开发流程，问题描述不可再修改；如需补充信息，请在评论里留言"})
		return
	}

	// Validate the incoming snapshot is JSON so a later GET of the task
	// doesn't crash the frontend's JSON.parse.
	if req.IssueSnapshot != "" {
		if !json.Valid([]byte(req.IssueSnapshot)) {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "issue_snapshot 不是合法 JSON"})
			return
		}
	}

	_, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET note = COALESCE(NULLIF(?, ''), note),
		     issue_title = COALESCE(NULLIF(?, ''), issue_title),
		     issue_snapshot = COALESCE(NULLIF(?, ''), issue_snapshot)
		 WHERE id = ?`,
		req.Note, req.IssueTitle, req.IssueSnapshot, id)
	if err != nil {
		log.Printf("[coord] UpdateCoordTask(%d) failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}
	uid, uname := currentUser(c)
	recordEvent(id, "edit", t.Status, t.Status, uid, uname, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已更新"})
}

// DeleteCoordTask refuses once the task reached done (per the留痕
// requirement). Before that point the creator or an admin can drop
// the task (e.g. duplicate entry) — it cascades the events + attachments
// rows so we don't leave orphans.
func DeleteCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	// Only the creator or an admin can delete.
	uid, _ := currentUser(c)
	role, _ := c.Get("role")
	if t.CreatedBy != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅创建人或管理员可删除"})
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	for _, q := range []string{
		"DELETE FROM coordination_attachments WHERE task_id = ?",
		"DELETE FROM coordination_events WHERE task_id = ?",
		"DELETE FROM coordination_tasks WHERE id = ?",
	} {
		if _, err := tx.Exec(q, id); err != nil {
			_ = tx.Rollback()
			log.Printf("[coord] DeleteCoordTask(%d) step failed: %s: %v", id, q, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	WriteAuditLog(c, "coord_delete", "coordination_task", fmt.Sprintf("%d", id), t.IssueTitle)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}

// ─── Stats / dashboard helper ───────────────────────────────────────────────

// CoordStats returns per-status counts (overall or scoped to a site) so
// the list page can render tab badges.
func CoordStats(c *gin.Context) {
	siteID := c.Query("site_id")

	type row struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	q := "SELECT status, COUNT(*) FROM coordination_tasks"
	args := []interface{}{}
	if siteID != "" {
		q += " WHERE site_id = ?"
		args = append(args, siteID)
	}
	q += " GROUP BY status"

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	counts := map[string]int{
		coordStatusPending:        0,
		coordStatusInProgress:     0,
		coordStatusAwaitingAnswer: 0,
		coordStatusUnderReview:    0,
		coordStatusDone:           0,
	}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.Status, &r.Count); err == nil {
			counts[r.Status] = r.Count
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": counts})
}
