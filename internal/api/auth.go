package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
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
	// min=12 matches ValidatePassword — keep these in sync or the user
	// sees two different "密码至少 N 位" messages back-to-back.
	NewPassword string `json:"new_password" binding:"required,min=12"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	// #37: Normalize email to lowercase before all DB operations.
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Account-level lockout check — prevents brute force from rotating IPs.
	if locked, retry := IsAccountLocked(req.Email); locked {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"code":  -1,
			"error": "账户因多次失败已临时锁定，请 " + retry.Round(time.Second).String() + " 后重试",
		})
		return
	}

	var id int64
	var name, email, password, role, registerStatus string
	// Allow login by email OR employee_id (case-insensitive). Self-
	// registered users get a synthesized email like hf001@register.local
	// but typically log in with their employee id ("HF001").
	err := db.DB.QueryRow(
		`SELECT id, name, email, password, role, COALESCE(register_status,'approved')
		 FROM users
		 WHERE LOWER(email) = $1 OR LOWER(employee_id) = $1`, req.Email,
	).Scan(&id, &name, &email, &password, &role, &registerStatus)
	if err != nil {
		IncLoginFailure()
		if RecordLoginFailure(req.Email) {
			IncLoginLockout()
		}
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "邮箱或密码错误"})
		return
	}

	// Self-registered users sit pending until an admin approves —
	// reject login during that window with a clear message rather than
	// the generic "邮箱或密码错误" they'd retry forever against.
	if registerStatus != "approved" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":  -1,
			"error": "您的注册申请正在等待管理员审核，审核通过后即可登录",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(req.Password)); err != nil {
		IncLoginFailure()
		if RecordLoginFailure(req.Email) {
			IncLoginLockout()
		}
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": "邮箱或密码错误"})
		return
	}
	RecordLoginSuccess(req.Email)

	token, err := middleware.GenerateToken(id, email, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "生成Token失败"})
		return
	}

	if _, err := db.DB.Exec("UPDATE users SET last_login = $1 WHERE id = $2", time.Now(), id); err != nil {
		// Non-critical: don't fail the login, but log so we notice DB issues.
		log.Printf("[auth] update last_login user=%d failed: %v", id, err)
	}

	// Set the JWT as an httpOnly cookie so an XSS can't exfiltrate it. The
	// cookie is Secure when the request was TLS (either directly or via an
	// X-Forwarded-Proto header from a trusted proxy).
	secure := isHTTPS(c)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(middleware.AuthCookieName, token, 24*60*60, "/", "", secure, true /* httpOnly */)
	// Pair the session with a fresh CSRF token (readable by JS for the
	// double-submit header).
	csrf := middleware.IssueCSRFToken(c, secure)

	// Warm the platform data cache asynchronously so the user lands on the
	// data management page with everything already in memory. Uses TryLock
	// internally, so simultaneous logins collapse into a single reload.
	go WarmDataCache()

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"csrf_token": csrf,
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
	// Invalidate all existing tokens for this user by setting
	// token_invalidated_after to the current unix timestamp.
	userID := c.GetInt64("user_id")
	if userID > 0 {
		now := time.Now().Unix()
		db.DB.Exec("UPDATE users SET token_invalidated_after = $1 WHERE id = $2", now, userID)
		middleware.InvalidateTokenCache(userID)
	}

	// Expire both cookies. Empty value + MaxAge<0 tells the browser to drop them.
	secure := isHTTPS(c)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(middleware.AuthCookieName, "", -1, "/", "", secure, true)
	c.SetCookie(middleware.CSRFCookieName, "", -1, "/", "", secure, false)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已注销"})
}

func ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "新密码至少 12 位，且需含大小写字母和数字"})
		return
	}
	if err := ValidatePassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	userID := c.GetInt64("user_id")

	var currentHash string
	err := db.DB.QueryRow("SELECT password FROM users WHERE id = $1", userID).Scan(&currentHash)
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

	_, err = db.DB.Exec("UPDATE users SET password = $1 WHERE id = $2", string(newHash), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "更新密码失败"})
		return
	}

	// Invalidate all existing tokens so other sessions are forced to re-login.
	now := time.Now().Unix()
	db.DB.Exec("UPDATE users SET token_invalidated_after = $1 WHERE id = $2", now, userID)
	middleware.InvalidateTokenCache(userID)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "密码修改成功"})
}

func GetMe(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var id int64
	var name, email, role, telegramID string
	err := db.DB.QueryRow(
		"SELECT id, name, email, role, telegram_id FROM users WHERE id = $1", userID,
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

// GetOnlineUsers returns all users active in the last 5 minutes.
// Same user on different IPs shows as separate entries.
func GetOnlineUsersAPI(c *gin.Context) {
	entries := middleware.GetOnlineUsers()
	// Group by user, collect IPs
	type userInfo struct {
		UserID int64    `json:"user_id"`
		Email  string   `json:"email"`
		Role   string   `json:"role"`
		IPs    []string `json:"ips"`
		Name   string   `json:"name"`
	}
	userMap := make(map[int64]*userInfo)
	for _, e := range entries {
		if u, ok := userMap[e.UserID]; ok {
			found := false
			for _, ip := range u.IPs {
				if ip == e.IP {
					found = true
					break
				}
			}
			if !found {
				u.IPs = append(u.IPs, e.IP)
			}
		} else {
			userMap[e.UserID] = &userInfo{UserID: e.UserID, Email: e.Email, Role: e.Role, IPs: []string{e.IP}}
		}
	}
	// Batch-fetch names from DB to avoid N+1 queries
	userIDs := make([]interface{}, 0, len(userMap))
	placeholders := make([]string, 0, len(userMap))
	for id := range userMap {
		userIDs = append(userIDs, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(placeholders)+1))
	}
	nameMap := make(map[int64]string)
	if len(userIDs) > 0 {
		rows, err := db.DB.Query(
			"SELECT id, name FROM users WHERE id IN ("+strings.Join(placeholders, ",")+`)`,
			userIDs...,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var uid int64
				var n string
				if rows.Scan(&uid, &n) == nil {
					nameMap[uid] = n
				}
			}
		}
	}

	var list []gin.H
	for _, u := range userMap {
		name := nameMap[u.UserID]
		if name == "" {
			name = u.Email
		}
		list = append(list, gin.H{
			"user_id":       u.UserID,
			"name":          name,
			"email":         u.Email,
			"role":          u.Role,
			"ips":           u.IPs,
			"session_count": len(u.IPs),
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"online_count": len(entries), // total sessions (user+IP combos)
		"user_count":   len(userMap), // unique users
		"users":        list,
	}})
}

