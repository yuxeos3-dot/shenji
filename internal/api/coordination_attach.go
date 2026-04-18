package api

// Attachment handling for coordination tasks. Files are stored on local
// disk under a sharded directory tree so a single directory never blows
// past a few thousand entries. The path is never derived from the
// client-provided filename — we generate a UUID + preserve the extension
// — so path traversal is not possible.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ── Config ────────────────────────────────────────────────────────────────

// coordUploadRoot is where attachments land on disk. Configurable via
// COORD_UPLOAD_DIR; defaults to /opt/seo-system/uploads/coord which
// matches where the service runs in production.
func coordUploadRoot() string {
	if v := os.Getenv("COORD_UPLOAD_DIR"); v != "" {
		return v
	}
	return "/opt/seo-system/uploads/coord"
}

// maxAttachmentBytes: 10 MB per file. Enough for screenshots and most
// docs; stops an accidental video upload from filling the disk.
const maxAttachmentBytes = 10 * 1024 * 1024

// allowedAttachmentExts whitelists what can be uploaded. Keep in sync
// with the frontend accept="" string in AttachmentUploader.
var allowedAttachmentExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".xls":  true,
	".xlsx": true,
	".txt":  true,
	".csv":  true,
	".md":   true,
}

// ── Helpers ───────────────────────────────────────────────────────────────

// randomHex returns n*2 hex characters from crypto/rand. Used to build
// opaque filenames on disk so we never echo the user's filename into
// the filesystem path.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// storagePathFor returns the absolute disk path for a stored attachment,
// given the relative path we persisted to the DB. The relative path is
// constructed as `<hexShard>/<hexName><ext>`, so both segments are
// guaranteed clean and the Join is safe — but we still check that the
// final path stays under uploadRoot in case someone hand-edits the DB.
func storagePathFor(relPath string) (string, error) {
	root := coordUploadRoot()
	abs := filepath.Join(root, relPath)
	rootAbs, _ := filepath.Abs(root)
	absAbs, _ := filepath.Abs(abs)
	if !strings.HasPrefix(absAbs, rootAbs+string(filepath.Separator)) && absAbs != rootAbs {
		return "", errors.New("path escapes upload root")
	}
	return abs, nil
}

// ── Handlers ──────────────────────────────────────────────────────────────

// UploadCoordAttachment accepts a multipart POST and stores the file.
// Body is form-data: `file` (required), `note` (optional comment added
// to the timeline alongside the upload event).
//
// Attachments are permitted in every state EXCEPT `done`, matching the
// admin's need to keep adding context through the whole fix cycle.
func UploadCoordAttachment(c *gin.Context) {
	id := parseInt64Param(c, "id")
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "id 无效"})
		return
	}

	// assertEditable rejects `done` tasks.
	t, ok := assertEditable(c, id)
	if !ok {
		return
	}

	// Pull the single file out of the request.
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请选择文件"})
		return
	}
	if fh.Size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "文件为空"})
		return
	}
	if fh.Size > maxAttachmentBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"code": -1, "error": fmt.Sprintf("文件过大，最大 %d MB", maxAttachmentBytes/1024/1024),
		})
		return
	}

	// Extension check (case-insensitive). We don't sniff MIME here —
	// the whitelist of extensions is enough to keep out the obvious
	// foot-guns (exe/sh/php), and the browser won't execute anything
	// served with the right Content-Type anyway since we force a safe
	// Content-Disposition on download.
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !allowedAttachmentExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1, "error": "不支持的文件类型，仅允许：图片/PDF/Office/TXT/Markdown",
		})
		return
	}

	// Build storage path. Shard by the first byte of the random name so
	// we get at most ~256 entries per directory no matter how many
	// attachments accumulate.
	name := randomHex(16)
	rel := filepath.Join(name[:2], name+ext)
	abs, err := storagePathFor(rel)
	if err != nil {
		log.Printf("[coord-upload] path sanity failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "存储路径错误"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		log.Printf("[coord-upload] mkdir %s: %v", filepath.Dir(abs), err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "创建目录失败"})
		return
	}

	// Save the file. c.SaveUploadedFile handles the copy stream-wise.
	if err := c.SaveUploadedFile(fh, abs); err != nil {
		log.Printf("[coord-upload] save %s: %v", abs, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	uid, uname := currentUser(c)

	// Derive a MIME from extension for correct rendering on download
	// (image previews, PDF inline display). mime.TypeByExtension returns
	// an empty string if the mapping isn't registered — fine, the
	// download handler falls back to octet-stream.
	mimeType := mime.TypeByExtension(ext)

	// Record an event first so the timeline shows WHO uploaded WHAT,
	// and we can point the attachment row back at that event.
	var eventID int64
	err := db.DB.QueryRow(
		`INSERT INTO coordination_events
		 (task_id, event_type, from_status, to_status, actor_id, actor_name, note)
		 VALUES ($1, 'upload', $2, $3, $4, $5, $6) RETURNING id`,
		id, t.Status, t.Status, uid, uname, "📎 上传："+fh.Filename).Scan(&eventID)
	if err != nil {
		log.Printf("[coord-upload] event insert failed: %v", err)
	}

	// Now the attachment row.
	var aID int64
	err = db.DB.QueryRow(
		`INSERT INTO coordination_attachments
		 (task_id, event_id, filename, filepath, size, mime, uploaded_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		id, eventID, fh.Filename, rel, fh.Size, mimeType, uid).Scan(&aID)
	if err != nil {
		// Roll back the file on DB failure so we don't leak orphans.
		_ = os.Remove(abs)
		log.Printf("[coord-upload] attachment insert failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	WriteAuditLog(c, "coord_upload", "coordination_attachment", fmt.Sprintf("%d", aID), fh.Filename)
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"id":       aID,
			"filename": fh.Filename,
			"size":     fh.Size,
			"mime":     mimeType,
		},
	})
}

// ServeCoordAttachment streams a stored file. Permission to read a
// single file is the same as permission to view the task: the route is
// already gated by `coord.view` in main.go.
//
// We set Content-Disposition based on MIME: images/PDFs render inline
// (so `<img src="...">` works), everything else forces download so a
// malicious HTML file can't run in the user's session origin.
func ServeCoordAttachment(c *gin.Context) {
	id := parseInt64Param(c, "id")
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "id 无效"})
		return
	}

	var filename, rel, mimeType string
	err := db.DB.QueryRow(
		`SELECT filename, filepath, mime FROM coordination_attachments WHERE id = $1`, id,
	).Scan(&filename, &rel, &mimeType)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "附件不存在"})
		return
	}
	if err != nil {
		log.Printf("[coord-attach] load failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}

	abs, err := storagePathFor(rel)
	if err != nil {
		log.Printf("[coord-attach] path unsafe for id=%d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "附件路径异常"})
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		log.Printf("[coord-attach] open %s: %v", abs, err)
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "文件已丢失"})
		return
	}
	defer f.Close()

	// Figure out whether to render inline or force download.
	inline := strings.HasPrefix(mimeType, "image/") || mimeType == "application/pdf"
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	// RFC 5987 encoded filename so Chinese names survive the round trip.
	encName := strings.ReplaceAll(filename, `"`, `\"`)
	c.Header("Content-Disposition",
		fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`,
			disposition, encName, pathEscape(filename)))
	if mimeType != "" {
		c.Header("Content-Type", mimeType)
	} else {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=3600")

	// Stream it. io.Copy handles large files without loading into RAM.
	if _, err := io.Copy(c.Writer, f); err != nil {
		log.Printf("[coord-attach] stream %d: %v", id, err)
	}
}

// pathEscape percent-encodes bytes outside the RFC 5987 attr-char set so
// a filename like "示例.pdf" survives as `%E7%A4%BA%E4%BE%8B.pdf`.
// (url.PathEscape is slightly too permissive for this header.)
func pathEscape(s string) string {
	const safe = "!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9',
			c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			strings.IndexByte(safe, c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// DeleteCoordAttachment removes a single attachment (both DB row and
// on-disk file). Mirrors task-level rules: forbidden once the task is
// done. Only the uploader or an admin may delete.
func DeleteCoordAttachment(c *gin.Context) {
	id := parseInt64Param(c, "id")
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "id 无效"})
		return
	}

	var taskID, uploadedBy int64
	var rel, filename string
	err := db.DB.QueryRow(
		`SELECT task_id, filepath, filename, uploaded_by
		 FROM coordination_attachments WHERE id = $1`, id,
	).Scan(&taskID, &rel, &filename, &uploadedBy)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "附件不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}

	if _, ok := assertEditable(c, taskID); !ok {
		return
	}

	uid, uname := currentUser(c)
	role, _ := c.Get("role")
	if uploadedBy != uid && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "仅上传者或管理员可删除"})
		return
	}

	if _, err := db.DB.Exec(`DELETE FROM coordination_attachments WHERE id = $1`, id); err != nil {
		log.Printf("[coord-attach] delete row %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "删除失败"})
		return
	}
	// Remove the on-disk file. Don't fail the request if the file is
	// already gone — the authoritative record is the DB row, which
	// we just deleted.
	if abs, err := storagePathFor(rel); err == nil {
		_ = os.Remove(abs)
	}

	// Timeline event so留痕 is complete.
	recordEvent(taskID, "attach_delete", "", "", uid, uname, "🗑 删除附件："+filename)
	WriteAuditLog(c, "coord_attach_delete", "coordination_attachment", fmt.Sprintf("%d", id), filename)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已删除"})
}
