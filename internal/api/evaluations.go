package api

// Report evaluations (汇报评价).
//
// Polymorphic: one evaluations table serves daily/weekly/monthly
// reports via (report_type, report_id). Admins write; report owners
// + any admin can read.
//
// We keep it a single small module because the three report variants
// all share the same evaluation shape — pulling them apart into three
// tables would triple the code without adding clarity.

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// validEvalReportType ensures the caller isn't passing arbitrary
// strings into SQL. Keep in sync with the frontend's
// report_type union and with every place we insert a polymorphic
// row.
func validEvalReportType(s string) bool {
	switch s {
	case "daily", "weekly", "monthly":
		return true
	}
	return false
}

// evaluationRow is the API shape; matches frontend expectations.
type evaluationRow struct {
	ID            int64  `json:"id"`
	ReportType    string `json:"report_type"`
	ReportID      int64  `json:"report_id"`
	EvaluatorID   int64  `json:"evaluator_id"`
	EvaluatorName string `json:"evaluator_name"`
	Content       string `json:"content"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// lookupReportOwner returns the user_id that owns a given report.
// Used to gate read access so non-admin, non-owner callers can't
// poke at other people's evaluations. Returns 0 if the report
// doesn't exist (handler treats that as 404).
func lookupReportOwner(reportType string, reportID int64) int64 {
	var table string
	switch reportType {
	case "daily":
		table = "daily_reports"
	case "weekly":
		table = "weekly_reports"
	case "monthly":
		table = "monthly_reports"
	default:
		return 0
	}
	var uid int64
	// Safe: table is from a whitelist above, not user input.
	q := "SELECT user_id FROM " + table + " WHERE id = ?"
	if err := db.DB.QueryRow(q, reportID).Scan(&uid); err == sql.ErrNoRows {
		return 0
	} else if err != nil {
		log.Printf("[eval] lookupReportOwner(%s,%d): %v", reportType, reportID, err)
		return 0
	}
	return uid
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// ListEvaluations returns every evaluation attached to a given
// (report_type, report_id). Readable by the report's owner and any
// admin — the detail page shows evaluations inline.
func ListEvaluations(c *gin.Context) {
	reportType := c.Query("report_type")
	reportIDStr := c.Query("report_id")
	if !validEvalReportType(reportType) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "report_type 非法"})
		return
	}
	var reportID int64
	fmt.Sscanf(reportIDStr, "%d", &reportID)
	if reportID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "report_id 必填"})
		return
	}

	// Access check: owner or admin only.
	ownerID := lookupReportOwner(reportType, reportID)
	if ownerID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "汇报不存在"})
		return
	}
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	if role != "admin" && ownerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权查看该汇报的评价"})
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, report_type, report_id, evaluator_id,
		       COALESCE(evaluator_name, ''), content,
		       created_at, updated_at
		FROM report_evaluations
		WHERE report_type = ? AND report_id = ?
		ORDER BY created_at ASC`,
		reportType, reportID,
	)
	if err != nil {
		log.Printf("[eval] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	items := []evaluationRow{}
	for rows.Next() {
		var r evaluationRow
		if err := rows.Scan(&r.ID, &r.ReportType, &r.ReportID, &r.EvaluatorID,
			&r.EvaluatorName, &r.Content, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		items = append(items, r)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

// CreateEvaluationRequest covers the upsert POST body.
type CreateEvaluationRequest struct {
	ReportType string `json:"report_type" binding:"required"`
	ReportID   int64  `json:"report_id" binding:"required"`
	Content    string `json:"content" binding:"required"`
}

// CreateEvaluation upserts the evaluation for (report, evaluator).
// SQLite handles the "UPDATE on conflict" via the unique index. We
// explicitly update updated_at + content on conflict so an admin who
// edits their evaluation bumps the timestamp for audit.
//
// Admin-only.
func CreateEvaluation(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员可评价"})
		return
	}

	var req CreateEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	if !validEvalReportType(req.ReportType) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "report_type 非法"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "评价内容不能为空"})
		return
	}
	if ownerID := lookupReportOwner(req.ReportType, req.ReportID); ownerID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "汇报不存在"})
		return
	}

	uid := c.GetInt64("user_id")
	// Look up evaluator display name for the stored row — we denormalise
	// so the list endpoint doesn't need to join users (which would
	// confuse the later "admin renamed" case, where we want the
	// evaluation to show the name that was in effect at write time).
	var name string
	db.DB.QueryRow(`SELECT name FROM users WHERE id = ?`, uid).Scan(&name)

	_, err := db.DB.Exec(`
		INSERT INTO report_evaluations
		  (report_type, report_id, evaluator_id, evaluator_name, content)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(report_type, report_id, evaluator_id) DO UPDATE SET
		  content = excluded.content,
		  evaluator_name = excluded.evaluator_name,
		  updated_at = CURRENT_TIMESTAMP
	`, req.ReportType, req.ReportID, uid, name, req.Content)
	if err != nil {
		log.Printf("[eval] upsert: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	WriteAuditLog(c, "eval_upsert", "report_evaluation",
		fmt.Sprintf("%s:%d", req.ReportType, req.ReportID), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已保存评价"})
}

// DeleteEvaluation removes the caller's own evaluation. Admin can only
// delete evaluations they wrote themselves — we never let one admin
// nuke another admin's comment silently.
func DeleteEvaluation(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅管理员"})
		return
	}
	id := parseInt64Param(c, "id")
	uid := c.GetInt64("user_id")

	// Confirm ownership before delete. Doing this in a single exec
	// with a WHERE id=? AND evaluator_id=? is also safe, but the
	// explicit lookup gives a clearer error message when the id is
	// valid-but-not-yours.
	var evalUID int64
	err := db.DB.QueryRow(
		`SELECT evaluator_id FROM report_evaluations WHERE id = ?`, id,
	).Scan(&evalUID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "评价不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	if evalUID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "只能删除自己发表的评价"})
		return
	}

	if _, err := db.DB.Exec(`DELETE FROM report_evaluations WHERE id = ?`, id); err != nil {
		log.Printf("[eval] delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	WriteAuditLog(c, "eval_delete", "report_evaluation", fmt.Sprintf("%d", id), "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}
