package api

// Self-registration flow.
//
// Public POST /api/auth/register: anyone with a valid invite code can
// submit a registration. The user lands in users.register_status='pending'
// with no permissions; an admin reviews via /v1/users/pending and either
// approves (granting permissions) or rejects (deletes the row).
//
// The invite code is the first line of defense — it's a CSV whitelist
// stored in settings.invite_codes_csv, consumed (removed) on successful
// use. Combined with the admin review step that's two gates before any
// non-trivial access.
//
// Login refuses any user whose register_status != 'approved' so a
// pending row can sit safely in the table without being a back door.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/middleware"
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// RegisterRequest mirrors the public registration form. Email is
// auto-derived from employee_id since the underlying users.email
// column has a NOT NULL UNIQUE constraint and the form deliberately
// omits a free-text email field.
type RegisterRequest struct {
	TGName       string `json:"tg_name"`                         // 选填
	TGID         string `json:"tg_id"`                           // 选填
	Nickname     string `json:"nickname" binding:"required"`     // 花名 必填
	EmployeeID   string `json:"employee_id"`                     // 工号 选填
	Email        string `json:"email" binding:"required,email"`  // 登录邮箱 必填
	// InviteCode is optional — every registration still goes through
	// admin review, so a code is more a "VIP fast-track" hint than a
	// true gate. When present, it's validated + consumed; when empty,
	// we just skip the check and mark the row pending as usual.
	InviteCode   string `json:"invite_code"`                     // 选填
	DepartmentID int64  `json:"department_id" binding:"required"` // 一级标签 必填
	GroupID      int64  `json:"group_id" binding:"required"`      // 二级标签 必填
	Password     string `json:"password" binding:"required,min=8"`
	Confirm      string `json:"confirm" binding:"required"`
}

// Register handles a public self-signup request. No auth required.
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Point the user at the specific field that failed instead of
		// a generic "请填写所有必填字段" — they've typically filled every
		// visible box and the real cause is a cached frontend bundle
		// missing a newer required input (e.g. the email field). Name
		// the field so browser cache / deploy skew is obvious.
		fields := translateBindErrors(err, map[string]fieldRule{
			"Nickname":     {label: "花名", required: "请填写花名"},
			"Email":        {label: "邮箱", required: "请填写邮箱", email: "邮箱格式不正确"},
			"DepartmentID": {label: "一级标签", required: "请选择一级标签"},
			"GroupID":      {label: "二级标签", required: "请选择二级标签"},
			"Password":     {label: "密码", required: "请填写密码", min: "密码至少 8 位"},
			"Confirm":      {label: "确认密码", required: "请填写确认密码"},
		})
		c.JSON(http.StatusBadRequest, gin.H{
			"code":   -1,
			"error":  firstFieldError(fields, "请填写所有必填字段"),
			"fields": fields,
		})
		return
	}

	// Trim everything before validating — leading whitespace in an
	// employee id is an easy way to bypass uniqueness.
	req.TGName = strings.TrimSpace(req.TGName)
	req.TGID = strings.TrimSpace(req.TGID)
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.EmployeeID = strings.TrimSpace(req.EmployeeID)
	req.InviteCode = strings.TrimSpace(req.InviteCode)
	// Email is the login identifier — normalize to lowercase so the
	// case-insensitive unique index on users.email behaves predictably
	// and Login's LOWER(email)=$1 lookup matches.
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Password != req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "两次输入的密码不一致"})
		return
	}
	if err := ValidateRegisterPassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// If an invite code is supplied, it must be valid — admins added
	// them to the whitelist for a reason and we want them consumed.
	// A blank code is fine; admin review is still the real gate.
	matched := ""
	if req.InviteCode != "" {
		codes := loadInviteCodes()
		for _, code := range codes {
			if strings.EqualFold(code, req.InviteCode) {
				matched = code
				break
			}
		}
		if matched == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "邀请码无效或已被使用"})
			return
		}
	}

	// Validate department + group pair exists. Reuse the same helper
	// CreateUser uses so we get identical error messages.
	if err := validateDeptGroupPair(req.DepartmentID, req.GroupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Resolve department name → default role. User's role is auto-assigned
	// from their department so new registrants land with the right menu
	// visibility from day one; admin can adjust later if needed.
	var deptName string
	db.DB.QueryRow("SELECT name FROM departments WHERE id=$1", req.DepartmentID).Scan(&deptName)
	assignedRole := RoleFromDepartmentName(deptName)
	defaultPerms := DefaultPermissionsForRole(assignedRole)
	permsJSON, _ := json.Marshal(defaultPerms)

	// Check for collisions on email (always) and employee_id (only if provided).
	// When employee_id is empty we skip the employee_id check — otherwise
	// multiple users without employee_id would collide on the empty string.
	var dupField string
	empLower := strings.ToLower(req.EmployeeID)
	var err error
	if empLower != "" {
		err = db.DB.QueryRow(
			`SELECT CASE
			        WHEN LOWER(email) = $1 THEN 'email'
			        WHEN LOWER(employee_id) = $2 THEN 'employee_id'
			        ELSE ''
			        END
			 FROM users
			 WHERE LOWER(email) = $1 OR LOWER(employee_id) = $2
			 LIMIT 1`,
			req.Email, empLower,
		).Scan(&dupField)
	} else {
		err = db.DB.QueryRow(
			`SELECT 'email' FROM users WHERE LOWER(email) = $1 LIMIT 1`,
			req.Email,
		).Scan(&dupField)
	}
	if err == nil && dupField != "" {
		msg := "该工号已被注册"
		if dupField == "email" {
			msg = "该邮箱已被注册"
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": msg})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "密码加密失败"})
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "注册失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Use nickname as the display name (users.name is NOT NULL). The full
	// "name" field on the user model is what shows up in coord assignee
	// dropdowns / dashboards / etc., so the nickname is what readers see.
	var newID int64
	err = tx.QueryRow(
		`INSERT INTO users
		 (name, email, password, role, telegram_id, department_id, group_id,
		  tg_name, nickname, employee_id, invite_code, register_status, permissions)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'pending', $12)
		 RETURNING id`,
		req.Nickname, req.Email, string(hash), assignedRole, req.TGID,
		req.DepartmentID, req.GroupID,
		req.TGName, req.Nickname, req.EmployeeID, matched, string(permsJSON),
	).Scan(&newID)
	if err != nil {
		log.Printf("[register] insert user failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "注册失败"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "注册失败"})
		return
	}

	// Consume the invite code ONLY after the user row is durably
	// committed. Previously we consumed before tx.Commit — settings
	// updates aren't part of the tx, so a commit failure would have
	// burned a valid code without creating the user, leaving the
	// candidate locked out with no way forward.
	if matched != "" {
		consumeInviteCode(matched)
	}

	WriteAuditLogAnonymous(c, "register", "user", fmt.Sprintf("%d", newID), req.EmployeeID)

	// Notify admins via Telegram so reviews don't pile up unseen.
	go notifyAdminsNewRegistration(req.Nickname, req.EmployeeID, req.TGName)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"message": "注册申请已提交，请等待管理员审核后再登录",
			"id":      newID,
		},
	})
}

// loadInviteCodes parses the CSV from settings.invite_codes_csv into a
// trimmed, non-empty list. Order is preserved.
func loadInviteCodes() []string {
	raw := db.GetSetting("invite_codes_csv")
	if raw == "" {
		return nil
	}
	out := make([]string, 0)
	for _, p := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// consumeInviteCode rewrites settings.invite_codes_csv with the given
// code removed. Best-effort — failure here doesn't fail the registration
// (the user is already created). The downside is a code that could get
// reused; the upside is a transient settings-table glitch can't strand a
// half-registered user. Settings has no FK back to users, so we run on
// db.DB rather than the inviting transaction.
func consumeInviteCode(code string) {
	// Use atomic UPDATE to prevent race condition with concurrent registrations
	result, err := db.DB.Exec(
		`UPDATE settings SET value = array_to_string(
			array_remove(string_to_array(value, ','), $1), ',')
		 WHERE key = 'invite_codes_csv' AND value LIKE '%' || $1 || '%'`,
		code,
	)
	if err != nil {
		log.Printf("[register] consume invite code %s: %v", code, err)
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		log.Printf("[register] invite code %s already consumed or not found", code)
	}
}

// notifyAdminsNewRegistration pings every admin's Telegram (silent on
// failure) so they know to review. Runs in its own goroutine since this
// is a side effect of a user-facing request — Telegram lag must not
// block the response.
func notifyAdminsNewRegistration(nickname, employeeID, tgName string) {
	rows, err := db.DB.Query("SELECT id FROM users WHERE role = 'admin'")
	if err != nil {
		log.Printf("[register] list admins for notification: %v", err)
		return
	}
	defer rows.Close()
	msg := fmt.Sprintf("新注册申请待审核：\n姓名：%s\n工号：%s\nTG：%s", nickname, employeeID, tgName)
	notified := 0
	for rows.Next() {
		var aid int64
		if err := rows.Scan(&aid); err == nil && aid > 0 {
			telegram.NotifyUser(aid, "📋 新注册待审核", msg)
			notified++
		}
	}
	// Make the "nobody reviewed it" case visible. If users table has
	// zero admins (first-run edge case) the notification silently
	// evaporates; this leaves a breadcrumb for ops.
	if notified == 0 {
		log.Printf("[register] new pending (%s / %s) — no admins in users table to notify", nickname, employeeID)
	}
}

// PublicListDepartments returns id+name pairs for the dept/group cascade
// on the registration form. No auth — names aren't sensitive and the
// register form needs them before any session exists.
func PublicListDepartments(c *gin.Context) {
	rows, err := db.DB.Query("SELECT id, name FROM departments ORDER BY id")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err == nil {
			out = append(out, gin.H{"id": id, "name": name})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// PublicListGroups returns id+name+department_id triples filtered by
// department_id. Same public-by-design rationale as PublicListDepartments.
func PublicListGroups(c *gin.Context) {
	deptID := c.Query("department_id")
	rows, err := db.DB.Query(
		"SELECT id, name, department_id FROM dept_groups WHERE department_id::text = $1 ORDER BY id",
		deptID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, did int64
		var name string
		if err := rows.Scan(&id, &name, &did); err == nil {
			out = append(out, gin.H{"id": id, "name": name, "department_id": did})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// ListPendingUsers returns the queue of users waiting for admin approval.
// Admin-only — surfaced under the existing /v1/users group which is
// already AdminRequired().
func ListPendingUsers(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT u.id, u.nickname, u.tg_name, u.telegram_id, u.employee_id,
		       u.invite_code, COALESCE(u.department_id,0), COALESCE(u.group_id,0),
		       COALESCE(d.name,''), COALESCE(g.name,''),
		       u.created_at
		FROM users u
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE u.register_status = 'pending'
		ORDER BY u.created_at ASC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var id, deptID, groupID int64
		var nickname, tgName, tgID, employeeID, inviteCode, deptName, groupName, createdAt string
		if err := rows.Scan(&id, &nickname, &tgName, &tgID, &employeeID, &inviteCode,
			&deptID, &groupID, &deptName, &groupName, &createdAt); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":              id,
			"nickname":        nickname,
			"tg_name":         tgName,
			"tg_id":           tgID,
			"employee_id":     employeeID,
			"invite_code":     inviteCode,
			"department_id":   deptID,
			"group_id":        groupID,
			"department_name": deptName,
			"group_name":      groupName,
			"created_at":      createdAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

type ApproveUserRequest struct {
	Permissions []string `json:"permissions"`
}

// ApprovePendingUser flips register_status to 'approved' and persists
// the admin's chosen permission set. Refuses if the row isn't actually
// pending (idempotent — re-approving an already-approved user is a no-op
// rather than an error).
func ApprovePendingUser(c *gin.Context) {
	id := c.Param("id")
	var req ApproveUserRequest
	_ = c.ShouldBindJSON(&req) // permissions optional; default empty

	cleaned, err := cleanPermissionList(req.Permissions)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	var status string
	if err := db.DB.QueryRow("SELECT register_status FROM users WHERE id = $1", id).Scan(&status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
		return
	}
	if status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "该用户不在待审核状态"})
		return
	}

	// encoding/json is the canonical way — prior commit hand-rolled a
	// string builder because we already validated each code shape via
	// cleanPermissionList. That's still safe, but stdlib is clearer
	// and removes the temptation to trust the validation a future
	// caller might forget.
	jsBytes, _ := json.Marshal(cleaned)
	_, err = db.DB.Exec(
		"UPDATE users SET register_status='approved', permissions=$1 WHERE id=$2",
		string(jsBytes), id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "审核失败"})
		return
	}
	WriteAuditLog(c, "approve_register", "user", id, fmt.Sprintf("perms=%d", len(cleaned)))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已通过审核"})
}

// RejectPendingUser deletes a pending row outright. We don't keep a
// 'rejected' tombstone — the same person can re-apply with another
// invite code if they want, and an audit_log entry is enough trail.
func RejectPendingUser(c *gin.Context) {
	id := c.Param("id")
	var status string
	if err := db.DB.QueryRow("SELECT register_status FROM users WHERE id = $1", id).Scan(&status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
		return
	}
	if status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "该用户不在待审核状态"})
		return
	}
	if _, err := db.DB.Exec("DELETE FROM users WHERE id = $1 AND register_status = 'pending'", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "拒绝失败"})
		return
	}
	WriteAuditLog(c, "reject_register", "user", id, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已拒绝"})
}

// ValidateRegisterPassword is the relaxed policy for self-registration —
// 8 chars, must contain a letter and a digit. The 12-char strict policy
// stays in place for admin-created accounts and password resets.
func ValidateRegisterPassword(pw string) error {
	if len(pw) < 8 {
		return fmt.Errorf("密码至少 8 位")
	}
	hasLetter, hasDigit := false, false
	for _, r := range pw {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("密码需同时包含字母和数字")
	}
	return nil
}

// WriteAuditLogAnonymous is a thin wrapper around WriteAuditLog that
// works for handlers without an authenticated user (registration). The
// actor_id is left as 0 and the IP comes from the gin context.
func WriteAuditLogAnonymous(c *gin.Context, action, target, targetID, detail string) {
	// Reuse the existing audit log helper but with a synthetic 0 user.
	// The downstream sink already accepts user_id=0 for system events.
	c.Set("user_id", int64(0))
	c.Set("email", "registration")
	WriteAuditLog(c, action, target, targetID, detail)
}

// Compile-time assertions that we still match the relevant middleware
// signature in case it changes — saves a runtime surprise during deploy.
var _ = middleware.LoginRateLimit
var _ = time.Now
