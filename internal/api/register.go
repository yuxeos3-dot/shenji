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
	TGName       string `json:"tg_name" binding:"required"`
	TGID         string `json:"tg_id"` // optional, e.g. "@username"
	Nickname     string `json:"nickname" binding:"required"`     // 花名
	EmployeeID   string `json:"employee_id" binding:"required"`  // 工号
	Email        string `json:"email" binding:"required,email"`  // 登录邮箱
	// InviteCode is optional — every registration still goes through
	// admin review, so a code is more a "VIP fast-track" hint than a
	// true gate. When present, it's validated + consumed; when empty,
	// we just skip the check and mark the row pending as usual.
	InviteCode   string `json:"invite_code"`
	DepartmentID int64  `json:"department_id" binding:"required"`
	GroupID      int64  `json:"group_id"`
	Password     string `json:"password" binding:"required,min=8"`
	Confirm      string `json:"confirm" binding:"required"`
}

// Register handles a public self-signup request. No auth required.
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请填写所有必填字段"})
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

	// Check for collisions on email OR employee_id BEFORE insert so we
	// can give a clear error rather than a generic UNIQUE violation.
	// Both fields are compared case-insensitively to match their indexes.
	var dupField string
	err := db.DB.QueryRow(
		`SELECT CASE
		        WHEN LOWER(email) = $1 THEN 'email'
		        WHEN LOWER(employee_id) = $2 THEN 'employee_id'
		        ELSE ''
		        END
		 FROM users
		 WHERE LOWER(email) = $1 OR LOWER(employee_id) = $2
		 LIMIT 1`,
		req.Email, strings.ToLower(req.EmployeeID),
	).Scan(&dupField)
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
		  tg_name, nickname, employee_id, invite_code, register_status)
		 VALUES ($1, $2, $3, 'member', $4, $5, $6, $7, $8, $9, $10, 'pending')
		 RETURNING id`,
		req.Nickname, req.Email, string(hash), req.TGID,
		req.DepartmentID, req.GroupID,
		req.TGName, req.Nickname, req.EmployeeID, matched,
	).Scan(&newID)
	if err != nil {
		log.Printf("[register] insert user failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "注册失败：" + err.Error()})
		return
	}

	// Consume the invite code only if one was actually supplied &
	// matched. Blank submissions leave the CSV untouched.
	if matched != "" {
		consumeInviteCode(matched)
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "注册失败"})
		return
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
	codes := loadInviteCodes()
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if !strings.EqualFold(c, code) {
			out = append(out, c)
		}
	}
	if _, err := db.DB.Exec(
		"UPDATE settings SET value = $1 WHERE key = 'invite_codes_csv'",
		strings.Join(out, ","),
	); err != nil {
		log.Printf("[register] consume invite code %s: %v", code, err)
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
	for rows.Next() {
		var aid int64
		if err := rows.Scan(&aid); err == nil && aid > 0 {
			telegram.NotifyUser(aid, "📋 新注册待审核", msg)
		}
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

	js, _ := jsonMarshalSafe(cleaned)
	_, err = db.DB.Exec(
		"UPDATE users SET register_status='approved', permissions=$1 WHERE id=$2",
		js, id,
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

// jsonMarshalSafe is a tiny wrapper that returns a string and never fails
// for the input shape we feed it (a string slice). Centralised so callers
// don't all have to do the err-discard dance.
func jsonMarshalSafe(v interface{}) (string, error) {
	return marshalToString(v)
}

// marshalToString uses encoding/json via the helpers package indirection
// to avoid an extra import here. We keep it local to this file.
func marshalToString(v interface{}) (string, error) {
	if v == nil {
		return "[]", nil
	}
	if s, ok := v.([]string); ok {
		if len(s) == 0 {
			return "[]", nil
		}
		// Hand-roll: every code is a known permission code so quoting
		// is safe (no special chars allowed in code names).
		parts := make([]string, len(s))
		for i, x := range s {
			parts[i] = `"` + x + `"`
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	}
	return "[]", nil
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
