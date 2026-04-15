package api

import (
	"net/http"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	var id int64
	var name, email, password, role string
	err := db.DB.QueryRow(
		"SELECT id, name, email, password, role FROM users WHERE email = ?", req.Email,
	).Scan(&id, &name, &email, &password, &role)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "邮箱或密码错误"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "邮箱或密码错误"})
		return
	}

	token, err := middleware.GenerateToken(id, email, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "生成Token失败"})
		return
	}

	db.DB.Exec("UPDATE users SET last_login = ? WHERE id = ?", time.Now(), id)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"token": token,
			"user": gin.H{
				"id":    id,
				"name":  name,
				"email": email,
				"role":  role,
			},
		},
	})
}

func Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已注销"})
}

func ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "新密码至少8个字符"})
		return
	}

	userID := c.GetInt64("user_id")

	var currentHash string
	err := db.DB.QueryRow("SELECT password FROM users WHERE id = ?", userID).Scan(&currentHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "用户不存在"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "旧密码错误"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "密码加密失败"})
		return
	}

	_, err = db.DB.Exec("UPDATE users SET password = ? WHERE id = ?", string(newHash), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新密码失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "密码修改成功"})
}

func GetMe(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var id int64
	var name, email, role, telegramID string
	err := db.DB.QueryRow(
		"SELECT id, name, email, role, telegram_id FROM users WHERE id = ?", userID,
	).Scan(&id, &name, &email, &role, &telegramID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"id":          id,
			"name":        name,
			"email":       email,
			"role":        role,
			"telegram_id": telegramID,
		},
	})
}

func GetInt64(c *gin.Context, key string) int64 {
	val, _ := c.Get(key)
	if v, ok := val.(int64); ok {
		return v
	}
	return 0
}
