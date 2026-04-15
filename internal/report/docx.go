// Package report builds SEO audit reports as Word (.docx) files using only the
// Go standard library — no Python / python-docx dependency.
//
// A .docx file is a ZIP archive with a specific layout of XML parts:
//
//	[Content_Types].xml
//	_rels/.rels
//	word/document.xml
//	word/styles.xml  (optional — we inline run properties instead)
//
// We emit the minimum viable set that Word / LibreOffice / Google Docs render.
package report

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// V6Report mirrors the JSON structure produced by the AI.
// Only the fields we render are listed.
type V6Report struct {
	Domain      string      `json:"domain"`
	GeneratedAt string      `json:"generated_at"`
	TotalIssues int         `json:"total_issues"`
	Fatal       int         `json:"fatal"`
	Serious     int         `json:"serious"`
	Medium      int         `json:"medium"`
	Low         int         `json:"low"`
	Summary     string      `json:"summary"`
	Chapters    []V6Chapter `json:"chapters"`
}

type V6Chapter struct {
	Title  string    `json:"title"`
	Issues []V6Issue `json:"issues"`
}

type V6Issue struct {
	ID            int    `json:"id"`
	Severity      string `json:"severity"`
	Module        string `json:"module"`
	Title         string `json:"title"`
	Scope         string `json:"scope"`
	EvidenceLabel string `json:"evidence_label"`
	EvidenceCode  string `json:"evidence_code"`
	CurrentStatus string `json:"current_status"`
	FixSuggestion string `json:"fix_suggestion"`
}

// Severity color palette (hex, no leading #). Aligned with the previous Python
// palette for visual continuity.
var sevColor = map[string]string{
	"致命":  "C0392B",
	"严重":  "E67E22",
	"中等":  "F39C12",
	"低":   "27AE60",
}

const (
	colorTitle   = "C0392B"
	colorChapter = "2C3E50"
	colorSublbl  = "34495E"
	colorBody    = "2C3E50"
	colorScope   = "7F8C8D"
	colorGrey    = "95A5A6"
	colorFix     = "27AE60"
	colorCurrent = "C0392B"
	colorCode    = "ECF0F1"
	codeBg       = "2C3E50"
)

// BuildDocx returns a zipped .docx byte stream for the given report.
func BuildDocx(r *V6Report, domain string) ([]byte, error) {
	if domain == "" {
		domain = r.Domain
	}

	var doc bytes.Buffer
	doc.WriteString(docPrefix)

	// ── Cover ────────────────────────────────────────────────────────────
	writeHeading(&doc, "SEO 深度审计报告", 28, colorTitle, true)
	writeLine(&doc, fmt.Sprintf("域名：%s", domain), 14, colorChapter, true)
	writeLine(&doc,
		fmt.Sprintf("生成时间：%s    |    总问题数：%d    |    致命：%d  严重：%d  中等：%d  低：%d",
			r.GeneratedAt, r.TotalIssues, r.Fatal, r.Serious, r.Medium, r.Low),
		10, colorGrey, false)

	if s := strings.TrimSpace(r.Summary); s != "" {
		writeLabelPara(&doc, "总结：", colorChapter, s, colorBody)
	}
	writeEmpty(&doc)

	// ── Chapters ─────────────────────────────────────────────────────────
	for _, ch := range r.Chapters {
		writeHeading(&doc, ch.Title, 18, colorChapter, true)
		for _, iss := range ch.Issues {
			writeIssue(&doc, iss)
		}
	}

	doc.WriteString(docSuffix)

	// ── Assemble zip ─────────────────────────────────────────────────────
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	if err := addZipFile(zw, "[Content_Types].xml", contentTypesXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zw, "_rels/.rels", relsXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zw, "word/document.xml", doc.String()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zbuf.Bytes(), nil
}

// ─── XML writers ─────────────────────────────────────────────────────────────

func writeHeading(buf *bytes.Buffer, text string, ptSize int, hexColor string, bold bool) {
	fmt.Fprintf(buf, `<w:p><w:pPr><w:spacing w:before="200" w:after="120"/></w:pPr>`)
	writeRun(buf, text, ptSize, hexColor, bold, "")
	buf.WriteString(`</w:p>`)
}

func writeLine(buf *bytes.Buffer, text string, ptSize int, hexColor string, bold bool) {
	buf.WriteString(`<w:p><w:pPr><w:spacing w:after="60"/></w:pPr>`)
	writeRun(buf, text, ptSize, hexColor, bold, "")
	buf.WriteString(`</w:p>`)
}

func writeEmpty(buf *bytes.Buffer) {
	buf.WriteString(`<w:p/>`)
}

// writeLabelPara writes "[label] body" with separate colors.
func writeLabelPara(buf *bytes.Buffer, label, labelColor, body, bodyColor string) {
	buf.WriteString(`<w:p><w:pPr><w:spacing w:after="60"/></w:pPr>`)
	writeRun(buf, label, 12, labelColor, true, "")
	for i, para := range strings.Split(body, "\n\n") {
		if i > 0 {
			buf.WriteString(`<w:br/>`)
		}
		for j, line := range strings.Split(para, "\n") {
			if j > 0 {
				buf.WriteString(`<w:br/>`)
			}
			writeRun(buf, line, 11, bodyColor, false, "")
		}
	}
	buf.WriteString(`</w:p>`)
}

// writeRun emits a <w:r> with font + color + optional bold.
// If font is "", defaults to 微软雅黑 (which Word maps to Microsoft YaHei).
func writeRun(buf *bytes.Buffer, text string, ptSize int, hexColor string, bold bool, font string) {
	if font == "" {
		font = "微软雅黑"
	}
	halfPt := ptSize * 2
	buf.WriteString(`<w:r><w:rPr>`)
	fmt.Fprintf(buf, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:eastAsia="%s"/>`, font, font, font)
	fmt.Fprintf(buf, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, halfPt, halfPt)
	if hexColor != "" {
		fmt.Fprintf(buf, `<w:color w:val="%s"/>`, hexColor)
	}
	if bold {
		buf.WriteString(`<w:b/><w:bCs/>`)
	}
	buf.WriteString(`</w:rPr>`)
	// Preserve whitespace so leading/trailing spaces aren't trimmed.
	fmt.Fprintf(buf, `<w:t xml:space="preserve">%s</w:t>`, xmlEscape(text))
	buf.WriteString(`</w:r>`)
}

// writeIssue renders one V6Issue as a block with header, evidence code-box,
// current status and fix suggestion sections.
func writeIssue(buf *bytes.Buffer, iss V6Issue) {
	sev := iss.Severity
	if sev == "" {
		sev = "中等"
	}
	sevHex := sevColor[sev]
	if sevHex == "" {
		sevHex = sevColor["中等"]
	}

	// Title row: [severity] [module] #id：title
	buf.WriteString(`<w:p><w:pPr><w:spacing w:before="200" w:after="40"/></w:pPr>`)
	writeRun(buf, "["+sev+"] ", 11, sevHex, true, "")
	if mod := strings.TrimSpace(iss.Module); mod != "" {
		writeRun(buf, "["+mod+"] ", 11, colorSublbl, true, "")
	}
	writeRun(buf, fmt.Sprintf("#%d：%s", iss.ID, iss.Title), 14, colorTitle, true, "")
	buf.WriteString(`</w:p>`)

	if s := strings.TrimSpace(iss.Scope); s != "" {
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="40" w:after="40"/></w:pPr>`)
		writeRun(buf, "影响范围：", 11, colorScope, true, "")
		writeRun(buf, s, 11, colorScope, false, "")
		buf.WriteString(`</w:p>`)
	}

	if s := strings.TrimSpace(iss.EvidenceLabel); s != "" {
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="60" w:after="40"/></w:pPr>`)
		writeRun(buf, s, 11, colorSublbl, true, "")
		buf.WriteString(`</w:p>`)
	}

	if s := strings.TrimSpace(iss.EvidenceCode); s != "" {
		writeCodeBlock(buf, s)
	}

	if s := strings.TrimSpace(iss.CurrentStatus); s != "" {
		writeLabelPara(buf, "目前现状：", colorCurrent, s, colorBody)
	}
	if s := strings.TrimSpace(iss.FixSuggestion); s != "" {
		writeLabelPara(buf, "修改建议：", colorFix, s, colorBody)
	}
	writeEmpty(buf)
}

// writeCodeBlock renders the code inside a single-column shaded table, mirroring
// the previous Python output.
func writeCodeBlock(buf *bytes.Buffer, code string) {
	lines := strings.Split(code, "\n")
	buf.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9000" w:type="dxa"/><w:tblBorders>`)
	buf.WriteString(`<w:top w:val="single" w:sz="4" w:color="1F2D3D"/>`)
	buf.WriteString(`<w:left w:val="single" w:sz="4" w:color="1F2D3D"/>`)
	buf.WriteString(`<w:bottom w:val="single" w:sz="4" w:color="1F2D3D"/>`)
	buf.WriteString(`<w:right w:val="single" w:sz="4" w:color="1F2D3D"/>`)
	buf.WriteString(`</w:tblBorders></w:tblPr>`)
	buf.WriteString(`<w:tblGrid><w:gridCol w:w="9000"/></w:tblGrid>`)

	for _, line := range lines {
		if line == "" {
			line = " "
		}
		buf.WriteString(`<w:tr><w:tc><w:tcPr><w:tcW w:w="9000" w:type="dxa"/>`)
		fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, codeBg)
		buf.WriteString(`</w:tcPr>`)
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="0" w:after="0"/></w:pPr>`)
		writeRun(buf, line, 10, colorCode, false, "Consolas")
		buf.WriteString(`</w:p></w:tc></w:tr>`)
	}
	buf.WriteString(`</w:tbl>`)
	writeEmpty(buf)
}

// ─── XML helpers ─────────────────────────────────────────────────────────────

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func addZipFile(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}

// ─── Static skeleton XML ─────────────────────────────────────────────────────

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const docPrefix = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>`

const docSuffix = `<w:sectPr>
<w:pgSz w:w="12240" w:h="15840"/>
<w:pgMar w:top="1152" w:right="1296" w:bottom="1152" w:left="1296" w:header="720" w:footer="720" w:gutter="0"/>
</w:sectPr>
</w:body>
</w:document>`
