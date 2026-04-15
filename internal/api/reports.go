package api

import (
	"encoding/json"
	"net/http"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/telegram"

	"github.com/gin-gonic/gin"
)

type ReportRequest struct {
	Content   string   `json:"content" binding:"required"`
	TasksDone []string `json:"tasks_done"`
	TasksTodo []string `json:"tasks_todo"`
	Issues    string   `json:"issues"`
}

func ListReports(c *gin.Context) {
	date := c.Query("date")
	userIDFilter := c.Query("user_id")

	query := `SELECT r.id, r.user_id, u.name, r.report_date, r.content, r.tasks_done, r.tasks_todo, r.issues, r.created_at
		FROM daily_reports r JOIN users u ON r.user_id = u.id WHERE 1=1`
	args := []interface{}{}

	if date != "" {
		query += " AND r.report_date = ?"
		args = append(args, date)
	}
	if userIDFilter != "" {
		query += " AND r.user_id = ?"
		args = append(args, userIDFilter)
	}
	query += " ORDER BY r.report_date DESC, r.user_id"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var reports []gin.H
	for rows.Next() {
		var id, userID int64
		var userName, reportDate, content, tasksDone, tasksTodo, issues, createdAt string
		if err := rows.Scan(&id, &userID, &userName, &reportDate, &content, &tasksDone, &tasksTodo, &issues, &createdAt); err != nil {
			continue
		}
		reports = append(reports, gin.H{
			"id": id, "user_id": userID, "user_name": userName,
			"report_date": reportDate, "content": content,
			"tasks_done": json.RawMessage(tasksDone), "tasks_todo": json.RawMessage(tasksTodo),
			"issues": issues, "created_at": createdAt,
		})
	}
	if reports == nil {
		reports = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": reports})
}

func SubmitReport(c *gin.Context) {
	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	userID := c.GetInt64("user_id")
	reportDate := time.Now().Format("2006-01-02")

	tasksDoneJSON, _ := json.Marshal(req.TasksDone)
	tasksTodoJSON, _ := json.Marshal(req.TasksTodo)

	_, err := db.DB.Exec(
		`INSERT INTO daily_reports (user_id, report_date, content, tasks_done, tasks_todo, issues, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id, report_date) DO UPDATE SET
		 content=excluded.content, tasks_done=excluded.tasks_done, tasks_todo=excluded.tasks_todo,
		 issues=excluded.issues, updated_at=CURRENT_TIMESTAMP`,
		userID, reportDate, req.Content, string(tasksDoneJSON), string(tasksTodoJSON), req.Issues,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "提交失败"})
		return
	}

	// Get user name for notification
	var userName string
	db.DB.QueryRow("SELECT name FROM users WHERE id = ?", userID).Scan(&userName)
	telegram.SendNotification("每日汇报", userName+" 提交了 "+reportDate+" 的工作汇报")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "汇报提交成功"})
}

func GetReportByDate(c *gin.Context) {
	date := c.Param("date")
	userID := c.GetInt64("user_id")

	var id int64
	var content, tasksDone, tasksTodo, issues, createdAt string
	err := db.DB.QueryRow(
		"SELECT id, content, tasks_done, tasks_todo, issues, created_at FROM daily_reports WHERE user_id = ? AND report_date = ?",
		userID, date,
	).Scan(&id, &content, &tasksDone, &tasksTodo, &issues, &createdAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "汇报不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"id": id, "content": content,
		"tasks_done": json.RawMessage(tasksDone), "tasks_todo": json.RawMessage(tasksTodo),
		"issues": issues, "created_at": createdAt,
	}})
}

func ExportReports(c *gin.Context) {
	startDate := c.Query("start")
	endDate := c.Query("end")

	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请提供start和end日期参数"})
		return
	}

	rows, err := db.DB.Query(
		`SELECT r.report_date, u.name, r.content, r.tasks_done, r.tasks_todo, r.issues
		 FROM daily_reports r JOIN users u ON r.user_id = u.id
		 WHERE r.report_date BETWEEN ? AND ? ORDER BY r.report_date, u.name`,
		startDate, endDate,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var data []gin.H
	for rows.Next() {
		var date, name, content, tasksDone, tasksTodo, issues string
		if err := rows.Scan(&date, &name, &content, &tasksDone, &tasksTodo, &issues); err != nil {
			continue
		}
		data = append(data, gin.H{
			"date": date, "name": name, "content": content,
			"tasks_done": json.RawMessage(tasksDone), "tasks_todo": json.RawMessage(tasksTodo),
			"issues": issues,
		})
	}
	if data == nil {
		data = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
