package api

import (
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// WriteAuditLog records a sensitive operation for compliance / forensics.
func WriteAuditLog(c *gin.Context, action, targetType, targetID, detail string) {
	userID := c.GetInt64("user_id")
	email, _ := c.Get("email")
	emailStr, _ := email.(string)
	ip := c.ClientIP()
	db.DB.Exec(
		"INSERT INTO audit_logs (user_id, user_email, action, target_type, target_id, detail, ip) VALUES (?,?,?,?,?,?,?)",
		userID, emailStr, action, targetType, targetID, detail, ip,
	)
}

// ListAuditLogs returns the most recent 200 audit log entries (admin only).
func ListAuditLogs(c *gin.Context) {
	rows, err := db.DB.Query(
		`SELECT id, user_id, user_email, action, target_type, target_id, detail, ip, created_at
		 FROM audit_logs ORDER BY id DESC LIMIT 200`)
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
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": logs})
}
