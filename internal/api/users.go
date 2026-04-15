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
	Password string `json:"password" binding:"required,min=8"`
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
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	_, err := db.DB.Exec(
		"UPDATE users SET name = COALESCE(NULLIF(?, ''), name), email = COALESCE(NULLIF(?, ''), email), role = COALESCE(NULLIF(?, ''), role), telegram_id = ? WHERE id = ?",
		req.Name, req.Email, req.Role, req.TelegramID, id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新失败"})
		return
	}

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

	_, err := db.DB.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "删除成功"})
}
