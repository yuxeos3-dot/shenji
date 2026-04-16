package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// DownloadAuditReport generates a docx report and returns it for download
func DownloadAuditReport(c *gin.Context) {
	taskID := c.Param("task_id")

	// Get task info
	var siteID int64
	var status string
	db.DB.QueryRow("SELECT site_id, status FROM audit_tasks WHERE id = ?", taskID).Scan(&siteID, &status)

	if status != "done" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "审计任务尚未完成"})
		return
	}

	// Get domain
	var domain string
	db.DB.QueryRow("SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)
	domain = strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
	domain = strings.TrimRight(domain, "/")

	// Get v6_report
	var v6Report string
	err := db.DB.QueryRow("SELECT COALESCE(v6_report,'') FROM audit_results WHERE task_id = ?", taskID).Scan(&v6Report)
	if err != nil || v6Report == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "审计报告数据不存在"})
		return
	}

	// Strip markdown code blocks if present
	v6Report = strings.TrimSpace(v6Report)
	if strings.HasPrefix(v6Report, "```") {
		lines := strings.Split(v6Report, "\n")
		if len(lines) > 2 {
			inner := strings.Join(lines[1:], "\n")
			if idx := strings.LastIndex(inner, "```"); idx >= 0 {
				inner = inner[:idx]
			}
			v6Report = strings.TrimSpace(inner)
		}
	}
	// Find JSON start
	if start := strings.Index(v6Report, "{"); start > 0 {
		v6Report = v6Report[start:]
	}

	// Validate JSON
	var reportData map[string]interface{}
	if err := json.Unmarshal([]byte(v6Report), &reportData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "报告数据格式异常，请重新运行巡检"})
		return
	}

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
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": fmt.Sprintf("生成报告失败: %v\n%s", err, string(out))})
		return
	}

	// Check file exists
	if _, err := os.Stat(docxPath); os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "报告文件生成失败"})
		return
	}

	// Read file and send
	data, err := os.ReadFile(docxPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "读取报告文件失败"})
		return
	}

	filename := fmt.Sprintf("%s_SEO审计报告_%s.docx", domain, time.Now().Format("20060102"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data)
}

const genReportScript = `
import sys, json
from docx import Document
from docx.shared import Pt, Cm, RGBColor, Inches
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml.ns import qn
from docx.oxml import OxmlElement

C = {
    "title":   RGBColor(0xC0, 0x39, 0x2B),
    "fatal":   RGBColor(0xC0, 0x39, 0x2B),
    "serious": RGBColor(0xE6, 0x7E, 0x22),
    "medium":  RGBColor(0xF3, 0x9C, 0x12),
    "low":     RGBColor(0x27, 0xAE, 0x60),
    "chapter": RGBColor(0x2C, 0x3E, 0x50),
    "scope":   RGBColor(0x7F, 0x8C, 0x8D),
    "sublabel":RGBColor(0x34, 0x49, 0x5E),
    "current": RGBColor(0xC0, 0x39, 0x2B),
    "fix":     RGBColor(0x27, 0xAE, 0x60),
    "body":    RGBColor(0x2C, 0x3E, 0x50),
    "code":    RGBColor(0xEC, 0xF0, 0xF1),
    "grey":    RGBColor(0x95, 0xA5, 0xA6),
}
CODE_BG = "2C3E50"
SEV_BG = {"致命":"C0392B","严重":"E67E22","中等":"F39C12","低":"27AE60"}

def add_run(para, text, size=11, bold=False, color=None, font=None):
    run = para.add_run(str(text))
    run.font.size = Pt(size)
    run.bold = bold
    if color:
        run.font.color.rgb = color
    if font:
        run.font.name = font
        run._r.rPr.rFonts.set(qn("w:eastAsia"), font)
    return run

def set_cell_bg(cell, hex_color):
    tc = cell._tc
    tcPr = tc.get_or_add_tcPr()
    shd = OxmlElement("w:shd")
    shd.set(qn("w:val"), "clear")
    shd.set(qn("w:color"), "auto")
    shd.set(qn("w:fill"), hex_color)
    tcPr.append(shd)

def add_chapter_heading(doc, title):
    doc.add_paragraph()
    p = doc.add_paragraph()
    p.paragraph_format.space_before = Pt(10)
    p.paragraph_format.space_after = Pt(6)
    add_run(p, title, size=16, bold=True, color=C["chapter"])

SEVERITY_COLORS = {"致命": C["fatal"], "严重": C["serious"], "中等": C["medium"], "低": C["low"]}

def add_issue_title(doc, issue):
    p = doc.add_paragraph()
    p.paragraph_format.space_before = Pt(10)
    p.paragraph_format.space_after = Pt(2)
    sev = issue.get("severity", "中等")
    sev_color = SEVERITY_COLORS.get(sev, C["medium"])
    add_run(p, f"[{sev}] ", size=11, bold=True, color=sev_color)
    mod = issue.get("module", "")
    if mod:
        add_run(p, f"[{mod}] ", size=11, bold=True, color=C["sublabel"])
    add_run(p, f"#{issue.get('id',0)}：{issue.get('title','')}", size=14, bold=True, color=C["title"])

def add_code_block(doc, code_text):
    if not code_text:
        return
    lines = code_text.split("\\n")
    tbl = doc.add_table(rows=len(lines), cols=1)
    tbl.autofit = False
    for i, line in enumerate(lines):
        cell = tbl.rows[i].cells[0]
        set_cell_bg(cell, CODE_BG)
        cell.width = Cm(16)
        p = cell.paragraphs[0]
        p.paragraph_format.space_before = Pt(0)
        p.paragraph_format.space_after = Pt(0)
        add_run(p, line if line else " ", size=10, color=C["code"], font="Consolas")
    doc.add_paragraph()

def add_section(doc, label_text, label_color, body_text):
    if not body_text:
        return
    h = doc.add_paragraph()
    h.paragraph_format.space_before = Pt(6)
    h.paragraph_format.space_after = Pt(2)
    add_run(h, label_text, size=12, bold=True, color=label_color)
    for para in (body_text or "").split("\\n\\n"):
        if not para.strip():
            continue
        p = doc.add_paragraph()
        p.paragraph_format.space_after = Pt(4)
        for i, line in enumerate(para.split("\\n")):
            if i > 0:
                add_run(p, "\\n", size=11, color=C["body"])
            add_run(p, line, size=11, color=C["body"])

def add_issue(doc, issue):
    add_issue_title(doc, issue)
    scope = issue.get("scope", "")
    if scope:
        p = doc.add_paragraph()
        p.paragraph_format.space_before = Pt(2)
        p.paragraph_format.space_after = Pt(4)
        add_run(p, "影响范围：", size=11, bold=True, color=C["scope"])
        add_run(p, scope, size=11, color=C["scope"])
    ev_label = issue.get("evidence_label", "")
    if ev_label:
        p = doc.add_paragraph()
        p.paragraph_format.space_before = Pt(4)
        p.paragraph_format.space_after = Pt(2)
        add_run(p, ev_label, size=11, bold=True, color=C["sublabel"])
    add_code_block(doc, issue.get("evidence_code", ""))
    add_section(doc, "目前现状", C["current"], issue.get("current_status", ""))
    add_section(doc, "修改建议", C["fix"], issue.get("fix_suggestion", ""))
    doc.add_paragraph()

def build_report(data, output_path, domain):
    doc = Document()
    sec = doc.sections[0]
    sec.left_margin = Inches(0.9)
    sec.right_margin = Inches(0.9)
    sec.top_margin = Inches(0.8)
    sec.bottom_margin = Inches(0.8)
    doc.styles["Normal"].font.name = "微软雅黑"
    doc.styles["Normal"].font.size = Pt(11)

    # Cover
    cover = doc.add_paragraph()
    cover.alignment = WD_ALIGN_PARAGRAPH.LEFT
    add_run(cover, f"SEO 深度审计报告", size=20, bold=True, color=C["title"])
    cover2 = doc.add_paragraph()
    add_run(cover2, f"域名：{domain}", size=13, color=C["chapter"])
    date_p = doc.add_paragraph()
    total = data.get("total_issues", 0)
    add_run(date_p,
        f"生成时间：{data.get('generated_at','')}   |   "
        f"总问题数：{total}   |   "
        f"致命：{data.get('fatal',0)}   严重：{data.get('serious',0)}   "
        f"中等：{data.get('medium',0)}   低：{data.get('low',0)}",
        size=10, color=C["grey"])
    summary = data.get("summary", "")
    if summary:
        sp = doc.add_paragraph()
        sp.paragraph_format.space_before = Pt(6)
        add_run(sp, "总结：", size=12, bold=True, color=C["chapter"])
        add_run(sp, summary, size=11, color=C["body"])
    doc.add_paragraph()

    for chapter in data.get("chapters", []):
        add_chapter_heading(doc, chapter["title"])
        for issue in chapter.get("issues", []):
            add_issue(doc, issue)

    doc.save(output_path)
    print(f"报告已生成: {output_path}")

if __name__ == "__main__":
    data_path = sys.argv[1]
    output_path = sys.argv[2]
    domain = sys.argv[3] if len(sys.argv) > 3 else "site"
    with open(data_path, encoding="utf-8") as f:
        data = json.load(f)
    build_report(data, output_path, domain)
`
