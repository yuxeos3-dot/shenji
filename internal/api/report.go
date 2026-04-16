package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	if err := db.DB.QueryRow("SELECT site_id, status FROM audit_tasks WHERE id = ?", taskID).
		Scan(&siteID, &status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "审计任务不存在"})
		return
	}
	if status != "done" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "审计任务尚未完成"})
		return
	}

	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)
	domain = strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
	domain = strings.TrimRight(domain, "/")

	var v6Report string
	if err := db.DB.QueryRow("SELECT COALESCE(v6_report,'') FROM audit_results WHERE task_id = ?", taskID).
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

<<<<<<< HEAD
	// Write temp files
	tmpDir := "/tmp/seo-reports"
	os.MkdirAll(tmpDir, 0755)

	jsonPath := filepath.Join(tmpDir, fmt.Sprintf("report_%s.json", taskID))
	docxPath := filepath.Join(tmpDir, fmt.Sprintf("%s_SEO审计报告_%s.docx", domain, time.Now().Format("20060102")))

	if err := os.WriteFile(jsonPath, []byte(v6Report), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "写入临时文件失败"})
		return
	}
	defer os.Remove(jsonPath)
	defer os.Remove(docxPath)

	// Write gen_report.py if not exists
	scriptPath := "/tmp/gen_report.py"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		if err := os.WriteFile(scriptPath, []byte(genReportScript), 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "写入生成脚本失败"})
			return
		}
	}

	// Install python-docx only once (skip if already installed)
	if err := exec.Command("python3", "-c", "import docx").Run(); err != nil {
		exec.Command("pip3", "install", "python-docx", "--break-system-packages", "-q").Run()
	}

	// Generate docx
	cmd := exec.Command("python3", scriptPath, jsonPath, docxPath, domain)
	out, err := cmd.CombinedOutput()
=======
	docx, err := report.BuildDocx(&data, domain)
>>>>>>> 8543e622ba5aebfb08d6ab5cdfcc71e30d89c895
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": fmt.Sprintf("生成报告失败: %v", err)})
		return
	}

	filename := fmt.Sprintf("%s_SEO审计报告_%s.docx", domain, time.Now().Format("20060102"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx)
}
