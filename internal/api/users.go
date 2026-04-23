package api

import (
	"encoding/json"
	"strings"
	"fmt"
	"log"
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type CreateUserRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required,email"`
	// min=12 matches ValidatePassword below; the binding tag fails fast with
	// a uniform rule so we never end up telling the user "密码至少 8 位" in
	// one place and "12 位" in another.
	Password     string `json:"password" binding:"required,min=12"`
	Role         string `json:"role"`
	DepartmentID int64  `json:"department_id"`
	GroupID      int64  `json:"group_id"`
	// Optional: initial permissions for member users. If the caller passed
	// this along with the create request, we'll persist them right after
	// the INSERT so the admin doesn't have to open a second dialog.
	// Ignored for admins (they implicitly have all perms).
	Permissions []string `json:"permissions"`
}

type UpdateUserRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	TelegramID   string `json:"telegram_id"`
	DepartmentID *int64 `json:"department_id"` // pointer so "0" is distinguishable from "not sent"
	GroupID      *int64 `json:"group_id"`
}

// ListUsers returns user data for the admin panel. SELECT explicitly names
// columns to avoid leaking password, frw_apikey, frw_config, or permissions.
func ListUsers(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT u.id, u.name, u.email, u.role, u.telegram_id, u.created_at, u.last_login,
		       COALESCE(u.department_id, 0), COALESCE(u.group_id, 0),
		       COALESCE(d.name, ''), COALESCE(g.name, '')
		FROM users u
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		ORDER BY u.id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, deptID, groupID int64
		var name, email, role, telegramID, deptName, groupName string
		var createdAt string
		var lastLogin *string
		if err := rows.Scan(&id, &name, &email, &role, &telegramID, &createdAt, &lastLogin,
			&deptID, &groupID, &deptName, &groupName); err != nil {
			continue
		}
		users = append(users, gin.H{
			"id":              id,
			"name":            name,
			"email":           email,
			"role":            role,
			"telegram_id":     telegramID,
			"created_at":      createdAt,
			"last_login":      lastLogin,
			"department_id":   deptID,
			"group_id":        groupID,
			"department_name": deptName,
			"group_name":      groupName,
		})
	}

	if users == nil {
		users = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": users})
}

// ListTeamMembers returns a minimal "company directory" — just enough
// for non-admin members to populate assignee / reviewer pickers in the
// coordination workflow without exposing email, role, telegram_id, or
// last_login (which would be a lateral information disclosure across
// teammates). Active members only (excludes pending self-registrations).
//
// Anyone authenticated can call it; the data is the same names you'd see
// on a coord task assignee/creator field anyway, plus department/group
// for picker labels.
func ListTeamMembers(c *gin.Context) {
	permFilter := strings.TrimSpace(c.Query("permission"))

	baseQuery := `
		SELECT u.id, u.name,
		       COALESCE(d.name, ''), COALESCE(g.name, '')
		FROM users u
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN dept_groups g ON g.id = u.group_id
		WHERE COALESCE(u.register_status, 'approved') = 'approved'`

	args := []interface{}{}
	if permFilter != "" {
		// Admins implicitly have all permissions; members need explicit code in JSON array.
		// users.permissions is stored as JSON like ["coord.view","coord.submit"] —
		// LIKE '%"<code>"%' matches the quoted code inside the array string.
		baseQuery += ` AND (u.role = 'admin' OR COALESCE(u.permissions,'') LIKE $1)`
		args = append(args, "%\""+permFilter+"\"%")
	}
	baseQuery += " ORDER BY u.name"

	rows, err := db.DB.Query(baseQuery, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var id int64
		var name, deptName, groupName string
		if err := rows.Scan(&id, &name, &deptName, &groupName); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":              id,
			"name":            name,
			"department_name": deptName,
			"group_name":      groupName,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

func CreateUser(c *gin.Context) {
	var req CreateUserRequest
	// Use manual binding so we can emit per-field errors. Default Gin
	// bindJSON returns a validator.ValidationErrors whose .Error() is
	// the raw jargon you saw in the screenshot ("Field validation for
	// 'Email' failed on the 'email' tag") — unacceptable as user copy.
	if err := c.ShouldBindJSON(&req); err != nil {
		fields := translateBindErrors(err, map[string]fieldRule{
			"Name":     {label: "姓名", required: "请填写姓名"},
			"Email":    {label: "邮箱", required: "请填写邮箱", email: "邮箱格式不正确"},
			"Password": {label: "密码", required: "请填写密码", min: "密码至少 12 位"},
		})
		c.JSON(http.StatusBadRequest, gin.H{
			"code":   -1,
			"error":  firstFieldError(fields, "参数错误"),
			"fields": fields,
		})
		return
	}
	if err := ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":   -1,
			"error":  err.Error(),
			"fields": gin.H{"password": err.Error()},
		})
		return
	}

	// #37: Normalize email to lowercase before all DB operations.
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Role == "" {
		req.Role = "user"
	}
	if !isValidUserRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "角色无效"})
		return
	}

	// Validate the dept/group pair. group_id > 0 requires that the group
	// actually belongs to the given department_id — otherwise we'd end
	// up with misattributed reports later.
	if err := validateDeptGroupPair(req.DepartmentID, req.GroupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "密码加密失败"})
		return
	}

	// Wrap INSERT + permission UPDATE in a single transaction so the
	// user row is never visible without its intended permissions.
	tx, txErr := db.DB.Begin()
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRow(
		`INSERT INTO users (name, email, password, role, department_id, group_id)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		req.Name, req.Email, string(hash), req.Role, req.DepartmentID, req.GroupID,
	).Scan(&id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "创建失败，邮箱可能已存在"})
		return
	}

	// Initial permissions - skip for admins (they have all perms implicitly).
	// If the caller passed an explicit list, validate & save that. Otherwise
	// seed the role's default permission set so new users land with the
	// right menu visibility without a second round-trip.
	if req.Role != "admin" {
		if len(req.Permissions) > 0 {
			cleaned, permErr := cleanPermissionList(req.Permissions)
			if permErr == nil {
				js, _ := json.Marshal(cleaned)
				tx.Exec("UPDATE users SET permissions=$1 WHERE id=$2", string(js), id)
			} else {
				log.Printf("[create_user] id=%d perms rejected: %v", id, permErr)
			}
		} else if defaults := DefaultPermissionsForRole(req.Role); len(defaults) > 0 {
			js, _ := json.Marshal(defaults)
			tx.Exec("UPDATE users SET permissions=$1 WHERE id=$2", string(js), id)
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}

	WriteAuditLog(c, "create_user", "user", fmt.Sprintf("%d", id), req.Email)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// isValidUserRole reports whether the given role string is a legal value
// for users.role. Empty means "no change" on update and is accepted;
// CreateUser substitutes its default before calling anything that would
// care. Any other value is rejected so we never store something like
// "admin2" that bypasses the admin-count guard rails.
//
// Under the v2 role system, "member" is no longer created for new users
// (self-registration now picks a role based on department), but it's
// still accepted here for backwards-compatibility with any tooling that
// hasn't migrated yet.
func isValidUserRole(role string) bool {
	if role == "" || role == "member" {
		return true
	}
	return IsValidRole(role)
}

// validateDeptGroupPair ensures the provided (department, group) pair is
// consistent. Both 0 = unassigned is fine; group without department is
// not; group under the wrong department is not.
func validateDeptGroupPair(deptID, groupID int64) error {
	if deptID == 0 && groupID == 0 {
		return nil
	}
	if deptID > 0 {
		var exists int
		db.DB.QueryRow(`SELECT 1 FROM departments WHERE id = $1`, deptID).Scan(&exists)
		if exists != 1 {
			return fmt.Errorf("指定的部门不存在")
		}
	}
	if groupID > 0 {
		if deptID == 0 {
			return fmt.Errorf("选择组必须先选部门")
		}
		var gDept int64
		db.DB.QueryRow(`SELECT department_id FROM dept_groups WHERE id = $1`, groupID).Scan(&gDept)
		if gDept == 0 {
			return fmt.Errorf("指定的组不存在")
		}
		if gDept != deptID {
			return fmt.Errorf("所选组不属于该部门")
		}
	}
	return nil
}

func UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	// Validate the requested role. Empty = no change. See isValidUserRole
	// for the accepted set (admin, user, group_leader, dev, operations,
	// product, plus legacy "member").
	if !isValidUserRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "角色无效"})
		return
	}

	// #37: Normalize email on update too — otherwise a later Login would
	// miss the user, since Login lowercases before lookup.
	if req.Email != "" {
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	}

	// Use a transaction with SELECT FOR UPDATE to prevent TOCTOU race
	// when checking admin count then updating the role.
	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the user row to prevent concurrent role changes.
	var curRole string
	var curDept, curGroup int64
	if err := tx.QueryRow(
		"SELECT role, COALESCE(department_id, 0), COALESCE(group_id, 0) FROM users WHERE id = $1 FOR UPDATE", id,
	).Scan(&curRole, &curDept, &curGroup); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
		return
	}

	// If this call would demote the last admin, refuse.
	if req.Role != "" && req.Role != "admin" && curRole == "admin" {
		var adminCount int
		tx.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "不能降级最后一个管理员"})
			return
		}
	}

	// If department or group is being changed, validate the pair.
	if req.DepartmentID != nil || req.GroupID != nil {
		newDept := curDept
		newGroup := curGroup
		if req.DepartmentID != nil {
			newDept = *req.DepartmentID
		}
		if req.GroupID != nil {
			newGroup = *req.GroupID
		}
		if req.DepartmentID != nil && req.GroupID == nil && newDept != curDept {
			newGroup = 0
		}
		if err := validateDeptGroupPair(newDept, newGroup); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
			return
		}
		if _, err := tx.Exec(
			"UPDATE users SET department_id = $1, group_id = $2 WHERE id = $3",
			newDept, newGroup, id,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新部门/组失败"})
			return
		}
	}

	_, err = tx.Exec(
		"UPDATE users SET name = COALESCE(NULLIF($1, ''), name), email = COALESCE(NULLIF($2, ''), email), role = COALESCE(NULLIF($3, ''), role), telegram_id = $4 WHERE id = $5",
		req.Name, req.Email, req.Role, req.TelegramID, id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}

	// If role was changed, auto-reset permissions to the new role's defaults.
	// Admin bypasses permission checks so we clear to empty array for cleanliness.
	// This is the "role binds permissions" model: user → role → perms.
	if req.Role != "" && req.Role != curRole {
		var newPerms []string
		if req.Role == "admin" {
			newPerms = []string{}
		} else {
			newPerms = DefaultPermissionsForRole(req.Role)
		}
		permsJSON, _ := json.Marshal(newPerms)
		if _, err := tx.Exec("UPDATE users SET permissions = $1 WHERE id = $2", string(permsJSON), id); err != nil {
			log.Printf("[users] reset perms for user %s on role change: %v", id, err)
			// Non-fatal — the role change already went through; just log
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}

	WriteAuditLog(c, "update_user", "user", id, req.Email)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "更新成功"})
}

func DeleteUser(c *gin.Context) {
	id := c.Param("id")

	// Prevent deleting self
	currentUserID := c.GetInt64("user_id")
	if id == fmt.Sprintf("%d", currentUserID) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "不能删除自己"})
		return
	}

	// Cascade delete in a single transaction with the admin-count check
	// inside so there is no TOCTOU gap between checking and deleting.
	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the user row and check admin count atomically.
	var role string
	if err := tx.QueryRow("SELECT role FROM users WHERE id = $1 FOR UPDATE", id).Scan(&role); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
		return
	}
	if role == "admin" {
		var adminCount int
		tx.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "不能删除最后一个管理员"})
			return
		}
	}

		steps := []struct {
		q    string
		args []interface{}
	}{
		// Reassign site ownership to the deleting admin so sites don't orphan.
		{"UPDATE sites SET created_by = $1 WHERE created_by = $2", []interface{}{currentUserID, id}},
		// Audit tasks keep history; null out the user reference.
		{"UPDATE audit_tasks SET triggered_by = NULL WHERE triggered_by = $1", []interface{}{id}},
		// Reports belong to the user — delete them rather than leaving orphans.
		{"DELETE FROM daily_reports WHERE user_id = $1", []interface{}{id}},
		// Shared backlink pool: preserve data, null-out the creator reference.
		{"UPDATE extlink_pool SET created_by = 0 WHERE created_by = $1", []interface{}{id}},
		// #51: Cascade-clean per-user platform config / credentials tables.
		{"DELETE FROM user_oauth_config WHERE user_id = $1", []interface{}{id}},
		{"DELETE FROM user_oauth_tokens WHERE user_id = $1", []interface{}{id}},
		{"DELETE FROM user_telegram WHERE user_id = $1", []interface{}{id}},
		{"DELETE FROM user_bing WHERE user_id = $1", []interface{}{id}},
		// Finally, delete the user.
		{"DELETE FROM users WHERE id = $1", []interface{}{id}},
	}
	for _, s := range steps {
		if _, err := tx.Exec(s.q, s.args...); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	WriteAuditLog(c, "delete_user", "user", id, "")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}
