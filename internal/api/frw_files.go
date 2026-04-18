package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// filenameRe matches the FRW asset naming convention: u<uid>_gen<id>.<ext>
var filenameRe = regexp.MustCompile(`^u(\d+)_gen(\d+)\.\w+$`)

// ServeFRWFile serves files from ./data/frw/:kind/:filename with
// authentication and ownership checks. Mounted on the authenticated v1
// group so AuthRequired + CSRF are already enforced.
func ServeFRWFile(c *gin.Context) {
	kind := c.Param("kind")
	filename := c.Param("filename")

	// Validate kind (only known subdirectories).
	switch kind {
	case "image", "video":
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "invalid kind"})
		return
	}

	// Parse user_id and gen_id from filename.
	matches := filenameRe.FindStringSubmatch(filename)
	if matches == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "invalid filename format"})
		return
	}
	fileOwnerID, _ := strconv.ParseInt(matches[1], 10, 64)
	genID, _ := strconv.ParseInt(matches[2], 10, 64)

	// Verify caller is file owner or admin.
	callerID := c.GetInt64("user_id")
	callerRole, _ := c.Get("role")
	if callerID != fileOwnerID && callerRole != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "无权访问该文件"})
		return
	}

	// Check that the generation record exists in DB.
	var exists int
	err := db.DB.QueryRow(
		"SELECT 1 FROM ai_generations WHERE id = ? AND user_id = ?",
		genID, fileOwnerID,
	).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": "generation record not found"})
		return
	}

	// Build path and prevent traversal.
	relPath := filepath.Join("data", "frw", kind, filename)
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "invalid path"})
		return
	}
	// Ensure the resolved path is under our data/frw directory.
	baseDir, _ := filepath.Abs(filepath.Join("data", "frw"))
	if !strings.HasPrefix(absPath, baseDir+string(filepath.Separator)) {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "path traversal denied"})
		return
	}

	c.File(absPath)
}
