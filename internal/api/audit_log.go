package api

import (
	"fmt"
	"net/http"
	"strconv"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// WriteAuditLog records a sensitive operation for compliance / forensics.
func WriteAuditLog(c *gin.Context, action, targetType, targetID, detail string) {
	if len(detail) > 2048 {
		detail = detail[:2048]
	}
	userID := c.GetInt64("user_id")
	email, _ := c.Get("email")
	emailStr, _ := email.(string)
	ip := c.ClientIP()
	if _, err := db.DB.Exec(
		"INSERT INTO audit_logs (user_id, user_email, action, target_type, target_id, detail, ip) VALUES ($1,$2,$3,$4,$5,$6,$7)",
		userID, emailStr, action, targetType, targetID, detail, ip,
	); err != nil {
		log.Printf("[audit_log] failed to write audit log (action=%s, target=%s/%s): %v", action, targetType, targetID, err)
	}
}

// ListAuditLogs returns paginated, filterable audit log entries (admin only).
// Query params: page (default 1), per_page (default 50, max 200),
// start_date, end_date (YYYY-MM-DD), user_id, action.
func ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	offset := (page - 1) * perPage

	// Build WHERE clause from optional filters.
	where := "1=1"
	args := []interface{}{}

	if uid := c.Query("user_id"); uid != "" {
		where += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, uid)
	}
	if act := c.Query("action"); act != "" {
		where += fmt.Sprintf(" AND action = $%d", len(args)+1)
		args = append(args, act)
	}
	if sd := c.Query("start_date"); sd != "" {
		where += fmt.Sprintf(" AND created_at >= $%d", len(args)+1)
		args = append(args, sd)
	}
	if ed := c.Query("end_date"); ed != "" {
		where += fmt.Sprintf(" AND created_at <= $%d", len(args)+1)
		args = append(args, ed+" 23:59:59")
	}

	// Total count for pagination.
	var total int64
	countSQL := "SELECT COUNT(*) FROM audit_logs WHERE " + where
	if err := db.DB.QueryRow(countSQL, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}

	// Fetch page.
	querySQL := "SELECT id, user_id, user_email, action, target_type, target_id, detail, ip, created_at FROM audit_logs WHERE " +
		where + fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	queryArgs := append(args, perPage, offset)
	rows, err := db.DB.Query(querySQL, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var logs []gin.H
	for rows.Next() {
		var id, userID int64
		var email, action, targetType, targetID, detail, ip, createdAt string
		if err := rows.Scan(&id, &userID, &email, &action, &targetType, &targetID, &detail, &ip, &createdAt); err != nil {
			continue
		}
		logs = append(logs, gin.H{
			"id": id, "user_id": userID, "user_email": email,
			"action": action, "target_type": targetType, "target_id": targetID,
			"detail": detail, "ip": ip, "created_at": createdAt,
		})
	}
	if logs == nil {
		logs = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":     0,
		"data":     logs,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
