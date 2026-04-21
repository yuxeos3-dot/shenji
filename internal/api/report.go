package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/report"

	"github.com/gin-gonic/gin"
)

// DownloadAuditReport generates a docx report and returns it for download.
// The report is rendered entirely in Go (no Python / python-docx dependency).
func DownloadAuditReport(c *gin.Context) {
	taskID := c.Param("task_id")

	var siteID int64
	var status string
	if err := db.DB.QueryRow("SELECT site_id, status FROM audit_tasks WHERE id = $1", taskID).
		Scan(&siteID, &status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "审计任务不存在"})
		return
	}
	if status != "done" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "审计任务尚未完成"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", siteID)) {
		return
	}

	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", siteID).Scan(&domain)
	domain = strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
	domain = strings.TrimRight(domain, "/")

	var v6Report string
	if err := db.DB.QueryRow("SELECT COALESCE(v6_report,'') FROM audit_results WHERE task_id = $1", taskID).
		Scan(&v6Report); err != nil || v6Report == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "审计报告数据不存在"})
		return
	}

	// Normalise: strip any lingering markdown wrapper before JSON decode.
	v6Report = strings.TrimSpace(v6Report)
	if strings.HasPrefix(v6Report, "```") {
		if nl := strings.Index(v6Report, "\n"); nl >= 0 {
			v6Report = v6Report[nl+1:]
		}
		if idx := strings.LastIndex(v6Report, "```"); idx >= 0 {
			v6Report = v6Report[:idx]
		}
		v6Report = strings.TrimSpace(v6Report)
	}
	if start := strings.Index(v6Report, "{"); start > 0 {
		v6Report = v6Report[start:]
	}

	var data report.V6Report
	if err := json.Unmarshal([]byte(v6Report), &data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "报告数据格式异常，请重新运行巡检"})
		return
	}

	docx, err := report.BuildDocx(&data, domain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": fmt.Sprintf("生成报告失败: %v", err)})
		return
	}

	filename := fmt.Sprintf("%s_SEO审计报告_%s.docx", domain, time.Now().Format("20060102"))
	encodedName := url.PathEscape(filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, encodedName))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx)
}
