// Package report builds SEO audit reports as Word (.docx) files using only the
// Go standard library — no Python / python-docx dependency.
//
// A .docx file is a ZIP archive with a specific layout of XML parts:
//
//	[Content_Types].xml
//	_rels/.rels
//	word/document.xml
//	word/styles.xml
//
// We emit the minimum viable set that Word / LibreOffice / Google Docs render,
// plus custom formatting for a professional-looking SEO audit report:
//   - Cover page with colored title bar
//   - Statistics summary table
//   - Table of contents (chapter list)
//   - Page break between sections
//   - Severity-colored issue headers
//   - Shaded code blocks
//   - Left-bar accented current-status / fix-suggestion sections
package report

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// V6Report mirrors the JSON structure produced by the AI.
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

// Severity color palette (hex, no leading #).
var sevColor = map[string]string{
	"致命": "C0392B",
	"严重": "E67E22",
	"中等": "F39C12",
	"低":  "27AE60",
}

var sevBG = map[string]string{
	"致命": "FDEDEC",
	"严重": "FEF5E7",
	"中等": "FEF9E7",
	"低":  "EAFAF1",
}

const (
	colorTitle   = "C0392B"
	colorBrand   = "1F6FEB"
	colorChapter = "1A365D"
	colorSublbl  = "34495E"
	colorBody    = "2C3E50"
	colorScope   = "7F8C8D"
	colorGrey    = "95A5A6"
	colorFix     = "27AE60"
	colorCurrent = "C0392B"
	colorCode    = "ECF0F1"
	colorAccent  = "2E86DE"
	codeBg       = "1E293B"
	coverBg      = "1F6FEB"
	tableHdrBg   = "E8F0FE"
	tableRowAlt  = "F8FAFC"
)

// BuildDocx returns a zipped .docx byte stream for the given report.
func BuildDocx(r *V6Report, domain string) ([]byte, error) {
	if domain == "" {
		domain = r.Domain
	}

	var doc bytes.Buffer
	doc.WriteString(docPrefix)

	// ── Cover page ────────────────────────────────────────────────────────
	writeCoverPage(&doc, r, domain)
	writePageBreak(&doc)

	// ── Statistics summary ───────────────────────────────────────────────
	writeHeading(&doc, "📊 审计摘要", 20, colorChapter, true, 360)
	writeStatsTable(&doc, r)
	writeEmpty(&doc)

	if s := strings.TrimSpace(r.Summary); s != "" {
		writeHeading(&doc, "🔍 站点总体评估", 16, colorChapter, true, 240)
		writeBodyParagraph(&doc, s, colorBody)
	}

	// ── Table of contents ────────────────────────────────────────────────
	if len(r.Chapters) > 0 {
		writeEmpty(&doc)
		writeHeading(&doc, "📑 章节目录", 16, colorChapter, true, 240)
		for i, ch := range r.Chapters {
			writeTOCEntry(&doc, i+1, ch.Title, len(ch.Issues))
		}
	}
	writePageBreak(&doc)

	// ── Chapters ─────────────────────────────────────────────────────────
	for ci, ch := range r.Chapters {
		writeChapterBanner(&doc, ci+1, ch.Title, len(ch.Issues))
		for _, iss := range ch.Issues {
			writeIssue(&doc, iss)
		}
		// Page break between chapters (but not after the last one).
		if ci < len(r.Chapters)-1 {
			writePageBreak(&doc)
		}
	}

	// ── Footer note ──────────────────────────────────────────────────────
	writeEmpty(&doc)
	writeFooterLine(&doc, fmt.Sprintf("—— 报告由 SEO 巡站系统 v6 生成 · %s ——", r.GeneratedAt))

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
	if err := addZipFile(zw, "word/_rels/document.xml.rels", docRelsXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zw, "word/styles.xml", stylesXML); err != nil {
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

// ─── High-level block writers ────────────────────────────────────────────────

func writeCoverPage(buf *bytes.Buffer, r *V6Report, domain string) {
	// Title banner (shaded paragraph).
	buf.WriteString(`<w:p><w:pPr>`)
	fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, coverBg)
	buf.WriteString(`<w:spacing w:before="600" w:after="0"/>`)
	buf.WriteString(`<w:jc w:val="center"/></w:pPr>`)
	writeRun(buf, "SEO 深度审计报告", 32, "FFFFFF", true, "")
	buf.WriteString(`</w:p>`)

	// Sub-banner with domain.
	buf.WriteString(`<w:p><w:pPr>`)
	fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, coverBg)
	buf.WriteString(`<w:spacing w:before="0" w:after="600"/>`)
	buf.WriteString(`<w:jc w:val="center"/></w:pPr>`)
	writeRun(buf, domain, 16, "FFFFFF", false, "")
	buf.WriteString(`</w:p>`)

	// Spacer.
	writeEmpty(buf)
	writeEmpty(buf)

	// Meta info block centered.
	buf.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="120"/></w:pPr>`)
	writeRun(buf, "生成时间 · ", 12, colorGrey, false, "")
	writeRun(buf, r.GeneratedAt, 12, colorChapter, true, "")
	buf.WriteString(`</w:p>`)

	buf.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="120"/></w:pPr>`)
	writeRun(buf, "总问题数 · ", 12, colorGrey, false, "")
	writeRun(buf, fmt.Sprintf("%d", r.TotalIssues), 16, colorTitle, true, "")
	buf.WriteString(`</w:p>`)

	// Severity badge line (centered colored inlines).
	buf.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="600"/></w:pPr>`)
	writeSevBadge(buf, "致命", r.Fatal)
	writeRun(buf, "  ", 12, colorGrey, false, "")
	writeSevBadge(buf, "严重", r.Serious)
	writeRun(buf, "  ", 12, colorGrey, false, "")
	writeSevBadge(buf, "中等", r.Medium)
	writeRun(buf, "  ", 12, colorGrey, false, "")
	writeSevBadge(buf, "低", r.Low)
	buf.WriteString(`</w:p>`)
}

func writeSevBadge(buf *bytes.Buffer, sev string, n int) {
	hex := sevColor[sev]
	if hex == "" {
		hex = colorGrey
	}
	writeRun(buf, fmt.Sprintf("● %s %d", sev, n), 13, hex, true, "")
}

// writeStatsTable renders a 4-column summary table.
func writeStatsTable(buf *bytes.Buffer, r *V6Report) {
	buf.WriteString(`<w:tbl><w:tblPr>`)
	buf.WriteString(`<w:tblW w:w="5000" w:type="pct"/>`)
	buf.WriteString(`<w:tblBorders>`)
	buf.WriteString(`<w:top w:val="single" w:sz="4" w:color="CBD5E1"/>`)
	buf.WriteString(`<w:left w:val="single" w:sz="4" w:color="CBD5E1"/>`)
	buf.WriteString(`<w:bottom w:val="single" w:sz="4" w:color="CBD5E1"/>`)
	buf.WriteString(`<w:right w:val="single" w:sz="4" w:color="CBD5E1"/>`)
	buf.WriteString(`<w:insideH w:val="single" w:sz="4" w:color="CBD5E1"/>`)
	buf.WriteString(`<w:insideV w:val="single" w:sz="4" w:color="CBD5E1"/>`)
	buf.WriteString(`</w:tblBorders></w:tblPr>`)
	buf.WriteString(`<w:tblGrid><w:gridCol w:w="2250"/><w:gridCol w:w="2250"/><w:gridCol w:w="2250"/><w:gridCol w:w="2250"/></w:tblGrid>`)

	// Header row.
	buf.WriteString(`<w:tr>`)
	for _, h := range []string{"🔴 致命", "🟠 严重", "🟡 中等", "🟢 低"} {
		writeStatsCell(buf, h, "", tableHdrBg, colorChapter, true)
	}
	buf.WriteString(`</w:tr>`)

	// Value row.
	buf.WriteString(`<w:tr>`)
	writeStatsCell(buf, fmt.Sprintf("%d", r.Fatal), "个问题", "", sevColor["致命"], true)
	writeStatsCell(buf, fmt.Sprintf("%d", r.Serious), "个问题", "", sevColor["严重"], true)
	writeStatsCell(buf, fmt.Sprintf("%d", r.Medium), "个问题", "", sevColor["中等"], true)
	writeStatsCell(buf, fmt.Sprintf("%d", r.Low), "个问题", "", sevColor["低"], true)
	buf.WriteString(`</w:tr>`)

	buf.WriteString(`</w:tbl>`)
}

func writeStatsCell(buf *bytes.Buffer, main, sub, bg, color string, bold bool) {
	buf.WriteString(`<w:tc><w:tcPr><w:tcW w:w="2250" w:type="dxa"/>`)
	if bg != "" {
		fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, bg)
	}
	buf.WriteString(`</w:tcPr>`)
	buf.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="120" w:after="120"/></w:pPr>`)
	size := 24
	if sub == "" {
		size = 14
	}
	writeRun(buf, main, size, color, bold, "")
	if sub != "" {
		writeRun(buf, " "+sub, 10, colorGrey, false, "")
	}
	buf.WriteString(`</w:p></w:tc>`)
}

func writeTOCEntry(buf *bytes.Buffer, idx int, title string, issueCount int) {
	buf.WriteString(`<w:p><w:pPr><w:spacing w:before="60" w:after="60"/>`)
	buf.WriteString(`<w:ind w:left="360"/></w:pPr>`)
	writeRun(buf, fmt.Sprintf("%02d. ", idx), 12, colorAccent, true, "")
	writeRun(buf, title, 12, colorChapter, true, "")
	writeRun(buf, fmt.Sprintf("   （%d 条问题）", issueCount), 10, colorGrey, false, "")
	buf.WriteString(`</w:p>`)
}

func writeChapterBanner(buf *bytes.Buffer, idx int, title string, issueCount int) {
	// Colored left strip + large title.
	buf.WriteString(`<w:p><w:pPr>`)
	fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, tableHdrBg)
	buf.WriteString(`<w:spacing w:before="240" w:after="240"/>`)
	buf.WriteString(`<w:pBdr><w:left w:val="single" w:sz="24" w:space="8" w:color="1F6FEB"/></w:pBdr>`)
	buf.WriteString(`</w:pPr>`)
	writeRun(buf, fmt.Sprintf("  第 %d 章 · ", idx), 14, colorAccent, true, "")
	writeRun(buf, title, 18, colorChapter, true, "")
	writeRun(buf, fmt.Sprintf("    （%d 条）", issueCount), 11, colorGrey, false, "")
	buf.WriteString(`</w:p>`)
}

func writeIssue(buf *bytes.Buffer, iss V6Issue) {
	sev := iss.Severity
	if sev == "" {
		sev = "中等"
	}
	sevHex := sevColor[sev]
	if sevHex == "" {
		sevHex = sevColor["中等"]
	}
	sevBgHex := sevBG[sev]
	if sevBgHex == "" {
		sevBgHex = sevBG["中等"]
	}

	// Severity pill + title, shaded background.
	buf.WriteString(`<w:p><w:pPr>`)
	fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, sevBgHex)
	buf.WriteString(`<w:spacing w:before="200" w:after="80"/>`)
	buf.WriteString(`<w:pBdr><w:left w:val="single" w:sz="24" w:space="8" w:color="`)
	buf.WriteString(sevHex)
	buf.WriteString(`"/></w:pBdr></w:pPr>`)
	writeRun(buf, " "+sev+" ", 11, "FFFFFF", true, "")
	writeRun(buf, "  ", 11, colorBody, false, "")
	if mod := strings.TrimSpace(iss.Module); mod != "" {
		writeRun(buf, "["+mod+"] ", 11, colorSublbl, true, "")
	}
	writeRun(buf, fmt.Sprintf("#%d  %s", iss.ID, iss.Title), 14, colorChapter, true, "")
	buf.WriteString(`</w:p>`)

	if s := strings.TrimSpace(iss.Scope); s != "" {
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="40" w:after="40"/><w:ind w:left="280"/></w:pPr>`)
		writeRun(buf, "📍 影响范围：", 10, colorScope, true, "")
		writeRun(buf, s, 10, colorScope, false, "")
		buf.WriteString(`</w:p>`)
	}

	if s := strings.TrimSpace(iss.EvidenceLabel); s != "" {
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="80" w:after="40"/><w:ind w:left="280"/></w:pPr>`)
		writeRun(buf, "🔍 "+s, 11, colorSublbl, true, "")
		buf.WriteString(`</w:p>`)
	}

	if s := strings.TrimSpace(iss.EvidenceCode); s != "" {
		writeCodeBlock(buf, s)
	}

	if s := strings.TrimSpace(iss.CurrentStatus); s != "" {
		writeBarSection(buf, "⚠️ 目前现状", colorCurrent, s, colorBody)
	}
	if s := strings.TrimSpace(iss.FixSuggestion); s != "" {
		writeBarSection(buf, "✅ 修改建议", colorFix, s, colorBody)
	}
	writeEmpty(buf)
}

// writeBarSection renders a left-bordered block with a colored label.
func writeBarSection(buf *bytes.Buffer, label, labelColor, body, bodyColor string) {
	// Label paragraph.
	buf.WriteString(`<w:p><w:pPr><w:spacing w:before="120" w:after="60"/>`)
	buf.WriteString(`<w:ind w:left="280"/>`)
	fmt.Fprintf(buf, `<w:pBdr><w:left w:val="single" w:sz="18" w:space="8" w:color="%s"/></w:pBdr>`, labelColor)
	buf.WriteString(`</w:pPr>`)
	writeRun(buf, label, 12, labelColor, true, "")
	buf.WriteString(`</w:p>`)

	// Body paragraphs — split on blank line for paragraphs, single newline for br.
	for _, para := range strings.Split(body, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		buf.WriteString(`<w:p><w:pPr><w:spacing w:after="80"/>`)
		buf.WriteString(`<w:ind w:left="280"/>`)
		fmt.Fprintf(buf, `<w:pBdr><w:left w:val="single" w:sz="18" w:space="8" w:color="%s"/></w:pBdr>`, labelColor)
		buf.WriteString(`</w:pPr>`)
		for i, line := range strings.Split(para, "\n") {
			if i > 0 {
				buf.WriteString(`<w:br/>`)
			}
			writeRun(buf, line, 11, bodyColor, false, "")
		}
		buf.WriteString(`</w:p>`)
	}
}

// writeCodeBlock renders code inside a single-column shaded table.
func writeCodeBlock(buf *bytes.Buffer, code string) {
	lines := strings.Split(code, "\n")
	buf.WriteString(`<w:tbl><w:tblPr>`)
	buf.WriteString(`<w:tblW w:w="4800" w:type="pct"/>`)
	buf.WriteString(`<w:tblInd w:w="280" w:type="dxa"/>`)
	buf.WriteString(`<w:tblBorders>`)
	buf.WriteString(`<w:top w:val="single" w:sz="4" w:color="334155"/>`)
	buf.WriteString(`<w:left w:val="single" w:sz="4" w:color="334155"/>`)
	buf.WriteString(`<w:bottom w:val="single" w:sz="4" w:color="334155"/>`)
	buf.WriteString(`<w:right w:val="single" w:sz="4" w:color="334155"/>`)
	buf.WriteString(`</w:tblBorders></w:tblPr>`)
	buf.WriteString(`<w:tblGrid><w:gridCol w:w="9000"/></w:tblGrid>`)

	for _, line := range lines {
		if line == "" {
			line = " "
		}
		buf.WriteString(`<w:tr><w:tc><w:tcPr><w:tcW w:w="9000" w:type="dxa"/>`)
		fmt.Fprintf(buf, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, codeBg)
		buf.WriteString(`</w:tcPr>`)
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="40" w:after="40"/></w:pPr>`)
		writeRun(buf, line, 10, colorCode, false, "Consolas")
		buf.WriteString(`</w:p></w:tc></w:tr>`)
	}
	buf.WriteString(`</w:tbl>`)
	writeEmpty(buf)
}

func writeHeading(buf *bytes.Buffer, text string, ptSize int, hexColor string, bold bool, before int) {
	fmt.Fprintf(buf, `<w:p><w:pPr><w:spacing w:before="%d" w:after="120"/></w:pPr>`, before)
	writeRun(buf, text, ptSize, hexColor, bold, "")
	buf.WriteString(`</w:p>`)
}

func writeBodyParagraph(buf *bytes.Buffer, text, color string) {
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		buf.WriteString(`<w:p><w:pPr><w:spacing w:after="120"/></w:pPr>`)
		for i, line := range strings.Split(para, "\n") {
			if i > 0 {
				buf.WriteString(`<w:br/>`)
			}
			writeRun(buf, line, 11, color, false, "")
		}
		buf.WriteString(`</w:p>`)
	}
}

func writeFooterLine(buf *bytes.Buffer, text string) {
	buf.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="600"/></w:pPr>`)
	writeRun(buf, text, 9, colorGrey, false, "")
	buf.WriteString(`</w:p>`)
}

func writeEmpty(buf *bytes.Buffer) {
	buf.WriteString(`<w:p/>`)
}

func writePageBreak(buf *bytes.Buffer) {
	buf.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
}

// writeRun emits a <w:r> with font + color + optional bold.
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
	fmt.Fprintf(buf, `<w:t xml:space="preserve">%s</w:t>`, xmlEscape(text))
	buf.WriteString(`</w:r>`)
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
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults>
<w:rPrDefault><w:rPr>
<w:rFonts w:ascii="微软雅黑" w:hAnsi="微软雅黑" w:eastAsia="微软雅黑" w:cs="微软雅黑"/>
<w:sz w:val="22"/><w:szCs w:val="22"/>
<w:lang w:val="zh-CN" w:eastAsia="zh-CN"/>
</w:rPr></w:rPrDefault>
<w:pPrDefault><w:pPr>
<w:spacing w:line="320" w:lineRule="auto"/>
</w:pPr></w:pPrDefault>
</w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal">
<w:name w:val="Normal"/><w:qFormat/>
</w:style>
</w:styles>`

const docPrefix = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>`

const docSuffix = `<w:sectPr>
<w:pgSz w:w="12240" w:h="15840"/>
<w:pgMar w:top="1152" w:right="1296" w:bottom="1152" w:left="1296" w:header="720" w:footer="720" w:gutter="0"/>
</w:sectPr>
</w:body>
</w:document>`
