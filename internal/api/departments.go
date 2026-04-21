package api

// Organization structure: departments (部门) and groups (组). Two-level
// only — a group belongs to exactly one department, a user can belong
// to at most one department + group. 0 means "unassigned" everywhere.
//
// Admin-only: the whole module is gated by AdminRequired in main.go, so
// handlers don't re-check role. We keep error messages in Chinese
// because they surface directly in the Element Plus toast UI.

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Types ──────────────────────────────────────────────────────────────────

type departmentRow struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at,omitempty"`
	GroupCount int    `json:"group_count"`
	UserCount  int    `json:"user_count"`
}

type groupRow struct {
	ID             int64  `json:"id"`
	DepartmentID   int64  `json:"department_id"`
	DepartmentName string `json:"department_name,omitempty"`
	Name           string `json:"name"`
	CreatedAt      string `json:"created_at,omitempty"`
	UserCount      int    `json:"user_count"`
}

// ─── Departments ────────────────────────────────────────────────────────────

// ListDepartments returns every department, each with the number of
// groups underneath and members attached. Used by the settings page's
// department table + the user-create dropdown.
func ListDepartments(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT d.id, d.name, d.created_at,
		       (SELECT COUNT(*) FROM dept_groups g WHERE g.department_id = d.id),
		       (SELECT COUNT(*) FROM users u WHERE u.department_id = d.id)
		FROM departments d
		ORDER BY d.name 
	`)
	if err != nil {
		log.Printf("[departments] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []departmentRow{}
	for rows.Next() {
		var d departmentRow
		if err := rows.Scan(&d.ID, &d.Name, &d.CreatedAt, &d.GroupCount, &d.UserCount); err != nil {
			continue
		}
		items = append(items, d)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

// CreateDepartmentRequest / UpdateDepartmentRequest bodies.
type departmentInput struct {
	Name string `json:"name" binding:"required"`
}

func CreateDepartment(c *gin.Context) {
	var req departmentInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "部门名称不能为空"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "部门名称不能为空"})
		return
	}
	uid := c.GetInt64("user_id")
	var id int64
	err := db.DB.QueryRow(
		`INSERT INTO departments (name, created_by) VALUES ($1, $2) RETURNING id`,
		name, uid).Scan(&id)
	if err != nil {
		// SQLite UNIQUE constraint violation → "UNIQUE constraint failed"
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(err.Error(), "23505") {
			c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "部门名称已存在"})
			return
		}
		log.Printf("[departments] create: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	WriteAuditLog(c, "dept_create", "department", itoaID(id), name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id, "name": name}})
}

func UpdateDepartment(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var req departmentInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "部门名称不能为空"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "部门名称不能为空"})
		return
	}
	res, err := db.DB.Exec(`UPDATE departments SET name = $1 WHERE id = $2`, name, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(err.Error(), "23505") {
			c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "部门名称已存在"})
			return
		}
		log.Printf("[departments] update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "部门不存在"})
		return
	}
	WriteAuditLog(c, "dept_update", "department", itoaID(id), name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已更新"})
}

// DeleteDepartment removes the department, cascades-deletes its groups,
// and re-points any users that were under it back to "unassigned" (0).
// Wrapped in a transaction so a mid-delete crash doesn't leave a
// half-orphaned state.
func DeleteDepartment(c *gin.Context) {
	id := parseInt64Param(c, "id")

	// Read current row first so the audit log has the old name.
	var name string
	err := db.DB.QueryRow(`SELECT name FROM departments WHERE id = $1`, id).Scan(&name)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "部门不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	// Detach users: both department_id and group_id go to 0. The groups
	// about to be deleted would orphan the group_id too, so wipe both
	// in the same statement.
	if _, err := tx.Exec(
		`UPDATE users SET department_id = 0, group_id = 0 WHERE department_id = $1`, id,
	); err != nil {
		log.Printf("[departments] delete detach users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解除用户绑定失败"})
		return
	}
	if _, err := tx.Exec(`DELETE FROM dept_groups WHERE department_id = $1`, id); err != nil {
		log.Printf("[departments] delete groups: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除组失败"})
		return
	}
	if _, err := tx.Exec(`DELETE FROM departments WHERE id = $1`, id); err != nil {
		log.Printf("[departments] delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	WriteAuditLog(c, "dept_delete", "department", itoaID(id), name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}

// ─── Groups ─────────────────────────────────────────────────────────────────

// ListGroups returns all groups; optional ?department_id=N to scope.
// The response includes department name for display in the user form.
func ListGroups(c *gin.Context) {
	where := ""
	args := []interface{}{}
	if dept := c.Query("department_id"); dept != "" {
		where = "WHERE g.department_id = $1"
		args = append(args, dept)
	}

	q := `
		SELECT g.id, g.department_id, COALESCE(d.name, ''), g.name, g.created_at,
		       (SELECT COUNT(*) FROM users u WHERE u.group_id = g.id)
		FROM dept_groups g
		LEFT JOIN departments d ON d.id = g.department_id
		` + where + `
		ORDER BY d.name , g.name 
	`
	rows, err := db.DB.Query(q, args...)
	if err != nil {
		log.Printf("[groups] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []groupRow{}
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.ID, &g.DepartmentID, &g.DepartmentName, &g.Name, &g.CreatedAt, &g.UserCount); err != nil {
			continue
		}
		items = append(items, g)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

type groupInput struct {
	DepartmentID int64  `json:"department_id" binding:"required"`
	Name         string `json:"name" binding:"required"`
}

func CreateGroup(c *gin.Context) {
	var req groupInput
	if err := c.ShouldBindJSON(&req); err != nil || req.DepartmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "组名称不能为空"})
		return
	}

	// Confirm the department actually exists — prevents the foreign-key-
	// in-spirit from getting dangling rows.
	var exists int
	db.DB.QueryRow(`SELECT 1 FROM departments WHERE id = $1`, req.DepartmentID).Scan(&exists)
	if exists != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "指定的部门不存在"})
		return
	}

	uid := c.GetInt64("user_id")
	var id int64
	err := db.DB.QueryRow(
		`INSERT INTO dept_groups (department_id, name, created_by) VALUES ($1, $2, $3) RETURNING id`,
		req.DepartmentID, name, uid).Scan(&id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(err.Error(), "23505") {
			c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "该部门下已有同名组"})
			return
		}
		log.Printf("[groups] create: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	WriteAuditLog(c, "group_create", "group", itoaID(id), name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id, "name": name}})
}

func UpdateGroup(c *gin.Context) {
	id := parseInt64Param(c, "id")
	var req groupInput
	if err := c.ShouldBindJSON(&req); err != nil || req.DepartmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "组名称不能为空"})
		return
	}

	res, err := db.DB.Exec(
		`UPDATE dept_groups SET department_id = $1, name = $2 WHERE id = $3`,
		req.DepartmentID, name, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(err.Error(), "23505") {
			c.JSON(http.StatusConflict, gin.H{"code": -1, "error": "该部门下已有同名组"})
			return
		}
		log.Printf("[groups] update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "组不存在"})
		return
	}
	WriteAuditLog(c, "group_update", "group", itoaID(id), name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已更新"})
}

func DeleteGroup(c *gin.Context) {
	id := parseInt64Param(c, "id")

	var name string
	err := db.DB.QueryRow(`SELECT name FROM dept_groups WHERE id = $1`, id).Scan(&name)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "组不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Detach users pointed at this group, but leave their department_id
	// intact — the user is still in the department, just without a
	// specific group.
	if _, err := tx.Exec(`UPDATE users SET group_id = 0 WHERE group_id = $1`, id); err != nil {
		log.Printf("[groups] delete detach users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解除用户绑定失败"})
		return
	}
	if _, err := tx.Exec(`DELETE FROM dept_groups WHERE id = $1`, id); err != nil {
		log.Printf("[groups] delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	WriteAuditLog(c, "group_delete", "group", itoaID(id), name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}

// itoaID wraps strconv.Itoa-ish int64 formatting into a one-liner
// without pulling strconv everywhere. We only use it for audit log
// target_id which is a string column.
func itoaID(id int64) string {
	if id == 0 {
		return ""
	}
	// Small allocation; fine for admin-only endpoints.
	const digits = "0123456789"
	if id < 0 {
		return "-" + itoaID(-id)
	}
	if id < 10 {
		return string(digits[id])
	}
	return itoaID(id/10) + string(digits[id%10])
}
