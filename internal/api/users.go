package api

import (
	"fmt"
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
}

type UpdateUserRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	TelegramID   string `json:"telegram_id"`
	DepartmentID *int64 `json:"department_id"` // pointer so "0" is distinguishable from "not sent"
	GroupID      *int64 `json:"group_id"`
}

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

func CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误: " + err.Error()})
		return
	}
	if err := ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	if req.Role == "" {
		req.Role = "member"
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

	result, err := db.DB.Exec(
		`INSERT INTO users (name, email, password, role, department_id, group_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.Name, req.Email, string(hash), req.Role, req.DepartmentID, req.GroupID,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "创建失败，邮箱可能已存在"})
		return
	}

	id, _ := result.LastInsertId()

	WriteAuditLog(c, "create_user", "user", fmt.Sprintf("%d", id), req.Email)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
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
		db.DB.QueryRow(`SELECT 1 FROM departments WHERE id = ?`, deptID).Scan(&exists)
		if exists != 1 {
			return fmt.Errorf("指定的部门不存在")
		}
	}
	if groupID > 0 {
		if deptID == 0 {
			return fmt.Errorf("选择组必须先选部门")
		}
		var gDept int64
		db.DB.QueryRow(`SELECT department_id FROM dept_groups WHERE id = ?`, groupID).Scan(&gDept)
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

	// If this call would demote the last admin, refuse. This protects the
	// system from "locking everyone out" when the sole admin edits their
	// own record and accidentally picks role=member.
	if req.Role != "" && req.Role != "admin" {
		var curRole string
		if err := db.DB.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&curRole); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
			return
		}
		if curRole == "admin" {
			var adminCount int
			db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
			if adminCount <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "不能降级最后一个管理员"})
				return
			}
		}
	}

	// If department or group is being changed, validate the pair.
	// The pointer trick on the request type lets us tell "not sent"
	// from "sent as 0" (unassigning).
	if req.DepartmentID != nil || req.GroupID != nil {
		// Load current values to fill in whichever side the caller
		// didn't send — so a partial update ("just change the group")
		// still gets validated against the existing department.
		var curDept, curGroup int64
		db.DB.QueryRow(
			"SELECT COALESCE(department_id, 0), COALESCE(group_id, 0) FROM users WHERE id = ?", id,
		).Scan(&curDept, &curGroup)
		newDept := curDept
		newGroup := curGroup
		if req.DepartmentID != nil {
			newDept = *req.DepartmentID
		}
		if req.GroupID != nil {
			newGroup = *req.GroupID
		}
		// If the caller changed department but didn't explicitly
		// specify a group, reset group to 0 — a group under the old
		// department would be invalid under the new one.
		if req.DepartmentID != nil && req.GroupID == nil && newDept != curDept {
			newGroup = 0
		}
		if err := validateDeptGroupPair(newDept, newGroup); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
			return
		}
		if _, err := db.DB.Exec(
			"UPDATE users SET department_id = ?, group_id = ? WHERE id = ?",
			newDept, newGroup, id,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新部门/组失败"})
			return
		}
	}

	_, err := db.DB.Exec(
		"UPDATE users SET name = COALESCE(NULLIF(?, ''), name), email = COALESCE(NULLIF(?, ''), email), role = COALESCE(NULLIF(?, ''), role), telegram_id = ? WHERE id = ?",
		req.Name, req.Email, req.Role, req.TelegramID, id,
	)
	if err != nil {
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

	// Prevent deleting the last admin — losing every admin wedges the
	// system until someone edits the DB by hand.
	var role string
	if err := db.DB.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&role); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "用户不存在"})
		return
	}
	if role == "admin" {
		var adminCount int
		db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "不能删除最后一个管理员"})
			return
		}
	}

	// Cascade delete in a single transaction. Without this, sites.created_by
	// and daily_reports.user_id end up pointing at a non-existent user id,
	// which (a) silently breaks canAccessSite ownership checks and (b) leaves
	// orphan rows that COUNT/JOIN queries still see.
	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	steps := []struct {
		q    string
		args []interface{}
	}{
		// Reassign site ownership to the deleting admin so sites don't orphan.
		{"UPDATE sites SET created_by = ? WHERE created_by = ?", []interface{}{currentUserID, id}},
		// Audit tasks keep history; null out the user reference.
		{"UPDATE audit_tasks SET triggered_by = NULL WHERE triggered_by = ?", []interface{}{id}},
		// Reports belong to the user — delete them rather than leaving orphans.
		{"DELETE FROM daily_reports WHERE user_id = ?", []interface{}{id}},
		// Shared backlink pool: preserve data, null-out the creator reference.
		{"UPDATE extlink_pool SET created_by = 0 WHERE created_by = ?", []interface{}{id}},
		// Finally, delete the user.
		{"DELETE FROM users WHERE id = ?", []interface{}{id}},
	}
	for _, s := range steps {
		if _, err := tx.Exec(s.q, s.args...); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	WriteAuditLog(c, "delete_user", "user", id, "")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}
