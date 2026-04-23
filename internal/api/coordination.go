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
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
)

// ─── Domain types ───────────────────────────────────────────────────────────

const (
	coordStatusPending        = "pending"
	coordStatusPendingReview  = "pending_review" // awaiting 组长/经理 approvals before going to dev
	coordStatusInProgress     = "in_progress"
	coordStatusAwaitingAnswer = "awaiting_answer" // developer raised a question, waiting on creator
	coordStatusUnderReview    = "under_review"
	coordStatusDone           = "done"
)

// validCoordStatus reports whether s is one of the legal states.
func validCoordStatus(s string) bool {
	switch s {
	case coordStatusPending, coordStatusPendingReview, coordStatusInProgress, coordStatusAwaitingAnswer,
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
	IssueScope      string  `json:"issue_scope"` // 影响页面：全站 / 首页 / 详情页 / 列表页 / 等
	IssueSnapshot   string  `json:"issue_snapshot"`
	Note            string  `json:"note"`
	Status          string  `json:"status"`
	CreatedBy       int64   `json:"created_by"`
	CreatorName     string  `json:"creator_name,omitempty"`
	CreatorDept     string  `json:"creator_dept,omitempty"`
	CreatorGroup    string  `json:"creator_group,omitempty"`
	AssigneeID      int64   `json:"assignee_id"`
	AssigneeName    string  `json:"assignee_name,omitempty"`
	AssigneeDept    string  `json:"assignee_dept,omitempty"`
	AssigneeGroup   string  `json:"assignee_group,omitempty"`
	CreatedAt       string  `json:"created_at"`
	StartedAt       *string `json:"started_at"`
	SubmittedAt     *string `json:"submitted_at"`
	VerifiedAt      *string `json:"verified_at"`
	PreAuditVerdict string  `json:"pre_audit_verdict,omitempty"`
	PreAuditResult  string  `json:"pre_audit_result,omitempty"`
	PreAuditAt      *string `json:"pre_audit_at,omitempty"`
	// When the task is in pending_review, pending_assignee_id captures
	// who the creator wants the task to land with once all reviewers
	// have approved. Zero / empty on tasks that skipped the review step.
	PendingAssigneeID   int64  `json:"pending_assignee_id,omitempty"`
	PendingAssigneeName string `json:"pending_assignee_name,omitempty"`
	PendingAssignNote   string `json:"pending_assign_note,omitempty"`
}

// coordReviewer is one row in coord_task_reviewers, exposed to the
// frontend for rendering the reviewer panel on the task detail page.
type coordReviewer struct {
	ID           int64   `json:"id"`
	TaskID       int64   `json:"task_id"`
	ReviewerID   int64   `json:"reviewer_id"`
	ReviewerName string  `json:"reviewer_name"`
	ReviewerDept string  `json:"reviewer_dept,omitempty"`
	Decision     string  `json:"decision"` // pending / approved / rejected
	DecisionNote string  `json:"decision_note,omitempty"`
	DecidedAt    *string `json:"decided_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type coordEvent struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	EventType  string `json:"event_type"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	ActorID    int64  `json:"actor_id"`
	ActorName  string `json:"actor_name"`
	ActorDept  string `json:"actor_dept,omitempty"`
	ActorGroup string `json:"actor_group,omitempty"`
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
		       t.issue_module, t.issue_chapter, COALESCE(t.issue_scope,''),
		       t.issue_snapshot, t.note,
		       t.status,
		       t.created_by, COALESCE(u1.name,''), COALESCE(d1.name,''), COALESCE(g1.name,''),
		       t.assignee_id, COALESCE(u2.name,''), COALESCE(d2.name,''), COALESCE(g2.name,''),
		       t.created_at, t.started_at, t.submitted_at, t.verified_at,
		       COALESCE(t.pre_audit_verdict,''), COALESCE(t.pre_audit_result,''),
		       t.pre_audit_at,
		       COALESCE(t.pending_assignee_id,0), COALESCE(u3.name,''),
		       COALESCE(t.pending_assign_note,'')
		FROM coordination_tasks t
		LEFT JOIN sites s ON s.id = t.site_id
		LEFT JOIN users u1 ON u1.id = t.created_by
		LEFT JOIN departments d1 ON d1.id = u1.department_id
		LEFT JOIN dept_groups g1 ON g1.id = u1.group_id
		LEFT JOIN users u2 ON u2.id = t.assignee_id
		LEFT JOIN departments d2 ON d2.id = u2.department_id
		LEFT JOIN dept_groups g2 ON g2.id = u2.group_id
		LEFT JOIN users u3 ON u3.id = t.pending_assignee_id
		WHERE t.id = $1`
	var t coordTask
	var started, submitted, verified, preAuditAt sql.NullString
	err := db.DB.QueryRow(q, id).Scan(
		&t.ID, &t.SiteID, &t.SiteName, &t.SiteDomain,
		&t.AuditTaskID, &t.IssueID, &t.IssueTitle, &t.IssueSeverity,
		&t.IssueModule, &t.IssueChapter, &t.IssueScope,
		&t.IssueSnapshot, &t.Note,
		&t.Status,
		&t.CreatedBy, &t.CreatorName, &t.CreatorDept, &t.CreatorGroup,
		&t.AssigneeID, &t.AssigneeName, &t.AssigneeDept, &t.AssigneeGroup,
		&t.CreatedAt, &started, &submitted, &verified,
		&t.PreAuditVerdict, &t.PreAuditResult, &preAuditAt,
		&t.PendingAssigneeID, &t.PendingAssigneeName, &t.PendingAssignNote,
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
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
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
	if err := db.DB.QueryRow("SELECT name FROM users WHERE id = $1", uid).Scan(&name); err != nil && err != sql.ErrNoRows {
		log.Printf("[coord] currentUser(%d) lookup failed: %v", uid, err)
	}
	return uid, name
}

// extractIssueScope pulls the "scope" field out of an issue snapshot
// JSON blob. The audit pipeline (see ai/analyzer.go::V6Issue) stores
// strings like "全站" / "首页" / "详情页" / "播放列表页" — that's the
// "影响页面" the coordination list filters by. Returns empty string
// for malformed JSON or missing field; the caller writes "" to DB.
func extractIssueScope(snapshot string) string {
	s := strings.TrimSpace(snapshot)
	if s == "" || (s[0] != '{' && s[0] != '[') {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return ""
	}
	if v, ok := m["scope"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// BackfillCoordIssueScope runs once at startup (keyed by settings flag
// `coord_scope_backfilled_v1`) and fills issue_scope for every old
// coordination task created before the column existed.
func BackfillCoordIssueScope() {
	var done string
	db.DB.QueryRow(`SELECT COALESCE(value,'') FROM settings WHERE key='coord_scope_backfilled_v1'`).Scan(&done)
	if done == "1" {
		return
	}
	rows, err := db.DB.Query(
		`SELECT id, COALESCE(issue_snapshot,'') FROM coordination_tasks
		 WHERE COALESCE(issue_scope,'') = ''`)
	if err != nil {
		log.Printf("[coord-backfill] list: %v", err)
		return
	}
	type r struct {
		id   int64
		snap string
	}
	var todo []r
	for rows.Next() {
		var x r
		if err := rows.Scan(&x.id, &x.snap); err == nil {
			todo = append(todo, x)
		}
	}
	rows.Close()
	updated := 0
	for _, x := range todo {
		scope := extractIssueScope(x.snap)
		if scope == "" {
			continue
		}
		if _, err := db.DB.Exec(
			`UPDATE coordination_tasks SET issue_scope = $1 WHERE id = $2`,
			scope, x.id,
		); err == nil {
			updated++
		}
	}
	db.DB.Exec(`INSERT INTO settings(key, value) VALUES('coord_scope_backfilled_v1','1')
		ON CONFLICT(key) DO UPDATE SET value='1'`)
	if updated > 0 {
		log.Printf("[coord-backfill] filled issue_scope on %d task(s)", updated)
	}
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

// ─── Authorization helpers for per-role actions ────────────────────────────

// isAdmin reports whether the current caller has the admin role.
func isAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	return role == "admin"
}

// requireCreatorOrAdmin blocks actions that only the task creator (or an
// admin) may perform: assign, edit, verify, reject. Writes a 403 and
// returns false when the caller is neither.
func requireCreatorOrAdmin(c *gin.Context, t *coordTask, action string) bool {
	uid := c.GetInt64("user_id")
	if t.CreatedBy == uid || isAdmin(c) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅任务创建人或管理员可" + action})
	return false
}

// requireAssignee blocks actions that only the assigned developer may
// perform: submit ("开发完毕"), ask ("提问"). Strict — no admin bypass.
// An admin who needs to act as the developer must reassign the task to
// themselves first; this avoids the footgun where admin-as-creator
// ends up self-submitting their own task.
func requireAssignee(c *gin.Context, t *coordTask, action string) bool {
	uid := c.GetInt64("user_id")
	if t.AssigneeID == uid {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅任务开发者可" + action + "（管理员如需代操作，请先把任务重新分配给自己）"})
	return false
}

// notifyCoord delivers a Telegram push to the given user if they've
// configured the integration. Silent on failure — a notification failure
// must never break the state transition that triggered it.
func notifyCoord(userID int64, title string, t *coordTask, extra string) {
	if userID <= 0 || t == nil {
		return
	}
	body := fmt.Sprintf("任务 #%d · %s", t.ID, t.IssueTitle)
	if t.SiteName != "" {
		body += "\n站点：" + t.SiteName
	}
	if extra != "" {
		body += "\n" + extra
	}
	telegram.NotifyUser(userID, title, body)
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

	// Wrap task INSERT + event INSERTs in a single transaction.
	tx, txErr := db.DB.Begin()
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Extract scope ("影响页面" — 全站 / 首页 / 详情页 / 列表页 / etc.)
	// from the issue_snapshot JSON so the list page can show + filter
	// by it without re-parsing on every render. Store empty string if
	// the snapshot doesn't carry a scope.
	issueScope := extractIssueScope(req.IssueSnapshot)

	var taskID int64
	err := tx.QueryRow(
		`INSERT INTO coordination_tasks
		 (site_id, audit_task_id, issue_id, issue_title, issue_severity,
		  issue_module, issue_chapter, issue_snapshot, issue_scope, note, status,
		  created_by, assignee_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11, $12) RETURNING id`,
		req.SiteID, req.AuditTaskID, req.IssueID, req.IssueTitle, req.IssueSeverity,
		req.IssueModule, req.IssueChapter, req.IssueSnapshot, issueScope, req.Note,
		uid, req.AssigneeID,
	).Scan(&taskID)
	if err != nil {
		log.Printf("[coord] CreateCoordTask insert failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}

	// Timeline: task was created (+ optionally assigned in the same call).
	tx.Exec(
		`INSERT INTO coordination_events
		 (task_id, event_type, from_status, to_status, actor_id, actor_name, note)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		taskID, "create", "", coordStatusPending, uid, uname, req.Note,
	)
	if req.AssigneeID > 0 {
		var asignName string
		db.DB.QueryRow("SELECT name FROM users WHERE id = $1", req.AssigneeID).Scan(&asignName)
		tx.Exec(
			`INSERT INTO coordination_events
			 (task_id, event_type, from_status, to_status, actor_id, actor_name, note)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			taskID, "assign", coordStatusPending, coordStatusPending, uid, uname,
			fmt.Sprintf("分配给 %s", asignName),
		)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[coord] CreateCoordTask commit failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
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
		where = append(where, fmt.Sprintf("t.site_id = $%d", len(args)+1))
		args = append(args, sid)
	} else {
		role, _ := c.Get("role")
		if role != "admin" {
			userID := c.GetInt64("user_id")
			// Non-admin can see tasks where they are:
			//   - site owner (created the site)
			//   - task creator
			//   - task assignee (currently in progress)
			//   - pending assignee (during review phase)
			//   - reviewer (on the roster)
			n := len(args) + 1
			where = append(where, fmt.Sprintf(
				`(t.site_id IN (SELECT id FROM sites WHERE created_by = $%d)
				  OR t.created_by = $%d
				  OR t.assignee_id = $%d
				  OR COALESCE(t.pending_assignee_id,0) = $%d
				  OR EXISTS (SELECT 1 FROM coord_task_reviewers r WHERE r.task_id = t.id AND r.reviewer_id = $%d))`,
				n, n+1, n+2, n+3, n+4))
			args = append(args, userID, userID, userID, userID, userID)
		}
	}
	if s := c.Query("status"); s != "" {
		if !validCoordStatus(s) {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "status 取值非法"})
			return
		}
		where = append(where, fmt.Sprintf("t.status = $%d", len(args)+1))
		args = append(args, s)
	}
	if a := c.Query("assignee_id"); a != "" {
		where = append(where, fmt.Sprintf("t.assignee_id = $%d", len(args)+1))
		args = append(args, a)
	}
	// 影响页面筛选 — exact match against the extracted scope ("全站" /
	// "首页" / "详情页" / etc.). Frontend gets the option list from the
	// distinct values endpoint below.
	if sc := strings.TrimSpace(c.Query("scope")); sc != "" {
		where = append(where, fmt.Sprintf("COALESCE(t.issue_scope,'') = $%d", len(args)+1))
		args = append(args, sc)
	}
	// Admin 协同管理 page filters: by 部门 / 组 / 姓名搜索. These scope
	// by the ASSIGNEE's user profile — a 部门经理 wants "show me every
	// task assigned to my people", not every task my team members created.
	// Creator-side matching is handled by user_search (OR) so typing
	// someone's name surfaces tasks they're involved in either role.
	if d := c.Query("department_id"); d != "" {
		where = append(where, fmt.Sprintf(
			"t.assignee_id IN (SELECT id FROM users WHERE department_id::text = $%d)", len(args)+1))
		args = append(args, d)
	}
	if g := c.Query("group_id"); g != "" {
		where = append(where, fmt.Sprintf(
			"t.assignee_id IN (SELECT id FROM users WHERE group_id::text = $%d)", len(args)+1))
		args = append(args, g)
	}
	if q := strings.TrimSpace(c.Query("user_search")); q != "" {
		// Match on either creator or assignee: "show me tasks 李信 is
		// involved in" regardless of which side he's on. Use subqueries
		// rather than alias-referencing u1/u2 directly — the COUNT(*)
		// query below doesn't JOIN users, so the WHERE must be
		// standalone-evaluable against coordination_tasks alone.
		needle := "%" + strings.ToLower(q) + "%"
		where = append(where, fmt.Sprintf(
			"(t.created_by IN (SELECT id FROM users WHERE LOWER(COALESCE(name,'')) LIKE $%d)"+
				" OR t.assignee_id IN (SELECT id FROM users WHERE LOWER(COALESCE(name,'')) LIKE $%d))",
			len(args)+1, len(args)+2))
		args = append(args, needle, needle)
	}
	// Admin 用户监控 page passes ?involved_user_id=N to list every task
	// the given user is involved in — either as creator OR assignee.
	// Only honored for admins; non-admins can't filter across other
	// users anyway since the endpoint is site-scoped downstream.
	if v := c.Query("involved_user_id"); v != "" {
		if role, _ := c.Get("role"); role == "admin" {
			where = append(where, fmt.Sprintf("(t.created_by = $%d OR t.assignee_id = $%d)", len(args)+1, len(args)+2))
			args = append(args, v, v)
		}
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM coordination_tasks t WHERE "+strings.Join(where, " AND "), args...).Scan(&total)

	q := `
		SELECT t.id, t.site_id, COALESCE(s.name,''), COALESCE(s.domain,''),
		       t.audit_task_id, t.issue_id, t.issue_title, t.issue_severity,
		       t.issue_module, t.issue_chapter, COALESCE(t.issue_scope,''),
		       t.issue_snapshot, t.note,
		       t.status,
		       t.created_by, COALESCE(u1.name,''), COALESCE(d1.name,''), COALESCE(g1.name,''),
		       t.assignee_id, COALESCE(u2.name,''), COALESCE(d2.name,''), COALESCE(g2.name,''),
		       t.created_at, t.started_at, t.submitted_at, t.verified_at,
		       COALESCE(t.pre_audit_verdict,'')
		FROM coordination_tasks t
		LEFT JOIN sites s ON s.id = t.site_id
		LEFT JOIN users u1 ON u1.id = t.created_by
		LEFT JOIN departments d1 ON d1.id = u1.department_id
		LEFT JOIN dept_groups g1 ON g1.id = u1.group_id
		LEFT JOIN users u2 ON u2.id = t.assignee_id
		LEFT JOIN departments d2 ON d2.id = u2.department_id
		LEFT JOIN dept_groups g2 ON g2.id = u2.group_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY
			CASE t.status
				WHEN 'awaiting_answer' THEN 0
				WHEN 'pending_review'  THEN 1
				WHEN 'under_review'    THEN 2
				WHEN 'in_progress'     THEN 3
				WHEN 'pending'         THEN 4
				WHEN 'done'            THEN 5
				ELSE 6
			END,
			t.created_at DESC
		LIMIT ` + fmt.Sprintf("$%d", len(args)+1) + ` OFFSET ` + fmt.Sprintf("$%d", len(args)+2)

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
			&t.IssueModule, &t.IssueChapter, &t.IssueScope,
			&t.IssueSnapshot, &t.Note,
			&t.Status,
			&t.CreatedBy, &t.CreatorName, &t.CreatorDept, &t.CreatorGroup,
			&t.AssigneeID, &t.AssigneeName, &t.AssigneeDept, &t.AssigneeGroup,
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

	// Timeline — JOIN users + departments + dept_groups so each event
	// carries the actor's dept/group at read time. We resolve it live
	// rather than snapshot at write time so that post-hoc org changes
	// (re-org'ing a department) are reflected accurately on re-view.
	events := []coordEvent{}
	rows, err := db.DB.Query(
		`SELECT e.id, e.task_id, e.event_type, e.from_status, e.to_status,
		        e.actor_id, e.actor_name,
		        COALESCE(d.name,''), COALESCE(g.name,''),
		        e.note, e.created_at
		 FROM coordination_events e
		 LEFT JOIN users u ON u.id = e.actor_id
		 LEFT JOIN departments d ON d.id = u.department_id
		 LEFT JOIN dept_groups g ON g.id = u.group_id
		 WHERE e.task_id = $1
		 ORDER BY e.created_at ASC, e.id ASC`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var e coordEvent
			if err := rows.Scan(&e.ID, &e.TaskID, &e.EventType, &e.FromStatus, &e.ToStatus,
				&e.ActorID, &e.ActorName, &e.ActorDept, &e.ActorGroup, &e.Note, &e.CreatedAt); err == nil {
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
		 WHERE task_id = $1 ORDER BY uploaded_at ASC`, id)
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

	// Reviewer roster — empty for tasks that skipped the review gate.
	reviewers, _ := loadCoordReviewers(id)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"task":        t,
			"events":      events,
			"attachments": attachments,
			"reviewers":   reviewers,
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
	// Optional multi-select of 组长 / 经理 who must approve before the task
	// can be handed to the developer. Empty = legacy direct assign (no
	// gating). Any non-empty list puts the task into pending_review and
	// requires ALL reviewers to approve.
	ReviewerIDs []int64 `json:"reviewer_ids"`
	// Reviewer decision body (used by /review). "approved" or "rejected".
	Decision string `json:"decision"`
}

// AssignCoordTask routes the task to development. Two variants:
//
//  1. Direct (legacy) — caller sends assignee_id only, no reviewer_ids.
//     Task jumps straight from pending → in_progress, assignee set,
//     started_at stamped, developer pinged. Same as before.
//
//  2. Review-gated — caller sends assignee_id plus reviewer_ids=[…].
//     Task moves pending → pending_review. We record pending_assignee_id
//     (who it *will* go to once approved), wipe any previous reviewer
//     rows, insert one coord_task_reviewers row per listed reviewer with
//     decision='pending', and notify each reviewer. The actual assignee_id
//     stays zero / unchanged until the final approval promotes it.
//
// Reassigning an already in_progress task (moving it to a different dev)
// still works direct, no review — changing hands mid-development is an
// ops fix, not a new work item.
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
	if !requireCreatorOrAdmin(c, t, "分配任务") {
		return
	}

	var assignName string
	if err := db.DB.QueryRow("SELECT name FROM users WHERE id = $1", body.AssigneeID).Scan(&assignName); err != nil && err != sql.ErrNoRows {
		log.Printf("[coord] AssignCoordTask(%d) assignee lookup failed: %v", id, err)
	}
	if assignName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "指定的开发账号不存在"})
		return
	}

	// De-duplicate + validate reviewer ids. The creator can't be their
	// own reviewer (defeats the gate-keeping purpose), and the chosen
	// developer can't review the request to work on themselves.
	uid, uname := currentUser(c)
	reviewerIDs := sanitizeReviewerIDs(body.ReviewerIDs, uid, body.AssigneeID)

	// Review-gated path — available from the initial 'pending' state
	// and from 'pending_review' (resend after a reviewer rejected).
	// Reassigning an already in-progress task bypasses the gate; the
	// original assignment is already approved.
	if len(reviewerIDs) > 0 && (t.Status == coordStatusPending || t.Status == coordStatusPendingReview) {
		if err := startReviewFlow(c, id, t, uid, uname, body, assignName, reviewerIDs); err != nil {
			log.Printf("[coord] startReviewFlow(%d) failed: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "送审失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusPendingReview}})
		return
	}

	now := time.Now().UTC()
	var newStatus string
	if t.Status == coordStatusPending || t.Status == coordStatusPendingReview {
		// Direct assign (no reviewers requested) from either of the two
		// pre-dev states. Wipe any leftover reviewer rows + pending_*
		// columns so the task doesn't display stale review data.
		newStatus = coordStatusInProgress
		res, err := db.DB.Exec(
			`UPDATE coordination_tasks
			 SET assignee_id = $1, status = $2, started_at = $3,
			     pending_assignee_id = 0, pending_assign_note = ''
			 WHERE id = $4 AND (status = $5 OR status = $6)`,
			body.AssigneeID, newStatus, now, id, coordStatusPending, coordStatusPendingReview)
		if err != nil {
			log.Printf("[coord] AssignCoordTask(%d) update failed: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "分配失败"})
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
			return
		}
		// Clean up leftover reviewer rows from a prior (cancelled) review.
		db.DB.Exec(`DELETE FROM coord_task_reviewers WHERE task_id = $1`, id)
	} else {
		// In-flight reassignment (in_progress, awaiting_answer). State
		// unchanged, only swap assignee.
		newStatus = t.Status
		_, err := db.DB.Exec(
			`UPDATE coordination_tasks SET assignee_id = $1 WHERE id = $2`,
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
	t.AssigneeID = body.AssigneeID
	t.Status = newStatus
	notifyCoord(body.AssigneeID, "🛠 新协同任务", t, fmt.Sprintf("由 %s 分配", uname))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": newStatus}})
}

// sanitizeReviewerIDs dedups the incoming list and strips out the creator
// and the proposed assignee — neither may review the request.
func sanitizeReviewerIDs(ids []int64, creatorID, assigneeID int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, r := range ids {
		if r <= 0 || r == creatorID || r == assigneeID || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// startReviewFlow transitions a pending task into pending_review, wiping
// any prior reviewer rows, stamping pending_assignee_id + pending_assign_note,
// inserting one reviewer row per id, and notifying each reviewer.
func startReviewFlow(c *gin.Context, taskID int64, t *coordTask, uid int64, uname string, body transitionBody, assigneeName string, reviewerIDs []int64) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`UPDATE coordination_tasks
		 SET status = $1, pending_assignee_id = $2, pending_assign_note = $3
		 WHERE id = $4 AND (status = $5 OR status = $6)`,
		coordStatusPendingReview, body.AssigneeID, body.Note, taskID,
		coordStatusPending, coordStatusPendingReview)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("task status changed before we could lock it")
	}
	if _, err := tx.Exec(`DELETE FROM coord_task_reviewers WHERE task_id = $1`, taskID); err != nil {
		return err
	}
	for _, rid := range reviewerIDs {
		if _, err := tx.Exec(
			`INSERT INTO coord_task_reviewers (task_id, reviewer_id, decision)
			 VALUES ($1, $2, 'pending')`, taskID, rid); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	summary := fmt.Sprintf("送审 · 拟派 %s · %d 位审核人", assigneeName, len(reviewerIDs))
	if body.Note != "" {
		summary += " · " + body.Note
	}
	recordEvent(taskID, "review_submit", t.Status, coordStatusPendingReview, uid, uname, summary)
	WriteAuditLog(c, "coord_submit_review", "coordination_task", fmt.Sprintf("%d", taskID), assigneeName)

	// Notify each reviewer — copy t so we don't mutate the caller's view.
	snapshot := *t
	snapshot.Status = coordStatusPendingReview
	for _, rid := range reviewerIDs {
		notifyCoord(rid, "👀 协同任务待审核", &snapshot, fmt.Sprintf("%s 送来一项任务待你审核，拟派 %s", uname, assigneeName))
	}
	return nil
}

// loadCoordReviewers returns the reviewer roster + each decision for a
// given task, ordered by created_at so the timeline feels stable.
func loadCoordReviewers(taskID int64) ([]coordReviewer, error) {
	rows, err := db.DB.Query(
		`SELECT r.id, r.task_id, r.reviewer_id, COALESCE(u.name,''), COALESCE(d.name,''),
		        r.decision, COALESCE(r.decision_note,''), r.decided_at, r.created_at
		 FROM coord_task_reviewers r
		 LEFT JOIN users u ON u.id = r.reviewer_id
		 LEFT JOIN departments d ON d.id = u.department_id
		 WHERE r.task_id = $1
		 ORDER BY r.created_at ASC, r.id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coordReviewer{}
	for rows.Next() {
		var r coordReviewer
		var decidedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.TaskID, &r.ReviewerID, &r.ReviewerName, &r.ReviewerDept,
			&r.Decision, &r.DecisionNote, &decidedAt, &r.CreatedAt); err != nil {
			continue
		}
		if decidedAt.Valid {
			r.DecidedAt = &decidedAt.String
		}
		out = append(out, r)
	}
	return out, nil
}

// SubmitCoordReviewerDecision is called by a 组长 / 经理 clicking 同意 or 拒绝
// on a task they were added as reviewer for. When the LAST pending reviewer
// approves the task auto-promotes to in_progress and pings the dev. A
// reject locks the task in pending_review until the creator re-submits.
func SubmitCoordReviewerDecision(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	decision := strings.ToLower(strings.TrimSpace(body.Decision))
	if decision != "approved" && decision != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "decision 必须是 approved 或 rejected"})
		return
	}
	if decision == "rejected" && strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "拒绝时请填写理由"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if t.Status != coordStatusPendingReview {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「待审核」的任务才能审批"})
		return
	}

	uid, uname := currentUser(c)

	// Caller must be on the reviewer roster, and their row must still be
	// pending (no re-voting after they've already decided).
	var rowID int64
	var currentDecision string
	err := db.DB.QueryRow(
		`SELECT id, decision FROM coord_task_reviewers
		 WHERE task_id = $1 AND reviewer_id = $2`, id, uid).Scan(&rowID, &currentDecision)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "你不在该任务的审核人列表里"})
		return
	}
	if err != nil {
		log.Printf("[coord] SubmitCoordReviewerDecision(%d) reviewer lookup failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "加载失败"})
		return
	}
	if currentDecision != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "你已经审核过，不可重复操作"})
		return
	}

	// Flip the reviewer row. Keep the WHERE decision='pending' for
	// belt-and-suspenders concurrency (two tabs, same reviewer, racing clicks).
	now := time.Now().UTC()
	res, err := db.DB.Exec(
		`UPDATE coord_task_reviewers
		 SET decision = $1, decision_note = $2, decided_at = $3
		 WHERE id = $4 AND decision = 'pending'`,
		decision, body.Note, now, rowID)
	if err != nil {
		log.Printf("[coord] SubmitCoordReviewerDecision(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "审核失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "该审核已被处理，请刷新"})
		return
	}

	// Timeline + audit log.
	eventType := "review_approve"
	verb := "同意"
	if decision == "rejected" {
		eventType = "review_reject"
		verb = "拒绝"
	}
	noteForTimeline := verb
	if body.Note != "" {
		noteForTimeline = verb + "：" + body.Note
	}
	recordEvent(id, eventType, coordStatusPendingReview, coordStatusPendingReview, uid, uname, noteForTimeline)
	WriteAuditLog(c, "coord_review_"+decision, "coordination_task", fmt.Sprintf("%d", id), body.Note)

	// Recount remaining pending reviewers. If this was a reject, the
	// task stays in pending_review regardless — the creator must act
	// (cancel + resend, or edit the issue).
	if decision == "rejected" {
		// Ping the creator so they know the review came back with a concern.
		notifyCoord(t.CreatedBy, "❌ 协同任务被拒绝", t, fmt.Sprintf("%s 拒绝：%s", uname, body.Note))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusPendingReview, "promoted": false}})
		return
	}

	// Approved: does any row still read 'pending' or 'rejected'?
	var remainingPending, anyRejected int
	db.DB.QueryRow(
		`SELECT
		   COUNT(*) FILTER (WHERE decision = 'pending'),
		   COUNT(*) FILTER (WHERE decision = 'rejected')
		 FROM coord_task_reviewers WHERE task_id = $1`, id).Scan(&remainingPending, &anyRejected)

	if remainingPending > 0 || anyRejected > 0 {
		// Not the last approval — ping the creator as a progress update
		// but leave the task in pending_review for now.
		notifyCoord(t.CreatedBy, "✅ 审核人同意", t, fmt.Sprintf("%s 同意（还剩 %d 位待审核）", uname, remainingPending))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusPendingReview, "promoted": false}})
		return
	}

	// All reviewers approved — auto-promote to in_progress, copy the
	// pending_assignee_id over to assignee_id, stamp started_at.
	promoteRes, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET status = $1, assignee_id = pending_assignee_id, started_at = $2,
		     pending_assignee_id = 0, pending_assign_note = ''
		 WHERE id = $3 AND status = $4`,
		coordStatusInProgress, now, id, coordStatusPendingReview)
	if err != nil || promoteRes == nil {
		log.Printf("[coord] SubmitCoordReviewerDecision(%d) promote failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "审核保存成功但推送给开发失败，请刷新"})
		return
	}
	if n, _ := promoteRes.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
		return
	}

	recordEvent(id, "assign", coordStatusPendingReview, coordStatusInProgress, uid, uname,
		fmt.Sprintf("全体审核人同意，任务自动推给 %s", t.PendingAssigneeName))

	// Reload so the Telegram notification carries the final assignee_id.
	updated, err := loadCoordTask(id)
	if err == nil && updated != nil {
		notifyCoord(updated.AssigneeID, "🛠 新协同任务", updated, "所有审核人已同意，任务已分配给你")
		notifyCoord(updated.CreatedBy, "✅ 任务已进入开发", updated, "所有审核人已同意，任务已推给开发")
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusInProgress, "promoted": true}})
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
	// Only the developer on the hook can declare the work done. An admin
	// who happens to be the creator must NOT be able to self-submit —
	// that would collapse the 开发 / 审核 separation.
	if !requireAssignee(c, t, "提交修改完毕") {
		return
	}

	uid, uname := currentUser(c)
	now := time.Now().UTC()
	res, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET status = $1, submitted_at = $2
		 WHERE id = $3 AND (status = $4 OR status = $5)`,
		coordStatusUnderReview, now, id, coordStatusInProgress, coordStatusAwaitingAnswer)
	if err != nil {
		log.Printf("[coord] SubmitCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}
	// Optimistic-lock loss: another admin flipped this task state in
	// the ~ms window between our load and our UPDATE. Bail cleanly
	// with 409 rather than write a phantom timeline + Telegram ping.
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
		return
	}
	recordEvent(id, "submit", t.Status, coordStatusUnderReview, uid, uname, body.Note)
	WriteAuditLog(c, "coord_submit", "coordination_task", fmt.Sprintf("%d", id), "")
	// Flip the ball back to the creator for verification.
	t.Status = coordStatusUnderReview
	notifyCoord(t.CreatedBy, "✅ 开发完毕待验收", t, fmt.Sprintf("%s 提交了修改，等待验收", uname))
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
	// Verification is the creator's call (or admin's). The developer
	// approving their own fix would trivialize the review step.
	if !requireCreatorOrAdmin(c, t, "验收任务") {
		return
	}

	// #36: Require a completed audit (status='done') for the site created
	// after the task was submitted, so the verifier has fresh evidence.
	var auditCount int
	submittedRef := t.SubmittedAt
	if submittedRef == nil {
		submittedRef = &t.CreatedAt
	}
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM audit_tasks
		 WHERE site_id = $1 AND status = 'done' AND created_at > $2`,
		t.SiteID, *submittedRef,
	).Scan(&auditCount)
	if auditCount == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "验收前需要先完成一次新的站点审计"})
		return
	}

	uid, uname := currentUser(c)
	now := time.Now().UTC()
	res, err := db.DB.Exec(
		`UPDATE coordination_tasks
		 SET status = $1, verified_at = $2
		 WHERE id = $3 AND status = $4`,
		coordStatusDone, now, id, coordStatusUnderReview)
	if err != nil {
		log.Printf("[coord] VerifyCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "验收失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
		return
	}
	recordEvent(id, "verify", coordStatusUnderReview, coordStatusDone, uid, uname, body.Note)
	WriteAuditLog(c, "coord_verify", "coordination_task", fmt.Sprintf("%d", id), "")
	// Final beat: let the developer know their fix was accepted.
	t.Status = coordStatusDone
	notifyCoord(t.AssigneeID, "🎯 任务已验收通过", t, fmt.Sprintf("%s 已确认修复", uname))
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

	// #35: Only the assignee may ask a question. No admin bypass —
	// questions are a developer's tool to unblock themselves, not a
	// generic comment. Admin can still use 评论 to drop a note.
	uid, uname := currentUser(c)
	if !requireAssignee(c, t, "发起提问") {
		return
	}

	if t.Status != coordStatusInProgress {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「开发中」的任务才能发起提问"})
		return
	}
	res, err := db.DB.Exec(
		`UPDATE coordination_tasks SET status = $1
		 WHERE id = $2 AND status = $3`,
		coordStatusAwaitingAnswer, id, coordStatusInProgress)
	if err != nil {
		log.Printf("[coord] AskCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
		return
	}
	recordEvent(id, "ask", coordStatusInProgress, coordStatusAwaitingAnswer, uid, uname, body.Note)
	WriteAuditLog(c, "coord_ask", "coordination_task", fmt.Sprintf("%d", id), "")
	// Developer is blocked waiting on the creator — ping them with the question.
	t.Status = coordStatusAwaitingAnswer
	notifyCoord(t.CreatedBy, "❓ 开发提问", t, fmt.Sprintf("%s 提问：%s", uname, body.Note))
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

	// #35: Only the task creator (or admin) may answer.
	uid, uname := currentUser(c)
	role, _ := c.Get("role")
	if t.CreatedBy != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只有任务创建者或管理员才能回复"})
		return
	}

	if t.Status != coordStatusAwaitingAnswer {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "只有「待回复」的任务才能回复"})
		return
	}
	res, err := db.DB.Exec(
		`UPDATE coordination_tasks SET status = $1
		 WHERE id = $2 AND status = $3`,
		coordStatusInProgress, id, coordStatusAwaitingAnswer)
	if err != nil {
		log.Printf("[coord] AnswerCoordTask(%d) update failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "回复失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "任务状态已变更，请刷新后重试"})
		return
	}
	recordEvent(id, "answer", coordStatusAwaitingAnswer, coordStatusInProgress, uid, uname, body.Note)
	WriteAuditLog(c, "coord_answer", "coordination_task", fmt.Sprintf("%d", id), "")
	// Hand the ball back to the developer so they can keep going.
	t.Status = coordStatusInProgress
	notifyCoord(t.AssigneeID, "💬 任务已回复", t, fmt.Sprintf("%s 回复：%s", uname, body.Note))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": coordStatusInProgress}})
}

// PingDevCoordTask is the creator's "📣 发送给开发" button — push an
// ad-hoc note to the developer without changing the task state.
// Typical use: the creator remembers a follow-up requirement a few
// minutes after handing the task over ("also please fix the H2 on the
// article pages"). Rather than clutter the timeline through comments
// (which don't ping anyone) this goes straight to the dev's Telegram.
//
// Only available once the task has an assignee — pinging a pending
// unassigned task has no recipient. Status stays wherever it was.
func PingDevCoordTask(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var body transitionBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写要发送给开发的内容"})
		return
	}

	t, ok := assertEditable(c, id)
	if !ok {
		return
	}
	if !requireCreatorOrAdmin(c, t, "发送消息给开发") {
		return
	}
	if t.AssigneeID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "任务还没有指派开发，请先分配"})
		return
	}

	uid, uname := currentUser(c)
	// Record on the timeline so the note sticks even if Telegram is off.
	// Reuse the 'comment' event type with a distinguishing prefix so the
	// existing timeline renderer shows it without schema changes.
	notePrefixed := "📣 发送给开发：" + body.Note
	recordEvent(id, "comment", t.Status, t.Status, uid, uname, notePrefixed)
	WriteAuditLog(c, "coord_ping_dev", "coordination_task", fmt.Sprintf("%d", id), "")
	notifyCoord(t.AssigneeID, "📣 任务有新补充", t, fmt.Sprintf("%s：%s", uname, body.Note))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已发送"})
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
	// Only the creator (or admin) can polish the issue description —
	// the assignee isn't yet picked for a pending task, but we still
	// gate this explicitly in case a future state allowed edits.
	if !requireCreatorOrAdmin(c, t, "编辑问题描述") {
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
		 SET note = COALESCE(NULLIF($1, ''), note),
		     issue_title = COALESCE(NULLIF($2, ''), issue_title),
		     issue_snapshot = COALESCE(NULLIF($3, ''), issue_snapshot)
		 WHERE id = $4`,
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
		"DELETE FROM coordination_attachments WHERE task_id = $1",
		"DELETE FROM coordination_events WHERE task_id = $1",
		"DELETE FROM coordination_tasks WHERE id = $1",
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

// ListCoordScopes returns the distinct issue_scope values currently in
// use, sorted by frequency desc — populates the 影响页面 filter dropdown
// on the coord list page. Empty / null scopes are skipped.
func ListCoordScopes(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT COALESCE(issue_scope,'') AS s, COUNT(*) AS n
		FROM coordination_tasks
		WHERE COALESCE(issue_scope,'') <> ''
		GROUP BY s
		ORDER BY n DESC, s ASC
		LIMIT 50`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err == nil {
			out = append(out, gin.H{"scope": s, "count": n})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
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
	q := "SELECT status, COUNT(*) FROM coordination_tasks t"
	args := []interface{}{}
	if siteID != "" {
		q += " WHERE t.site_id = $1"
		args = append(args, siteID)
	} else {
		role, _ := c.Get("role")
		if role != "admin" {
			userID := c.GetInt64("user_id")
			q += ` WHERE (t.site_id IN (SELECT id FROM sites WHERE created_by = $1)
				OR t.created_by = $1
				OR t.assignee_id = $1
				OR COALESCE(t.pending_assignee_id,0) = $1
				OR EXISTS (SELECT 1 FROM coord_task_reviewers r WHERE r.task_id = t.id AND r.reviewer_id = $1))`
			args = append(args, userID)
		}
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
		coordStatusPendingReview:  0,
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
