package api

import (
	"fmt"
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type CreateUserRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	// min=12 matches ValidatePassword below; the binding tag fails fast with
	// a uniform rule so we never end up telling the user "密码至少 8 位" in
	// one place and "12 位" in another.
	Password string `json:"password" binding:"required,min=12"`
	Role     string `json:"role"`
}

type UpdateUserRequest struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	TelegramID string `json:"telegram_id"`
}

func ListUsers(c *gin.Context) {
	rows, err := db.DB.Query("SELECT id, name, email, role, telegram_id, created_at, last_login FROM users ORDER BY id")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id int64
		var name, email, role, telegramID string
		var createdAt string
		var lastLogin *string
		if err := rows.Scan(&id, &name, &email, &role, &telegramID, &createdAt, &lastLogin); err != nil {
			continue
		}
		users = append(users, gin.H{
			"id":          id,
			"name":        name,
			"email":       email,
			"role":        role,
			"telegram_id": telegramID,
			"created_at":  createdAt,
			"last_login":  lastLogin,
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

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "密码加密失败"})
		return
	}

	result, err := db.DB.Exec(
		"INSERT INTO users (name, email, password, role) VALUES (?, ?, ?, ?)",
		req.Name, req.Email, string(hash), req.Role,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "创建失败，邮箱可能已存在"})
		return
	}

	id, _ := result.LastInsertId()

	WriteAuditLog(c, "create_user", "user", fmt.Sprintf("%d", id), req.Email)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
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
